package service

import (
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"wow-live/backend/models"
)

const (
	// suggestShare is the share of the top players that must do something for it to be suggested.
	suggestShare = 0.6
	// suggestStatGap is how many percentage points of the secondary stats a top player's median share must lead the
	// owner's by for a stat to be suggested.
	suggestStatGap = 3.0
	// maxTalentSuggestions is how many talent swaps are suggested, by how many top players took the talent.
	maxTalentSuggestions = 8
)

// suggestInput is what buildSuggestions reads: the reference, the character (its talents in Picks) and the
// consumables its sim uses, the top players' builds, the stat weights (SimStatWeight.Stat "Crit", "Haste", "Mastery",
// "Vers"), the character's own secondary stats (nil when not known) and the spec's talent tree.
type suggestInput struct {
	ref         models.SimReference
	you         models.SimProfile
	consumables models.SimConsumables
	tops        []models.RankedPlayer
	weights     []models.SimStatWeight
	yourStats   *models.RankedStats
	talentTree  models.TalentTree
}

var qualitySuffix = regexp.MustCompile(` \(Quality \d\)$`)

// buildSuggestions are the changes the top players make that the character doesn't: the enchant and gem most of them
// have in a slot, the flask and potion most of them have up, the talents most of them took (each as a swap) and the
// secondary stats their split favors, with notes on what was left out. Each is a single change, so the sim can test
// them one by one. Pure: no network.
func buildSuggestions(in suggestInput) ([]models.Suggestion, []string) {
	out := gearSuggestions(in)
	out = append(out, consumableSuggestions(in)...)
	talents, notes := talentSuggestions(in)
	out = append(out, talents...)
	return append(out, statSuggestions(in)...), notes
}

// evidence is "14 of 20 top players".
func evidence(n, of int) string { return fmt.Sprintf("%d of %d top players", n, of) }

// popular is the most common key of counts (the lowest on a tie), and whether at least suggestShare of of did it.
func popular(counts map[int]int, of int) (key, n int, ok bool) {
	for _, k := range slices.Sorted(maps.Keys(counts)) {
		if counts[k] > n {
			key, n = k, counts[k]
		}
	}
	return key, n, n > 0 && float64(n) >= suggestShare*float64(of)
}

// gearSuggestions are the enchant and gem most top players have in each slot group, for the first of the character's
// items of the group that can take it and doesn't have it.
func gearSuggestions(in suggestInput) []models.Suggestion {
	of := 0
	enchants, gems := map[string]map[int]int{}, map[string]map[int]int{}
	count := func(m map[string]map[int]int, group string, id int) {
		if m[group] == nil {
			m[group] = map[int]int{}
		}
		m[group][id]++
	}
	for _, p := range in.tops {
		if len(p.Gear) == 0 {
			continue
		}
		of++
		seen := map[string]bool{} // each player counts once for a group's enchant or gem
		for _, g := range p.Gear {
			group := models.SlotGroup(g.Slot)
			if key := fmt.Sprint("e", group, g.Enchant); g.Enchant != 0 && !seen[key] {
				seen[key] = true
				count(enchants, group, g.Enchant)
			}
			for _, gem := range g.Gems {
				if key := fmt.Sprint("g", group, gem); !seen[key] {
					seen[key] = true
					count(gems, group, gem)
				}
			}
		}
	}
	var out []models.Suggestion
	done := map[string]bool{}
	for i, item := range in.you.Equipped {
		group := models.SlotGroup(item.Slot)
		id, n, ok := popular(enchants[group], of)
		if ok && !done["e"+group] && !slices.Contains(item.Opts["enchant_id"], id) {
			if e := slices.IndexFunc(in.ref.Enchants, func(e models.SimEnchant) bool { return e.ID == id }); e >= 0 &&
				slices.Contains(in.ref.Enchants[e].Slots, item.Slot) {
				done["e"+group] = true
				out = append(out, models.Suggestion{Kind: "enchant", Slot: item.Slot,
					What:     fmt.Sprintf("Enchant %s on %s", in.ref.Enchants[e].Name, item.Slot),
					Evidence: evidence(n, of), Variant: &models.SimVariant{Worn: true, Index: i, Enchant: id}})
			}
		}
		first := 0
		if g := item.Opts["gem_id"]; len(g) > 0 {
			first = g[0]
		}
		if id, n, ok := popular(gems[group], of); ok && !done["g"+group] && id != first && socketsOf(in.ref, item) > 0 {
			if g := slices.IndexFunc(in.ref.Gems, func(g models.SimGem) bool { return g.ID == id }); g >= 0 {
				done["g"+group] = true
				out = append(out, models.Suggestion{Kind: "gem", Slot: item.Slot,
					What:     fmt.Sprintf("Gem %s in %s", in.ref.Gems[g].Name, item.Slot),
					Evidence: evidence(n, of), Variant: &models.SimVariant{Worn: true, Index: i, Gem: id}})
			}
		}
	}
	return out
}

// consumableSuggestions are the flask and potion most top players had up that the character's sim doesn't use. A buff
// matches the consumable of the same name, ignoring its quality (the highest quality is suggested). Food isn't
// suggested: every food's buff is named "Well Fed" in the logs, so the buff can't say which food it was.
func consumableSuggestions(in suggestInput) []models.Suggestion {
	of := len(in.tops)
	up := map[string]int{} // top players with the buff up, by name
	for _, p := range in.tops {
		seen := map[string]bool{}
		for _, b := range p.Buffs {
			if !seen[b.Name] {
				seen[b.Name] = true
				up[b.Name]++
			}
		}
	}
	var out []models.Suggestion
	for _, kind := range []string{"flask", "potion"} {
		var best models.SimConsumable
		n := 0
		for _, c := range in.ref.Consumables[kind] {
			count := up[qualitySuffix.ReplaceAllString(c.Name, "")]
			if count > n || (count == n && count > 0 && qualitySuffix.ReplaceAllString(best.Name, "") ==
				qualitySuffix.ReplaceAllString(c.Name, "") && c.Quality > best.Quality) {
				best, n = c, count
			}
		}
		if n > 0 && float64(n) >= suggestShare*float64(of) && !slices.Contains(in.consumables[kind], best.Value) {
			out = append(out, models.Suggestion{Kind: "consumable", What: "Use " + best.Name, Evidence: evidence(n, of),
				Consumables: models.SimConsumables{kind: {best.Value}}})
		}
	}
	return out
}

// talentSuggestions are the talents at least suggestShare of the top logs took that the character (its Picks) didn't,
// the most taken first (maxTalentSuggestions), each as a swap the sim can test (replacedTalent). Free and tiered nodes
// are left out: the loadout string names no entry for them, so the character's can't be told from the top logs'. So
// are the hero trees the character doesn't play: the string holds both hero trees' talents, and simc ignores the
// other's. A talent with no swap validTalents allows is left out, and those left out are counted in notes, as is an
// export whose talents can't be read.
func talentSuggestions(in suggestInput) ([]models.Suggestion, []string) {
	if in.you.Picks == nil {
		return nil, []string{"Talent swaps aren't tried: the export has no talent string this app can read"}
	}
	t := talentCompare(compareInput{you: models.RankedPlayer{Talents: in.you.Picks}, top: in.tops})
	if t == nil {
		return nil, nil
	}
	hero := 0 // the hero tree the character plays: the one its hero tree choice picks
	for _, e := range in.talentTree.HeroChoice.Entries {
		if slices.ContainsFunc(in.you.Picks, func(p models.TalentPick) bool {
			return p.NodeID == in.talentTree.HeroChoice.ID && p.EntryID == e.ID
		}) {
			hero = e.SubTree
		}
	}
	nodes, names := map[int]models.TalentNode{}, map[int]string{}
	for _, n := range in.talentTree.Nodes {
		if !n.Free && (n.Choice || len(n.Entries) == 1) {
			nodes[n.ID] = n
		}
		for _, e := range n.Entries {
			names[e.ID] = e.Name
		}
	}
	taken := map[[2]int]int{} // top logs that took each node's entry
	var shares []models.TalentShare
	otherHero := 0
	for _, s := range t.Top {
		taken[[2]int{s.NodeID, s.EntryID}] = s.Taken
		took := slices.ContainsFunc(in.you.Picks, func(p models.TalentPick) bool {
			return p.NodeID == s.NodeID && p.EntryID == s.EntryID
		})
		n, ok := nodes[s.NodeID]
		switch {
		case !ok || took || float64(s.Taken) < suggestShare*float64(t.Of):
		case n.Tree == "hero" && n.SubTree != hero:
			otherHero++
		default:
			shares = append(shares, s)
		}
	}
	slices.SortStableFunc(shares, func(a, b models.TalentShare) int { return cmp.Compare(b.Taken, a.Taken) })
	var out []models.Suggestion
	refused := 0
	given := map[models.TalentPick]bool{} // each swap gives up another talent, so the combined row can take them all
	for _, s := range shares {
		if len(out) == maxTalentSuggestions {
			break
		}
		sg := models.Suggestion{Kind: "talent", Evidence: evidence(s.Taken, t.Of), Talent: &s,
			Replaces: replacedTalent(in.talentTree, in.you.Picks, nodes, taken, given, s)}
		if sg.Replaces == nil {
			refused++
			continue
		}
		given[*sg.Replaces] = true
		sg.What = fmt.Sprintf("Take %s instead of %s", cmp.Or(names[s.EntryID], "a talent"),
			cmp.Or(names[sg.Replaces.EntryID], "a talent"))
		out = append(out, sg)
	}
	var notes []string
	if refused > 0 {
		notes = append(notes, fmt.Sprintf("%d of the top players' talents weren't tried: no swap for a talent of the "+
			"same rank in their section keeps your talent tree valid", refused))
	}
	if otherHero > 0 {
		notes = append(notes, fmt.Sprintf("%d of the top players' talents weren't tried: top players play another "+
			"hero tree", otherHero))
	}
	return out, notes
}

// replacedTalent is the character's talent (of picks) a swap to s gives up, the first one validTalents allows in tree:
// in a choice node, the entry taken; else, of the talents of s's rank taken in its section (class, spec or a hero tree)
// in nodes (neither free nor tiered), the one the fewest top logs took first; never one of given. Nil when none is
// allowed.
func replacedTalent(tree models.TalentTree, picks []models.TalentPick, nodes map[int]models.TalentNode,
	taken map[[2]int]int, given map[models.TalentPick]bool, s models.TalentShare) *models.TalentPick {
	node := nodes[s.NodeID]
	options := slices.DeleteFunc(slices.Clone(picks), func(p models.TalentPick) bool { return p.NodeID != node.ID })
	if len(options) == 0 {
		options = slices.DeleteFunc(slices.Clone(picks), func(p models.TalentPick) bool {
			n, ok := nodes[p.NodeID]
			return !ok || n.Tree != node.Tree || n.SubTree != node.SubTree || p.Rank != s.Rank
		})
		slices.SortStableFunc(options, func(a, b models.TalentPick) int {
			return cmp.Compare(taken[[2]int{a.NodeID, a.EntryID}], taken[[2]int{b.NodeID, b.EntryID}])
		})
	}
	for _, p := range options {
		if !given[p] && validTalents(tree, swapTalent(picks, models.Suggestion{Talent: &s, Replaces: &p})) == nil {
			return &p
		}
	}
	return nil
}

// swapTalent is picks with talent swap s's Talent in place of the talent it Replaces.
func swapTalent(picks []models.TalentPick, s models.Suggestion) []models.TalentPick {
	out := slices.DeleteFunc(slices.Clone(picks), func(p models.TalentPick) bool { return p == *s.Replaces })
	return append(out, models.TalentPick{NodeID: s.Talent.NodeID, EntryID: s.Talent.EntryID, Rank: s.Talent.Rank})
}

// secondaryShares are each secondary stat's share of the four, in percent; nil when there are none.
func secondaryShares(s models.RankedStats) (map[string]float64, float64) {
	total := float64(s.Crit + s.Haste + s.Mastery + s.Vers)
	if total == 0 {
		return nil, 0
	}
	return map[string]float64{"Crit": 100 * float64(s.Crit) / total, "Haste": 100 * float64(s.Haste) / total,
		"Mastery": 100 * float64(s.Mastery) / total, "Vers": 100 * float64(s.Vers) / total}, total
}

// statSuggestions are the secondary stats the top players' median split gives a bigger share than the character's by
// more than suggestStatGap points, each with the DPS closing the gap is worth by the stat weights, the most first.
func statSuggestions(in suggestInput) []models.Suggestion {
	if in.yourStats == nil {
		return nil
	}
	mine, total := secondaryShares(*in.yourStats)
	var tops []map[string]float64
	for _, p := range in.tops {
		if p.Stats == nil {
			continue
		}
		if s, _ := secondaryShares(*p.Stats); s != nil {
			tops = append(tops, s)
		}
	}
	if mine == nil || len(tops) == 0 {
		return nil
	}
	var out []models.Suggestion
	for _, stat := range []string{"Crit", "Haste", "Mastery", "Vers"} {
		values := make([]float64, len(tops))
		for i, s := range tops {
			values[i] = s[stat]
		}
		slices.Sort(values)
		median := values[len(values)/2]
		gap := median - mine[stat]
		if gap <= suggestStatGap {
			continue
		}
		expected := 0.0
		if w := slices.IndexFunc(in.weights, func(w models.SimStatWeight) bool {
			return strings.EqualFold(w.Stat, stat)
		}); w >= 0 {
			expected = gap / 100 * total * in.weights[w].DPS
		}
		out = append(out, models.Suggestion{Kind: "stat", What: "More " + stat, Expected: expected,
			Evidence: fmt.Sprintf("the top players' median is %.0f%% %s, yours %.0f%%", median, stat, mine[stat])})
	}
	slices.SortStableFunc(out, func(a, b models.Suggestion) int { return cmp.Compare(b.Expected, a.Expected) })
	return out
}

// upgradeSuggestions are the character's worn items upgraded to the top of their track (topRank), with Ascendant
// Venomstones where they fit, each a change the sim tests; its gains rank the biggest upgrades first.
func upgradeSuggestions(ref models.SimReference, p models.SimProfile) []models.Suggestion {
	var out []models.Suggestion
	for i, item := range p.Equipped {
		track, current, ok := upgradeOf(ref.Upgrades, item)
		if !ok {
			continue
		}
		v := models.SimVariant{Worn: true, Index: i,
			Venomstone: venomstoneFits(track, item.Slot) && item.Level <= current.ItemLevel}
		to := fmt.Sprintf("%s %d/%d", track.Name, current.Level, len(track.Levels))
		if current.Level < topRank(track) {
			v.Upgrade = topRank(track)
			to = fmt.Sprintf("%s %d/%d", track.Name, v.Upgrade, len(track.Levels))
		}
		if v.Upgrade == 0 && !v.Venomstone {
			continue
		}
		if v.Venomstone {
			to += " with Venomstones"
		}
		out = append(out, models.Suggestion{Kind: "upgrade", Slot: item.Slot, Variant: &v,
			What:     fmt.Sprintf("Upgrade %s to %s", cmp.Or(item.Name, "your "+item.Slot), to),
			Evidence: fmt.Sprintf("your %s at %s %d/%d", item.Slot, track.Name, current.Level, len(track.Levels))})
	}
	return out
}
