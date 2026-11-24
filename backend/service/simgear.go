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
	// maxCombos is the most gear combinations a Top Gear sim runs.
	maxCombos = 100
	// setTargetError is the finest target error (in % of the DPS) profilesets run at, so 100 of them stay near a minute.
	setTargetError = 0.2
)

// gearPairs are the slots Top Gear fills together, with two different items.
var gearPairs = [][2]string{{"finger1", "finger2"}, {"trinket1", "trinket2"}}

// gearCandidate is an item Top Gear may put on, and the slot it is equipped in ("" for one from the bags).
type gearCandidate struct {
	item models.SimItem
	worn string
}

// gearCombo is one profileset a sim runs: its name and the slots it changes, for a Droptimizer's drop where it drops,
// and for a sim that compares consumables the simc line it sets (line), the value it stands for (option, for a Quick
// Sim) or the name of the consumable a gear combination runs with (consumable).
type gearCombo struct {
	name       string
	swaps      []models.SimSwap
	source     string
	boss       string
	line       string
	option     *models.SimOptionRow
	consumable string
}

// currentGear is the equipped gear as a combination, with no swaps (an empty list, not nil, which the page reads), run
// beside the others when a Top Gear sim compares consumables.
func currentGear() gearCombo { return gearCombo{name: "current", swaps: []models.SimSwap{}} }

// gearGroups are the ways to fill each group of slots with the picked items (indexes into p's Equipped and Bags) and
// extra candidates (a picked item's changed copies): a combination takes one way from each group. A slot with nothing
// picked keeps its item; two rings or trinkets are never the same item, and a two-handed weapon (twoHanded says which
// item ID is) empties the off-hand.
func gearGroups(p models.SimProfile, equipped, bags []int, extra []gearCandidate,
	twoHanded func(id int) (bool, error)) ([][][]models.SimSwap, error) {
	pool, err := gearPool(p, equipped, bags)
	if err != nil {
		return nil, err
	}
	for _, c := range extra {
		pool[gearKey(c.item.Slot)] = append(pool[gearKey(c.item.Slot)], c)
	}
	worn := map[string]*models.SimItem{}
	for i := range p.Equipped {
		worn[p.Equipped[i].Slot] = &p.Equipped[i]
	}
	weapons, err := weaponOptions(pool, worn, twoHanded)
	if err != nil {
		return nil, err
	}
	groups := [][][]models.SimSwap{weapons}
	for _, pair := range gearPairs {
		groups = append(groups, pairOptions(pair, pool[gearKey(pair[0])], worn))
	}
	for _, slot := range simSlots {
		if gearKey(slot) == slot && slot != "main_hand" && slot != "off_hand" {
			groups = append(groups, slotOptions(slot, pool[slot], worn))
		}
	}
	return groups, nil
}

// comboCount is how many combinations the groups make other than the equipped gear (the sim's baseline: every
// group's no-change options taken together), stopping at limit.
func comboCount(groups [][][]models.SimSwap, limit int) int {
	if slices.ContainsFunc(groups, func(g [][]models.SimSwap) bool { return len(g) == 0 }) {
		return 0
	}
	all, same := 1, 1
	for _, g := range groups {
		all *= len(g)
		same *= len(slices.DeleteFunc(slices.Clone(g), func(o []models.SimSwap) bool { return len(o) > 0 }))
		if all-same > limit { // the count never shrinks, as every group has an option
			return limit
		}
	}
	return all - same
}

// comboRefusal is why a Top Gear sim of n combinations (times the consumables compared) is refused
// (ErrBadSimOptions), nil when it may run.
func comboRefusal(n int) error {
	switch {
	case n == 0:
		return fmt.Errorf("%w: tick at least one item that isn't equipped, or add a variant", ErrBadSimOptions)
	case n > maxCombos:
		return fmt.Errorf("%w: the picked items and consumables make over %d sims; untick some", ErrBadSimOptions,
			maxCombos)
	}
	return nil
}

// topGearCombos is every legal combination of the picked items and extra candidates other than the equipped gear
// (gearGroups). More than maxCombos is ErrBadSimOptions, and so is none unless the sim is comparing consumables (then
// the equipped gear alone is worth a run): nothing is dropped.
func topGearCombos(p models.SimProfile, equipped, bags []int, extra []gearCandidate, comparing bool,
	twoHanded func(id int) (bool, error)) ([]gearCombo, error) {
	groups, err := gearGroups(p, equipped, bags, extra, twoHanded)
	if err != nil {
		return nil, err
	}
	n := comboCount(groups, maxCombos+1)
	if n == 0 && comparing {
		return nil, nil
	}
	if err := comboRefusal(n); err != nil {
		return nil, err
	}
	var combos []gearCombo
	var walk func(g int, swaps []models.SimSwap)
	walk = func(g int, swaps []models.SimSwap) {
		if g == len(groups) {
			if len(swaps) > 0 { // none is the equipped gear, which the sim's baseline runs
				combos = append(combos, gearCombo{name: "combo" + strconv.Itoa(len(combos)+1), swaps: slices.Clone(swaps)})
			}
			return
		}
		for _, option := range groups[g] {
			walk(g+1, append(swaps, option...))
		}
	}
	walk(0, nil)
	return combos, nil
}

// gearKey is the pool a slot's candidates share: "finger" for both rings, "trinket" for both trinkets, else the slot.
func gearKey(slot string) string {
	return strings.TrimRight(slot, "12")
}

// gearPool is the picked items by gearKey, equipped ones first, each once; an index outside p is ErrBadSimOptions.
func gearPool(p models.SimProfile, equipped, bags []int) (map[string][]gearCandidate, error) {
	pool := map[string][]gearCandidate{}
	for _, list := range []struct {
		items  []models.SimItem
		picked []int
		worn   bool
	}{{p.Equipped, equipped, true}, {p.Bags, bags, false}} {
		for _, i := range slices.Compact(slices.Sorted(slices.Values(list.picked))) {
			if i < 0 || i >= len(list.items) {
				return nil, fmt.Errorf("%w: no item %d to pick", ErrBadSimOptions, i)
			}
			c := gearCandidate{item: list.items[i]}
			if list.worn {
				c.worn = c.item.Slot
			}
			pool[gearKey(c.item.Slot)] = append(pool[gearKey(c.item.Slot)], c)
		}
	}
	return pool, nil
}

// slotOptions are the ways to fill one slot: each picked item, or keeping its item when none is picked.
func slotOptions(slot string, picked []gearCandidate, worn map[string]*models.SimItem) [][]models.SimSwap {
	if len(picked) == 0 {
		return [][]models.SimSwap{nil}
	}
	options := [][]models.SimSwap{}
	for _, c := range picked {
		options = append(options, gearSwaps(worn, gearFill{slot, &c}))
	}
	return options
}

// distinctItems is how many different items (by ID) the candidates are.
func distinctItems(picked []gearCandidate) int {
	ids := map[int]bool{}
	for _, c := range picked {
		ids[c.item.ID] = true
	}
	return len(ids)
}

// pairOptions are the ways to fill two ring or trinket slots with two different picked items, the equipped ones
// making up the two when fewer are picked. An equipped item stays in its own slot.
func pairOptions(slots [2]string, picked []gearCandidate, worn map[string]*models.SimItem) [][]models.SimSwap {
	if len(picked) == 0 {
		return [][]models.SimSwap{nil}
	}
	short := distinctItems(picked) < 2 // variants of one item count once: the pair needs another
	for _, slot := range slots {
		if short && worn[slot] != nil && !slices.ContainsFunc(picked, func(c gearCandidate) bool { return c.worn == slot }) {
			picked = append(picked, gearCandidate{item: *worn[slot], worn: slot})
		}
	}
	if len(picked) == 1 {
		return [][]models.SimSwap{gearSwaps(worn, gearFill{cmp.Or(picked[0].worn, slots[0]), &picked[0]})}
	}
	options := [][]models.SimSwap{}
	for i := range picked {
		for j := i + 1; j < len(picked); j++ {
			a, b := &picked[i], &picked[j]
			if a.item.ID == b.item.ID {
				continue // unique-equipped: the same ring or trinket can't be worn twice
			}
			if a.worn == slots[1] || b.worn == slots[0] {
				a, b = b, a
			}
			options = append(options, gearSwaps(worn, gearFill{slots[0], a}, gearFill{slots[1], b}))
		}
	}
	return options
}

// weaponOptions are the ways to fill the main and off hands. A two-handed main hand empties the off-hand; any other
// goes with each picked off-hand (the equipped one when none is picked). The export doesn't say which weapon is
// two-handed, so twoHanded is asked, only for a main hand an off-hand could go beside: one worn with an off-hand is
// one-handed.
func weaponOptions(pool map[string][]gearCandidate, worn map[string]*models.SimItem,
	twoHanded func(id int) (bool, error)) ([][]models.SimSwap, error) {
	mains, offs := pool["main_hand"], pool["off_hand"]
	if len(mains) == 0 && len(offs) == 0 {
		return [][]models.SimSwap{nil}, nil
	}
	if len(mains) == 0 && worn["main_hand"] != nil {
		mains = []gearCandidate{{item: *worn["main_hand"], worn: "main_hand"}}
	}
	if len(offs) == 0 && worn["off_hand"] != nil {
		offs = []gearCandidate{{item: *worn["off_hand"], worn: "off_hand"}}
	}
	options := [][]models.SimSwap{}
	for i := range mains {
		m := &mains[i]
		alone := len(offs) == 0
		if !alone && (m.worn == "" || worn["off_hand"] == nil) {
			var err error
			if alone, err = twoHanded(m.item.ID); err != nil {
				return nil, fmt.Errorf("reading whether %s (%d) is two-handed: %w", m.item.Name, m.item.ID, err)
			}
		}
		if alone {
			options = append(options, gearSwaps(worn, gearFill{"main_hand", m}, gearFill{"off_hand", nil}))
			continue
		}
		for j := range offs {
			options = append(options, gearSwaps(worn, gearFill{"main_hand", m}, gearFill{"off_hand", &offs[j]}))
		}
	}
	if len(mains) == 0 {
		for j := range offs {
			options = append(options, gearSwaps(worn, gearFill{"off_hand", &offs[j]}))
		}
	}
	return options, nil
}

// wearsTwoHander is whether the equipped main hand is two-handed: the export doesn't say, so it is when no off-hand is
// equipped beside it.
func wearsTwoHander(worn map[string]*models.SimItem) bool {
	return worn["main_hand"] != nil && worn["off_hand"] == nil
}

// gearFill is one slot of a combination and what goes in it (nil empties it).
type gearFill struct {
	slot string
	put  *gearCandidate
}

// gearSwaps are the fills that change what is equipped, as swaps.
func gearSwaps(worn map[string]*models.SimItem, fills ...gearFill) []models.SimSwap {
	var swaps []models.SimSwap
	for _, f := range fills {
		if f.put == nil && worn[f.slot] == nil || f.put != nil && f.put.worn == f.slot {
			continue
		}
		swap := models.SimSwap{Slot: f.slot, Out: worn[f.slot]}
		if f.put != nil {
			in := f.put.item
			in.Slot = f.slot
			swap.In = &in
		}
		swaps = append(swaps, swap)
	}
	return swaps
}

// gearProfilesets are the combinations as simc profileset lines, one per changed slot or set option.
func gearProfilesets(combos []gearCombo) string {
	var b strings.Builder
	for _, c := range combos {
		if c.line != "" {
			fmt.Fprintf(&b, "profileset.%s+=%s", c.name, c.line)
		}
		for _, s := range c.swaps {
			line := s.Slot + "=\n"
			if s.In != nil {
				line = SimItemLine(*s.In)
			}
			fmt.Fprintf(&b, "profileset.%s+=%s", c.name, line)
		}
	}
	return b.String()
}

// simSetJSON is one profileset's result in simc's json2 file.
type simSetJSON struct {
	Name       string  `json:"name"`
	Mean       float64 `json:"mean"`
	MeanStdDev float64 `json:"mean_stddev"`
}

// gearResults are the combinations simc ran, best first, with their gain over the baseline's DPS. A drop that fills
// two slots (a ring) keeps its better row.
func gearResults(sets []simSetJSON, baseline float64, combos []gearCombo) []models.SimCombo {
	rows := []models.SimCombo{}
	for _, set := range sets {
		if i := slices.IndexFunc(combos, func(c gearCombo) bool { return c.name == set.Name }); i >= 0 {
			rows = append(rows, models.SimCombo{DPS: set.Mean, DPSError: set.MeanStdDev, Gain: set.Mean - baseline,
				Swaps: combos[i].swaps, Source: combos[i].source, Boss: combos[i].boss, Consumable: combos[i].consumable})
		}
	}
	slices.SortStableFunc(rows, func(a, b models.SimCombo) int { return cmp.Compare(b.DPS, a.DPS) })
	type dropKey struct {
		source, boss, consumable string
		id                       int
	}
	seen := map[dropKey]bool{}
	return slices.DeleteFunc(rows, func(r models.SimCombo) bool {
		if r.Source == "" {
			return false
		}
		key := dropKey{r.Source, r.Boss, r.Consumable, r.Swaps[0].In.ID}
		dup := seen[key]
		seen[key] = true
		return dup
	})
}
