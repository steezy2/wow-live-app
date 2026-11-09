package service

import (
	"cmp"
	"errors"
	"fmt"
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
	// recapHits is how many of a raider's last hits their death shows.
	recapHits = 3
	// nightStartsHour: a pull before this hour belongs to the raid night before.
	nightStartsHour = 6
	// bossShare: hostile units with at least this share of the biggest one's max health are the boss (a boss with a
	// tail, or two or three bosses, adds up).
	bossShare = 0.5
	// shownAdds and shownDispels: at most this many kinds of add and slowest dispels are shown.
	shownAdds, shownDispels = 5, 5
)

// raidSetup is what raid pulls are read with: the raid rules, each boss's mechanics, the store of finished pulls,
// and the raid cooldowns by name.
type raidSetup struct {
	rules     models.RaidRules
	bosses    map[string]models.RaidBoss
	store     models.PullStore
	cooldowns map[string]bool
}

// raidRun is what a raid pull adds to its run: the encounter, the boss's mechanics and phases, the hostile units
// seen, each raider's last hits, and the mechanics' tallies.
type raidRun struct {
	difficulty string
	boss       models.RaidBoss
	phases     []models.PhaseTrigger
	phase      string
	units      map[string]*raidUnit // hostile creature GUID -> what it is and its health
	recent     map[string][]hitAt   // raider -> their last hits
	recaps     [][]models.RaidHit   // each death's last hits, in the order of the run's deaths
	hits       map[[2]string]int    // (ability, raider) -> avoidable direct hits
	soaks      map[int]*soakTally   // soak spell ID -> its casts
	stacks     map[[2]string]int    // (debuff, raider) -> most stacks
	limits     map[string]int       // debuff name -> stacks that are too many
	priority   map[string]int64     // unit to burst -> the raid's damage into it
	cooldowns  []models.KeyCast     // raid cooldowns used
	names      map[int]string       // soak spell ID -> name
}

// raidUnit is a hostile creature in a pull: its name, health (from advanced logging), when it was first seen and
// died (0 while up), and whether a raider hit it.
type raidUnit struct {
	hit        bool
	name       string
	hp, maxHP  int64
	seen, died int64
}

// hitAt is a hit a raider took.
type hitAt struct {
	ability, source string
	amount, at      int64
}

// soakTally is one soak spell's casts: when the last hit came, and who each cast hit.
type soakTally struct {
	last  int64
	casts []map[string]bool
}

// EnableRaid has the tracker read raid pulls (every raid difficulty) with the raid rules and each boss's mechanics
// (by boss name), keeping finished pulls in store; it reads the saved ones.
func (k *KeyTracker) EnableRaid(rules models.RaidRules, bosses map[string]models.RaidBoss, store models.PullStore) error {
	pulls, err := store.ListPulls()
	if err != nil {
		return fmt.Errorf("reading saved raid pulls: %w", err)
	}
	cooldowns := map[string]bool{}
	for _, name := range rules.Cooldowns.Names {
		cooldowns[name] = true
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.raid, k.pulls = &raidSetup{rules: rules, bosses: bosses, store: store, cooldowns: cooldowns}, pulls
	return nil
}

// startPull opens a raid pull: ENCOUNTER_START,3180,"Ula'tek",16,20,2939 (encounter, boss, difficulty, size,
// instance).
func (k *KeyTracker) startPull(line models.LogLine) {
	f := line.Fields
	run := newRun(models.Key{Dungeon: f[1], Start: line.Time, Status: "running"}, f[4])
	boss := k.raid.bosses[f[1]]
	run.facts = &models.DungeonFacts{Name: f[1], Avoidable: boss.Avoidable, Soak: boss.Soak, Dispel: boss.Dispel,
		MustStop: boss.MustStop}
	run.raid = &raidRun{difficulty: raidDifficulties[f[2]], boss: boss, phases: k.raid.rules.Phases[f[1]],
		units: map[string]*raidUnit{}, recent: map[string][]hitAt{}, hits: map[[2]string]int{},
		soaks: map[int]*soakTally{}, stacks: map[[2]string]int{}, limits: map[string]int{},
		priority: map[string]int64{}, names: map[int]string{}}
	if len(run.raid.phases) > 0 {
		run.raid.phase = "P1"
	}
	k.current = run
}

// raidLine reads a line inside a raid pull before the run's own handling; true when it ended the pull.
func (k *KeyTracker) raidLine(line models.LogLine) bool {
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
		if line.Event == "UNIT_DIED" && len(f) >= 9 && strings.HasPrefix(f[4], "Player-") && f[len(f)-1] != "1" {
			name, _ := splitName(f[5])
			r.recaps = append(r.recaps, recap(r.recent[name], at))
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
		}
	}
	victim := ""
	if slices.Contains(run.party, f[4]) {
		victim = k.names[f[4]]
	}
	switch line.Event {
	case "SPELL_DAMAGE", "SPELL_PERIODIC_DAMAGE", "RANGE_DAMAGE", "SWING_DAMAGE", "SPELL_MISSED":
		k.raidHit(at, line.Event, f, victim)
	case "SPELL_AURA_APPLIED", "SPELL_AURA_APPLIED_DOSE":
		if limit, ok := r.boss.Stacks[atoi(f[8])]; ok && victim != "" && len(f) > 9 {
			n := 1
			if line.Event == "SPELL_AURA_APPLIED_DOSE" {
				n = atoi(f[len(f)-1])
			}
			key := [2]string{f[9], victim}
			r.stacks[key], r.limits[f[9]] = max(r.stacks[key], n), limit
		}
	case "SPELL_CAST_SUCCESS":
		if who := k.player(f[0], f[2]); who != "" && len(f) > 9 && k.raid.cooldowns[f[9]] &&
			!slices.ContainsFunc(r.cooldowns, func(c models.KeyCast) bool {
				return c.Player == who && c.Ability == f[9] && run.clock(at)-c.ClockMS < cooldownRepeatMS
			}) {
			r.cooldowns = append(r.cooldowns, models.KeyCast{Player: who, Ability: f[9], Kind: "raid cooldown",
				ClockMS: run.clock(at)})
		}
	}
	return false
}

// raidUnits notes the hostile creatures a line names, and their health from its advanced-logging block.
func (k *KeyTracker) raidUnits(at int64, event string, f []string) {
	r := k.current.raid
	for _, i := range []int{0, 4} {
		guid := f[i]
		if (strings.HasPrefix(guid, "Creature-") || strings.HasPrefix(guid, "Vehicle-")) && hasFlag(f[i+2], hostileFlag) &&
			r.units[guid] == nil {
			r.units[guid] = &raidUnit{name: f[i+1], seen: at}
		}
	}
	if i := advancedAt(event); i > 0 && len(f) > i+3 {
		if u := r.units[f[i]]; u != nil && u.died == 0 {
			u.hp, u.maxHP = int64(atoi(f[i+2])), int64(atoi(f[i+3]))
		}
	}
}

// raidHit counts a hit: a raider's last hits, avoidable direct hits and soaks taken, and the raid's damage into a
// unit to burst.
func (k *KeyTracker) raidHit(at int64, event string, f []string, victim string) {
	r := k.current.raid
	if victim != "" {
		ability, id := "Melee", 0
		if !strings.HasPrefix(event, "SWING") && len(f) > 9 {
			ability, id = f[9], atoi(f[8])
		}
		if amount := hitAmount(event, f); amount > 0 {
			hits := append(r.recent[victim], hitAt{ability: ability, source: unitName(f[1]), amount: amount, at: at})
			r.recent[victim] = hits[max(0, len(hits)-recapHits):]
		}
		if event == "SPELL_DAMAGE" && slices.Contains(r.boss.Avoidable, id) {
			r.hits[[2]string{ability, victim}]++
		}
		if slices.Contains(r.boss.Soak, id) && (event == "SPELL_DAMAGE" || event == "SPELL_MISSED") {
			r.soak(id, ability, victim, at, k.raid.rules.Soaks.WindowMS)
		}
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
func (r *raidRun) soak(id int, ability, victim string, at, window int64) {
	t := r.soaks[id]
	if t == nil {
		t = &soakTally{}
		r.soaks[id], r.names[id] = t, ability
	}
	if len(t.casts) == 0 || at-t.last > window {
		t.casts = append(t.casts, map[string]bool{})
	}
	t.casts[len(t.casts)-1][victim], t.last = true, at
}

// recap is a raider's last hits before dying at at, oldest first.
func recap(hits []hitAt, at int64) []models.RaidHit {
	out := []models.RaidHit{}
	for _, h := range hits {
		out = append(out, models.RaidHit{Ability: h.ability, Source: h.source, Amount: h.amount, BeforeMS: at - h.at})
	}
	return out
}

// finishPull closes the running raid pull (a kill or a wipe), keeps it as the newest pull and queues it for saving.
// A pull the game ended (ENCOUNTER_END) has its end set; one closed by anything else ends at its last fighting.
func (k *KeyTracker) finishPull(kill bool) {
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
	pull := k.raidPull(run, k.details(run, at), at, kill)
	k.current = nil
	if i := slices.IndexFunc(k.pulls, func(p models.RaidPull) bool { return p.Start == pull.Start }); i >= 0 {
		k.pulls[i] = pull
	} else {
		k.pulls = append([]models.RaidPull{pull}, k.pulls...)
	}
	k.unsavedPulls = append(k.unsavedPulls, pull)
}

// raidPull is a finished run as the raid overview shows it, from the run's key view (deaths, meters, dispels,
// must-stops, casts) and its raid tallies.
func (k *KeyTracker) raidPull(run *keyRun, key models.Key, at int64, kill bool) models.RaidPull {
	r, rules := run.raid, k.raid.rules
	p := models.RaidPull{Start: key.Start, End: at, Night: night(key.Start), Boss: key.Dungeon,
		Difficulty: r.difficulty, DurationMS: at - key.Start, Kill: kill, BossPct: r.bossPct(kill), Phase: r.phase,
		Players: key.Players, Meter: key.Overall, Lusts: key.Lusts, Dispels: list(key.Debuffs), WentOff: []models.KeyStop{}}
	p.Number = 1 + countFunc(k.pulls, func(o models.RaidPull) bool {
		return o.Night == p.Night && o.Boss == p.Boss && o.Difficulty == p.Difficulty && o.Start < p.Start
	})
	for i, d := range key.Deaths {
		early := i < rules.Deaths.First || p.DurationMS-d.ClockMS > int64(rules.Deaths.BeforeWipeSeconds*1000)
		death := models.RaidDeath{KeyDeath: d, Early: early, Recap: []models.RaidHit{}}
		if i < len(r.recaps) {
			death.Recap = r.recaps[i]
		}
		p.Deaths = append(p.Deaths, death)
	}
	p.Deaths = list(p.Deaths)
	p.MassKills = massKills(key.Deaths, rules.Deaths.MassKillPlayers, int64(rules.Deaths.MassKillWindowSeconds*1000))
	p.Avoidable, p.Stacks = byAbility(r.hits), []models.RaidStacks{}
	for _, s := range byAbility(r.stacks) {
		p.Stacks = append(p.Stacks, models.RaidStacks{Ability: s.Ability, Limit: r.limits[s.Ability], Players: s.Players})
	}
	p.Soaks = []models.RaidSoak{}
	for id, t := range r.soaks {
		soak := models.RaidSoak{Ability: r.names[id], Casts: len(t.casts), Fewest: len(t.casts[0]), Died: []string{}}
		for _, c := range t.casts {
			soak.Fewest, soak.Most = min(soak.Fewest, len(c)), max(soak.Most, len(c))
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
	return p
}

// bossPct is the boss's health left: the units with at least bossShare of the biggest one's max health, together;
// 0 on a kill, -1 without advanced logging.
func (r *raidRun) bossPct(kill bool) float64 {
	var biggest, hp, maxHP int64
	for _, u := range r.units {
		biggest = max(biggest, u.maxHP)
	}
	switch {
	case kill:
		return 0
	case biggest == 0:
		return -1
	}
	for _, u := range r.units {
		if float64(u.maxHP) >= bossShare*float64(biggest) {
			hp, maxHP = hp+u.hp, maxHP+u.maxHP
		}
	}
	return 100 * float64(hp) / float64(maxHP)
}

// adds is each kind of hostile unit the raid hit that isn't the boss (not a soak circle or a lure): how many showed
// up, died and were up at the end, and the longest one lived; those still up first, at most shownAdds.
func (r *raidRun) adds(end int64) []models.RaidAdd {
	var biggest int64
	for _, u := range r.units {
		biggest = max(biggest, u.maxHP)
	}
	byName := map[string]*models.RaidAdd{}
	for _, u := range r.units {
		if !u.hit || biggest > 0 && float64(u.maxHP) >= bossShare*float64(biggest) {
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
	}
	adds := []models.RaidAdd{}
	for _, a := range byName {
		adds = append(adds, *a)
	}
	slices.SortFunc(adds, func(a, b models.RaidAdd) int {
		return cmp.Or(cmp.Compare(b.AliveEnd, a.AliveEnd), cmp.Compare(b.LongestMS, a.LongestMS),
			cmp.Compare(a.Name, b.Name))
	})
	return adds[:min(len(adds), shownAdds)]
}

// byAbility groups per-raider counts by ability, most per raider first and the ability with the most first.
func byAbility(counts map[[2]string]int) []models.RaidHits {
	grouped := map[string][]models.PlayerCount{}
	for key, n := range counts {
		grouped[key[0]] = append(grouped[key[0]], models.PlayerCount{Player: key[1], Count: n})
	}
	out := []models.RaidHits{}
	for ability, players := range grouped {
		slices.SortFunc(players, func(a, b models.PlayerCount) int {
			return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Player, b.Player))
		})
		out = append(out, models.RaidHits{Ability: ability, Players: players})
	}
	slices.SortFunc(out, func(a, b models.RaidHits) int {
		return cmp.Or(cmp.Compare(total(b.Players), total(a.Players)), cmp.Compare(a.Ability, b.Ability))
	})
	return out
}

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
		byAbility[d.Ability] = append(byAbility[d.Ability], d.ClockMS)
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
