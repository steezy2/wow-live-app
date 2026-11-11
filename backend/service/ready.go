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
	members  map[string]bool           // party GUIDs seen
	up       map[string]map[int]string // GUID -> buffs up (aura ID -> name, "" for one the log never named)
	faded    map[string]map[int]string // GUID -> buffs seen fade
	seeded   map[string]bool           // GUIDs whose buffs a raid pull's start listed: what they lack is missing
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
			seeded: map[string]bool{}, up: map[string]map[int]string{}, faded: map[string]map[int]string{}}
	}
}

// seedReady sets the raid's ready check to what a raid pull's start listed: each raider's auras, but the party buffs
// of those who died in the pull, which a death takes off (flask, food and Vantus Rune stay).
func (k *LogTracker) seedReady(auras map[string][]int, died map[string]bool) {
	r := k.ready
	if r == nil || !r.raid {
		return
	}
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
		for _, buff := range r.up[guid] {
			anyone[buff] = true
		}
	}
	check := models.ReadyCheck{SinceMS: r.since, Dungeon: r.dungeon, Players: []models.ReadyPlayer{}}
	for guid := range r.members {
		p := models.ReadyPlayer{Player: k.names[guid], Missing: []string{}, NotSeen: []string{}}
		up, faded := slices.Collect(maps.Values(r.up[guid])), slices.Collect(maps.Values(r.faded[guid]))
		for kind, c := range k.consumables(r.raid) {
			switch {
			case c.had(r.up[guid]):
			case r.seeded[guid] || c.had(r.faded[guid]):
				p.Missing = append(p.Missing, kind)
			default:
				p.NotSeen = append(p.NotSeen, kind)
			}
		}
		for buff, class := range k.rules.Buffs {
			switch {
			case slices.Contains(up, buff):
			case anyone[buff] || slices.Contains(faded, buff):
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

// consumable is what counts as one kind of consumable buff: an aura whose name contains one of the patterns ("Flask
// of", "Well Fed"), or one of the IDs, for an aura the log never names (only COMBATANT_INFO's list shows it).
type consumable struct {
	patterns []string
	ids      []int
}

// consumables are the buffs everyone should have by kind ("flask", "food", and in a raid "Vantus Rune").
func (k *LogTracker) consumables(raid bool) map[string]consumable {
	c := k.rules.Consumables
	kinds := map[string]consumable{"flask": {patterns: c.Flask}, "food": {patterns: c.Food}}
	if !raid {
		return kinds
	}
	if v := k.raid.rules.Consumables; len(v.Vantus)+len(v.VantusIDs) > 0 {
		kinds["Vantus Rune"] = consumable{patterns: v.Vantus, ids: v.VantusIDs}
	}
	return kinds
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
