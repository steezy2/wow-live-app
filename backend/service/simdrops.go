package service

import (
	"fmt"
	"slices"
	"strconv"

	"wow-live/backend/models"
)

// maxDrops is the most drops a Droptimizer sims: 300 profilesets took 150 s on a 16-thread PC.
const maxDrops = 300

// simDifficulties are the raid difficulties a Droptimizer can sim the raid at.
var simDifficulties = []string{"lfr", "normal", "heroic", "mythic"}

// dropSlots are the slots a drop fills when its own slot's name isn't one: a ring or trinket either of its two, a
// weapon (one-handed or two-handed) the main hand.
var dropSlots = map[string][]string{"finger": {"finger1", "finger2"}, "trinket": {"trinket1", "trinket2"},
	"one_hand": {"main_hand"}, "two_hand": {"main_hand"}}

// raidLevel is the item level a raid boss drops on difficulty.
func raidLevel(l models.DifficultyLevels, difficulty string) int {
	return []int{l.LFR, l.Normal, l.Heroic, l.Mythic}[slices.Index(simDifficulties, difficulty)]
}

// dropCombos is one combination per piece of gear the season's raid bosses (at difficulty) and dungeons (at key level
// keyLevel) drop that the character's spec can loot, each replacing what is worn in its slot. A ring or trinket is
// tried in both slots, except the one beside a copy of it (unique-equipped); a two-hander empties the off-hand. A
// one-hander only ever goes in the main hand, so a dual-wielder's off-hand isn't covered, and beside a worn two-hander
// it is simmed with an empty off-hand. Nothing to sim, or over maxDrops, is ErrBadSimOptions.
func dropCombos(p models.SimProfile, d models.SimDrops, difficulty string, keyLevel int) ([]gearCombo, error) {
	spec := 0
	for id, s := range d.Specs {
		if s.Class == p.Class && s.Spec == p.Spec {
			spec = id
		}
	}
	key := slices.IndexFunc(d.Keys, func(k models.KeyLevels) bool { return k.Level == keyLevel })
	if spec == 0 || key < 0 || !slices.Contains(simDifficulties, difficulty) {
		return nil, fmt.Errorf("%w: no drops known for this spec, difficulty (%v) or key level", ErrBadSimOptions,
			simDifficulties)
	}
	worn := map[string]*models.SimItem{}
	for i := range p.Equipped {
		worn[p.Equipped[i].Slot] = &p.Equipped[i]
	}

	var combos []gearCombo
	add := func(source models.DropSource, boss models.DropBoss, level int) {
		for _, item := range boss.Items {
			if len(item.Specs) > 0 && !slices.Contains(item.Specs, spec) {
				continue
			}
			slots, ok := dropSlots[item.Slot]
			if !ok {
				slots = []string{item.Slot}
			}
			for i, slot := range slots {
				pair := worn[slots[len(slots)-1-i]]
				if slot == "off_hand" && wearsTwoHander(worn) || len(slots) == 2 && pair != nil && pair.ID == item.ID {
					continue
				}
				fills := []gearFill{{slot, &gearCandidate{item: models.SimItem{Name: item.Name, Level: level, ID: item.ID,
					Opts: map[string][]int{"ilevel": {level}}}}}}
				if item.Slot == "two_hand" {
					fills = append(fills, gearFill{"off_hand", nil})
				}
				combos = append(combos, gearCombo{name: "drop" + strconv.Itoa(len(combos)+1),
					swaps: gearSwaps(worn, fills...), source: source.Name, boss: boss.Name})
			}
		}
	}
	for _, raid := range d.Raids {
		for _, boss := range raid.Bosses {
			if boss.Levels != nil {
				add(raid, boss, raidLevel(*boss.Levels, difficulty))
			}
		}
	}
	for _, dungeon := range d.Dungeons {
		for _, boss := range dungeon.Bosses {
			add(dungeon, boss, d.Keys[key].Loot)
		}
	}
	if len(combos) == 0 || len(combos) > maxDrops {
		return nil, fmt.Errorf("%w: %d drops fit this spec, and a Droptimizer sims 1 to %d", ErrBadSimOptions, len(combos),
			maxDrops)
	}
	return combos, nil
}
