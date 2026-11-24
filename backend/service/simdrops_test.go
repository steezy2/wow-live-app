package service

import (
	"encoding/json"
	"errors"
	"os"
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
			Items: []models.DropItem{{ID: 1, Name: "Ring", Slot: "finger"},
				{ID: 2, Name: "Chest", Slot: "chest", Specs: []int{2}},
				{ID: 3, Name: "Staff", Slot: "two_hand", Specs: []int{1}}, {ID: 4, Name: "Wand", Slot: "one_hand"}}}}}},
		Dungeons: []models.DropSource{{Name: "Dungeon", Bosses: []models.DropBoss{{Name: "Last",
			Items: []models.DropItem{{ID: 5, Name: "Trinket", Slot: "trinket", Specs: []int{1}}}}}}},
		Keys: []models.KeyLevels{{Level: 0, Loot: 90}, {Level: 10, Loot: 105}},
	}
}

func TestDropCombosFollowTheSpecAndDifficulty(t *testing.T) {
	p := gearProfile(false)
	p.Class, p.Spec = "shaman", "elemental"
	tests := []struct {
		name       string
		difficulty string
		key        int
		ring2      int      // the worn second ring's ID
		want       []string // each combo's slots and the item level of what goes in
	}{
		{"heroic raid and a +10", "heroic", 10, 12,
			[]string{"finger1:120", "finger2:120", "main_hand:120", "main_hand:120", "trinket1:105", "trinket2:105"}},
		{"lfr raid and mythic 0", "lfr", 0, 12,
			[]string{"finger1:100", "finger2:100", "main_hand:100", "main_hand:100", "trinket1:90", "trinket2:90"}},
		{"a ring already worn in the other slot is unique-equipped", "heroic", 10, 1,
			[]string{"finger2:120", "main_hand:120", "main_hand:120", "trinket1:105", "trinket2:105"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.Equipped[2].ID = tt.ring2
			combos, err := dropCombos(p, testDrops(), tt.difficulty, tt.key, nil)
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

func TestMaxedLevel(t *testing.T) {
	track := func(group int, name string, levels ...int) models.SimUpgradeTrack {
		t := models.SimUpgradeTrack{Group: group, Name: name}
		for i, l := range levels {
			t.Levels = append(t.Levels, models.SimUpgradeLevel{Level: i + 1, ItemLevel: l})
		}
		return t
	}
	tracks := []models.SimUpgradeTrack{track(615, "Veteran", 279, 282, 285, 289, 292, 295),
		track(616, "Champion", 292, 295, 298, 302, 305, 308), track(618, "Myth", 318, 321, 324, 328, 331, 334)}
	for level, want := range map[int]struct {
		level int
		track string
	}{279: {295, "Veteran"}, 292: {308, "Champion"}, 295: {308, "Champion"}, 318: {334, "Myth"}, 334: {334, "Myth"},
		344: {344, ""}, 100: {100, ""}} {
		if got, track := maxedLevel(tracks, level); got != want.level || track != want.track {
			t.Errorf("maxedLevel(%d) = %d, %q, want %d, %q", level, got, track, want.level, want.track)
		}
	}
	if got, _ := maxedLevel(nil, 292); got != 292 {
		t.Errorf("no tracks: %d, want the level kept", got)
	}
}

func TestDropCombosUpgradeToTheTop(t *testing.T) {
	p := gearProfile(false)
	p.Class, p.Spec = "shaman", "elemental"
	tracks := []models.SimUpgradeTrack{{Group: 1, Name: "Hero",
		Levels: []models.SimUpgradeLevel{{Level: 1, ItemLevel: 120}, {Level: 2, ItemLevel: 130}}}}
	combos, err := dropCombos(p, testDrops(), "heroic", 0, tracks)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range combos {
		in := c.swaps[0].In
		want := map[string]int{"Ring": 130, // the top rank
			"Staff": 130 + venomstoneLevels, "Wand": 130 + venomstoneLevels, // a Hero weapon takes Venomstones
			"Trinket": 90, // the dungeon's level is on no track
		}[in.Name]
		if in.Level != want || in.Opts["ilevel"][0] != want {
			t.Errorf("%s: level %d, ilevel %v, want %d", in.Name, in.Level, in.Opts["ilevel"], want)
		}
	}
}

func TestDropCombosKeepTheWornEnchant(t *testing.T) {
	p := gearProfile(false)
	p.Class, p.Spec = "shaman", "elemental"
	p.Equipped[1].Opts = map[string][]int{"enchant_id": {7}} // finger1
	combos, err := dropCombos(p, testDrops(), "heroic", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range combos {
		in := c.swaps[0].In
		if got, want := in.Opts["enchant_id"], c.swaps[0].Slot == "finger1"; (got != nil) != want {
			t.Errorf("%s drop into %s has enchant %v, want one only in finger1", in.Name, c.swaps[0].Slot, got)
		}
	}
}

func TestDropCombosEmptyTheOffHandForATwoHander(t *testing.T) {
	p := gearProfile(false) // wears an off-hand
	p.Class, p.Spec = "shaman", "elemental"
	combos, err := dropCombos(p, testDrops(), "normal", 0, nil)
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
		if _, err := dropCombos(tc.p, tc.d, tc.difficulty, tc.key, nil); !errors.Is(err, ErrBadSimOptions) {
			t.Errorf("%s: err %v, want ErrBadSimOptions", name, err)
		}
	}
}

func TestGearResultsKeepARingsBetterSlot(t *testing.T) {
	p := gearProfile(false)
	p.Class, p.Spec = "shaman", "elemental"
	combos, _ := dropCombos(p, testDrops(), "heroic", 0, nil)
	sets := []simSetJSON{{Name: "drop1", Mean: 90}, {Name: "drop2", Mean: 110}, {Name: "drop3", Mean: 100}}
	rows := gearResults(sets, 100, combos)
	if len(rows) != 2 || rows[0].Swaps[0].Slot != "finger2" || rows[0].Gain != 10 || rows[0].Source != "Raid" ||
		rows[0].Boss != "Boss" {
		t.Errorf("rows %+v: want the ring's better slot once, then the other drop", rows)
	}
}

func TestDropCombosSkipAnOffHandBesideATwoHander(t *testing.T) {
	d := testDrops()
	d.Raids[0].Bosses[0].Items = []models.DropItem{{ID: 6, Name: "Shield", Slot: "off_hand"}}
	p := gearProfile(true) // wears a two-hander
	p.Class, p.Spec = "shaman", "elemental"
	combos, err := dropCombos(p, d, "normal", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range combos {
		if c.swaps[0].In.ID == 6 {
			t.Errorf("combo %s puts an off-hand beside a two-hander", c.name)
		}
	}
}

func TestSimDropsNameClassesTheWayTheExportDoes(t *testing.T) {
	raw, err := os.ReadFile("../../reference/sim-drops.json")
	if err != nil {
		t.Fatal(err)
	}
	var d models.SimDrops
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	for id, s := range d.Specs {
		if !slices.Contains(simClasses, s.Class) {
			t.Errorf("spec %d: class %q is not one ParseSimC accepts", id, s.Class)
		}
	}
}
