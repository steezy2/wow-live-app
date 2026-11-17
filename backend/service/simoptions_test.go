package service

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"wow-live/backend/models"
)

// testRef is a small reference: one track of three levels, a gem, an enchant for rings, a flask and a food and an
// Omnium Folio of one choice node and one plain node.
func testRef() models.SimReference {
	return models.SimReference{
		Consumables: map[string][]models.SimConsumable{
			"flask":  {{Value: "flask_a_2", Name: "Flask A"}, {Value: "flask_b_2", Name: "Flask B"}},
			"food":   {{Value: "feast", Name: "Feast"}},
			"weapon": {{Value: "oil_2", Name: "Oil"}},
		},
		Enchants: []models.SimEnchant{{ID: 7964, Name: "Amani Mastery", Slots: []string{"finger1", "finger2"}}},
		Gems:     []models.SimGem{{ID: 240883, Name: "Quick Lapis"}},
		Upgrades: []models.SimUpgradeTrack{{Group: 618, Name: "Myth", Levels: []models.SimUpgradeLevel{
			{BonusID: 100, Level: 1, ItemLevel: 318}, {BonusID: 101, Level: 2, ItemLevel: 321},
			{BonusID: 102, Level: 3, ItemLevel: 324}}}},
		SocketBonus: 10397,
		Omnium: []models.TalentNode{
			{ID: 1, Choice: true, Entries: []models.TalentEntry{{ID: 11}, {ID: 12}}},
			{ID: 2, Entries: []models.TalentEntry{{ID: 21}}},
		},
	}
}

func ring() models.SimItem {
	return models.SimItem{Slot: "finger1", ID: 5, Name: "Band", Level: 318,
		Opts: map[string][]int{"bonus_id": {7, 100}}}
}

func TestApplyVariant(t *testing.T) {
	tests := []struct {
		name     string
		variant  models.SimVariant
		wantLine string // the item's simc line
		wantNote string
		wantLvl  int
		wantErr  string
	}{
		{"an upgrade swaps the track's bonus", models.SimVariant{Upgrade: 3},
			"finger1=,id=5,bonus_id=7/102\n", "Myth 3/3", 324, ""},
		{"a socket and a gem", models.SimVariant{Socket: true, Gem: 240883},
			"finger1=,id=5,bonus_id=7/100/10397,gem_id=240883\n", "socket, Quick Lapis", 318, ""},
		{"an enchant", models.SimVariant{Enchant: 7964},
			"finger1=,id=5,bonus_id=7/100,enchant_id=7964\n", "Amani Mastery", 318, ""},
		{"nothing changed", models.SimVariant{}, "", "", 0, "changes something"},
		{"an upgrade past the track", models.SimVariant{Upgrade: 4}, "", "", 0, "no upgrade level 4"},
		{"an unknown gem", models.SimVariant{Gem: 1}, "", "", 0, "no gem 1"},
		{"an enchant for another slot", models.SimVariant{Enchant: 7964}, "", "", 0, "no enchant"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := ring()
			if tt.name == "an enchant for another slot" {
				item.Slot = "head"
			}
			got, err := applyVariant(testRef(), item, tt.variant)
			if tt.wantErr != "" {
				if !errors.Is(err, ErrBadSimOptions) || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want ErrBadSimOptions with %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if line := SimItemLine(got); line != tt.wantLine || got.Note != tt.wantNote || got.Level != tt.wantLvl {
				t.Errorf("got %q %q %d, want %q %q %d", line, got.Note, got.Level, tt.wantLine, tt.wantNote, tt.wantLvl)
			}
			if !slices.Equal(item.Opts["bonus_id"], []int{7, 100}) {
				t.Errorf("the original item changed: %v", item.Opts)
			}
		})
	}
}

func TestApplyVariantNeedsATrackForAnUpgrade(t *testing.T) {
	item := ring()
	item.Opts = map[string][]int{"bonus_id": {7}}
	if _, err := applyVariant(testRef(), item, models.SimVariant{Upgrade: 2}); !errors.Is(err, ErrBadSimOptions) {
		t.Errorf("err = %v, want ErrBadSimOptions for an item with no track", err)
	}
}

func TestConsumableLines(t *testing.T) {
	got, err := consumableLines(testRef(), models.SimConsumables{Flask: "flask_b_2", Weapon: "oil_2", Food: "feast"})
	want := "flask=flask_b_2\nfood=feast\ntemporary_enchant=main_hand:oil_2\n"
	if err != nil || got != want {
		t.Errorf("consumableLines = %q, %v, want %q", got, err, want)
	}
	if got, err := consumableLines(testRef(), models.SimConsumables{}); err != nil || got != "" {
		t.Errorf("no picks = %q, %v, want simc's defaults (nothing written)", got, err)
	}
	for _, bad := range []models.SimConsumables{{Flask: "feast"}, {Potion: "x\nthreads=1"}} {
		if _, err := consumableLines(testRef(), bad); !errors.Is(err, ErrBadSimOptions) {
			t.Errorf("%+v: err = %v, want ErrBadSimOptions", bad, err)
		}
	}
}

func TestCompareCombos(t *testing.T) {
	combos, err := compareCombos(testRef(), &models.SimCompare{Kind: "flask", Values: []string{"flask_b_2", "flask_a_2"}})
	if err != nil || len(combos) != 2 {
		t.Fatalf("compareCombos = %v, %v", combos, err)
	}
	want := "profileset.option1+=flask=flask_b_2\nprofileset.option2+=flask=flask_a_2\n"
	if got := gearProfilesets(combos); got != want {
		t.Errorf("profilesets = %q", got)
	}
	tests := []struct {
		name string
		in   *models.SimCompare
	}{
		{"nothing asked", nil},
		{"an unknown kind", &models.SimCompare{Kind: "ring", Values: []string{"a"}}},
		{"no values", &models.SimCompare{Kind: "flask"}},
		{"a value twice", &models.SimCompare{Kind: "flask", Values: []string{"flask_a_2", "flask_a_2"}}},
		{"a value of another kind", &models.SimCompare{Kind: "flask", Values: []string{"feast"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := compareCombos(testRef(), tt.in); !errors.Is(err, ErrBadSimOptions) {
				t.Errorf("err = %v, want ErrBadSimOptions", err)
			}
		})
	}
}

func TestOptionResults(t *testing.T) {
	combos := []gearCombo{
		{name: "option1", option: &models.SimOptionRow{Value: "a", Name: "A"}},
		{name: "option2", option: &models.SimOptionRow{Value: "b", Name: "B"}},
	}
	sets := []simSetJSON{{Name: "option1", Mean: 90, MeanStdDev: 1}, {Name: "option2", Mean: 110, MeanStdDev: 2}}
	got := optionResults(sets, 100, combos)
	if len(got) != 2 || got[0].Value != "b" || got[0].Gain != 10 || got[1].Value != "a" || got[1].Gain != -10 {
		t.Errorf("optionResults = %+v, want b (+10) before a (-10)", got)
	}
}

func TestOmniumInput(t *testing.T) {
	p := models.SimProfile{Omnium: "11:1/21:1"}
	tests := []struct {
		name, requested, want string
		wantErr               bool
	}{
		{"the export's when none is sent", "", "11:1/21:1", false},
		{"a pick is kept in the tree's order", "21:1/12:1", "12:1/21:1", false},
		{"two entries of a node", "11:1/12:1", "", true},
		{"an entry of no node", "99:1", "", true},
		{"not a pair", "11", "", true},
		{"a rank other than 1", "11:2", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := omniumInput(testRef(), p, tt.requested)
			if (err != nil) != tt.wantErr || got != tt.want || (err != nil && !errors.Is(err, ErrBadSimOptions)) {
				t.Errorf("omniumInput(%q) = %q, %v, want %q (error %v)", tt.requested, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestVariantsJoinTheTopGearCandidates(t *testing.T) {
	p := gearProfile(false)
	p.Equipped[1].Opts = map[string][]int{"bonus_id": {100}} // the worn finger1 ring, on the Myth track
	ref := testRef()
	extra, err := variantCandidates(ref, p, []models.SimVariant{{Worn: true, Index: 1, Upgrade: 3}})
	if err != nil || len(extra) != 1 {
		t.Fatalf("variantCandidates = %v, %v", extra, err)
	}
	for _, c := range []struct {
		name  string
		extra []gearCandidate
		want  int
	}{
		{"without the variant the worn rings are the only pair", nil, 0},
		{"with it, the upgraded ring is another", extra, 1},
	} {
		groups, err := gearGroups(p, []int{1, 2}, nil, c.extra)
		if err != nil || countCombos(groups, maxPlanCombos) != c.want {
			t.Errorf("%s: %d combinations, %v; want %d", c.name, countCombos(groups, maxPlanCombos), err, c.want)
		}
	}
	_, err = variantCandidates(ref, p, []models.SimVariant{{Index: 99, Upgrade: 1}})
	if !errors.Is(err, ErrBadSimOptions) {
		t.Errorf("a variant of no item: err = %v, want ErrBadSimOptions", err)
	}
}

func TestCountCombos(t *testing.T) {
	one := func(n int) [][]models.SimSwap { return make([][]models.SimSwap, n) }
	tests := []struct {
		name   string
		groups [][][]models.SimSwap
		limit  int
		want   int
	}{
		{"nothing to change", [][][]models.SimSwap{one(1), one(1)}, 100, 0},
		{"the product less the baseline", [][][]models.SimSwap{one(2), one(3), one(1)}, 100, 5},
		{"stops at the limit", [][][]models.SimSwap{one(50), one(50)}, 100, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countCombos(tt.groups, tt.limit); got != tt.want {
				t.Errorf("countCombos = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPlanOfScalesWithTheCombinations(t *testing.T) {
	if p := planOf(0); p.Seconds != 0 || p.Max != maxCombos {
		t.Errorf("planOf(0) = %+v", p)
	}
	if a, b := planOf(10), planOf(100); b.Seconds <= a.Seconds || b.Combos != 100 {
		t.Errorf("planOf(10) = %+v, planOf(100) = %+v, want the longer run to cost more", a, b)
	}
}
