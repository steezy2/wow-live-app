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
// by venomstoneLevels item levels (Myth 334 to 341, Hero 321 to 328: the owner's check in the game and Raidbots' "328
// Venomforged"). Wowhead's item 280562 (patch 12.1.5) names the tracks and slots but not the amount. A crafted item
// gets +6, to 337, but none drops.
const venomstoneLevels = 7

var (
	venomstoneTracks = []string{"Hero", "Myth"}
	venomstoneSlots  = []string{"one_hand", "two_hand", "main_hand", "off_hand", "trinket", "neck"}
)

// trackOf is the upgrade track a drop of item level level starts on: the track that has level at its lowest rank (the
// higher track of two), or nil when none has it (an item past its track's top).
func trackOf(tracks []models.SimUpgradeTrack, level int) *models.SimUpgradeTrack {
	var found *models.SimUpgradeTrack
	rank := 0
	for i, t := range tracks {
		for _, l := range t.Levels {
			if l.ItemLevel == level && (found == nil || l.Level < rank || l.Level == rank && t.Group > found.Group) {
				found, rank = &tracks[i], l.Level
			}
		}
	}
	return found
}

// upgradeRanks is how far a Droptimizer upgrades a drop: the ranks past it of an eight-rank track are Mythic raid only.
const upgradeRanks = 6

// rankLevel is the item level at rank (from 1) of t: its top rank (upgradeRanks at most) when rank is 0 or past it.
func rankLevel(t models.SimUpgradeTrack, rank int) int {
	top := min(len(t.Levels), upgradeRanks)
	if rank <= 0 || rank > top {
		rank = top
	}
	return t.Levels[rank-1].ItemLevel
}

// dropUpgrade is how a Droptimizer upgrades its drops along their tracks: Rank is the rank each is moved to (0 the top
// of its track, with Venomstones), and Match moves each to the rank of the item worn in its slot. No Tracks keeps the
// level a drop has.
type dropUpgrade struct {
	Tracks []models.SimUpgradeTrack
	Rank   int
	Match  bool
}

// dropCombos is one combination per piece of gear the season's raid bosses (at difficulty) and dungeons (at key level
// keyLevel) drop that the character's spec can loot, each replacing what is worn in its slot, at the top rank of its
// upgrade track in upgrades (none keeps the level it drops at). A ring or trinket is tried in both slots, except the
// one beside a copy of it (unique-equipped); a two-hander empties the off-hand. A one-hander only ever goes in the
// main hand, so a dual-wielder's off-hand isn't covered, and beside a worn two-hander it is simmed with an empty
// off-hand. Nothing to sim, or over maxDrops, is ErrBadSimOptions.
func dropCombos(p models.SimProfile, d models.SimDrops, difficulty string, keyLevel int,
	up dropUpgrade) ([]gearCombo, error) {
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
		track := trackOf(up.Tracks, dropped)
		for _, item := range boss.Items {
			if len(item.Specs) > 0 && !slices.Contains(item.Specs, spec) {
				continue
			}
			slots, ok := dropSlots[item.Slot]
			if !ok {
				slots = []string{item.Slot}
			}
			for i, slot := range slots {
				pair, own, level := worn[slots[len(slots)-1-i]], worn[slot], dropped
				if track != nil {
					level = upgradedLevel(up, *track, item.Slot, own, dropped)
				}
				if slot == "off_hand" && wearsTwoHander(worn) || len(slots) == 2 && pair != nil && pair.ID == item.ID ||
					own != nil && own.ID == item.ID && level <= own.Level {
					continue
				}
				opts := map[string][]int{"ilevel": {level}}
				if own != nil && own.Opts["enchant_id"] != nil {
					opts["enchant_id"] = own.Opts["enchant_id"] // the enchant goes on the new piece, as in game
				}
				drop := models.SimItem{Group: models.SlotGroup(slot), Name: item.Name, Level: level, ID: item.ID, Opts: opts}
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

// upgradedLevel is the item level of a drop (of level dropped, for the slot named slot) that starts on track, when it
// replaces worn: at the rank up asks for. Matching a worn item that is on no track keeps the dropped level, and a fully
// upgraded Hero or Myth weapon, trinket or neck also takes its Venomstones.
func upgradedLevel(up dropUpgrade, track models.SimUpgradeTrack, slot string, worn *models.SimItem, dropped int) int {
	rank := up.Rank
	if up.Match {
		if worn == nil {
			return dropped
		}
		_, current, ok := upgradeOf(up.Tracks, *worn)
		if !ok {
			return dropped
		}
		rank = current.Level
	}
	level := rankLevel(track, rank)
	if rank == 0 && slices.Contains(venomstoneTracks, track.Name) && slices.Contains(venomstoneSlots, slot) {
		level += venomstoneLevels
	}
	return level
}
