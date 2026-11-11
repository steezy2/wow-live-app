package service

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"wow-live/backend/models"
)

const (
	// keyCountdownMS is how long after CHALLENGE_MODE_START the key's clock starts (the countdown): 8.5 and 9.0 s
	// on two timed keys (Nov 3 and Nov 4), whose game times this reproduces to within 0.5 s.
	keyCountdownMS = 9_000
	// deathPenaltyMS is the time each death adds to the key's clock.
	deathPenaltyMS = 15_000
	// keyDifficulty is the Mythic+ difficulty in the combat log's ENCOUNTER lines.
	keyDifficulty = "8"
	// plus2Share and plus3Share: a key upgrades by two or three levels when done in this share of its timer.
	plus2Share = 0.8
	plus3Share = 0.6
	// pullGapMS: fighting that stops this long ends a pull (bosses are pulls of their own).
	pullGapMS = 5_000
	// minSavedMS: keys shorter than this (reset at the start) aren't saved.
	minSavedMS = 60_000
	// levelSpread: the estimate prefers earlier runs within this many keystone levels.
	levelSpread = 2
	// quietMS: a running key with nothing logged for this long is closed as abandoned. The game logs no end for a key
	// the party left, and it writes its log in batches as fights start and end (Nov 10: one write held 2:45 of a boss
	// fight), so the line that says they left can come minutes later, and a boss fight is as silent as an empty
	// dungeon: this has to outlast the longest fight.
	quietMS = 10 * 60_000
	// lustMS: Bloodlust landing this long after the last counts as a new one (it lands on everyone at once).
	lustMS = 60_000
)

// activity is the events that show a player busy: casting, or a cast or channel landing.
var activity = map[string]bool{"SPELL_CAST_START": true, "SPELL_CAST_SUCCESS": true, "SPELL_DAMAGE": true,
	"SPELL_HEAL": true}

// LogTracker follows Mythic+ keys and raid pulls through combat log lines: the running one live, and the last one
// that ended. Finished keys and pulls go to their stores (SaveFinished), keys also to the history the projection
// learns from. Safe for one writer
// (the log follower) and many readers (the page).
type LogTracker struct {
	mu           sync.RWMutex
	timers       map[int]int64
	affixes      map[int]string
	rules        models.MPlusRules
	kinds        map[string]string
	store        models.KeyStore
	facts        map[int]models.DungeonFacts // by map ID, from reference/mplus-dungeons.json
	spells       map[int]string              // spell ID -> name, from every line seen
	specs        map[string]int              // player GUID -> spec, from COMBATANT_INFO
	owners       map[string]string           // pet GUID -> owner GUID
	names        map[string]string           // player GUID -> name
	up           map[[2]string]int64         // (player GUID, defensive) -> when the buff went up, in a key or not
	history      []models.Key                // saved keys, newest first
	unsaved      []savedRun
	lastTrack    *models.KeyTrack
	current      *keyRun
	left         *keyRun // a key Abandon closed, taken up again if the log goes on in it
	last         *models.Key
	ready        *readyWatch        // the party's buffs before the next key, while in a Mythic dungeon
	top          map[[2]int][]int64 // (map ID, level) -> top Warcraft Logs run times, for a first run's estimate
	asked        map[[2]int]bool    // (map ID, level) -> top runs asked for already
	version      int64              // goes up with every line read during a key or pull, when one starts, and as the ready check changes
	lastAt       int64              // the time of the last line read before the one being read
	raid         *raidSetup         // nil until EnableRaid: raid pulls aren't read
	pulls        []models.RaidPull  // finished raid pulls, newest first
	unsavedPulls []savedPull
}

// keyRun is the running key and what it takes to show it.
type keyRun struct {
	key       models.Key
	instance  string
	deaths    []models.Death
	casts     []models.Cast
	buffs     []models.BuffUp
	recent    map[string][]hitAt           // player GUID -> their last hits (absorbed ones too), newest last
	dead      map[string]bool              // mobs whose death was handled, so its second death line does nothing
	uncounted map[int]*models.UncountedNPC // enemies that died without being in the dungeon file, by NPC ID
	down      map[string]bool              // party members dead in this pull, by name
	fighting  map[string]map[int64]bool    // player -> seconds of the log they were alive in a pull
	acting    map[string]map[int64]bool    // player -> seconds of the log with a cast or hit of theirs
	casting   map[string]*openCast         // caster GUID -> its must-stop cast in progress
	kick      *pendingKick                 // a kick waiting for its interrupt
	lastKick  map[string]int64             // player -> when they last interrupted
	debuffs   map[[2]string]int64          // (party GUID, debuff) -> when a dispellable debuff landed
	removed   map[[2]string][2]int64       // a dispellable debuff just removed: when it landed and came off
	track     models.KeyTrack              // where the party went and mobs died
	sampled   map[string]int64             // party GUID -> when its position was last kept
	mobAt     map[string]models.TrackPoint // mob GUID -> where it was last seen
	taken     map[string]map[int64]int64   // player -> second of the log -> damage taken
	pullOf    map[int64]int                // second of the log -> the pull damage was taken in
	avoidable map[[2]string]*models.KeyHit // (player, ability) -> avoidable damage taken and direct hits
	pulls     map[int]*pullStat            // pull -> what it did to the priority mobs
	mobHP     map[string]int64             // mob GUID -> max health, from advanced logging
	bossPull  map[int]bool                 // pulls that are boss fights
	mobPull   map[string]int               // mob GUID -> the pull it was first hit in
	meterAt   int64                        // when the meters were last sampled for the replay
	party     []string                     // player GUIDs, from COMBATANT_INFO at the start
	auras     map[string][]int             // player GUID -> aura IDs at the start
	pull      int
	pullFrom  int64
	pullAt    int64 // the pull's start on the key's clock
	pullLast  int64
	facts     *models.DungeonFacts // nil when the reference file doesn't have the dungeon
	total     map[string]*models.KeyMeterRow
	inPull    map[string]*models.KeyMeterRow
	lastPull  models.KeyMeter
	raid      *raidRun                         // nil for a key
	abilities map[[2]string]*models.KeyAbility // (player, ability) -> what it did
	takenBy   map[[2]string]int64              // (player, ability) -> damage taken from it
}

// NewLogTracker tracks keys with each dungeon's time limit (map ID -> ms), affix names (ID -> name), the page's
// rules from mplus.toml, the season's dungeon facts by map ID, and the store of finished keys.
func NewLogTracker(timers map[int]int64, affixes map[int]string, rules models.MPlusRules,
	facts map[int]models.DungeonFacts, store models.KeyStore) (*LogTracker, error) {
	history, err := store.List()
	if err != nil {
		return nil, fmt.Errorf("reading saved keys: %w", err)
	}
	kinds := map[string]string{}
	for kind, names := range map[string][]string{"defensive": append(rules.Defensives.Names,
		rules.Defensives.Externals...), "healthstone": rules.Defensives.Healthstones,
		"health potion": rules.Defensives.HealthPotions, "combat potion": rules.Consumables.CombatPotions,
		"cc": rules.CC.Names} {
		for _, name := range names {
			kinds[name] = kind
		}
	}
	return &LogTracker{timers: timers, affixes: affixes, rules: rules, kinds: kinds, facts: facts, store: store,
		history: history,
		spells:  map[int]string{}, specs: map[string]int{}, owners: map[string]string{}, names: map[string]string{},
		up: map[[2]string]int64{}, top: map[[2]int][]int64{}, asked: map[[2]int]bool{}}, nil
}

// Reload reads the saved keys and raid pulls again (after Learn saved more from old logs), for the lists and the
// estimate.
func (k *LogTracker) Reload() error {
	history, err := k.store.List()
	if err != nil {
		return fmt.Errorf("reading saved keys: %w", err)
	}
	var pulls []models.RaidPull
	if k.raid != nil {
		if pulls, err = savedPulls(k.raid.store); err != nil {
			return err
		}
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.history, k.pulls = history, pulls
	k.version++
	return nil
}

// Version goes up whenever the keys change, so the page knows when to read them again.
func (k *LogTracker) Version() int64 {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.version
}

// Add reads one line. The game logs no end for a key the party left, so an abandoned key ends at the last line before
// the one that shows it's over, which can come much later. COMBAT_LOG_VERSION (the log starting, or a zone loading)
// says nothing about a key and isn't in step with the lines around it, so it's skipped.
func (k *LogTracker) Add(line models.LogLine) {
	if line.Event == "COMBAT_LOG_VERSION" {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	defer func() { k.lastAt = line.Time }()
	f := line.Fields
	k.resume(line.Event, f)
	if k.current != nil || line.Event == "CHALLENGE_MODE_START" {
		k.version++
	}
	if run := k.current; run != nil && run.raid != nil && line.Time-k.lastAt > quietMS {
		k.finishPull(false) // the game logged nothing for a while (a crash or a disconnect), so no end is coming
	}
	k.learn(line.Time, line.Event, f)
	switch line.Event {
	case "CHALLENGE_MODE_START":
		if len(f) >= 4 {
			k.start(line)
		}
		return
	case "CHALLENGE_MODE_END":
		if len(f) >= 4 {
			k.end(line)
		}
		return
	}
	run := k.current
	if run == nil && line.Event == "ENCOUNTER_START" && k.raid != nil && len(f) >= 5 && raidDifficulties[f[2]] != "" {
		k.startPull(line)
		return
	}
	if run != nil && run.raid != nil && k.raidLine(line) {
		return
	}
	if run == nil {
		if line.Event == "ZONE_CHANGE" {
			k.watchReady(line.Time, f)
			k.version++
		} else if k.ready != nil && k.readyLine(line.Event, f) {
			k.version++
		}
		return
	}
	switch line.Event {
	case "ZONE_CHANGE":
		if len(f) > 0 && f[0] != run.instance {
			k.closeCurrent(k.lastAt, "abandoned", 0)
		}
	case "COMBATANT_INFO":
		if len(f) > 0 && run.auras[f[0]] == nil {
			run.auras[f[0]] = auraIDs(f)
			run.party = append(run.party, f[0])
		}
	// A party's kill is logged as PARTY_KILL before the killing blow and UNIT_DIED after it, sometimes only one of the
	// two: the first one seen is the mob's death.
	case "UNIT_DIED", "PARTY_KILL":
		switch {
		case line.Event == "UNIT_DIED" && len(f) >= 9 && strings.HasPrefix(f[4], "Player-") && f[len(f)-1] != "1":
			k.death(line.Time, f)
		case len(f) >= 5 && !run.dead[f[4]]:
			run.dead[f[4]] = true
			k.mobDeathAt(line.Time, f[4])
			k.mobDown(line.Time, f[4])
			if len(f) >= 7 && run.facts != nil {
				k.mobDied(f[4], line.Time)
				k.kill(line.Time, f)
			}
		}
	case "ENCOUNTER_START":
		if len(f) >= 3 && f[2] == keyDifficulty {
			k.newPull(line.Time)
			run.bossPull[run.pull] = true
			run.key.Bosses = append(run.key.Bosses, models.KeyBoss{Name: f[1], StartMS: k.clockAt(line.Time)})
		}
	case "ENCOUNTER_END":
		if n := len(run.key.Bosses); n > 0 && len(f) >= 5 && f[2] == keyDifficulty && run.key.Bosses[n-1].EndMS == 0 {
			run.key.Bosses[n-1].EndMS, run.key.Bosses[n-1].Kill = k.clockAt(line.Time), f[4] == "1"
		}
	default:
		k.event(line.Time, line.Event, f)
	}
}

// Abandon closes the running key as abandoned at its last line, for a party that left when the log hasn't said so
// yet; false when no key is running. If the log then goes on in the key, it runs again. A raid pull is left alone:
// the game writes nothing during a fight, and a pull can outlast any silence worth waiting for.
func (k *LogTracker) Abandon() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	run := k.current
	if run == nil || run.raid != nil {
		return false
	}
	k.closeCurrent(k.lastAt, "abandoned", 0)
	k.left = run
	k.version++
	return true
}

// CloseQuiet abandons the running key once nothing was logged for quietMS before now (ms since the epoch); the log
// follower calls it between reads.
func (k *LogTracker) CloseQuiet(now int64) {
	k.mu.RLock()
	quiet := now-k.lastAt > quietMS
	k.mu.RUnlock()
	if quiet {
		k.Abandon()
	}
}

// resume takes the key Abandon closed up again when the log goes on in it (it was only written late): on the next
// line, unless that's a new key or a zone outside its dungeon.
func (k *LogTracker) resume(event string, f []string) {
	run := k.left
	if run == nil {
		return
	}
	k.left = nil
	if event == "CHALLENGE_MODE_START" || event == "ZONE_CHANGE" && len(f) > 0 && f[0] != run.instance {
		return
	}
	run.key.Status, run.key.End = "running", 0
	if n := len(run.track.Pulls); n > 0 {
		run.track.Pulls[n-1].EndMS = 0
	}
	k.current = run
}

// learn keeps what every line tells about names, specs, spell names, pets (from summons and advanced logging's owner
// field) and defensive buffs, in keys or not, so COMBATANT_INFO's aura IDs get names from lines seen elsewhere.
func (k *LogTracker) learn(at int64, event string, f []string) {
	if event == "COMBATANT_INFO" && len(f) >= 25 {
		k.specs[f[0]] = atoi(f[24])
		return
	}
	if len(f) < 8 {
		return
	}
	for _, i := range []int{0, 4} {
		if strings.HasPrefix(f[i], "Player-") && f[i+1] != "nil" {
			k.names[f[i]], _ = splitName(f[i+1])
		}
	}
	if strings.HasPrefix(event, "SPELL_") && len(f) > 10 {
		k.spells[atoi(f[8])] = f[9]
	}
	if event == "SPELL_SUMMON" {
		k.owners[f[4]] = f[0]
	}
	if (event == "SPELL_AURA_APPLIED" || event == "SPELL_AURA_REMOVED") && len(f) > 11 && f[11] == "BUFF" &&
		strings.HasPrefix(f[4], "Player-") && k.kinds[f[9]] == "defensive" {
		k.defensiveBuff(at, event == "SPELL_AURA_APPLIED", f[4], f[9])
	}
	if at := advancedAt(event); at > 0 && len(f) > at+1 && f[at] == f[0] && f[at+1] != "0000000000000000" {
		k.owners[f[0]] = f[at+1]
	}
}

// advancedAt is where an event's advanced-logging actor block starts, or 0 when it has none.
func advancedAt(event string) int {
	switch event {
	case "SWING_DAMAGE":
		return 8
	case "SPELL_DAMAGE", "SPELL_PERIODIC_DAMAGE", "RANGE_DAMAGE", "SPELL_HEAL", "SPELL_PERIODIC_HEAL",
		"SPELL_CAST_SUCCESS", "SPELL_ENERGIZE":
		return 11
	}
	return 0
}

// start opens a key: CHALLENGE_MODE_START,"Voidscar Arena",2923,585,17,[9,10,147].
func (k *LogTracker) start(line models.LogLine) {
	f := line.Fields
	k.closeCurrent(k.lastAt, "abandoned", 0)
	k.ready = nil // COMBATANT_INFO at the start has everyone's buffs
	mapID := atoi(f[2])
	run := newRun(models.Key{Dungeon: f[0], MapID: mapID, Level: atoi(f[3]), Start: line.Time, Status: "running",
		TimerMS: k.timers[mapID]}, f[1])
	if facts, ok := k.facts[mapID]; ok {
		run.facts, run.key.Reference = &facts, true
		run.key.Forces = &models.KeyForces{Total: facts.Forces.Total, At: []models.ForcesPoint{}}
	}
	if len(f) >= 5 {
		for _, id := range strings.Split(strings.Trim(f[4], "[]"), ",") {
			if v, err := strconv.Atoi(strings.TrimSpace(id)); err == nil {
				run.key.Affixes = append(run.key.Affixes, models.Affix{ID: v, Name: k.affixes[v]})
			}
		}
	}
	k.current = run
}

// newRun is a run starting with key in the instance the log names (a key's or a raid pull's), with nothing seen yet.
func newRun(key models.Key, instance string) *keyRun {
	return &keyRun{instance: instance, key: key, recent: map[string][]hitAt{}, dead: map[string]bool{},
		casting: map[string]*openCast{}, lastKick: map[string]int64{}, debuffs: map[[2]string]int64{},
		removed: map[[2]string][2]int64{}, uncounted: map[int]*models.UncountedNPC{},
		down: map[string]bool{}, fighting: map[string]map[int64]bool{}, acting: map[string]map[int64]bool{},
		avoidable: map[[2]string]*models.KeyHit{}, pullOf: map[int64]int{}, pulls: map[int]*pullStat{},
		mobHP: map[string]int64{}, bossPull: map[int]bool{}, mobPull: map[string]int{},
		taken: map[string]map[int64]int64{}, sampled: map[string]int64{}, mobAt: map[string]models.TrackPoint{},
		track: models.KeyTrack{Players: map[string][]models.TrackPoint{}, Deaths: []models.TrackPoint{},
			Meters: map[string][]models.MeterPoint{}, Bosses: map[string][]models.TrackPoint{}},
		auras: map[string][]int{}, total: map[string]*models.KeyMeterRow{}, inPull: map[string]*models.KeyMeterRow{},
		abilities: map[[2]string]*models.KeyAbility{}, takenBy: map[[2]string]int64{}}
}

// end closes the running key: CHALLENGE_MODE_END,2923,1,17,1686807,... (timed or not, level, the game's time).
// 0,0,0 is logged before the next key starts, so a key still running then was abandoned.
func (k *LogTracker) end(line models.LogLine) {
	f := line.Fields
	at, status, final := k.lastAt, "abandoned", int64(atoi(f[3]))
	switch {
	case final == 0:
	case f[1] == "1":
		at, status = line.Time, "timed"
	default:
		at, status = line.Time, "depleted"
	}
	k.closeCurrent(at, status, final)
}

// closeCurrent ends the running key, keeps it as the last one, and queues it for saving unless it was a reset.
func (k *LogTracker) closeCurrent(at int64, status string, final int64) {
	run := k.current
	if run == nil {
		return
	}
	if run.raid != nil {
		k.finishPull(false)
		return
	}
	run.key.End, run.key.Status, run.key.FinalMS = at, status, final
	endPull(run)
	key := k.details(run, at)
	key.Abilities, key.Taken = abilities(run)
	track := k.trackOf(run)
	k.last, k.lastTrack, k.current = &key, &track, nil
	if at-key.Start < minSavedMS {
		return
	}
	// A restart reads the log again, so a key already saved replaces its copy instead of adding a second.
	if i := slices.IndexFunc(k.history, func(h models.Key) bool { return h.Start == key.Start }); i >= 0 {
		k.history[i] = key
	} else {
		k.history = append([]models.Key{key}, k.history...)
	}
	k.unsaved = append(k.unsaved, savedRun{key, track})
}

// death records a player dying, killed by the last thing that hit them (absorbed hits too, as Warcraft Logs counts).
func (k *LogTracker) death(at int64, f []string) {
	run := k.current
	name, _ := splitName(f[5])
	var hit hitAt
	if hits := run.recent[f[4]]; len(hits) > 0 {
		hit = hits[len(hits)-1]
	}
	run.deaths = append(run.deaths, models.Death{Fight: run.pull, At: at, Player: name, Ability: hit.ability})
	run.down[name] = true
	run.key.Deaths = append(run.key.Deaths, models.KeyDeath{Player: name, ClockMS: k.clockAt(at), Killer: hit.source,
		Ability: hit.ability})
	if run.raid != nil {
		run.raid.recaps = append(run.raid.recaps, recap(run.recent[f[4]], at))
	}
	delete(run.recent, f[4])
}

// hitAt is a hit a player took.
type hitAt struct {
	ability, source string
	amount, at      int64
}

// recentHits is how many of a player's last hits a run keeps: the newest is their killer, all of them a raid
// death's recap.
const recentHits = 3

// took keeps a hit a player (by GUID) took, dropping the oldest past recentHits.
func (r *keyRun) took(guid string, hit hitAt) {
	hits := append(r.recent[guid], hit)
	r.recent[guid] = hits[max(0, len(hits)-recentHits):]
}

// kill counts a mob's forces when it dies (Creature-0-3882-2859-27547-245336-00006BE4EA: NPC 245336),
// or keeps an enemy the dungeon file doesn't have at all (bosses aside), so the file can be fixed.
func (k *LogTracker) kill(at int64, f []string) {
	run, guid := k.current, f[4]
	npc := npcOf(guid)
	count, listed := run.facts.Forces.NPCs[npc]
	if !listed && npc != 0 && hasFlag(f[6], hostileFlag) && !slices.Contains(run.facts.Bosses, npc) {
		u := run.uncounted[npc]
		if u == nil {
			u = &models.UncountedNPC{NPC: npc, Name: f[5]}
			run.uncounted[npc] = u
		}
		u.Deaths++
	}
	if count == 0 || run.facts.Forces.Total == 0 {
		return
	}
	forces := run.key.Forces
	forces.Count += count
	forces.Pct = 100 * float64(forces.Count) / float64(forces.Total)
	forces.At = append(forces.At, models.ForcesPoint{ClockMS: k.clockAt(at), Pct: forces.Pct})
}

// event handles the lines inside a key that the page reads: players, tracked casts and buffs, interrupts, damage
// and healing.
func (k *LogTracker) event(at int64, event string, f []string) {
	run := k.current
	if len(f) < 8 {
		return
	}
	k.position(at, event, f)
	src, dst := k.player(f[0], f[2]), k.player(f[4], f[6])
	if src != "" && strings.HasPrefix(f[0], "Player-") && activity[event] {
		mark(run.acting, src, at/1000, at/1000+1) // a cast or hit covers about a global cooldown
	}
	switch event {
	case "SPELL_AURA_APPLIED":
		if n := len(run.key.Lusts); dst != "" && len(f) > 9 && slices.Contains(k.rules.Lust.Names, f[9]) &&
			(n == 0 || k.clockAt(at)-run.key.Lusts[n-1] > lustMS) {
			run.key.Lusts = append(run.key.Lusts, k.clockAt(at))
		}
	case "SPELL_CAST_SUCCESS":
		if src != "" && strings.HasPrefix(f[0], "Player-") {
			run.ability(src, f[9]).Casts++
		}
		if kind := k.kinds[f[9]]; kind != "" && src != "" {
			if kind != "cc" && kind != "combat potion" { // the death rules' kit is survival buttons
				run.casts = append(run.casts, models.Cast{Fight: run.pull, At: at, Player: src, Ability: f[9]})
			}
			run.key.Casts = append(run.key.Casts, models.KeyCast{Player: src, Ability: f[9], Kind: kind,
				ClockMS: k.clockAt(at)})
		}
	case "SPELL_INTERRUPT":
		if src != "" && len(f) > 12 {
			run.key.Interrupts = append(run.key.Interrupts, models.KeyInterrupt{Player: src, Kick: f[9], Target: f[5],
				Spell: f[12], ClockMS: k.clockAt(at)})
			k.tally(src, func(r *models.KeyMeterRow) { r.Interrupts++ })
		}
	case "SPELL_DISPEL", "SPELL_STOLEN":
		if src != "" {
			ally := hasFlag(f[6], friendlyFlag)
			k.tally(src, func(r *models.KeyMeterRow) {
				if ally {
					r.Dispels++
				} else {
					r.Purges++
				}
			})
		}
	case "ENVIRONMENTAL_DAMAGE":
		if strings.HasPrefix(f[4], "Player-") && len(f) > 9+advancedFields {
			run.took(f[4], hitAt{ability: f[8+advancedFields], source: "Environment",
				amount: int64(atoi(f[9+advancedFields])), at: at})
		}
	case "SPELL_DAMAGE", "SPELL_PERIODIC_DAMAGE", "RANGE_DAMAGE", "SWING_DAMAGE", "SPELL_MISSED",
		"SPELL_PERIODIC_MISSED", "SWING_MISSED", "RANGE_MISSED":
		k.damage(at, event, f, src, dst)
	case "SPELL_HEAL", "SPELL_PERIODIC_HEAL":
		if src != "" && len(f) > 32 {
			k.heal(src, f[9], int64(atoi(f[30])-atoi(f[32])))
		}
	case "SPELL_ABSORBED":
		if n := len(f); n >= 18 {
			if caster := k.player(f[n-10], f[n-8]); caster != "" {
				k.heal(caster, f[n-5], int64(atoi(f[n-3])))
			}
		}
	}
	k.stopEvent(at, event, f, src, dst)
	k.sampleMeters(at)
}

// damage counts a hit: to the raid's meters when a player (or their pet) hit an enemy, as damage taken (and avoidable
// damage) when it hit a party member, and as the killer-to-be. Fighting between the two sides keeps the pull going.
func (k *LogTracker) damage(at int64, event string, f []string, src, dst string) {
	run := k.current
	ability := "Melee"
	if !strings.HasPrefix(event, "SWING") && len(f) > 9 {
		ability = f[9]
	}
	amount := hitAmount(event, f)
	if strings.HasPrefix(f[4], "Player-") {
		run.took(f[4], hitAt{ability: ability, source: unitName(f[1]), amount: amount, at: at})
	}
	dealt := src != "" && hasFlag(f[6], hostileFlag)
	if dealt || (dst != "" && hasFlag(f[2], hostileFlag)) {
		k.keepPull(at)
	}
	if dealt {
		k.tally(src, func(r *models.KeyMeterRow) { r.Damage += amount })
		a := run.ability(src, ability)
		a.Damage, a.Hits = a.Damage+amount, a.Hits+1
		k.hitMob(at, f[4], f[5], src, amount)
	}
	if hasFlag(f[6], friendlyFlag) && slices.Contains(run.party, f[4]) {
		avoidable := event == "SPELL_DAMAGE" && run.facts != nil && slices.Contains(run.facts.Avoidable, atoi(f[8]))
		name := k.names[f[4]]
		if run.taken[name] == nil {
			run.taken[name] = map[int64]int64{}
		}
		run.taken[name][at/1000] += amount
		run.takenBy[[2]string{name, ability}] += amount
		run.pullOf[at/1000] = run.pull
		k.tally(name, func(r *models.KeyMeterRow) {
			r.DamageTaken += amount
			if avoidable {
				r.Avoidable += amount
			}
		})
		if avoidable {
			hit := run.avoidable[[2]string{name, f[9]}]
			if hit == nil {
				hit = &models.KeyHit{Player: name, Ability: f[9]}
				run.avoidable[[2]string{name, f[9]}] = hit
			}
			hit.Damage, hit.Hits = hit.Damage+amount, hit.Hits+1
		}
	}
}

// heal counts a player's healing with an ability, to the meters and to the ability.
func (k *LogTracker) heal(player, ability string, amount int64) {
	k.tally(player, func(r *models.KeyMeterRow) { r.Healing += amount })
	a := k.current.ability(player, ability)
	a.Healing, a.Hits = a.Healing+amount, a.Hits+1
}

// ability is a player's row for an ability, made on its first use.
func (run *keyRun) ability(player, name string) *models.KeyAbility {
	id := [2]string{player, name}
	if run.abilities[id] == nil {
		run.abilities[id] = &models.KeyAbility{Player: player, Ability: name}
	}
	return run.abilities[id]
}

// abilities is what each player's abilities did over the run, and what each took damage from, most first.
func abilities(run *keyRun) ([]models.KeyAbility, []models.KeyHit) {
	done := make([]models.KeyAbility, 0, len(run.abilities))
	for _, a := range run.abilities {
		done = append(done, *a)
	}
	slices.SortFunc(done, func(a, b models.KeyAbility) int {
		return cmp.Or(cmp.Compare(b.Damage+b.Healing, a.Damage+a.Healing), cmp.Compare(b.Casts, a.Casts),
			cmp.Compare(a.Player, b.Player), cmp.Compare(a.Ability, b.Ability))
	})
	taken := make([]models.KeyHit, 0, len(run.takenBy))
	for id, damage := range run.takenBy {
		taken = append(taken, models.KeyHit{Player: id[0], Ability: id[1], Damage: damage})
	}
	slices.SortFunc(taken, func(a, b models.KeyHit) int {
		return cmp.Or(cmp.Compare(b.Damage, a.Damage), cmp.Compare(a.Player, b.Player), cmp.Compare(a.Ability, b.Ability))
	})
	return done, taken
}

// hitAmount is a damage line's amount plus what was absorbed, or a fully absorbed miss's amount.
func hitAmount(event string, f []string) int64 {
	suffix := 11 + advancedFields
	if strings.HasPrefix(event, "SWING") {
		suffix = 8 + advancedFields
	}
	if strings.HasSuffix(event, "_MISSED") {
		if miss := suffix - advancedFields; len(f) > miss+2 && f[miss] == "ABSORB" {
			return int64(atoi(f[miss+2]))
		}
		return 0
	}
	if len(f) > suffix+6 {
		return int64(atoi(f[suffix]) + atoi(f[suffix+6]))
	}
	return 0
}

// defensiveBuff opens or closes a defensive buff on a player, in a key or not, so one up before the key started
// still counts; a span ending in a key is kept for the party's deaths.
func (k *LogTracker) defensiveBuff(at int64, applied bool, guid, ability string) {
	key := [2]string{guid, ability}
	if applied {
		k.up[key] = at
		return
	}
	from, ok := k.up[key]
	delete(k.up, key)
	if run := k.current; ok && run != nil && slices.Contains(run.party, guid) {
		run.buffs = append(run.buffs, models.BuffUp{Fight: run.pull, Start: from, End: at, Player: k.names[guid],
			Ability: ability})
	}
}

// player is the party member a unit is or belongs to (a pet counts as its owner), or "" for anything else. The party
// is who COMBATANT_INFO listed at the key's start.
func (k *LogTracker) player(guid, flags string) string {
	if owner, ok := k.owners[guid]; ok {
		guid = owner
	}
	if hasFlag(flags, friendlyFlag) && slices.Contains(k.current.party, guid) {
		return k.names[guid]
	}
	return ""
}

// keepPull starts a new pull when fighting resumes after pullGapMS, else extends the current one.
func (k *LogTracker) keepPull(at int64) {
	run := k.current
	if run.pullLast == 0 || at-run.pullLast > pullGapMS {
		k.newPull(at)
	}
	for _, guid := range run.party {
		if name := k.names[guid]; !run.down[name] {
			for s := run.pullLast / 1000; s <= at/1000; s++ {
				mark(run.fighting, name, s)
			}
		}
	}
	run.pullLast = at
}

// newPull closes the current pull's meter and starts the next.
func (k *LogTracker) newPull(at int64) {
	run := k.current
	if len(run.inPull) > 0 {
		run.lastPull = k.pullMeter(run)
	}
	endPull(run)
	run.pull++
	run.pullFrom, run.pullLast, run.pullAt, run.inPull = at, at, k.clockAt(at), map[string]*models.KeyMeterRow{}
	run.down = map[string]bool{} // a member who died is back by the next pull (a release, or a battle rez)
	run.track.Pulls = append(run.track.Pulls, models.PullSpan{StartMS: run.pullAt})
}

// mark sets seconds in a player's set.
func mark(sets map[string]map[int64]bool, player string, seconds ...int64) {
	if sets[player] == nil {
		sets[player] = map[int64]bool{}
	}
	for _, s := range seconds {
		sets[player][s] = true
	}
}

// endPull closes the track's open pull span at the pull's last fighting.
func endPull(run *keyRun) {
	if n := len(run.track.Pulls); n > 0 && run.track.Pulls[n-1].EndMS == 0 {
		run.track.Pulls[n-1].EndMS = run.clock(run.pullLast)
	}
}

// tally updates a player's meter row for the pull and the key.
func (k *LogTracker) tally(player string, update func(*models.KeyMeterRow)) {
	for _, m := range []map[string]*models.KeyMeterRow{k.current.total, k.current.inPull} {
		if m[player] == nil {
			m[player] = &models.KeyMeterRow{Player: player}
		}
		update(m[player])
	}
}

// clockAt is the running run's clock at a log time (keyRun.clock).
func (k *LogTracker) clockAt(at int64) int64 {
	return k.current.clock(at)
}

// clock is the run's clock at a log time: a key's time since its countdown plus the death penalties, a raid pull's
// time since it started.
func (r *keyRun) clock(at int64) int64 {
	if r.raid != nil {
		return max(0, at-r.key.Start)
	}
	return clock(r.key.Start, at, len(r.deaths))
}

// elapsed is the run's fighting time at a log time: a key's from the end of its countdown, a raid pull's from its
// start.
func (r *keyRun) elapsed(at int64) int64 {
	if r.raid != nil {
		return max(0, at-r.key.Start)
	}
	return max(0, at-r.key.Start-keyCountdownMS)
}

func clock(start, at int64, deaths int) int64 {
	return max(0, at-start-keyCountdownMS) + int64(deaths)*deathPenaltyMS
}

// auraIDs reads COMBATANT_INFO's aura list, its last bracketed field: [source,auraID,stacks,source,auraID,...].
func auraIDs(f []string) []int {
	ids := []int{}
	for i := len(f) - 1; i >= 0; i-- {
		if strings.HasPrefix(f[i], "[") {
			parts := strings.Split(strings.Trim(f[i], "[]"), ",")
			for j := 1; j < len(parts); j += 3 {
				ids = append(ids, atoi(parts[j]))
			}
			break
		}
	}
	return ids
}
