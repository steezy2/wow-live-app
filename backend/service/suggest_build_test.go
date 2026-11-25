package service

import (
	"slices"
	"testing"

	"wow-live/backend/models"
)

// suggestFixture is a character with two rings and a head, each ring holding gem 201, and ten top players built by
// mutate: a reference with two ring enchants, two gems, a flask in two qualities and a spec tree with two talents.
func suggestFixture(mutate func(in *suggestInput)) suggestInput {
	in := suggestInput{
		ref: models.SimReference{
			Enchants: []models.SimEnchant{{ID: 100, Name: "Amani Mastery", Slots: []string{"finger1", "finger2"}}},
			Gems:     []models.SimGem{{ID: 200, Name: "Quick Lapis"}, {ID: 201, Name: "Masterful Garnet"}},
			Consumables: map[string][]models.SimConsumable{"flask": {
				{Value: "flask_a_1", Name: "Flask A (Quality 1)", Quality: 1},
				{Value: "flask_a_2", Name: "Flask A (Quality 2)", Quality: 2},
				{Value: "flask_b_2", Name: "Flask B (Quality 2)", Quality: 2}}},
		},
		you: models.SimProfile{Equipped: []models.SimItem{
			{Slot: "head", ID: 1},
			{Slot: "finger1", ID: 2, Opts: map[string][]int{"gem_id": {201}}},
			{Slot: "finger2", ID: 3, Opts: map[string][]int{"gem_id": {201}}}}},
		tops: make([]models.RankedPlayer, 10),
	}
	for i := range in.tops {
		in.tops[i].Gear = []models.RankedGear{{Slot: "head", ID: 1}}
	}
	mutate(&in)
	return in
}

// ringsWith gives the first n top players an item on a ring with the enchant and gem (0 for none).
func ringsWith(in *suggestInput, n, enchant, gem int) {
	for i := range n {
		g := models.RankedGear{Slot: "finger2", ID: 2, Enchant: enchant}
		if gem != 0 {
			g.Gems = []int{gem}
		}
		in.tops[i].Gear = append(in.tops[i].Gear, g)
	}
}

func TestBuildSuggestions(t *testing.T) {
	flask := func(n int) func(*suggestInput) {
		return func(in *suggestInput) {
			for i := range n {
				in.tops[i].Buffs = []models.RankedBuff{{Name: "Flask A"}}
			}
		}
	}
	tests := []struct {
		name   string
		mutate func(*suggestInput)
		want   []string // kind and what
	}{
		{"an enchant 6 of 10 wear", func(in *suggestInput) { ringsWith(in, 6, 100, 0) },
			[]string{"enchant Enchant Amani Mastery on finger1"}},
		{"an enchant 5 of 10 wear", func(in *suggestInput) { ringsWith(in, 5, 100, 0) }, nil},
		{"an enchant the rings already have", func(in *suggestInput) {
			ringsWith(in, 6, 100, 0)
			for i := 1; i <= 2; i++ {
				in.you.Equipped[i].Opts["enchant_id"] = []int{100}
			}
		}, nil},
		{"an enchant the reference doesn't know", func(in *suggestInput) { ringsWith(in, 6, 999, 0) }, nil},
		{"a gem 6 of 10 use, one suggestion for both rings", func(in *suggestInput) { ringsWith(in, 6, 0, 200) },
			[]string{"gem Gem Quick Lapis in finger1"}},
		{"the gem the rings already hold", func(in *suggestInput) { ringsWith(in, 6, 0, 201) }, nil},
		{"a flask 6 of 10 have up, in its best quality", flask(6), []string{"consumable Use Flask A (Quality 2)"}},
		{"a flask 5 of 10 have up", flask(5), nil},
		{"the flask the sim already uses", func(in *suggestInput) {
			flask(6)(in)
			in.consumables = models.SimConsumables{"flask": {"flask_a_2"}}
		}, nil},
		{"a stat the tops favor by 10 points", func(in *suggestInput) {
			in.yourStats = &models.RankedStats{Crit: 1000, Haste: 1000, Mastery: 1000, Vers: 1000}
			for i := range in.tops {
				in.tops[i].Stats = &models.RankedStats{Crit: 1000, Haste: 1400, Mastery: 800, Vers: 800}
			}
		}, []string{"stat More Haste"}},
		{"stats within the gap", func(in *suggestInput) {
			in.yourStats = &models.RankedStats{Crit: 1000, Haste: 1000, Mastery: 1000, Vers: 1000}
			for i := range in.tops {
				in.tops[i].Stats = &models.RankedStats{Crit: 1000, Haste: 1080, Mastery: 960, Vers: 960}
			}
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			suggestions, _ := buildSuggestions(suggestFixture(tt.mutate))
			for _, s := range suggestions {
				got = append(got, s.Kind+" "+s.What)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("suggestions = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatSuggestionsAreWorthTheGapTimesTheWeight(t *testing.T) {
	in := suggestFixture(func(in *suggestInput) {
		in.yourStats = &models.RankedStats{Crit: 1000, Haste: 1000, Mastery: 1000, Vers: 1000}
		for i := range in.tops {
			in.tops[i].Stats = &models.RankedStats{Crit: 1000, Haste: 1400, Mastery: 800, Vers: 800}
		}
		in.weights = []models.SimStatWeight{{Stat: "Haste", DPS: 2}}
	})
	got, _ := buildSuggestions(in)
	// Haste is 35% of the tops' 4000 against 25%: 10 points of 4000 rating at 2 DPS a point
	if len(got) != 1 || got[0].Expected != 800 {
		t.Errorf("suggestions %+v, want one worth 800 DPS", got)
	}
}

func TestTalentSuggestions(t *testing.T) {
	// a class tree of 7 points, a spec tree and two hero trees of a point each; the character took all class talents
	// but Thunderstorm and Primordial Wave, Ancestral Swiftness in the choice node and the tiered node at its first
	// entry (as the loadout string reads it), Ascendance before its spec talent, and hero tree 1, holding a talent of
	// each hero tree as the loadout string does
	tree := models.TalentTree{Points: map[string]int{"class": 7, "spec": 2, "hero": 1}, Nodes: []models.TalentNode{
		{ID: 1, Tree: "class", Choice: true, MaxRanks: 1,
			Entries: []models.TalentEntry{{ID: 11, Name: "Ancestral Swiftness"}, {ID: 12, Name: "Nature's Swiftness"}}},
		{ID: 2, Tree: "class", MaxRanks: 1, Entries: []models.TalentEntry{{ID: 21, Name: "Totemic Surge"}}},
		{ID: 3, Tree: "class", MaxRanks: 1, Entries: []models.TalentEntry{{ID: 31, Name: "Thunderstorm"}}},
		{ID: 4, Tree: "class", MaxRanks: 1, Free: true, Entries: []models.TalentEntry{{ID: 41}}},
		{ID: 5, Tree: "class", MaxRanks: 3, Entries: []models.TalentEntry{{ID: 51}, {ID: 52}}},
		{ID: 6, Tree: "class", MaxRanks: 1, Next: []int{7}, Entries: []models.TalentEntry{{ID: 61, Name: "Earth Shield"}}},
		{ID: 7, Tree: "class", MaxRanks: 1, Entries: []models.TalentEntry{{ID: 71}}},
		{ID: 8, Tree: "spec", MaxRanks: 1, Entries: []models.TalentEntry{{ID: 81}}},
		{ID: 9, Tree: "class", MaxRanks: 2, Entries: []models.TalentEntry{{ID: 91, Name: "Elemental Fury"}}},
		{ID: 10, Tree: "class", MaxRanks: 2, Entries: []models.TalentEntry{{ID: 101, Name: "Primordial Wave"}}},
		{ID: 13, Tree: "spec", MaxRanks: 1, Entries: []models.TalentEntry{{ID: 131, Name: "Ascendance"}}},
		{ID: 14, Tree: "spec", MaxRanks: 1, ReqPoints: 2, Entries: []models.TalentEntry{{ID: 141}}},
		{ID: 21, Tree: "hero", SubTree: 1, MaxRanks: 1, Entries: []models.TalentEntry{{ID: 211, Name: "Tempest"}}},
		{ID: 22, Tree: "hero", SubTree: 1, MaxRanks: 1, Entries: []models.TalentEntry{{ID: 221, Name: "Supercharge"}}},
		{ID: 23, Tree: "hero", SubTree: 2, MaxRanks: 1, Entries: []models.TalentEntry{{ID: 231, Name: "Earthen Rage"}}},
		{ID: 24, Tree: "hero", SubTree: 2, MaxRanks: 1, Entries: []models.TalentEntry{{ID: 241, Name: "Totemic Rebound"}}},
		{ID: 15, Tree: "class", MaxRanks: 1, Entries: []models.TalentEntry{{ID: 151, Name: "Spirit Walk"}}},
	}, HeroChoice: models.TalentNode{ID: 20, Tree: "hero", Choice: true,
		Entries: []models.TalentEntry{{ID: 201, SubTree: 1}, {ID: 202, SubTree: 2}}}}
	pick := func(node, entry int) models.TalentPick {
		return models.TalentPick{NodeID: node, EntryID: entry, Rank: 1}
	}
	picks := []models.TalentPick{pick(1, 11), pick(2, 21), pick(4, 41), pick(5, 51), pick(6, 61), pick(7, 71),
		{NodeID: 9, EntryID: 91, Rank: 2}, pick(8, 81), pick(13, 131), pick(20, 201), pick(21, 211), pick(23, 231)}
	tests := []struct {
		name  string
		took  map[models.TalentPick]int // how many of the 10 top players took each talent; nil: the export has none
		want  []string
		notes int
	}{
		{"a choice node swaps its other entry", map[models.TalentPick]int{pick(1, 12): 6},
			[]string{"Take Nature's Swiftness instead of Ancestral Swiftness"}, 0},
		{"a talent 5 of 10 took", map[models.TalentPick]int{pick(1, 12): 5}, nil, 0},
		{"a talent the character took", map[models.TalentPick]int{pick(1, 11): 10}, nil, 0},
		{"a talent replaces the least taken one of its section, never a free or tiered one",
			map[models.TalentPick]int{pick(3, 31): 10, pick(1, 11): 10, pick(2, 21): 8, pick(6, 61): 9, pick(7, 71): 9},
			[]string{"Take Thunderstorm instead of Totemic Surge"}, 0},
		{"two swaps in one section give up different talents", map[models.TalentPick]int{pick(3, 31): 10,
			pick(15, 151): 9, pick(1, 11): 10, pick(2, 21): 8, pick(6, 61): 9, pick(7, 71): 9},
			[]string{"Take Thunderstorm instead of Totemic Surge", "Take Spirit Walk instead of a talent"}, 0},
		{"a swap leaving a talent unreached replaces the next least taken one",
			map[models.TalentPick]int{pick(3, 31): 10, pick(1, 11): 10, pick(2, 21): 10, pick(6, 61): 7, pick(7, 71): 10},
			[]string{"Take Thunderstorm instead of Ancestral Swiftness"}, 0},
		{"a talent of 2 ranks replaces one of 2 ranks", map[models.TalentPick]int{{NodeID: 10, EntryID: 101, Rank: 2}: 10},
			[]string{"Take Primordial Wave instead of Elemental Fury"}, 0},
		{"a talent no swap of its section allows is dropped with a note", map[models.TalentPick]int{pick(14, 141): 10},
			nil, 1},
		{"a talent of the hero tree the character doesn't play is dropped with a note",
			map[models.TalentPick]int{pick(22, 221): 10, pick(24, 241): 10},
			[]string{"Take Supercharge instead of Tempest"}, 1},
		{"a tiered node isn't suggested", map[models.TalentPick]int{pick(5, 52): 10}, nil, 0},
		{"an export without talents", nil, nil, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := suggestFixture(func(in *suggestInput) {
				if in.talentTree = tree; tt.took != nil {
					in.you.Picks = picks
				}
				for i := range in.tops { // every top log has talents, so all 10 count
					in.tops[i].Talents = []models.TalentPick{pick(8, 81)}
				}
				for p, n := range tt.took {
					for i := range n {
						in.tops[i].Talents = append(in.tops[i].Talents, p)
					}
				}
			})
			got, notes := talentSuggestions(in)
			var what []string
			for _, s := range got {
				what = append(what, s.What)
			}
			if !slices.Equal(what, tt.want) || len(notes) != tt.notes {
				t.Errorf("suggestions %q, notes %q; want %q and %d notes", what, notes, tt.want, tt.notes)
			}
		})
	}
}
