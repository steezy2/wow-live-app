package service

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"wow-live/backend/models"
)

const (
	// maxCompareValues is the most values a "consumables" sim tries, so one stays about as long as a Top Gear sim.
	maxCompareValues = 40
	// maxPlanCombos is where a plan stops counting: a request that makes more than maxCombos is refused anyway.
	maxPlanCombos = 1_000_000
	// comboSeconds is how long one Top Gear combination took on a 16-thread PC at the default fight (100 took 73 s),
	// and referenceThreads the threads that PC ran it on.
	comboSeconds     = 0.73
	referenceThreads = 15
)

// simOptionLines are the start of the simc line each kind of consumable sets (models.SimConsumableKinds); a weapon oil
// goes on the main hand.
var simOptionLines = map[string]string{"flask": "flask=", "food": "food=", "potion": "potion=",
	"augment": "augmentation=", "weapon": "temporary_enchant=main_hand:"}

// consumableOf is the consumable of kind with simc name value, false when the reference has none.
func consumableOf(ref models.SimReference, kind, value string) (models.SimConsumable, bool) {
	i := slices.IndexFunc(ref.Consumables[kind], func(c models.SimConsumable) bool { return c.Value == value })
	if i < 0 {
		return models.SimConsumable{}, false
	}
	return ref.Consumables[kind][i], true
}

// consumableLine is the simc line that sets a consumable ("flask=flask_of_..."). Only a name from the reference gets
// here, so nothing else is written into the input.
func consumableLine(kind string, c models.SimConsumable) string {
	return simOptionLines[kind] + c.Value + "\n"
}

// consumableLines are the lines for the consumables picked (a kind left empty keeps simc's default), or
// ErrBadSimOptions for a kind or value the reference doesn't have.
func consumableLines(ref models.SimReference, picked models.SimConsumables) (string, error) {
	for kind := range picked {
		if !slices.Contains(models.SimConsumableKinds, kind) {
			return "", fmt.Errorf("%w: no consumable kind %q (%v)", ErrBadSimOptions, kind, models.SimConsumableKinds)
		}
	}
	var b strings.Builder
	for _, kind := range models.SimConsumableKinds {
		value := picked[kind]
		if value == "" {
			continue
		}
		c, ok := consumableOf(ref, kind, value)
		if !ok {
			return "", fmt.Errorf("%w: no %s called %q", ErrBadSimOptions, kind, value)
		}
		b.WriteString(consumableLine(kind, c))
	}
	return b.String(), nil
}

// compareCombos are the profilesets of a consumables comparison: the default (or picked) consumables with one of the
// compared kind changed, in the order given. Each is one value, at most maxCompareValues, none twice.
func compareCombos(ref models.SimReference, cmpr *models.SimCompare) ([]gearCombo, error) {
	if cmpr == nil || !slices.Contains(models.SimConsumableKinds, cmpr.Kind) {
		return nil, fmt.Errorf("%w: pick what to compare (%v)", ErrBadSimOptions, models.SimConsumableKinds)
	}
	n := len(cmpr.Values)
	if n == 0 || n > maxCompareValues || len(slices.Compact(slices.Sorted(slices.Values(cmpr.Values)))) != n {
		return nil, fmt.Errorf("%w: compare 1 to %d different %s values", ErrBadSimOptions, maxCompareValues, cmpr.Kind)
	}
	combos := make([]gearCombo, n)
	for i, v := range cmpr.Values {
		c, ok := consumableOf(ref, cmpr.Kind, v)
		if !ok {
			return nil, fmt.Errorf("%w: no %s called %q", ErrBadSimOptions, cmpr.Kind, v)
		}
		combos[i] = gearCombo{name: "option" + strconv.Itoa(i+1), line: consumableLine(cmpr.Kind, c),
			option: &models.SimOptionRow{Value: c.Value, Name: c.Name}}
	}
	return combos, nil
}

// withCompare is a sim's combinations with the consumables compared: a Quick Sim's are the values alone, any other
// sim's each gear combination run once with every value (ErrBadSimOptions when that is more than limit profilesets).
func withCompare(ref models.SimReference, cmpr *models.SimCompare, quick bool, gear []gearCombo, limit int,
) ([]gearCombo, error) {
	values, err := compareCombos(ref, cmpr)
	if err != nil || quick {
		return values, err
	}
	if len(gear)*len(values) > limit {
		return nil, fmt.Errorf("%w: %d combinations with %d consumables are %d sims, and this one runs at most %d",
			ErrBadSimOptions, len(gear), len(values), len(gear)*len(values), limit)
	}
	out := make([]gearCombo, 0, len(gear)*len(values))
	for _, g := range gear {
		for _, v := range values {
			c := g
			c.name, c.line, c.consumable = g.name+"x"+v.name, v.line, v.option.Name
			out = append(out, c)
		}
	}
	return out, nil
}

// optionResults are the compared values' DPS, best first, with their gain over the baseline's.
func optionResults(sets []simSetJSON, baseline float64, combos []gearCombo) []models.SimOptionRow {
	rows := []models.SimOptionRow{}
	for _, set := range sets {
		if i := slices.IndexFunc(combos, func(c gearCombo) bool { return c.name == set.Name }); i >= 0 {
			row := *combos[i].option
			row.DPS, row.DPSError, row.Gain = set.Mean, set.MeanStdDev, set.Mean-baseline
			rows = append(rows, row)
		}
	}
	slices.SortStableFunc(rows, func(a, b models.SimOptionRow) int { return cmp.Compare(b.DPS, a.DPS) })
	return rows
}

// omniumInput is the Omnium Folio's picks as the export writes them ("136822:1/136817:1"): the request's when it sends
// them (empty for none), else the export's. A pick must be an entry of the reference's tree, at most one to a node.
func omniumInput(ref models.SimReference, p models.SimProfile, requested *string) (string, error) {
	if requested == nil {
		return p.Omnium, nil
	}
	if *requested == "" {
		return "", nil
	}
	picks := map[int]int{} // node -> entry
	for pair := range strings.SplitSeq(*requested, "/") {
		id, rank, ok := strings.Cut(pair, ":")
		entry, err := strconv.Atoi(id)
		if r, rankErr := strconv.Atoi(rank); err != nil || rankErr != nil || !ok || r != 1 {
			return "", fmt.Errorf("%w: the Omnium Folio's picks are entry:1 pairs", ErrBadSimOptions)
		}
		i := slices.IndexFunc(ref.Omnium, func(n models.TalentNode) bool {
			return slices.ContainsFunc(n.Entries, func(e models.TalentEntry) bool { return e.ID == entry })
		})
		if i < 0 || picks[ref.Omnium[i].ID] != 0 {
			return "", fmt.Errorf("%w: entry %d is not a rune of the Omnium Folio, or its node has another pick",
				ErrBadSimOptions, entry)
		}
		picks[ref.Omnium[i].ID] = entry
	}
	out := make([]string, 0, len(picks))
	for _, n := range ref.Omnium { // in the tree's order, so one choice reads the same each time
		if entry, ok := picks[n.ID]; ok {
			out = append(out, strconv.Itoa(entry)+":1")
		}
	}
	return strings.Join(out, "/"), nil
}

// upgradeOf is the track an item is on and its level in it, found by the item's bonus IDs.
func upgradeOf(ref models.SimReference, item models.SimItem) (models.SimUpgradeTrack, models.SimUpgradeLevel, bool) {
	for _, track := range ref.Upgrades {
		for _, level := range track.Levels {
			if slices.Contains(item.Opts["bonus_id"], level.BonusID) {
				return track, level, true
			}
		}
	}
	return models.SimUpgradeTrack{}, models.SimUpgradeLevel{}, false
}

// applyVariant is item changed as v says: moved to an upgrade level of its track, given a socket, a gem and an enchant.
// What changed is written in its Note, and its item level follows the upgrade. Anything the reference doesn't know, or
// that doesn't fit the item, is ErrBadSimOptions.
func applyVariant(ref models.SimReference, item models.SimItem, v models.SimVariant) (models.SimItem, error) {
	bad := func(format string, args ...any) (models.SimItem, error) {
		return models.SimItem{}, fmt.Errorf("%w: "+format, append([]any{ErrBadSimOptions}, args...)...)
	}
	item.Opts = cloneOpts(item.Opts)
	var notes []string
	if v.Upgrade != 0 {
		track, current, ok := upgradeOf(ref, item)
		i := slices.IndexFunc(track.Levels, func(l models.SimUpgradeLevel) bool { return l.Level == v.Upgrade })
		if !ok || i < 0 {
			return bad("%s has no upgrade level %d to move to", cmp.Or(item.Name, "the item"), v.Upgrade)
		}
		item.Opts["bonus_id"] = slices.DeleteFunc(item.Opts["bonus_id"], func(id int) bool { return id == current.BonusID })
		item.Opts["bonus_id"] = append(item.Opts["bonus_id"], track.Levels[i].BonusID)
		item.Level = track.Levels[i].ItemLevel
		notes = append(notes, fmt.Sprintf("%s %d/%d", track.Name, v.Upgrade, len(track.Levels)))
	}
	if v.Socket {
		item.Opts["bonus_id"] = append(item.Opts["bonus_id"], ref.SocketBonus)
		notes = append(notes, "socket")
	}
	if v.Gem != 0 {
		i := slices.IndexFunc(ref.Gems, func(g models.SimGem) bool { return g.ID == v.Gem })
		if i < 0 {
			return bad("no gem %d", v.Gem)
		}
		if v.Socket { // the added socket comes last
			item.Opts["gem_id"] = append(item.Opts["gem_id"], v.Gem)
		} else {
			item.Opts["gem_id"] = append([]int{v.Gem}, item.Opts["gem_id"][min(1, len(item.Opts["gem_id"])):]...)
		}
		notes = append(notes, ref.Gems[i].Name)
	}
	if v.Enchant != 0 {
		i := slices.IndexFunc(ref.Enchants, func(e models.SimEnchant) bool { return e.ID == v.Enchant })
		if i < 0 || !slices.Contains(ref.Enchants[i].Slots, item.Slot) {
			return bad("no enchant %d for %s", v.Enchant, item.Slot)
		}
		item.Opts["enchant_id"] = []int{v.Enchant}
		notes = append(notes, ref.Enchants[i].Name)
	}
	if len(notes) == 0 {
		return bad("a variant of an item changes something")
	}
	item.Note = strings.Join(notes, ", ")
	return item, nil
}

// cloneOpts is a copy of an item's numbers that can be changed without touching the original.
func cloneOpts(opts map[string][]int) map[string][]int {
	out := make(map[string][]int, len(opts)+1)
	for k, v := range opts {
		out[k] = slices.Clone(v)
	}
	return out
}

// variantCandidates are the variants as Top Gear candidates, each a changed copy of a picked item that stays in its
// slot; indexes outside p are ErrBadSimOptions.
func variantCandidates(ref models.SimReference, p models.SimProfile,
	variants []models.SimVariant) ([]gearCandidate, error) {
	out := make([]gearCandidate, 0, len(variants))
	for _, v := range variants {
		list := p.Bags
		if v.Worn {
			list = p.Equipped
		}
		if v.Index < 0 || v.Index >= len(list) {
			return nil, fmt.Errorf("%w: no item %d to change", ErrBadSimOptions, v.Index)
		}
		item, err := applyVariant(ref, list[v.Index], v)
		if err != nil {
			return nil, err
		}
		out = append(out, gearCandidate{item: item})
	}
	return out, nil
}

// planOf is what a Top Gear request would run: the combinations it makes (counted without making them, so a request
// over the limit says by how much), why it is refused if it is, and about how long they take on this PC with o. A
// sim's time grows with the fight's length and the iterations, which go as 1/target error².
func planOf(combos int, o models.SimOptions) models.SimPlan {
	errorRatio := simTargetError(simDefaults, true) / simTargetError(o, true)
	seconds := float64(combos) * comboSeconds * referenceThreads / float64(simThreads()) *
		float64(o.Length) / float64(simDefaults.Length) * errorRatio * errorRatio
	plan := models.SimPlan{Combos: combos, Seconds: int(seconds + 0.5)}
	if err := comboRefusal(combos); err != nil {
		plan.Error = err.Error()
	}
	return plan
}
