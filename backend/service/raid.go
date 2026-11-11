package service

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"wow-live/backend/models"
)

// ErrNoSuchPull is returned for a raid pull that isn't saved.
var ErrNoSuchPull = errors.New("no saved raid pull starts then")

// raidDifficulties are the combat log's raid difficulty IDs and their names.
var raidDifficulties = map[string]string{"14": "Normal", "15": "Heroic", "16": "Mythic", "17": "LFR"}

// RaidLinger is how long the overlay keeps the last raid pull up between pulls, unless a new pull starts first.
const RaidLinger = 5 * time.Minute

const (
	// cooldownRepeatMS: a raid cooldown cast again this soon by the same raider is the same use (a channel logs
	// each tick).
	cooldownRepeatMS = 10_000
	// nightStartsHour: a pull before this hour belongs to the raid night before.
	nightStartsHour = 6
	// bossShare: hostile units with at least this share of the biggest one's max health are the boss (a boss with a
	// tail, or two or three bosses, adds up).
	bossShare = 0.5
	// shownAdds and shownDispels: at most this many kinds of add and slowest dispels are shown.
	shownAdds, shownDispels = 5, 5
	// breakDeathMS: a tether aura coming off a raider this close to their death went with them, it wasn't broken.
	breakDeathMS = 1_000
	// breakSetMS: a break this close after the one before it, with no effect of its own, is part of the same set.
	breakSetMS = 1_500
	// spawnSameMS: a spawn rule's aura coming off a raider this close to its cause landing on them (either order) is
	// the cause breaking it.
	spawnSameMS = 100
	// notPickedUp is the cause shown for an add whose carrier was never found.
	notPickedUp = "Not picked up"
)

// raidSetup is what raid pulls are read with: the raid rules, each boss's mechanics, the store of finished pulls,
// the raid cooldowns by name, and what the night's pulls so far showed each raider carries (a raider rarely presses
// every survival button in one pull, so a death's "still ready" list needs the earlier pulls; cooldowns start fresh
// each pull, as the game resets them when an encounter ends, and the kit each night, as talents change between).
type raidSetup struct {
	rules     models.RaidRules
	bosses    map[string]models.RaidBoss
	store     models.PullStore
	cooldowns map[string]bool
	tier      map[string][]int // class -> its tier items (EnableRaid)
	kit       models.Kit
	kitNight  string
}

// raidRun is what a raid pull adds to its run: the difficulty, the boss's mechanics and phases, the hostile units
// seen, each death's last hits, the mechanics' tallies: soaks, stacks, pickups, tethers broken and the last landing of
// each spell that spawns adds, and the raiders' buff gaps.
type raidRun struct {
	difficulty string
	boss       models.RaidBoss
	phases     []models.PhaseTrigger
	phase      string
	units      map[string]*raidUnit       // hostile creature GUID -> what it is and its health
	recaps     [][]models.RaidHit         // each death's last hits, kept by death() with the run's deaths
	soaks      map[int]*soakTally         // soak spell ID -> its casts
	stacks     map[int]map[string]int     // debuff spell ID -> raider -> most stacks
	priority   map[string]int64           // unit to burst -> the raid's damage into it
	cooldowns  []models.KeyCast           // raid cooldowns used
	lustEnds   map[int64]int64            // when a Bloodlust landed -> when it last left a raider, on the pull's clock
	marks      map[lane][]models.RaidMark // the timeline marks only seen as they happen (lanes adds the rest)
	// Pickup waves so far, the wave of each expected pickup (by name) its spell will close, and the pickup auras
	// raiders hold now.
	pickups   []models.RaidPickup
	openWaves map[string]int
	holds     map[spellOn]bool
	// Raiders dead now and when each was brought back (on the pull's clock), each raider's buff gap open (raider and
	// kind -> since), when each kind first came up on them, and the gaps closed.
	dead    map[string]bool
	rezzed  map[string][]int64
	gapFrom map[[2]string]int64
	firstUp map[[2]string]int64
	gaps    []models.RaidMissing
	breaks  []breakAt
	causes  map[int]*causeAt // spawn cause spell ID -> its last landing
	// For spawn rules with a lost aura: when each such aura last came off a raider and each cause last landed on one,
	// and the raiders who lost one as its cause landed, in order (spawners).
	lostAt, landedAt map[spellOn]int64
	spawners         []spawner
}

// breakAt is a raider breaking a tether, and how many raiders the effect it set off hit.
type breakAt struct {
	at     int64
	rule   models.RaidBreakRule
	player string
	hit    int
}

// spellOn is a spell on a raider.
type spellOn struct {
	id     int
	player string
}

// spawner is a raider who lost a spawn rule's aura as its cause landed on them: the next add of the rule's name is
// theirs (used once it is).
type spawner struct {
	at     int64
	rule   models.RaidSpawnRule
	player string
	used   bool
}

// causeAt is the last landing of a spell that spawns adds: when it first and last landed, and the raiders it landed
// on (none for an enemy's cast).
type causeAt struct {
	first, at int64
	players   []string
}

// raidUnit is a hostile creature in a pull: its name, health (from advanced logging), when it was first seen and
// died (0 while up), whether a raider hit it, and for an add the reference says how it spawns, what spawned it.
type raidUnit struct {
	hit        bool
	name       string
	hp, maxHP  int64
	seen, died int64
	spawn      *models.RaidSpawn
}

// soakTally is one soak spell's casts: when the last hit came, and when each cast first hit and who it hit.
type soakTally struct {
	last  int64
	at    []int64
	casts []map[string]bool
}

// EnableRaid has the tracker read raid pulls (every raid difficulty) with the raid rules, each boss's mechanics
// (by boss name) and the raid's tier items by class (raid-bosses.json's tier), which the pull-start check uses to say
// who lacks their 2-set or 4-set; it keeps finished pulls in store and reads the saved ones.
func (k *LogTracker) EnableRaid(rules models.RaidRules, bosses map[string]models.RaidBoss, tier map[string][]int,
	store models.PullStore) error {
	pulls, err := savedPulls(store)
	if err != nil {
		return err
	}
	if rules.Suggestions.SpikeSeconds < 1 {
		return fmt.Errorf("raid-rules.toml: spike_seconds must be at least 1")
	}
	cooldowns := map[string]bool{}
	for _, name := range slices.Concat(rules.Cooldowns.Names, k.rules.Defensives.RaidCooldowns) {
		cooldowns[name] = true
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.raid, k.pulls = &raidSetup{rules: rules, bosses: bosses, tier: tier, store: store, cooldowns: cooldowns}, pulls
	return nil
}

// savedPulls is the store's pulls that were fought.
func savedPulls(store models.PullStore) ([]models.RaidPull, error) {
	pulls, err := store.ListPulls()
	if err != nil {
		return nil, fmt.Errorf("reading saved raid pulls: %w", err)
	}
	return slices.DeleteFunc(pulls, func(p models.RaidPull) bool { return !fought(p) }), nil
}

// fought is whether anyone did damage or healing in a pull: an encounter that starts and ends at once (a reset on
// the way in) isn't a pull.
func fought(p models.RaidPull) bool {
	return len(p.Meter.Rows) > 0
}

// startPull opens a raid pull: ENCOUNTER_START,3180,"Ula'tek",16,20,2939 (encounter, boss, difficulty, size,
// instance).
func (k *LogTracker) startPull(line models.LogLine) {
	f := line.Fields
	if n := night(line.Time); n != k.raid.kitNight {
		k.raid.kit, k.raid.kitNight = LearnKit(nil), n
	}
	run := newRun(models.Key{Dungeon: f[1], Start: line.Time, Status: "running"}, f[4])
	boss := k.raid.bosses[f[1]]
	run.facts = &models.DungeonFacts{Name: f[1], Avoidable: boss.Avoidable, Soak: boss.Soak, Dispel: boss.Dispel,
		MustStop: boss.MustStop}
	run.raid = &raidRun{difficulty: raidDifficulties[f[2]], boss: boss, phases: k.raid.rules.Phases[f[1]],
		units: map[string]*raidUnit{}, soaks: map[int]*soakTally{}, stacks: map[int]map[string]int{},
		priority: map[string]int64{}, lustEnds: map[int64]int64{}, marks: map[lane][]models.RaidMark{},
		causes: map[int]*causeAt{}, lostAt: map[spellOn]int64{}, landedAt: map[spellOn]int64{},
		pickups: []models.RaidPickup{}, openWaves: map[string]int{}, holds: map[spellOn]bool{},
		dead: map[string]bool{}, rezzed: map[string][]int64{}, gapFrom: map[[2]string]int64{},
		firstUp: map[[2]string]int64{}}
	run.sample.every = abilityEveryMS
	if len(run.raid.phases) > 0 {
		run.raid.phase = "P1"
	}
	k.current = run
	k.version++
}

// raidLine reads a line inside a raid pull before the run's own handling; true when it ended the pull.
func (k *LogTracker) raidLine(line models.LogLine) bool {
	run, f, at := k.current, line.Fields, line.Time
	r := run.raid
	switch line.Event {
	case "ENCOUNTER_END":
		k.current.key.End = at
		k.finishPull(len(f) >= 5 && f[4] == "1")
		return true
	case "ENCOUNTER_START": // the last pull never logged its end
		k.finishPull(false)
		if len(f) >= 5 && raidDifficulties[f[2]] != "" {
			k.startPull(line)
		}
		return true
	case "UNIT_DIED", "PARTY_KILL":
		if len(f) < 5 {
			return false
		}
		if u := r.units[f[4]]; u != nil && u.died == 0 {
			u.died, u.hp = at, 0
		}
		if line.Event == "UNIT_DIED" && slices.Contains(run.party, f[4]) && f[len(f)-1] != "1" { // not feigned
			r.dead[k.names[f[4]]] = true
		}
		return false
	}
	if len(f) < 8 {
		return false
	}
	k.raidUnits(at, line.Event, f)
	for _, p := range r.phases {
		if p.Spell != 0 && p.Event == line.Event && len(f) > 8 && atoi(f[8]) == p.Spell {
			r.phase = "P" + strconv.Itoa(p.ID)
			r.mark("phases", "Phases", models.RaidMark{ClockMS: run.clock(at), Text: r.phase})
		}
	}
	victim := ""
	if slices.Contains(run.party, f[4]) {
		victim = k.names[f[4]]
	}
	window := k.raid.rules.Soaks.WindowMS
	if line.Event == "SPELL_CAST_START" || line.Event == "SPELL_CAST_SUCCESS" || line.Event == "SPELL_CAST_FAILED" {
		if len(f) > 8 && hasFlag(f[2], hostileFlag) {
			r.cause(atoi(f[8]), at, "", window)
			if line.Event == "SPELL_CAST_SUCCESS" {
				r.closeWaves(atoi(f[8]), run.clock(at), k.raiders(run))
			}
		}
		k.trackCast(at, line.Event, f)
	}
	switch line.Event {
	case "SPELL_DAMAGE", "SPELL_PERIODIC_DAMAGE", "RANGE_DAMAGE", "SWING_DAMAGE", "SPELL_MISSED":
		k.raidHit(at, line.Event, f, victim)
	case "SPELL_AURA_APPLIED", "SPELL_AURA_APPLIED_DOSE":
		id := atoi(f[8])
		if _, tracked := r.boss.Stacks[id]; tracked && victim != "" {
			n := 1
			if line.Event == "SPELL_AURA_APPLIED_DOSE" {
				n = atoi(f[len(f)-1])
			}
			if r.stacks[id] == nil {
				r.stacks[id] = map[string]int{}
			}
			if limit := r.boss.Stacks[id]; limit > 0 && n > limit && r.stacks[id][victim] <= limit {
				r.mark("stacks", f[9], models.RaidMark{ClockMS: run.clock(at), Text: fmt.Sprintf("%s %d", victim, n),
					Bad: true})
			}
			r.stacks[id][victim] = max(r.stacks[id][victim], n)
		}
		if line.Event == "SPELL_AURA_APPLIED" && victim != "" {
			r.auraOn(id, at, run.clock(at), victim, window)
		}
		if victim != "" && len(f) > 11 && f[11] == "BUFF" {
			k.buffChanged(run, run.clock(at), victim, id, f[9], true)
		}
	case "SPELL_AURA_REFRESH":
		if victim != "" && len(f) > 11 && f[11] == "BUFF" {
			k.buffChanged(run, run.clock(at), victim, atoi(f[8]), f[9], true)
		}
	case "SPELL_RESURRECT":
		if victim != "" {
			delete(r.dead, victim)
			r.rezzed[victim] = append(r.rezzed[victim], run.clock(at))
		}
	case "SPELL_CAST_SUCCESS":
		if who := k.player(f[0], f[2]); who != "" && len(f) > 9 && k.raidCooldown(f[0], f[9]) &&
			!slices.ContainsFunc(r.cooldowns, func(c models.KeyCast) bool {
				return c.Player == who && c.Ability == f[9] && run.clock(at)-c.ClockMS < cooldownRepeatMS
			}) {
			r.cooldowns = append(r.cooldowns, models.KeyCast{Player: who, Ability: f[9], Kind: "raid cooldown",
				ClockMS: run.clock(at)})
		}
	case "SPELL_AURA_REMOVED":
		if victim != "" && len(f) > 9 {
			k.buffLeft(run.clock(at), k.player(f[0], f[2]), f[9])
			r.lose(atoi(f[8]), at, victim)
			delete(r.holds, spellOn{atoi(f[8]), victim})
			if len(f) > 11 && f[11] == "BUFF" {
				k.buffChanged(run, run.clock(at), victim, atoi(f[8]), f[9], false)
			}
			for _, b := range r.boss.Breaks {
				if b.Aura == atoi(f[8]) {
					r.breaks = append(r.breaks, breakAt{at: at, rule: b, player: victim})
				}
			}
		}
	}
	return false
}

// raidCooldown says whether a raider's (by GUID) spell is a raid cooldown: one of healer_only's only from a healer.
func (k *LogTracker) raidCooldown(guid, spell string) bool {
	return k.raid.cooldowns[spell] &&
		(!slices.Contains(k.raid.rules.Cooldowns.HealerOnly, spell) || specs[k.specs[guid]].Healer)
}

// auraOn notes an aura landing on a raider at at (clockMS on the pull's clock): a pickup, the effect of the last
// tether broken, or a spawn cause.
func (r *raidRun) auraOn(id int, at, clockMS int64, victim string, window int64) {
	for _, p := range r.boss.Pickups {
		if p.Aura == id && r.happens(p) {
			r.pickUp(p, clockMS, victim, window)
		}
	}
	for i := len(r.breaks) - 1; i >= 0; i-- {
		if b := &r.breaks[i]; b.rule.Effect == id && at-b.at <= b.rule.WithinMS {
			b.hit++
			break
		}
	}
	r.cause(id, at, victim, window)
}

// happens is whether a pickup rule applies at the pull's difficulty.
func (r *raidRun) happens(p models.RaidPickupRule) bool {
	return len(p.Difficulties) == 0 || slices.Contains(p.Difficulties, r.difficulty)
}

// pickUp adds a raider picking up a rule's thing to its wave: an expected pickup's open wave (its spell closes it),
// else the last wave when it's of this name and began within window ms.
func (r *raidRun) pickUp(p models.RaidPickupRule, clockMS int64, player string, window int64) {
	r.holds[spellOn{p.Aura, player}] = true
	i, ok := r.openWaves[p.Name]
	if n := len(r.pickups); p.Expect == 0 && n > 0 {
		i, ok = n-1, r.pickups[n-1].Name == p.Name && clockMS-r.pickups[n-1].ClockMS <= window
	}
	if !ok {
		r.pickups, i = append(r.pickups, models.RaidPickup{ClockMS: clockMS, Name: p.Name}), len(r.pickups)
		if p.Expect != 0 {
			r.openWaves[p.Name] = i
		}
	}
	r.pickups[i].Players = append(r.pickups[i].Players, player)
}

// closeWaves ends each expected pickup's wave as the enemy spell it's needed for goes off at clockMS: the living
// raiders without its aura then missed it (a wave nobody picked up from is all of them), marked on its lane.
func (r *raidRun) closeWaves(spell int, clockMS int64, raiders []string) {
	for _, p := range r.boss.Pickups {
		if p.Expect != spell || !r.happens(p) {
			continue
		}
		i, ok := r.openWaves[p.Name]
		if !ok {
			r.pickups, i = append(r.pickups, models.RaidPickup{ClockMS: clockMS, Name: p.Name, Players: []string{}}),
				len(r.pickups)
		}
		delete(r.openWaves, p.Name)
		w := &r.pickups[i]
		w.Missed = []string{}
		for _, name := range raiders {
			if !r.dead[name] && !r.holds[spellOn{p.Aura, name}] {
				w.Missed = append(w.Missed, name)
			}
		}
		slices.Sort(w.Missed)
		m := models.RaidMark{ClockMS: clockMS, Text: fmt.Sprintf("%d picked up", len(w.Players))}
		if len(w.Missed) > 0 {
			m.Text, m.Bad = "Missed: "+strings.Join(w.Missed, ", "), true
		}
		r.mark("pickups", "Picked up "+p.Name, m)
	}
}

// raiders are the pull's raiders' names, in the order COMBATANT_INFO listed them.
func (k *LogTracker) raiders(run *keyRun) []string {
	names := make([]string, 0, len(run.party))
	for _, guid := range run.party {
		names = append(names, k.names[guid])
	}
	return names
}

// cause notes a spell that spawns adds landing at at (a cast, a direct hit or an aura applied, never a tick), on
// player ("" for an enemy's cast): within window ms of its first landing it's the same one (a wave hitting raiders
// for seconds doesn't gather the whole raid).
func (r *raidRun) cause(id int, at int64, player string, window int64) {
	if !slices.ContainsFunc(r.boss.Spawns, func(s models.RaidSpawnRule) bool { return s.Cause == id }) {
		return
	}
	c := r.causes[id]
	if c == nil || at-c.first > window {
		c = &causeAt{first: at}
		r.causes[id] = c
	}
	c.at = at
	if player == "" {
		return
	}
	r.land(id, at, player)
	if !slices.Contains(c.players, player) {
		c.players = append(c.players, player)
	}
}

// land notes a spawn cause landing on a raider: one who lost the rule's aura spawnSameMS before is a spawner.
func (r *raidRun) land(id int, at int64, player string) {
	r.landedAt[spellOn{id, player}] = at
	for _, rule := range r.boss.Spawns {
		lost := spellOn{rule.Lost, player}
		if t, ok := r.lostAt[lost]; ok && rule.Cause == id && at-t <= spawnSameMS {
			r.spawners = append(r.spawners, spawner{at: t, rule: rule, player: player})
			delete(r.lostAt, lost)
			return
		}
	}
}

// lose notes an aura coming off a raider: a spawn rule's lost aura whose cause landed on them spawnSameMS before
// makes them a spawner, else it waits for the cause.
func (r *raidRun) lose(id int, at int64, player string) {
	for _, rule := range r.boss.Spawns {
		if t, ok := r.landedAt[spellOn{rule.Cause, player}]; ok && rule.Lost == id && at-t <= spawnSameMS {
			r.spawners = append(r.spawners, spawner{at: at, rule: rule, player: player})
			return
		}
	}
	if slices.ContainsFunc(r.boss.Spawns, func(s models.RaidSpawnRule) bool { return s.Lost == id }) {
		r.lostAt[spellOn{id, player}] = at
	}
}

// spawnOf is an add of a name appearing at at (clockMS on the pull's clock), named by spells: the oldest unused
// spawner of its name within its rule's time; else, for a rule that needs a carrier (Lost), nobody picked it up
// (notPickedUp, no players); else the latest landing of a cause the reference gives for it within its rule's time;
// nil for an add the reference says nothing about.
func (r *raidRun) spawnOf(name string, at, clockMS int64, spells map[int]string) *models.RaidSpawn {
	for i := range r.spawners { // in order, so the first that fits is the oldest
		if s := &r.spawners[i]; !s.used && s.rule.Add == name && at >= s.at && at-s.at <= s.rule.WithinMS {
			s.used = true
			return &models.RaidSpawn{ClockMS: clockMS, Cause: spells[s.rule.Cause], Players: []string{s.player}}
		}
	}
	var seen *models.RaidSpawn
	var when int64
	for _, rule := range r.boss.Spawns {
		if rule.Add != name {
			continue
		}
		if seen == nil {
			seen = &models.RaidSpawn{ClockMS: clockMS, Players: []string{}}
		}
		if rule.Lost != 0 { // its carrier's lost buff names the spawner, not whoever the cause landed on
			seen.Cause = cmp.Or(seen.Cause, notPickedUp)
			continue
		}
		if c := r.causes[rule.Cause]; c != nil && at-c.at <= rule.WithinMS && c.at >= when {
			seen.Cause, seen.Players, when = spells[rule.Cause], slices.Clone(c.players), c.at
		}
	}
	return seen
}

// buffLeft notes a buff leaving a raider at a time on the pull's clock: the last Bloodlust lasted until then, and so
// did the caster's last raid cooldown of that name; a combat potion's buff the raider didn't drink this pull was
// drunk before it, so it counts at 0.
func (k *LogTracker) buffLeft(clockMS int64, caster, buff string) {
	run := k.current
	r := run.raid
	switch {
	case slices.Contains(k.rules.Lust.Names, buff):
		if n := len(run.key.Lusts); n > 0 {
			r.lustEnds[run.key.Lusts[n-1]] = clockMS
		}
	case k.raid.cooldowns[buff]:
		for i := len(r.cooldowns) - 1; i >= 0; i-- {
			if c := &r.cooldowns[i]; c.Player == caster && c.Ability == buff {
				c.EndMS = clockMS
				break
			}
		}
	case k.kinds[buff] == "combat potion" && caster != "" &&
		!castBy(run.key.Casts, caster, buff):
		run.key.Casts = append(run.key.Casts, models.KeyCast{Player: caster, Ability: buff, Kind: "combat potion",
			Prepot: true})
	}
}

// raidUnits notes the hostile creatures a line names, and their health from its advanced-logging block; a unit whose
// health drops to 0 died there, as some adds die without a UNIT_DIED (Nymrissa's Bubblefin Shorerunners), but one
// first seen at 0 (dormant, or not targetable yet) didn't.
func (k *LogTracker) raidUnits(at int64, event string, f []string) {
	r := k.current.raid
	for _, i := range []int{0, 4} {
		guid := f[i]
		if (strings.HasPrefix(guid, "Creature-") || strings.HasPrefix(guid, "Vehicle-")) && hasFlag(f[i+2], hostileFlag) &&
			r.units[guid] == nil {
			r.units[guid] = &raidUnit{name: f[i+1], seen: at,
				spawn: r.spawnOf(f[i+1], at, k.current.clock(at), k.spells)}
		}
	}
	if i := advancedAt(event); i > 0 && len(f) > i+3 {
		if u := r.units[f[i]]; u != nil && u.died == 0 {
			alive := u.hp > 0
			u.hp, u.maxHP = int64(atoi(f[i+2])), int64(atoi(f[i+3]))
			if alive && u.hp == 0 {
				u.died = at
			}
		}
	}
}

// raidHit counts a hit: soaks, avoidable hits and spawn causes raiders took (direct hits, not ticks), and the raid's
// damage into a unit to burst.
func (k *LogTracker) raidHit(at int64, event string, f []string, victim string) {
	r := k.current.raid
	if id := atoi(f[8]); victim != "" && (event == "SPELL_DAMAGE" || event == "SPELL_MISSED") {
		switch {
		case slices.Contains(r.boss.Soak, id):
			r.soak(id, victim, at, k.raid.rules.Soaks.WindowMS)
		case slices.Contains(r.boss.Avoidable, id):
			r.mark("avoidable", f[9], models.RaidMark{ClockMS: k.current.clock(at), Text: victim, Bad: true})
		}
		r.cause(id, at, victim, k.raid.rules.Soaks.WindowMS)
	}
	if k.player(f[0], f[2]) == "" {
		return
	}
	if u := r.units[f[4]]; u != nil {
		u.hit = true
	}
	if slices.Contains(r.boss.Priority, f[5]) {
		r.priority[f[5]] += hitAmount(event, f)
	}
}

// soak counts a raider hit by a soak spell, in the same cast as the last hit within window ms, else a new one.
func (r *raidRun) soak(id int, victim string, at, window int64) {
	t := r.soaks[id]
	if t == nil {
		t = &soakTally{}
		r.soaks[id] = t
	}
	if len(t.casts) == 0 || at-t.last > window {
		t.casts, t.at = append(t.casts, map[string]bool{}), append(t.at, at)
	}
	t.casts[len(t.casts)-1][victim], t.last = true, at
}

// recap is a raider's last hits that did damage before dying at at, oldest first.
func recap(hits []hitAt, at int64) []models.RaidHit {
	out := []models.RaidHit{}
	for _, h := range hits {
		if h.amount > 0 {
			out = append(out, models.RaidHit{Ability: h.ability, Source: h.source, Amount: h.amount, BeforeMS: at - h.at})
		}
	}
	return out
}

// finishPull closes the running raid pull (a kill or a wipe), keeps it as the newest pull and queues it for saving.
// A pull the game ended (ENCOUNTER_END) has its end set; one closed by anything else ends at its last fighting.
func (k *LogTracker) finishPull(kill bool) {
	run := k.current
	if run.key.End == 0 {
		run.key.End = max(run.pullLast, run.key.Start)
	}
	at := run.key.End
	run.key.Status = "wipe"
	if kill {
		run.key.Status = "kill"
	}
	endPull(run)
	run.sample.at = -run.sample.every // closes the last stretch, however short
	k.sampleAbilities(at)
	pull := k.raidPull(run, k.details(run, at), at, kill)
	k.current = nil
	k.version++
	if !fought(pull) {
		return
	}
	addKit(k.raid.kit, LearnKit(run.casts))
	died := map[string]bool{}
	for _, guid := range run.party {
		died[guid] = slices.ContainsFunc(run.key.Deaths, func(d models.KeyDeath) bool { return d.Player == k.names[guid] })
	}
	k.seedReady(run.auras, died)
	if i := slices.IndexFunc(k.pulls, func(p models.RaidPull) bool { return p.Start == pull.Start }); i >= 0 {
		k.pulls[i] = pull
	} else {
		k.pulls = append([]models.RaidPull{pull}, k.pulls...)
	}
	k.unsavedPulls = append(k.unsavedPulls, savedPull{pull, k.trackOf(run)})
}

// savedPull is a finished raid pull and its track, waiting to be saved.
type savedPull struct {
	pull  models.RaidPull
	track models.KeyTrack
}

// raidPull is a finished run as the raid overview shows it, from the run's key view (deaths, meters, dispels,
// must-stops, casts) and its raid tallies.
func (k *LogTracker) raidPull(run *keyRun, key models.Key, at int64, kill bool) models.RaidPull {
	r, rules := run.raid, k.raid.rules
	p := models.RaidPull{Start: key.Start, End: at, Night: night(key.Start), Boss: key.Dungeon,
		Difficulty: r.difficulty, DurationMS: at - key.Start, Kill: kill, BossPct: r.bossPct(kill), Phase: r.phase,
		Players: key.Players, Meter: key.Overall, Lusts: key.Lusts, Dispels: list(key.Debuffs),
		WentOff: []models.KeyStop{}}
	for i, d := range key.Deaths {
		early := i < rules.Deaths.First || p.DurationMS-d.ClockMS > int64(rules.Deaths.BeforeWipeSeconds*1000)
		p.Deaths = append(p.Deaths, models.RaidDeath{KeyDeath: d, Early: early, Recap: r.recaps[i]})
	}
	p.Deaths = list(p.Deaths)
	p.MassKills = massKills(key.Deaths, rules.Deaths.MassKillPlayers, int64(rules.Deaths.MassKillWindowSeconds*1000))
	hits := map[string]map[string]int{}
	for _, h := range key.AvoidableHits {
		if hits[h.Ability] == nil {
			hits[h.Ability] = map[string]int{}
		}
		hits[h.Ability][h.Player] = h.Hits
	}
	p.Avoidable, p.AvoidableHits, p.Stacks = byAbility(hits), list(key.AvoidableHits), []models.RaidStacks{}
	for id, players := range r.stacks {
		s := models.RaidStacks{Ability: k.spells[id], Limit: r.boss.Stacks[id], Players: playerCounts(players),
			Over: []string{}}
		for _, c := range s.Players {
			if s.Limit > 0 && c.Count > s.Limit {
				s.Over = append(s.Over, c.Player)
			}
		}
		p.Stacks = append(p.Stacks, s)
	}
	slices.SortFunc(p.Stacks, func(a, b models.RaidStacks) int { return cmp.Compare(a.Ability, b.Ability) })
	p.Soaks = []models.RaidSoak{}
	for id, t := range r.soaks {
		soak := models.RaidSoak{Ability: k.spells[id], Casts: len(t.casts), Fewest: len(t.casts[0]),
			Needed: r.boss.Soakers[id], Died: []string{}}
		for i, c := range t.casts {
			soak.Fewest, soak.Most = min(soak.Fewest, len(c)), max(soak.Most, len(c))
			soak.Hits = append(soak.Hits, models.RaidSoakCast{ClockMS: run.clock(t.at[i]),
				Players: slices.Sorted(maps.Keys(c))})
		}
		for _, d := range key.Deaths {
			if d.Ability == soak.Ability {
				soak.Died = append(soak.Died, d.Player)
			}
		}
		p.Soaks = append(p.Soaks, soak)
	}
	slices.SortFunc(p.Soaks, func(a, b models.RaidSoak) int { return cmp.Compare(a.Ability, b.Ability) })
	slices.SortFunc(p.Dispels, func(a, b models.KeyDispel) int { return cmp.Compare(b.AfterMS, a.AfterMS) })
	p.Dispels = p.Dispels[:min(len(p.Dispels), shownDispels)]
	for _, s := range key.Stops {
		if s.Outcome == "went off" {
			p.WentOff = append(p.WentOff, s)
		}
	}
	p.Adds = r.adds(at)
	p.Pickups, p.Breaks = r.pickups, []models.RaidBreak{}
	var setEnd int64
	for _, b := range r.breaks {
		if slices.ContainsFunc(key.Deaths, func(d models.KeyDeath) bool {
			return d.Player == b.player && max(d.ClockMS-run.clock(b.at), run.clock(b.at)-d.ClockMS) <= breakDeathMS
		}) {
			continue // the aura went with the raider, not a break
		}
		rb := models.RaidBreak{ClockMS: run.clock(b.at), Player: b.player, Aura: k.spells[b.rule.Aura], Hit: b.hit}
		if b.hit > 0 {
			rb.Effect = k.spells[b.rule.Effect]
		}
		if last := len(p.Breaks) - 1; last >= 0 && rb.Effect == "" && rb.ClockMS-setEnd <= breakSetMS {
			p.Breaks[last].Then = append(p.Breaks[last].Then, rb.Player)
		} else {
			p.Breaks = append(p.Breaks, rb)
		}
		setEnd = rb.ClockMS
	}
	p.Priority = []models.RaidPriority{}
	for _, target := range r.boss.Priority {
		if damage, ok := r.priority[target]; ok {
			p.Priority = append(p.Priority, models.RaidPriority{Target: target, Damage: damage})
		}
	}
	p.Cooldowns = list(r.cooldowns)
	for _, c := range key.Casts {
		if c.Kind == "combat potion" {
			p.Cooldowns = append(p.Cooldowns, c)
		}
	}
	slices.SortStableFunc(p.Cooldowns, func(a, b models.KeyCast) int { return cmp.Compare(a.ClockMS, b.ClockMS) })
	p.Spikes = key.Spikes
	p.Lanes = k.lanes(run, p, at)
	p.Abilities, p.Taken = abilities(run, k.schools)
	p.Targets = targets(run)
	p.Interrupts, p.Stops, p.WastedKicks, p.Debuffs = key.Interrupts, key.Stops, key.WastedKicks, key.Debuffs
	p.Activity, p.Casts = key.Activity, key.Casts
	p.BuffGaps = k.buffGaps(run, key.Buffs, key.Deaths, p.DurationMS)
	return p
}

// targets is each raider's damage into the enemies of each name over the run, most first.
func targets(run *keyRun) []models.RaidTarget {
	damage := targetDamage(run)
	out := make([]models.RaidTarget, 0, len(damage))
	for id, amount := range damage {
		out = append(out, models.RaidTarget{Player: id[0], Target: id[1], Damage: amount})
	}
	slices.SortFunc(out, func(a, b models.RaidTarget) int {
		return cmp.Or(cmp.Compare(b.Damage, a.Damage), cmp.Compare(a.Player, b.Player), cmp.Compare(a.Target, b.Target))
	})
	return out
}

// bossPct is the boss's health left, its units together (bossUnits); 0 on a kill, -1 without advanced logging.
func (r *raidRun) bossPct(kill bool) float64 {
	var hp, maxHP int64
	for _, u := range r.bossUnits() {
		hp, maxHP = hp+u.hp, maxHP+u.maxHP
	}
	switch {
	case kill:
		return 0
	case maxHP == 0:
		return -1
	}
	return 100 * float64(hp) / float64(maxHP)
}

// bossUnits are the pull's boss, the first of these that has units: the reference's health NPCs; its boss NPCs named
// as the encounter (Ula'tek, not its tail or heart, which Warcraft Logs' boss % leaves out); all its boss NPCs (a
// council); else the units with at least bossShare of the biggest unit's max health.
func (r *raidRun) bossUnits() map[string]*raidUnit {
	for _, pick := range []func(npc int, name string) bool{
		func(npc int, _ string) bool { return slices.Contains(r.boss.Health, npc) },
		func(npc int, name string) bool { return slices.Contains(r.boss.NPCs, npc) && name == r.boss.Name },
		func(npc int, _ string) bool { return slices.Contains(r.boss.NPCs, npc) },
	} {
		boss := map[string]*raidUnit{}
		for guid, u := range r.units {
			if pick(npcOf(guid), u.name) {
				boss[guid] = u
			}
		}
		if len(boss) > 0 {
			return boss
		}
	}
	boss := map[string]*raidUnit{}
	var biggest int64
	for _, u := range r.units {
		biggest = max(biggest, u.maxHP)
	}
	for guid, u := range r.units {
		if biggest > 0 && float64(u.maxHP) >= bossShare*float64(biggest) {
			boss[guid] = u
		}
	}
	return boss
}

// adds is each kind of hostile unit the raid hit that isn't one of the boss's NPCs (nor a soak circle or a lure):
// how many showed up, died and were up at the end, the longest one lived, and each one's spawn where the reference
// says how they spawn; those still up first, at most shownAdds plus those with spawns.
func (r *raidRun) adds(end int64) []models.RaidAdd {
	boss := r.bossUnits()
	byName := map[string]*models.RaidAdd{}
	for guid, u := range r.units {
		if !u.hit || boss[guid] != nil || slices.Contains(r.boss.NPCs, npcOf(guid)) {
			continue
		}
		a := byName[u.name]
		if a == nil {
			a = &models.RaidAdd{Name: u.name}
			byName[u.name] = a
		}
		a.Spawned++
		died := end
		if u.died > 0 {
			a.Killed, died = a.Killed+1, u.died
		} else {
			a.AliveEnd++
		}
		a.LongestMS = max(a.LongestMS, died-u.seen)
		if u.spawn != nil {
			a.Spawns = append(a.Spawns, *u.spawn)
		}
	}
	adds := []models.RaidAdd{}
	for _, a := range byName {
		slices.SortFunc(a.Spawns, func(x, y models.RaidSpawn) int { return cmp.Compare(x.ClockMS, y.ClockMS) })
		adds = append(adds, *a)
	}
	slices.SortFunc(adds, func(a, b models.RaidAdd) int {
		return cmp.Or(cmp.Compare(b.AliveEnd, a.AliveEnd), cmp.Compare(b.LongestMS, a.LongestMS),
			cmp.Compare(a.Name, b.Name))
	})
	shown := slices.Clip(adds[:min(len(adds), shownAdds)])
	for _, a := range adds[len(shown):] {
		if len(a.Spawns) > 0 { // the reference says how they spawn: always worth their lines
			shown = append(shown, a)
		}
	}
	return shown
}

// byAbility is per-raider counts by ability (ability -> raider -> count), the ability with the most first.
func byAbility(counts map[string]map[string]int) []models.RaidHits {
	out := []models.RaidHits{}
	for ability, players := range counts {
		out = append(out, models.RaidHits{Ability: ability, Players: playerCounts(players)})
	}
	slices.SortFunc(out, func(a, b models.RaidHits) int {
		return cmp.Or(cmp.Compare(total(b.Players), total(a.Players)), cmp.Compare(a.Ability, b.Ability))
	})
	return out
}

// playerCounts is raiders' counts, most first.
func playerCounts(counts map[string]int) []models.PlayerCount {
	out := []models.PlayerCount{}
	for player, n := range counts {
		out = append(out, models.PlayerCount{Player: player, Count: n})
	}
	slices.SortFunc(out, func(a, b models.PlayerCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Player, b.Player))
	})
	return out
}

// total is the sum of raiders' counts.
func total(players []models.PlayerCount) int {
	n := 0
	for _, p := range players {
		n += p.Count
	}
	return n
}

// massKills is each time one ability killed at least players raiders within window ms of the first of them.
func massKills(deaths []models.KeyDeath, players int, window int64) []models.MassKill {
	byAbility := map[string][]int64{}
	for _, d := range deaths {
		if d.Ability != "" { // no hit seen: not one ability
			byAbility[d.Ability] = append(byAbility[d.Ability], d.ClockMS)
		}
	}
	kills := []models.MassKill{}
	for ability, at := range byAbility {
		for i := 0; i < len(at); {
			j := i
			for j < len(at) && at[j]-at[i] <= window {
				j++
			}
			if j-i >= players {
				kills = append(kills, models.MassKill{Ability: ability, Killed: j - i, ClockMS: at[i]})
			}
			i = j
		}
	}
	slices.SortFunc(kills, func(a, b models.MassKill) int { return cmp.Compare(a.ClockMS, b.ClockMS) })
	return kills
}

// night is the raid night a pull starting at start belongs to: its local date, a pull before nightStartsHour
// counting as the night before.
func night(start int64) string {
	return time.UnixMilli(start).Add(-nightStartsHour * time.Hour).Format(time.DateOnly)
}
