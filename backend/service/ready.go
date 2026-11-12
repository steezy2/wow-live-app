package service

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"wow-live/backend/models"
)

// consumable is what counts as one kind of consumable buff: an aura whose name contains one of the patterns ("Flask
// of", "Well Fed"), or one of the IDs, for an aura the log never names (only COMBATANT_INFO's list shows it).
type consumable struct {
	patterns []string
	ids      []int
}

// consumables are the buffs everyone should have by kind ("flask", "food", and in a raid of a difficulty its rune
// works in, "Vantus Rune"); difficulty is empty in a dungeon. Callers only read it: it can be the tracker's own map.
func (k *LogTracker) consumables(difficulty string) map[string]consumable {
	kinds := k.everyday
	if difficulty == "" {
		return kinds
	}
	if v := k.raid.rules.Consumables; len(v.Vantus)+len(v.VantusIDs) > 0 &&
		slices.Contains(v.VantusDifficulties, difficulty) {
		kinds = maps.Clone(kinds)
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
