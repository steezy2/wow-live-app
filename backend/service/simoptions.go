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

// kindPicks are the consumables picked of one kind.
type kindPicks struct {
	kind  string
	picks []models.SimConsumable
}

// consumablesPicked are the consumables picked, by kind in the app's order (a kind with none is left out), or
// ErrBadSimOptions for a kind, or a value or a repeat of one, the reference doesn't have.
func consumablesPicked(ref models.SimReference, picked models.SimConsumables) ([]kindPicks, error) {
	for kind := range picked {
		if !slices.Contains(models.SimConsumableKinds, kind) {
			return nil, fmt.Errorf("%w: no consumable kind %q (%v)", ErrBadSimOptions, kind, models.SimConsumableKinds)
		}
	}
	var out []kindPicks
	for _, kind := range models.SimConsumableKinds {
		var picks []models.SimConsumable
		for _, value := range picked[kind] {
			c, ok := consumableOf(ref, kind, value)
			if !ok || slices.ContainsFunc(picks, func(p models.SimConsumable) bool { return p.Value == value }) {
				return nil, fmt.Errorf("%w: no %s called %q, or it is picked twice", ErrBadSimOptions, kind, value)
			}
			picks = append(picks, c)
		}
		if len(picks) > 0 {
			out = append(out, kindPicks{kind, picks})
		}
	}
	return out, nil
}

// consumableLines are the lines for the consumables a sim uses as they are: the kinds with one value picked (a kind
// with none keeps simc's default, with several is compared: see compareCombos), or ErrBadSimOptions for a bad pick.
func consumableLines(ref models.SimReference, picked models.SimConsumables) (string, error) {
	all, err := consumablesPicked(ref, picked)
	var b strings.Builder
	for _, k := range all {
		if len(k.picks) == 1 {
			b.WriteString(consumableLine(k.kind, k.picks[0]))
		}
	}
	return b.String(), err
}

// compareOption is one value of a thing a sim compares: the simc line that sets it, the value it stands for and its
// name.
type compareOption struct{ line, value, name string }

// compareDim is one thing a sim compares, a consumable kind or the Omnium Folio's runes: its values, two or more.
type compareDim []compareOption

// consumableDims are the kinds of consumable with two or more values picked, each a thing to compare.
func consumableDims(ref models.SimReference, picked models.SimConsumables) ([]compareDim, error) {
	all, err := consumablesPicked(ref, picked)
	var dims []compareDim
	for _, k := range all {
		if len(k.picks) > 1 {
			dim := make(compareDim, len(k.picks))
			for i, c := range k.picks {
				dim[i] = compareOption{consumableLine(k.kind, c), c.Value, c.Name}
			}
			dims = append(dims, dim)
		}
	}
	return dims, err
}

// compareCombos are the profilesets of a comparison: every combination of the values of dims (the last changes
// fastest), at most maxCompareValues of them; none when nothing is compared.
func compareCombos(dims []compareDim) ([]gearCombo, error) {
	n := 1
	for _, d := range dims {
		n *= len(d)
	}
	if len(dims) == 0 {
		return nil, nil
	}
	if n > maxCompareValues {
		return nil, fmt.Errorf("%w: what is picked makes %d combinations, and a sim compares at most %d",
			ErrBadSimOptions, n, maxCompareValues)
	}
	combos := make([]gearCombo, n)
	for i := range combos {
		var line string
		var values, names []string
		for j, rest := len(dims)-1, i; j >= 0; j-- {
			o := dims[j][rest%len(dims[j])]
			rest /= len(dims[j])
			line = o.line + line
			values, names = append([]string{o.value}, values...), append([]string{o.name}, names...)
		}
		combos[i] = gearCombo{name: "option" + strconv.Itoa(i+1), line: line,
			option: &models.SimOptionRow{Value: strings.Join(values, "+"), Name: strings.Join(names, " + ")}}
	}
	return combos, nil
}

// withCompare is a sim's combinations with dims compared: a Quick Sim's are the combinations of them alone, any other
// sim's each gear combination run once for every one (ErrBadSimOptions when that is more than limit profilesets).
// With nothing to compare it returns gear as it is.
func withCompare(dims []compareDim, quick bool, gear []gearCombo, limit int) ([]gearCombo, error) {
	values, err := compareCombos(dims)
	if err != nil || len(values) == 0 {
		return gear, err
	}
	if quick {
		return values, nil
	}
	if len(gear)*len(values) > limit {
		return nil, fmt.Errorf("%w: %d combinations with %d compared are %d sims, and this one runs at most %d",
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
// them (empty for none), else the export's. A pick must be an entry of the reference's tree. A node with several
// picks is compared: the picks used by the sim itself take its first, and the dimension has a value for every
// combination.
func omniumInput(ref models.SimReference, p models.SimProfile, requested *string) (string, compareDim, error) {
	if requested == nil {
		return p.Omnium, nil, nil
	}
	if *requested == "" {
		return "", nil, nil
	}
	picks := map[int][]models.TalentEntry{} // node -> its picked entries
	for pair := range strings.SplitSeq(*requested, "/") {
		id, rank, ok := strings.Cut(pair, ":")
		entry, err := strconv.Atoi(id)
		if r, rankErr := strconv.Atoi(rank); err != nil || rankErr != nil || !ok || r != 1 {
			return "", nil, fmt.Errorf("%w: the Omnium Folio's picks are entry:1 pairs", ErrBadSimOptions)
		}
		i := slices.IndexFunc(ref.Omnium, func(n models.TalentNode) bool {
			return slices.ContainsFunc(n.Entries, func(e models.TalentEntry) bool { return e.ID == entry })
		})
		if i < 0 || slices.ContainsFunc(picks[ref.Omnium[i].ID], func(e models.TalentEntry) bool { return e.ID == entry }) {
			return "", nil, fmt.Errorf("%w: entry %d is not a rune of the Omnium Folio, or is picked twice",
				ErrBadSimOptions, entry)
		}
		node := ref.Omnium[i]
		e := node.Entries[slices.IndexFunc(node.Entries, func(e models.TalentEntry) bool { return e.ID == entry })]
		picks[node.ID] = append(picks[node.ID], e)
	}
	// One entry from each picked node, in the tree's order so a choice reads the same each time.
	variants := [][]models.TalentEntry{nil}
	for _, n := range ref.Omnium {
		if entries := picks[n.ID]; len(entries) > 0 {
			var next [][]models.TalentEntry
			for _, v := range variants {
				for _, e := range entries {
					next = append(next, append(slices.Clone(v), e))
				}
			}
			variants = next
		}
	}
	text := func(v []models.TalentEntry) string {
		ids := make([]string, len(v))
		for i, e := range v {
			ids[i] = strconv.Itoa(e.ID) + ":1"
		}
		return strings.Join(ids, "/")
	}
	if len(variants) == 1 {
		return text(variants[0]), nil, nil
	}
	compared := map[int]bool{} // the entries of the nodes with several picks
	for _, entries := range picks {
		if len(entries) > 1 {
			for _, e := range entries {
				compared[e.ID] = true
			}
		}
	}
	dim := make(compareDim, len(variants))
	for i, v := range variants {
		var names []string
		for _, e := range v {
			if compared[e.ID] {
				names = append(names, e.Name)
			}
		}
		dim[i] = compareOption{"omnium_talents=" + text(v) + "\n", text(v), strings.Join(names, ", ")}
	}
	return text(variants[0]), dim, nil
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
