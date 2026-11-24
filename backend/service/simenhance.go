package service

import (
	"fmt"
	"slices"

	"wow-live/backend/models"
)

// topGearExtras are the candidates beyond the picked items: the request's variants and the enchants and gems its
// Enhancements try on the picked items.
func topGearExtras(ref models.SimReference, p models.SimProfile, req models.SimRequest) ([]gearCandidate, error) {
	if req.CatalystCharges < 0 || req.CatalystCharges > maxCatalystCharges {
		return nil, fmt.Errorf("%w: Catalyst charges are 0 to %d", ErrBadSimOptions, maxCatalystCharges)
	}
	converts := func(v models.SimVariant) bool { return v.Catalyst }
	if req.CatalystCharges == 0 && slices.ContainsFunc(req.Variants, converts) {
		return nil, fmt.Errorf("%w: a Catalyst conversion needs at least one Catalyst charge", ErrBadSimOptions)
	}
	extra, err := variantCandidates(ref, p, req.Variants)
	if err != nil || req.Enhancements == nil {
		return extra, err
	}
	more, err := enhancementCandidates(ref, p, req)
	return append(extra, more...), err
}

// enhancementCandidates are changed copies of the picked items, each in its slot: with each enchant picked for the
// slot (Enhancements.Enchants by gearKey, a ring's slot is "finger") that it can take, and with each gem picked in its
// sockets (a unique gem, one per limit category, in the first socket only; any other in every socket). An item
// already as changed makes no copy. An enchant or gem the reference doesn't know is ErrBadSimOptions.
func enhancementCandidates(ref models.SimReference, p models.SimProfile,
	req models.SimRequest) ([]gearCandidate, error) {
	var out []gearCandidate
	for _, list := range []struct {
		items  []models.SimItem
		picked []int
	}{{p.Equipped, req.Equipped}, {p.Bags, req.Bags}} {
		for _, i := range slices.Compact(slices.Sorted(slices.Values(list.picked))) {
			if i < 0 || i >= len(list.items) {
				continue // gearPool refuses the index
			}
			item := list.items[i]
			for _, id := range req.Enhancements.Enchants[gearKey(item.Slot)] {
				if slices.Contains(item.Opts["enchant_id"], id) {
					continue
				}
				changed, err := applyVariant(ref, item, models.SimVariant{Enchant: id})
				if err != nil {
					return nil, err
				}
				out = append(out, gearCandidate{item: changed})
			}
			for _, id := range req.Enhancements.Gems {
				changed, ok, err := withGem(ref, item, id)
				if err != nil {
					return nil, err
				}
				if ok {
					out = append(out, gearCandidate{item: changed})
				}
			}
		}
	}
	return out, nil
}

// withGem is item with gem id in its sockets, false when it has none or already holds it there.
func withGem(ref models.SimReference, item models.SimItem, id int) (models.SimItem, bool, error) {
	i := slices.IndexFunc(ref.Gems, func(g models.SimGem) bool { return g.ID == id })
	if i < 0 {
		return item, false, fmt.Errorf("%w: no gem %d", ErrBadSimOptions, id)
	}
	sockets := socketsOf(ref, item)
	if sockets == 0 {
		return item, false, nil
	}
	gems := append(slices.Clone(item.Opts["gem_id"]), make([]int, max(0, sockets-len(item.Opts["gem_id"])))...)
	for s := range gems {
		if s == 0 || ref.Gems[i].Unique == 0 {
			gems[s] = id
		}
	}
	if slices.Equal(gems, item.Opts["gem_id"]) {
		return item, false, nil
	}
	item.Opts = cloneOpts(item.Opts)
	item.Opts["gem_id"] = gems
	item.Note = ref.Gems[i].Name
	return item, true, nil
}

// uniqueGems are the gems limited to one equipped, by gem ID, with their limit category.
func uniqueGems(ref models.SimReference) map[int]int {
	limits := map[int]int{}
	for _, g := range ref.Gems {
		if g.Unique != 0 {
			limits[g.ID] = g.Unique
		}
	}
	return limits
}

// wornAfter are the items worn once swaps (on p's equipped items) are made.
func wornAfter(p models.SimProfile, swaps []models.SimSwap) []*models.SimItem {
	worn := map[string]*models.SimItem{}
	for i := range p.Equipped {
		worn[p.Equipped[i].Slot] = &p.Equipped[i]
	}
	for _, s := range swaps {
		worn[s.Slot] = s.In
	}
	var out []*models.SimItem
	for _, item := range worn {
		if item != nil {
			out = append(out, item)
		}
	}
	return out
}

// maxEmbellished is how many embellished items a character may wear.
const maxEmbellished = 2

// embellishedOK is whether the gear that results from swaps wears at most maxEmbellished embellished items.
func embellishedOK(ref models.SimReference, p models.SimProfile, swaps []models.SimSwap) bool {
	n := 0
	for _, item := range wornAfter(p, swaps) {
		if heldReagent(ref, *item, models.ReagentEmbellishment) != 0 {
			n++
		}
	}
	return n <= maxEmbellished
}

// uniqueGemsOK is whether the gear that results from swaps (on p's equipped items) carries at most one gem of each
// limit category in unique (uniqueGems).
func uniqueGemsOK(unique map[int]int, p models.SimProfile, swaps []models.SimSwap) bool {
	if len(unique) == 0 {
		return true
	}
	seen := map[int]bool{}
	for _, item := range wornAfter(p, swaps) {
		for _, id := range item.Opts["gem_id"] {
			if category := unique[id]; category != 0 {
				if seen[category] {
					return false
				}
				seen[category] = true
			}
		}
	}
	return true
}
