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
	since    int64
	instance string
	dungeon  string
	raid     bool
	members  map[string]bool            // party GUIDs seen
	up       map[string]map[string]bool // GUID -> buff names up
	faded    map[string]map[string]bool // GUID -> buff names seen fade
	seeded   map[string]bool            // GUIDs whose buffs a raid pull's start listed: what they lack is missing
}

// watchReady starts the ready check on entering a Mythic dungeon (ZONE_CHANGE,2813,"Murder Row",23) or, with raids
// read, a raid (difficulties 14-17), and stops it anywhere else. Zoning into the same place again keeps it: the log
// doesn't show the party's buffs a second time.
func (k *LogTracker) watchReady(at int64, f []string) {
	raid := len(f) >= 3 && k.raid != nil && raidDifficulties[f[2]] != ""
	switch {
	case len(f) < 3 || (f[2] != readyDifficulty && !raid):
		k.ready = nil
	case k.ready == nil || k.ready.instance != f[0]:
		k.ready = &readyWatch{since: at, instance: f[0], dungeon: f[1], raid: raid, members: map[string]bool{},
			seeded: map[string]bool{}, up: map[string]map[string]bool{}, faded: map[string]map[string]bool{}}
	}
}

// seedReady sets the raid's ready check to what a raid pull's start listed: each raider's auras, by name, but the
// party buffs of those who died in the pull, which a death takes off (flask, food and Vantus Rune stay).
func (k *LogTracker) seedReady(auras map[string][]int, died map[string]bool) {
	r := k.ready
	if r == nil || !r.raid {
		return
	}
	for guid, ids := range auras {
		r.members[guid], r.seeded[guid], r.up[guid], r.faded[guid] = true, true, map[string]bool{}, map[string]bool{}
		for _, id := range ids {
			if name := k.spells[id]; !died[guid] || k.rules.Buffs[name] == "" {
				r.up[guid][name] = true
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
	guid, buff := f[4], f[9]
	for _, m := range []map[string]map[string]bool{r.up, r.faded} {
		if m[guid] == nil {
			m[guid] = map[string]bool{}
		}
	}
	switch event {
	case "SPELL_AURA_APPLIED", "SPELL_AURA_REFRESH":
		r.up[guid][buff] = true
		delete(r.faded[guid], buff)
	case "SPELL_AURA_REMOVED":
		delete(r.up[guid], buff)
		r.faded[guid][buff] = true
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

// readyCheck is the ready check as Ready describes it, in a raid with Vantus Rune too; false while a key or pull
// runs. The caller holds k.mu.
func (k *LogTracker) readyCheck() (models.ReadyCheck, bool) {
	r := k.ready
	if r == nil || k.current != nil {
		return models.ReadyCheck{}, false
	}
	classes, anyone := map[string]bool{}, map[string]bool{}
	for guid := range r.members {
		classes[specs[k.specs[guid]].Class] = true // known from an earlier key's COMBATANT_INFO, else ""
		for buff := range r.up[guid] {
			anyone[buff] = true
		}
	}
	check := models.ReadyCheck{SinceMS: r.since, Dungeon: r.dungeon, Players: []models.ReadyPlayer{}}
	for guid := range r.members {
		p := models.ReadyPlayer{Player: k.names[guid], Missing: []string{}, NotSeen: []string{}}
		for kind, patterns := range k.consumables(r.raid) {
			had := func(buffs map[string]bool) bool {
				return slices.ContainsFunc(slices.Collect(maps.Keys(buffs)), func(b string) bool { return named(b, patterns) })
			}
			switch {
			case had(r.up[guid]):
			case r.seeded[guid] || had(r.faded[guid]):
				p.Missing = append(p.Missing, kind)
			default:
				p.NotSeen = append(p.NotSeen, kind)
			}
		}
		for buff, class := range k.rules.Buffs {
			switch {
			case r.up[guid][buff]:
			case anyone[buff] || r.faded[guid][buff]:
				p.Missing = append(p.Missing, buff)
			case classes[class]:
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

// consumables are the buffs everyone should have by kind ("flask", "food", and in a raid "Vantus Rune"), each as the
// name patterns that count.
func (k *LogTracker) consumables(raid bool) map[string][]string {
	c := k.rules.Consumables
	kinds := map[string][]string{"flask": c.Flask, "food": c.Food}
	if raid && len(k.raid.rules.Consumables.Vantus) > 0 {
		kinds["Vantus Rune"] = k.raid.rules.Consumables.Vantus
	}
	return kinds
}

// named says whether a buff's name contains one of the patterns ("Flask of", "Well Fed").
func named(buff string, patterns []string) bool {
	return slices.ContainsFunc(patterns, func(p string) bool { return strings.Contains(buff, p) })
}
