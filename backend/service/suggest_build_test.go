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
		talentTree: models.TalentTree{Nodes: []models.TalentNode{{ID: 1,
			Entries: []models.TalentEntry{{ID: 11, Name: "Ancestral Swiftness"}}}}},
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
	talents := func(taken int, you []models.TalentPick) func(*suggestInput) {
		return func(in *suggestInput) {
			in.talents = &models.CompareTalents{You: you, Of: 10,
				Top: []models.TalentShare{{NodeID: 1, EntryID: 11, Taken: taken}}}
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
		{"a talent 6 of 10 took", talents(6, nil), []string{"talent Take Ancestral Swiftness"}},
		{"a talent 5 of 10 took", talents(5, nil), nil},
		{"a talent the character took", talents(10, []models.TalentPick{{NodeID: 1, EntryID: 11, Rank: 1}}), nil},
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
			for _, s := range buildSuggestions(suggestFixture(tt.mutate)) {
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
	got := buildSuggestions(in)
	// Haste is 35% of the tops' 4000 against 25%: 10 points of 4000 rating at 2 DPS a point
	if len(got) != 1 || got[0].Expected != 800 {
		t.Errorf("suggestions %+v, want one worth 800 DPS", got)
	}
}
