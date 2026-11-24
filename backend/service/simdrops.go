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

// Ascendant Venomstones (10 of them, used at the Catalyst) raise a fully upgraded Hero or Myth weapon, trinket or neck
// by venomstoneLevels item levels. Wowhead's item 280562 (patch 12.1.5) names the tracks and slots but not the amount;
// +6 is a commenter's, NOT YET CHECKED in the game (the first note said +10).
const venomstoneLevels = 6

var (
	venomstoneTracks = []string{"Hero", "Myth"}
	venomstoneSlots  = []string{"one_hand", "two_hand", "main_hand", "off_hand", "trinket", "neck"}
)

// maxedLevel is the item level a drop of level reaches at the top rank of its upgrade track, and the track's name: the
// track that has level at its lowest rank (the higher track of two), or level itself and no track when none has it
// (an item past its track's top).
func maxedLevel(tracks []models.SimUpgradeTrack, level int) (int, string) {
	maxed, name, rank, group := level, "", 0, 0
	for _, t := range tracks {
		for _, l := range t.Levels {
			if l.ItemLevel == level && (rank == 0 || l.Level < rank || l.Level == rank && t.Group > group) {
				maxed, name, rank, group = t.Levels[len(t.Levels)-1].ItemLevel, t.Name, l.Level, t.Group
			}
		}
	}
	return maxed, name
}

// dropCombos is one combination per piece of gear the season's raid bosses (at difficulty) and dungeons (at key level
// keyLevel) drop that the character's spec can loot, each replacing what is worn in its slot, at the top rank of its
// upgrade track in upgrades (none keeps the level it drops at). A ring or trinket is tried in both slots, except the
// one beside a copy of it (unique-equipped); a two-hander empties the off-hand. A one-hander only ever goes in the
// main hand, so a dual-wielder's off-hand isn't covered, and beside a worn two-hander it is simmed with an empty
// off-hand. Nothing to sim, or over maxDrops, is ErrBadSimOptions.
func dropCombos(p models.SimProfile, d models.SimDrops, difficulty string, keyLevel int,
	upgrades []models.SimUpgradeTrack) ([]gearCombo, error) {
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
	add := func(source models.DropSource, boss models.DropBoss, dropped int) {
		maxed, track := maxedLevel(upgrades, dropped)
		for _, item := range boss.Items {
			if len(item.Specs) > 0 && !slices.Contains(item.Specs, spec) {
				continue
			}
			level := maxed
			if slices.Contains(venomstoneTracks, track) && slices.Contains(venomstoneSlots, item.Slot) {
				level += venomstoneLevels
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
				opts := map[string][]int{"ilevel": {level}}
				if w := worn[slot]; w != nil && w.Opts["enchant_id"] != nil {
					opts["enchant_id"] = w.Opts["enchant_id"] // the enchant goes on the new piece, as in game
				}
				drop := models.SimItem{Group: gearKey(slot), Name: item.Name, Level: level, ID: item.ID, Opts: opts}
				fills := []gearFill{{slot, &gearCandidate{item: drop}}}
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
