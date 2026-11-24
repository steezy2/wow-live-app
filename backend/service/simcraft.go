package service

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"wow-live/backend/models"
)

// craftStatNames are the stats a recraft's stat IDs mean (SimCrafting).
var craftStatNames = map[int]string{32: "Crit", 36: "Haste", 40: "Vers", 49: "Mastery"}

// removable are the things a variant can take off an item (SimVariant.Remove).
var removable = []string{"gems", "enchant", "socket", "embellishment"}

// itemReagents are the reagents of kind a crafted item's recipe takes, by ID.
func itemReagents(ref models.SimReference, item models.SimItem, kind string) []int {
	var ids []int
	for _, id := range ref.Crafting.Items[item.ID].Reagents {
		if ref.Crafting.Reagents[id].Kind == kind {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// heldReagent is the reagent of kind that item is made with: the one whose bonus IDs it all carries. 0 for none.
func heldReagent(ref models.SimReference, item models.SimItem, kind string) int {
	for _, id := range itemReagents(ref, item, kind) {
		if bonus := ref.Crafting.Reagents[id].BonusIDs; len(bonus) > 0 && hasAll(item.Opts["bonus_id"], bonus) {
			return id
		}
	}
	return 0
}

func hasAll(have, want []int) bool {
	return !slices.ContainsFunc(want, func(id int) bool { return !slices.Contains(have, id) })
}

// withoutBonus is item without the bonus IDs of reagent id (0 for none).
func withoutBonus(ref models.SimReference, item models.SimItem, id int) models.SimItem {
	drop := ref.Crafting.Reagents[id].BonusIDs
	item.Opts["bonus_id"] = slices.DeleteFunc(item.Opts["bonus_id"],
		func(b int) bool { return slices.Contains(drop, b) })
	return item
}

// recraftName is a recraft reagent's stats in words ("Crit / Vers").
func recraftName(r models.CraftingReagent) string {
	names := make([]string, len(r.Stats))
	for i, s := range r.Stats {
		names[i] = craftStatNames[s]
	}
	return strings.Join(names, " / ")
}

// swapReagent is item with the reagent id of kind (an embellishment or a recraft) in place of the one it is made
// with, and that reagent; a reagent the item's recipe doesn't take, or one it already has, is ErrBadSimOptions.
// item.Opts is already a copy.
func swapReagent(ref models.SimReference, item models.SimItem, kind string, id int) (models.SimItem,
	models.CraftingReagent, error) {
	held := heldReagent(ref, item, kind)
	if !slices.Contains(itemReagents(ref, item, kind), id) || id == held {
		return item, models.CraftingReagent{}, fmt.Errorf("%w: %s can't take %s %d", ErrBadSimOptions,
			cmp.Or(item.Name, "the item"), kind, id)
	}
	r := ref.Crafting.Reagents[id]
	item = withoutBonus(ref, item, held)
	item.Opts["bonus_id"] = append(item.Opts["bonus_id"], r.BonusIDs...)
	return item, r, nil
}

// changeCrafting is item with the embellishment or recraft stat pair (reagent IDs) a variant asks for, and the note
// saying so.
func changeCrafting(ref models.SimReference, item models.SimItem,
	v models.SimVariant) (models.SimItem, []string, error) {
	var notes []string
	if v.Embellishment != 0 {
		var r models.CraftingReagent
		var err error
		if item, r, err = swapReagent(ref, item, models.ReagentEmbellishment, v.Embellishment); err != nil {
			return item, nil, err
		}
		notes = append(notes, r.Name)
	}
	if v.Recraft != 0 {
		var r models.CraftingReagent
		var err error
		if item, r, err = swapReagent(ref, item, models.ReagentRecraft, v.Recraft); err != nil {
			return item, nil, err
		}
		item.Opts["crafted_stats"] = slices.Clone(r.Stats)
		notes = append(notes, "recraft "+recraftName(r))
	}
	return item, notes, nil
}

// removeFrom is item with what a variant removes taken off (SimVariant.Remove); nothing to remove, or a thing that
// isn't one of removable, is ErrBadSimOptions. item.Opts is already a copy.
func removeFrom(ref models.SimReference, item models.SimItem, what string) (models.SimItem, error) {
	bad := func() error {
		return fmt.Errorf("%w: %s has no %s to remove", ErrBadSimOptions, cmp.Or(item.Name, "the item"), what)
	}
	switch what {
	case "gems", "enchant":
		key := map[string]string{"gems": "gem_id", "enchant": "enchant_id"}[what]
		if len(item.Opts[key]) == 0 {
			return item, bad()
		}
		delete(item.Opts, key)
	case "embellishment":
		held := heldReagent(ref, item, models.ReagentEmbellishment)
		if held == 0 {
			return item, bad()
		}
		item = withoutBonus(ref, item, held)
	case "socket": // only the socket a variant added: an item's own ones stay
		i := slices.Index(item.Opts["bonus_id"], ref.SocketBonus)
		if i < 0 {
			return item, bad()
		}
		left := socketsOf(ref, item) - 1
		item.Opts["bonus_id"] = slices.Delete(item.Opts["bonus_id"], i, i+1)
		if left == 0 {
			delete(item.Opts, "gem_id")
		} else if len(item.Opts["gem_id"]) > left {
			item.Opts["gem_id"] = item.Opts["gem_id"][:left]
		}
	default:
		return item, fmt.Errorf("%w: remove one of %v", ErrBadSimOptions, removable)
	}
	return item, nil
}

// reagentChoices are the reagents of kind a crafted item can be changed to: not the one it has, named by name.
func reagentChoices(ref models.SimReference, item models.SimItem, kind string,
	name func(models.CraftingReagent) string) []models.SimChoice {
	var out []models.SimChoice
	held := heldReagent(ref, item, kind)
	for _, id := range itemReagents(ref, item, kind) {
		if r := ref.Crafting.Reagents[id]; id != held {
			out = append(out, models.SimChoice{ID: id, Name: name(r), Icon: r.Icon})
		}
	}
	return out
}

// maxCatalystCharges is the most Catalyst conversions a Top Gear combination can hold (SimRequest.CatalystCharges).
const maxCatalystCharges = 6

// slotInventoryTypes are the inventory types of the armor slots the Catalyst converts (5 and 20 are both a chest).
var slotInventoryTypes = map[string][]int{"head": {1}, "shoulder": {3}, "back": {16}, "chest": {5, 20}, "wrist": {9},
	"hands": {10}, "waist": {6}, "legs": {7}, "feet": {8}}

// catalystItem is the item the Catalyst makes of item for class (a simc class name): the item of the conversion group
// whose bonus ID the item carries that the class wears in the item's slot. False when it converts to none.
func catalystItem(ref models.SimReference, class string, item models.SimItem) (models.CatalystItem, bool) {
	for _, group := range ref.Crafting.Conversions {
		if !slices.ContainsFunc(group.BonusIDs, func(b int) bool { return slices.Contains(item.Opts["bonus_id"], b) }) {
			continue
		}
		for _, to := range group.Items {
			if to.ID == item.ID {
				return models.CatalystItem{}, false // already a Catalyst item
			}
		}
		for _, to := range group.Items {
			if to.Class == classIDs[class] && slices.Contains(slotInventoryTypes[item.Slot], to.InventoryType) {
				return to, true
			}
		}
	}
	return models.CatalystItem{}, false
}

// converted is item as the Catalyst makes it, with the same bonus IDs (so the same upgrade); ErrBadSimOptions when
// it converts to nothing.
func converted(ref models.SimReference, class string, item models.SimItem) (models.SimItem, error) {
	to, ok := catalystItem(ref, class, item)
	if !ok {
		return item, fmt.Errorf("%w: %s doesn't convert at the Catalyst", ErrBadSimOptions, cmp.Or(item.Name, "the item"))
	}
	item.ID, item.Name, item.Catalyst = to.ID, to.Name, true
	return item, nil
}
