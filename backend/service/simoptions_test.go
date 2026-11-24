package service

import (
	"errors"
	"slices"
	"strconv"
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
		Sockets:     map[int]int{9: 1, 10397: 1},
		SocketSlots: map[string]int{"head": 1, "wrist": 1, "waist": 1},
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
		slot     string
		variant  models.SimVariant
		wantLine string // the item's simc line
		wantNote string
		wantLvl  int
		wantErr  string
	}{
		{"an upgrade swaps the track's bonus", "finger1", models.SimVariant{Upgrade: 3},
			"finger1=,id=5,bonus_id=7/102\n", "Myth 3/3", 324, ""},
		{"a socket and a gem", "head", models.SimVariant{Socket: true, Gem: 240883},
			"head=,id=5,bonus_id=7/100/10397,gem_id=240883\n", "socket, Quick Lapis", 318, ""},
		{"an enchant", "finger1", models.SimVariant{Enchant: 7964},
			"finger1=,id=5,bonus_id=7/100,enchant_id=7964\n", "Amani Mastery", 318, ""},
		{"nothing changed", "finger1", models.SimVariant{}, "", "", 0, "changes something"},
		{"an upgrade past the track", "finger1", models.SimVariant{Upgrade: 4}, "", "", 0, "no upgrade level 4"},
		{"an unknown gem", "head", models.SimVariant{Socket: true, Gem: 1}, "", "", 0, "no gem 1"},
		{"an enchant for another slot", "head", models.SimVariant{Enchant: 7964}, "", "", 0, "no enchant"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := ring()
			item.Slot = tt.slot
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

func TestSocketRules(t *testing.T) {
	item := func(slot string, opts map[string][]int) models.SimItem { return models.SimItem{Slot: slot, Opts: opts} }
	bare, socketed := map[string][]int{"bonus_id": {7}}, map[string][]int{"bonus_id": {7, 9}}
	tests := []struct {
		name    string
		item    models.SimItem
		sockets int
		canAdd  bool
	}{
		{"a bare head takes one", item("head", bare), 0, true},
		{"a wrist takes one", item("wrist", bare), 0, true},
		{"a head with its socket takes no more", item("head", socketed), 1, false},
		{"a chest takes none", item("chest", bare), 0, false},
		{"a ring takes none", item("finger1", bare), 0, false},
		{"a neck takes none", item("neck", bare), 0, false},
		{"a trinket takes none", item("trinket1", bare), 0, false},
		{"a weapon takes none", item("main_hand", bare), 0, false},
		{"a gem counts as a socket", item("chest", map[string][]int{"gem_id": {240883}}), 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := socketsOf(testRef(), tt.item); got != tt.sockets {
				t.Errorf("sockets = %d, want %d", got, tt.sockets)
			}
			if got := canAddSocket(testRef(), tt.item); got != tt.canAdd {
				t.Errorf("canAddSocket = %v, want %v", got, tt.canAdd)
			}
			_, err := applyVariant(testRef(), tt.item, models.SimVariant{Socket: true})
			if (err == nil) != tt.canAdd {
				t.Errorf("a socket variant: err = %v, want it to work only when a socket can be added", err)
			}
		})
	}
	if _, err := applyVariant(testRef(), item("head", bare), models.SimVariant{Gem: 240883}); !errors.Is(err,
		ErrBadSimOptions) {
		t.Errorf("a gem in an item with no socket: err = %v, want ErrBadSimOptions", err)
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
	got, err := consumableLines(testRef(),
		models.SimConsumables{"flask": {"flask_b_2"}, "weapon": {"oil_2"}, "food": {"feast"}})
	want := "flask=flask_b_2\nfood=feast\ntemporary_enchant=main_hand:oil_2\n"
	if err != nil || got != want {
		t.Errorf("consumableLines = %q, %v, want %q", got, err, want)
	}
	compared := models.SimConsumables{"flask": {"flask_a_2", "flask_b_2"}, "food": {}}
	if got, err := consumableLines(testRef(), compared); err != nil || got != "" {
		t.Errorf("kinds compared or empty = %q, %v, want simc's defaults (nothing written)", got, err)
	}
	for _, bad := range []models.SimConsumables{{"flask": {"feast"}}, {"potion": {"x\nthreads=1"}}, {"ring": {"feast"}},
		{"flask": {"flask_a_2", "flask_a_2"}}} {
		if _, err := consumableLines(testRef(), bad); !errors.Is(err, ErrBadSimOptions) {
			t.Errorf("%+v: err = %v, want ErrBadSimOptions", bad, err)
		}
	}
}

// dimsOf are the consumable kinds picked with two or more values, failing the test for a bad pick.
func dimsOf(t *testing.T, ref models.SimReference, picked models.SimConsumables) []compareDim {
	t.Helper()
	dims, err := consumableDims(ref, picked)
	if err != nil {
		t.Fatal(err)
	}
	return dims
}

func TestCompareCombos(t *testing.T) {
	ref := testRef()
	ref.Consumables["food"] = append(ref.Consumables["food"], models.SimConsumable{Value: "snack", Name: "Snack"})
	combos, err := compareCombos(dimsOf(t, ref, models.SimConsumables{"flask": {"flask_b_2", "flask_a_2"},
		"food": {"feast", "snack"}, "weapon": {"oil_2"}}))
	if err != nil || len(combos) != 4 {
		t.Fatalf("compareCombos = %v, %v", combos, err)
	}
	want := "profileset.option1+=flask=flask_b_2\nprofileset.option1+=food=feast\n" +
		"profileset.option2+=flask=flask_b_2\nprofileset.option2+=food=snack\n" +
		"profileset.option3+=flask=flask_a_2\nprofileset.option3+=food=feast\n" +
		"profileset.option4+=flask=flask_a_2\nprofileset.option4+=food=snack\n"
	if got := gearProfilesets(combos); got != want {
		t.Errorf("profilesets = %q, want every flask with every food and the single oil left to the baseline", got)
	}
	if got := combos[1].option; got.Name != "Flask B + Snack" || got.Value != "flask_b_2+snack" {
		t.Errorf("second combination = %+v, want Flask B with the snack", got)
	}
	if got, err := compareCombos(dimsOf(t, ref, models.SimConsumables{"flask": {"flask_a_2"}})); err != nil || got != nil {
		t.Errorf("one value is used as it is, not compared: %v, %v", got, err)
	}
}

func TestCompareCombosRefuse(t *testing.T) {
	ref := testRef()
	var many []string
	for i := range maxCompareValues + 1 {
		v := "flask_" + strconv.Itoa(i)
		ref.Consumables["flask"] = append(ref.Consumables["flask"], models.SimConsumable{Value: v, Name: v})
		many = append(many, v)
	}
	if _, err := compareCombos(dimsOf(t, ref, models.SimConsumables{"flask": many})); !errors.Is(err, ErrBadSimOptions) {
		t.Errorf("too many combinations: err = %v, want ErrBadSimOptions", err)
	}
	for name, picked := range map[string]models.SimConsumables{
		"an unknown kind":         {"ring": {"a", "b"}},
		"a value twice":           {"flask": {"flask_a_2", "flask_b_2", "flask_a_2"}},
		"a value of another kind": {"flask": {"flask_a_2", "feast"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := consumableDims(ref, picked); !errors.Is(err, ErrBadSimOptions) {
				t.Errorf("err = %v, want ErrBadSimOptions", err)
			}
		})
	}
}

func TestWithCompareRunsEachGearComboOncePerValue(t *testing.T) {
	picked := models.SimConsumables{"flask": {"flask_b_2", "flask_a_2"}}
	gear := []gearCombo{{name: "drop1", source: "Raid"}, {name: "drop2"}}
	dims := dimsOf(t, testRef(), picked)
	combos, err := withCompare(dims, false, gear, 4)
	if err != nil || len(combos) != 4 {
		t.Fatalf("withCompare = %v, %v", combos, err)
	}
	want := "profileset.drop1xoption1+=flask=flask_b_2\nprofileset.drop1xoption2+=flask=flask_a_2\n" +
		"profileset.drop2xoption1+=flask=flask_b_2\nprofileset.drop2xoption2+=flask=flask_a_2\n"
	if got := gearProfilesets(combos); got != want {
		t.Errorf("profilesets = %q", got)
	}
	if c := combos[1]; c.consumable != "Flask A" || c.source != "Raid" || c.option != nil {
		t.Errorf("second combo = %+v, want Raid's drop with Flask A", c)
	}
	if _, err := withCompare(dims, false, gear, 3); !errors.Is(err, ErrBadSimOptions) {
		t.Errorf("over the limit: err = %v, want ErrBadSimOptions", err)
	}
	quick, err := withCompare(dims, true, nil, 0)
	if err != nil || len(quick) != 2 || quick[0].option == nil {
		t.Errorf("a quick sim compares the values alone: %v, %v", quick, err)
	}
	if same, err := withCompare(nil, false, gear, 4); err != nil ||
		len(same) != 2 {
		t.Errorf("nothing compared keeps the gear as it is: %v, %v", same, err)
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
		name      string
		requested *string
		want      string
		wantErr   bool
	}{
		{"the export's when none is sent", nil, "11:1/21:1", false},
		{"none when an empty one is sent", new(""), "", false},
		{"a pick is kept in the tree's order", new("21:1/12:1"), "12:1/21:1", false},
		{"an entry of no node", new("99:1"), "", true},
		{"not a pair", new("11"), "", true},
		{"a rank other than 1", new("11:2"), "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, dim, err := omniumInput(testRef(), p, tt.requested)
			if dim != nil {
				t.Errorf("omniumInput compares %v, want no comparison for one pick to a node", dim)
			}
			if (err != nil) != tt.wantErr || got != tt.want || (err != nil && !errors.Is(err, ErrBadSimOptions)) {
				t.Errorf("omniumInput = %q, %v, want %q (error %v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestOmniumInputComparesSeveralPicksOfANode(t *testing.T) {
	p := models.SimProfile{Omnium: "11:1/21:1"}
	got, dim, err := omniumInput(testRef(), p, new("21:1/11:1/12:1"))
	if err != nil || got != "11:1/21:1" || len(dim) != 2 {
		t.Fatalf("omniumInput = %q, %v, %v, want the first pick of the node and two values", got, dim, err)
	}
	if dim[0].line != "omnium_talents=11:1/21:1\n" || dim[1].line != "omnium_talents=12:1/21:1\n" {
		t.Errorf("lines = %q, %q", dim[0].line, dim[1].line)
	}
	if _, _, err := omniumInput(testRef(), p, new("11:1/11:1")); !errors.Is(err, ErrBadSimOptions) {
		t.Errorf("a rune twice: err = %v, want ErrBadSimOptions", err)
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
		groups, err := gearGroups(p, []int{1, 2}, nil, c.extra, twoHanders(t))
		if err != nil || comboCount(groups, maxPlanCombos) != c.want {
			t.Errorf("%s: %d combinations, %v; want %d", c.name, comboCount(groups, maxPlanCombos), err, c.want)
		}
	}
	_, err = variantCandidates(ref, p, []models.SimVariant{{Index: 99, Upgrade: 1}})
	if !errors.Is(err, ErrBadSimOptions) {
		t.Errorf("a variant of no item: err = %v, want ErrBadSimOptions", err)
	}
}

func TestPlanTimesEveryKindOfSim(t *testing.T) {
	s := newTestSims(t, "ok")
	s.Ref = testRef()
	req := simRequest(t)
	quick, err := s.Plan(req)
	if err != nil || quick.Combos != 0 || quick.Seconds == 0 || quick.Error != "" {
		t.Fatalf("quick plan = %+v, %v; want a time and no combinations", quick, err)
	}
	req.Consumables = models.SimConsumables{"flask": {"flask_a_2", "flask_b_2"}}
	compared, err := s.Plan(req)
	if err != nil || compared.Combos != 2 || compared.Seconds <= quick.Seconds {
		t.Errorf("compared plan = %+v, %v; want 2 combinations and a longer time than %d s", compared, err, quick.Seconds)
	}
	req.Kind = "stats"
	if plan, err := s.Plan(req); err != nil || plan.Error == "" {
		t.Errorf("stats plan with a comparison = %+v, %v; want Start's refusal", plan, err)
	}
	req.Consumables = nil
	stats, err := s.Plan(req)
	if err != nil || stats.Seconds <= quick.Seconds || stats.Error != "" {
		t.Errorf("stats plan = %+v, %v; want a longer time than a quick sim's %d s", stats, err, quick.Seconds)
	}
}

func TestPlanRefusesInStartsWords(t *testing.T) {
	s := newTestSims(t, "ok")
	req := simRequest(t)
	req.Kind, req.Equipped, req.Bags = "topgear", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, []int{0, 1}
	if plan, err := s.Plan(req); err != nil || plan.Combos != 5 || plan.Error != "" || plan.Seconds == 0 {
		t.Errorf("plan = %+v, %v; want 5 combinations that run", plan, err)
	}
	req.Bags = nil
	plan, err := s.Plan(req)
	_, startErr := s.Start(req)
	if err != nil || plan.Combos != 0 || startErr == nil || plan.Error != startErr.Error() {
		t.Errorf("plan = %+v, %v; want Start's refusal %v", plan, err, startErr)
	}
}

// craftedRef is testRef with ring() as a crafted item: two embellishments (the first on it) and two recraft pairs.
func craftedRef() models.SimReference {
	ref := testRef()
	ref.Crafting = models.SimCrafting{
		Items: map[int]models.CraftedItem{5: {Name: "Band", Reagents: []int{1, 2, 3, 4}}},
		Reagents: map[int]models.CraftingReagent{
			1: {Kind: models.ReagentEmbellishment, Name: "Charm", BonusIDs: []int{50, 60}},
			2: {Kind: models.ReagentEmbellishment, Name: "Binding", BonusIDs: []int{51, 60}},
			3: {Kind: models.ReagentRecraft, Name: "Missive A", BonusIDs: []int{8790}, Stats: []int{32, 36}},
			4: {Kind: models.ReagentRecraft, Name: "Missive B", BonusIDs: []int{8795}, Stats: []int{32, 40}},
		},
	}
	return ref
}

func TestApplyVariantCraftingAndRemoval(t *testing.T) {
	crafted := func() models.SimItem {
		item := ring()
		item.Opts = map[string][]int{"bonus_id": {7, 50, 60, 8795, 10397}, "gem_id": {240883}, "enchant_id": {7964},
			"crafted_stats": {32, 40}}
		return item
	}
	tests := []struct {
		name     string
		item     func() models.SimItem
		variant  models.SimVariant
		wantLine string
		wantNote string
		wantErr  string
	}{
		{"another embellishment", crafted, models.SimVariant{Embellishment: 2},
			"finger1=,id=5,bonus_id=7/8795/10397/51/60,crafted_stats=32/40,enchant_id=7964,gem_id=240883\n", "Binding", ""},
		{"no embellishment", crafted, models.SimVariant{Remove: "embellishment"},
			"finger1=,id=5,bonus_id=7/8795/10397,crafted_stats=32/40,enchant_id=7964,gem_id=240883\n", "no embellishment", ""},
		{"a recraft", crafted, models.SimVariant{Recraft: 3},
			"finger1=,id=5,bonus_id=7/50/60/10397/8790,crafted_stats=32/36,enchant_id=7964,gem_id=240883\n",
			"recraft Crit / Haste", ""},
		{"the embellishment it has", crafted, models.SimVariant{Embellishment: 1}, "", "", "can't take"},
		{"the recraft it has", crafted, models.SimVariant{Recraft: 4}, "", "", "can't take recraft"},
		{"a reagent of another recipe", crafted, models.SimVariant{Embellishment: 99}, "", "", "can't take"},
		{"an item that isn't crafted", func() models.SimItem { i := crafted(); i.ID = 6; return i },
			models.SimVariant{Embellishment: 2}, "", "", "can't take"},
		{"gems off", crafted, models.SimVariant{Remove: "gems"},
			"finger1=,id=5,bonus_id=7/50/60/8795/10397,crafted_stats=32/40,enchant_id=7964\n", "no gems", ""},
		{"the socket off takes its gem", crafted, models.SimVariant{Remove: "socket"},
			"finger1=,id=5,bonus_id=7/50/60/8795,crafted_stats=32/40,enchant_id=7964\n", "no socket", ""},
		{"nothing to remove", ring, models.SimVariant{Remove: "enchant"}, "", "", "no enchant to remove"},
		{"something else", crafted, models.SimVariant{Remove: "item"}, "", "", "remove one of"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyVariant(craftedRef(), tt.item(), tt.variant)
			if tt.wantErr != "" {
				if !errors.Is(err, ErrBadSimOptions) || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want ErrBadSimOptions with %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if line := SimItemLine(got); line != tt.wantLine || got.Note != tt.wantNote {
				t.Errorf("got %q %q, want %q %q", line, got.Note, tt.wantLine, tt.wantNote)
			}
		})
	}
}

func TestParseProfileSaysWhatACraftedItemCanChange(t *testing.T) {
	item := ring()
	item.Opts = map[string][]int{"bonus_id": {7, 50, 60, 8795, 10397}, "gem_id": {240883}}
	got := withChoices(craftedRef(), models.SimProfile{Equipped: []models.SimItem{item}}).Equipped[0]
	if len(got.Embellishments) != 1 || got.Embellishments[0].Name != "Binding" || len(got.Recrafts) != 1 ||
		got.Recrafts[0].Name != "Crit / Haste" {
		t.Errorf("choices %+v %+v, want the other embellishment and the other recraft", got.Embellishments, got.Recrafts)
	}
	if !slices.Equal(got.Removable, []string{"gems", "socket", "embellishment"}) {
		t.Errorf("removable %v, want gems, socket and the embellishment", got.Removable)
	}
}

// catalystRef is testRef with one Catalyst conversion group: a mage head and a mage chest.
func catalystRef() models.SimReference {
	ref := testRef()
	ref.Crafting.Conversions = map[int]models.ItemConversion{13: {BonusIDs: []int{13662}, Items: []models.CatalystItem{
		{ID: 900, Name: "Tier Crown", Class: 8, InventoryType: 1}, {ID: 901, Name: "Tier Robe", Class: 8, InventoryType: 20},
		{ID: 902, Name: "Warrior Helm", Class: 1, InventoryType: 1}}}}
	return ref
}

func TestCatalystConvertsOnlyWhatTheGroupCoversForTheClass(t *testing.T) {
	item := func(slot string, bonus ...int) models.SimItem {
		return models.SimItem{Slot: slot, ID: 5, Name: "Old", Opts: map[string][]int{"bonus_id": bonus}}
	}
	tests := []struct {
		name  string
		item  models.SimItem
		class string
		want  int // the converted item's ID, 0 for none
	}{
		{"a head of the group", item("head", 7, 13662), "mage", 900},
		{"a chest takes either chest type", item("chest", 13662), "mage", 901},
		{"another class's item", item("head", 13662), "warrior", 902},
		{"not of the group", item("head", 7), "mage", 0},
		{"a slot the Catalyst leaves alone", item("trinket1", 13662), "mage", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := converted(catalystRef(), tt.class, tt.item)
			if tt.want == 0 {
				if !errors.Is(err, ErrBadSimOptions) {
					t.Errorf("err = %v, want ErrBadSimOptions", err)
				}
				return
			}
			same := slices.Equal(got.Opts["bonus_id"], tt.item.Opts["bonus_id"])
			if err != nil || got.ID != tt.want || !got.Catalyst || !same {
				t.Errorf("got %+v (%v), want item %d, converted, with its bonus IDs kept", got, err, tt.want)
			}
		})
	}
}

func TestCatalystChargesLimitConversionsInACombination(t *testing.T) {
	p := gearProfile(false)
	p.Equipped = append(p.Equipped, gearItem("chest", 51))
	conv := func(i int) gearCandidate {
		item := p.Equipped[i]
		item.ID, item.Catalyst = item.ID+1000, true
		return gearCandidate{item: item}
	}
	extra := []gearCandidate{conv(0), conv(len(p.Equipped) - 1)} // the head and the chest
	picked := []int{0, len(p.Equipped) - 1}
	for charges, want := range map[int]int{0: 0, 1: 2, 2: 3} {
		combos, err := topGearCombos(gearRules{catalystCharges: charges}, p, picked, nil, extra, true, twoHanders(t))
		if err != nil || len(combos) != want {
			t.Errorf("%d charges: %d combinations (%v), want %d", charges, len(combos), err, want)
		}
	}
	req := models.SimRequest{Variants: []models.SimVariant{{Worn: true, Index: 0, Catalyst: true}}}
	if _, err := topGearExtras(catalystRef(), p, req); !errors.Is(err, ErrBadSimOptions) {
		t.Errorf("a conversion with no charges: err = %v, want ErrBadSimOptions", err)
	}
}

func TestRemoveSocketTakesOnlyTheAddedOneAndItsGem(t *testing.T) {
	item := func(bonus []int, gems ...int) models.SimItem {
		return models.SimItem{Slot: "head", ID: 5, Opts: map[string][]int{"bonus_id": bonus, "gem_id": gems}}
	}
	tests := []struct {
		name string
		item models.SimItem
		want []int // the gems left; nil for none
		err  bool
	}{
		{"its only socket was the added one", item([]int{7, 10397}, 1), nil, false},
		{"one of two sockets", item([]int{7, 9, 10397}, 1, 2), []int{1}, false},
		{"the added one is empty", item([]int{7, 9, 10397}, 1), []int{1}, false},
		{"an item's own socket stays", item([]int{7, 9}, 1), nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := removeFrom(testRef(), models.SimItem{Name: "Hat", Opts: cloneOpts(tt.item.Opts)}, "socket")
			if (err != nil) != tt.err {
				t.Fatalf("err = %v, want error %v", err, tt.err)
			}
			if err == nil && (!slices.Equal(got.Opts["gem_id"], tt.want) || slices.Contains(got.Opts["bonus_id"], 10397)) {
				t.Errorf("got %v and gems %v, want the added socket gone and gems %v", got.Opts["bonus_id"],
					got.Opts["gem_id"], tt.want)
			}
		})
	}
}
