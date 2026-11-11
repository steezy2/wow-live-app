package service

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"wow-live/backend/models"
)

const (
	// readyDifficulty is ZONE_CHANGE's difficulty in a Mythic dungeon before its key starts (8 once it runs).
	readyDifficulty = "23"
	// partyFlags: a unit that's the logging player (0x1) or in their party (0x2); raidFlags adds their raid (0x4).
	partyFlags = 0x3
	raidFlags  = 0x7
)

// readyWatch is what the log has shown of the party's buffs since they zoned into a Mythic dungeon, before its key, or
// into a raid, between pulls: each member's buffs up, and the ones seen fade (ran out, died, cancelled). Buffs from
// before logging started there are never seen, but a raid pull's start lists everyone's.
type readyWatch struct {
	since      int64
	instance   string
	dungeon    string
	raid       bool
	difficulty string                    // the raid's ("Heroic"), for the Vantus Rune; empty in a dungeon
	members    map[string]bool           // party GUIDs seen
	up         map[string]map[int]string // GUID -> buffs up (aura ID -> name, "" for one the log never named)
	faded      map[string]map[int]string // GUID -> buffs seen fade
	seeded     map[string]bool           // GUIDs whose buffs a raid pull's start listed: what they lack is missing
}

// watchReady starts the ready check on entering a Mythic dungeon (ZONE_CHANGE,2813,"Murder Row",23) or, with raids
// read, a raid (difficulties 14-17), and stops it anywhere else. Zoning into the same place again keeps it: the log
// doesn't show the party's buffs a second time, but a raid's difficulty can change.
func (k *LogTracker) watchReady(at int64, f []string) {
	raid := len(f) >= 3 && k.raid != nil && raidDifficulties[f[2]] != ""
	switch {
	case len(f) < 3 || (f[2] != readyDifficulty && !raid):
		k.ready = nil
		return
	case k.ready == nil || k.ready.instance != f[0]:
		k.ready = &readyWatch{since: at, instance: f[0], dungeon: f[1], raid: raid, members: map[string]bool{},
			seeded: map[string]bool{}, up: map[string]map[int]string{}, faded: map[string]map[int]string{}}
	}
	if raid {
		k.ready.difficulty = raidDifficulties[f[2]]
	}
}

// seedReady sets the raid's ready check to what a raid pull's start listed: its raiders (one who left before it is
// gone, with the buff they gave) and each one's auras, but the party buffs of those who died in the pull, which a
// death takes off (flask, food and Vantus Rune stay).
func (k *LogTracker) seedReady(auras map[string][]int, died map[string]bool) {
	r := k.ready
	if r == nil || !r.raid {
		return
	}
	r.members = map[string]bool{}
	for guid, ids := range auras {
		r.members[guid], r.seeded[guid], r.up[guid], r.faded[guid] = true, true, map[int]string{}, map[int]string{}
		for _, id := range ids {
			if name := k.spells[id]; !died[guid] || k.rules.Buffs[name] == "" {
				r.up[guid][id] = name
			}
		}
	}
}

// readyLine keeps what a line says about the party and their buffs for the ready check; true when a buff changed.
func (k *LogTracker) readyLine(event string, f []string) bool {
	r := k.ready
	if len(f) < 8 {
		return false
	}
	flags := int64(partyFlags)
	if r.raid {
		flags = raidFlags
	}
	for _, i := range []int{0, 4} {
		if strings.HasPrefix(f[i], "Player-") && hasFlag(f[i+2], flags) {
			r.members[f[i]] = true
		}
	}
	if len(f) < 12 || f[11] != "BUFF" || !r.members[f[4]] {
		return false
	}
	guid, id, buff := f[4], atoi(f[8]), f[9]
	for _, m := range []map[string]map[int]string{r.up, r.faded} {
		if m[guid] == nil {
			m[guid] = map[int]string{}
		}
	}
	switch event {
	case "SPELL_AURA_APPLIED", "SPELL_AURA_REFRESH":
		r.up[guid][id] = buff
		delete(r.faded[guid], id)
	case "SPELL_AURA_REMOVED":
		delete(r.up[guid], id)
		r.faded[guid][id] = buff
	default:
		return false
	}
	return true
}

// Ready is the ready check before the next key, while the party is in a Mythic dungeon with no key running: for each
// member, the flask, food and party buffs they're missing (seen fade, or a party buff someone else has) and the ones
// not seen since logging started there. False outside such a dungeon (a raid's is RaidLive's).
func (k *LogTracker) Ready() (models.ReadyCheck, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.ready == nil || k.ready.raid {
		return models.ReadyCheck{}, false
	}
	return k.readyCheck()
}

// readyCheck is the ready check as Ready describes it, in a raid with Vantus Rune too where the raid's rune works;
// false while a key or pull runs. Only the party buffs a class in the group gives are asked for: while a member's
// class isn't known yet (no COMBATANT_INFO this session), a buff someone has up counts its class in. The caller holds
// k.mu.
func (k *LogTracker) readyCheck() (models.ReadyCheck, bool) {
	r := k.ready
	if r == nil || k.current != nil {
		return models.ReadyCheck{}, false
	}
	classes, anyone, unknown := map[string]bool{}, map[string]bool{}, false
	for guid := range r.members {
		class := specs[k.specs[guid]].Class
		classes[class], unknown = true, unknown || class == ""
		for _, buff := range r.up[guid] {
			anyone[buff] = true
		}
	}
	for buff := range anyone {
		if unknown && k.rules.Buffs[buff] != "" {
			classes[k.rules.Buffs[buff]] = true
		}
	}
	buffs := k.applicableBuffs(classes)
	check := models.ReadyCheck{SinceMS: r.since, Dungeon: r.dungeon, Players: []models.ReadyPlayer{}}
	for guid := range r.members {
		p := models.ReadyPlayer{Player: k.names[guid], Class: specs[k.specs[guid]].Class, Missing: []string{},
			NotSeen: []string{}}
		up, faded := slices.Collect(maps.Values(r.up[guid])), slices.Collect(maps.Values(r.faded[guid]))
		for kind, c := range k.consumables(r.difficulty) {
			switch {
			case c.had(r.up[guid]):
			case r.seeded[guid] || c.had(r.faded[guid]):
				p.Missing = append(p.Missing, kind)
			default:
				p.NotSeen = append(p.NotSeen, kind)
			}
		}
		for _, buff := range buffs {
			switch {
			case slices.Contains(up, buff):
			case anyone[buff] || slices.Contains(faded, buff):
				p.Missing = append(p.Missing, buff)
			default:
				p.NotSeen = append(p.NotSeen, buff)
			}
		}
		slices.Sort(p.Missing)
		slices.Sort(p.NotSeen)
		check.Players = append(check.Players, p)
	}
	slices.SortFunc(check.Players, func(a, b models.ReadyPlayer) int { return cmp.Compare(a.Player, b.Player) })
	return check, true
}

// consumable is what counts as one kind of consumable buff: an aura whose name contains one of the patterns ("Flask
// of", "Well Fed"), or one of the IDs, for an aura the log never names (only COMBATANT_INFO's list shows it).
type consumable struct {
	patterns []string
	ids      []int
}

// consumables are the buffs everyone should have by kind ("flask", "food", and in a raid of a difficulty its rune
// works in, "Vantus Rune"); difficulty is empty in a dungeon.
func (k *LogTracker) consumables(difficulty string) map[string]consumable {
	c := k.rules.Consumables
	kinds := map[string]consumable{"flask": {patterns: c.Flask}, "food": {patterns: c.Food}}
	if difficulty == "" {
		return kinds
	}
	if v := k.raid.rules.Consumables; len(v.Vantus)+len(v.VantusIDs) > 0 &&
		slices.Contains(v.VantusDifficulties, difficulty) {
		kinds["Vantus Rune"] = consumable{patterns: v.Vantus, ids: v.VantusIDs}
	}
	return kinds
}

// applicableBuffs are the party buffs a class in the group (classes) gives: one nobody there can give (a warrior who
// left after buffing) isn't asked for.
func (k *LogTracker) applicableBuffs(classes map[string]bool) []string {
	var buffs []string
	for buff, class := range k.rules.Buffs {
		if classes[class] {
			buffs = append(buffs, buff)
		}
	}
	return buffs
}

// buffGapMS: a raid pull's buff gap shorter than this isn't listed (a buff put back by the next cast).
const buffGapMS = 5_000

// gapKind is what a raid pull's buff gaps track an aura as: a raid buff a class in the pull gives (its name), "flask"
// or "food"; empty for anything else.
func (k *LogTracker) gapKind(run *keyRun, id int, name string) string {
	if class := k.rules.Buffs[name]; class != "" {
		if slices.ContainsFunc(run.party, func(guid string) bool { return specs[k.specs[guid]].Class == class }) {
			return name
		}
		return ""
	}
	for kind, c := range k.consumables("") {
		if c.had(map[int]string{id: name}) {
			return kind
		}
	}
	return ""
}

// buffChanged notes a raider's buff coming up or off at clockMS in a raid pull: for a kind gaps track, off opens a
// gap and up closes it (the first up is also when a buff missing at the start came).
func (k *LogTracker) buffChanged(run *keyRun, clockMS int64, player string, id int, name string, up bool) {
	r, kind := run.raid, k.gapKind(run, id, name)
	if kind == "" {
		return
	}
	key := [2]string{player, kind}
	from, open := r.gapFrom[key]
	switch {
	case up && open:
		r.gaps = append(r.gaps, models.RaidMissing{Player: player, Buff: kind, FromMS: from, ToMS: clockMS})
		delete(r.gapFrom, key)
	case !up && !open:
		r.gapFrom[key] = clockMS
	}
	if _, seen := r.firstUp[key]; up && !seen {
		r.firstUp[key] = clockMS
	}
}

// buffGaps is a finished pull's buff gaps, in order: what each raider lacked at the start (start, from buffCheck) until
// it first came up, else to the end, then each gap in the pull (one still open ends at the end); none under
// buffGapMS, and none of a raider who died as it opened and wasn't brought back before it closed (their death says it).
func (k *LogTracker) buffGaps(run *keyRun, start []models.KeyBuffCheck, deaths []models.KeyDeath,
	endMS int64) []models.RaidMissing {
	r := run.raid
	gaps := slices.Clone(r.gaps)
	for _, b := range start {
		for _, buff := range b.Missing {
			to, ok := r.firstUp[[2]string{b.Player, buff}]
			if !ok {
				to = endMS
			}
			gaps = append(gaps, models.RaidMissing{Player: b.Player, Buff: buff, ToMS: to})
		}
	}
	for key, from := range r.gapFrom {
		gaps = append(gaps, models.RaidMissing{Player: key[0], Buff: key[1], FromMS: from, ToMS: endMS})
	}
	gaps = slices.DeleteFunc(gaps, func(g models.RaidMissing) bool {
		died := slices.ContainsFunc(deaths, func(d models.KeyDeath) bool {
			return d.Player == g.Player && max(d.ClockMS-g.FromMS, g.FromMS-d.ClockMS) <= breakDeathMS
		})
		back := slices.ContainsFunc(r.rezzed[g.Player], func(at int64) bool { return at > g.FromMS && at < g.ToMS })
		return g.ToMS-g.FromMS < buffGapMS || died && !back
	})
	slices.SortFunc(gaps, func(a, b models.RaidMissing) int {
		return cmp.Or(cmp.Compare(a.FromMS, b.FromMS), cmp.Compare(a.Player, b.Player), cmp.Compare(a.Buff, b.Buff))
	})
	return gaps
}

// had says whether one of the auras (ID -> name) is this consumable.
func (c consumable) had(auras map[int]string) bool {
	for id, name := range auras {
		if slices.Contains(c.ids, id) || slices.ContainsFunc(c.patterns, func(p string) bool {
			return strings.Contains(name, p)
		}) {
			return true
		}
	}
	return false
}
