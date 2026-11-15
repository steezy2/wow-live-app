package service

import (
	"errors"
	"slices"
	"strconv"
	"testing"

	"wow-live/backend/models"
)

// testDrops is a raid boss dropping a ring (for any spec), a plate chest (for spec 1 only), a two-hander and a
// one-hander, and a dungeon boss dropping a trinket.
func testDrops() models.SimDrops {
	return models.SimDrops{
		Specs: map[int]models.SimSpec{1: {Class: "shaman", Spec: "elemental"}, 2: {Class: "warrior", Spec: "arms"}},
		Raids: []models.DropSource{{Name: "Raid", Bosses: []models.DropBoss{{Name: "Boss",
			Levels: &models.DifficultyLevels{LFR: 100, Normal: 110, Heroic: 120, Mythic: 130},
			Items: []models.DropItem{{ID: 1, Name: "Ring", Slot: "finger"}, {ID: 2, Name: "Chest", Slot: "chest", Specs: []int{2}},
				{ID: 3, Name: "Staff", Slot: "two_hand", Specs: []int{1}}, {ID: 4, Name: "Wand", Slot: "one_hand"}}}}}},
		Dungeons: []models.DropSource{{Name: "Dungeon", Bosses: []models.DropBoss{{Name: "Last",
			Items: []models.DropItem{{ID: 5, Name: "Trinket", Slot: "trinket", Specs: []int{1}}}}}}},
		Keys: []models.KeyLevels{{Level: 0, Loot: 90, Vault: 95}, {Level: 10, Loot: 105, Vault: 110}},
	}
}

func TestDropCombosFollowTheSpecAndDifficulty(t *testing.T) {
	p := gearProfile(false)
	p.Class, p.Spec = "shaman", "elemental"
	tests := []struct {
		name       string
		difficulty string
		key        int
		want       []string // each combo's slots and the item level of what goes in
	}{
		{"heroic raid and a +10", "heroic", 10, []string{"finger1:120", "finger2:120", "main_hand:120", "main_hand:120", "trinket1:105", "trinket2:105"}},
		{"lfr raid and mythic 0", "lfr", 0, []string{"finger1:100", "finger2:100", "main_hand:100", "main_hand:100", "trinket1:90", "trinket2:90"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			combos, err := dropCombos(p, testDrops(), tt.difficulty, tt.key)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, c := range combos {
				got = append(got, c.swaps[0].Slot+":"+strconv.Itoa(c.swaps[0].In.Level))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v (the plate chest is for another spec)", got, tt.want)
			}
		})
	}
}

func TestDropCombosEmptyTheOffHandForATwoHander(t *testing.T) {
	p := gearProfile(false) // wears an off-hand
	p.Class, p.Spec = "shaman", "elemental"
	combos, err := dropCombos(p, testDrops(), "normal", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range combos {
		two := c.swaps[0].In.ID == 3
		if two != (len(c.swaps) == 2 && c.swaps[1].Slot == "off_hand" && c.swaps[1].In == nil) {
			t.Errorf("combo %s swaps %+v: the two-hander alone empties the off-hand", c.name, c.swaps)
		}
	}
}

func TestDropCombosRefuse(t *testing.T) {
	p := gearProfile(false)
	p.Class, p.Spec = "shaman", "elemental"
	big := testDrops()
	for range maxDrops {
		big.Raids[0].Bosses[0].Items = append(big.Raids[0].Bosses[0].Items, models.DropItem{ID: 9, Slot: "head"})
	}
	other := p
	other.Spec = "enhancement"
	for name, tc := range map[string]struct {
		p          models.SimProfile
		d          models.SimDrops
		difficulty string
		key        int
	}{
		"an unknown spec":       {other, testDrops(), "heroic", 0},
		"an unknown difficulty": {p, testDrops(), "normal+", 0},
		"an unknown key level":  {p, testDrops(), "heroic", 7},
		"more than maxDrops":    {p, big, "heroic", 0},
		"nothing that can drop": {p, models.SimDrops{Specs: testDrops().Specs, Keys: testDrops().Keys}, "heroic", 0},
	} {
		if _, err := dropCombos(tc.p, tc.d, tc.difficulty, tc.key); !errors.Is(err, ErrBadSimOptions) {
			t.Errorf("%s: err %v, want ErrBadSimOptions", name, err)
		}
	}
}

func TestGearResultsKeepARingsBetterSlot(t *testing.T) {
	p := gearProfile(false)
	p.Class, p.Spec = "shaman", "elemental"
	combos, _ := dropCombos(p, testDrops(), "heroic", 0)
	sets := []simSetJSON{{Name: "drop1", Mean: 90}, {Name: "drop2", Mean: 110}, {Name: "drop3", Mean: 100}}
	rows := gearResults(sets, 100, combos)
	if len(rows) != 2 || rows[0].Swaps[0].Slot != "finger2" || rows[0].Gain != 10 || rows[0].Source != "Raid" || rows[0].Boss != "Boss" {
		t.Errorf("rows %+v: want the ring's better slot once, then the other drop", rows)
	}
}
