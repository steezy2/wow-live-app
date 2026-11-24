package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wow-live/backend/clients/raidbots"
	"wow-live/backend/models"
)

func TestBuildReadsTheSeasonsCraftedItems(t *testing.T) {
	got, err := build(func(name string) (io.ReadCloser, error) { return os.Open(filepath.Join("testdata", name)) })
	if err != nil {
		t.Fatal(err)
	}
	want := models.SimCrafting{Build: "12.0.7.66666", Source: raidbots.Source,
		// The tool, the cloak with no reagent the sim uses and the older expansion's ring are left out; each kept
		// reagent stands for its other qualities (and the older expansion's missive with the same bonus).
		Items: map[int]models.CraftedItem{245769: {Name: "Aln'hara Lantern",
			Reagents: []int{244604, 245792, 248130}}},
		Reagents: map[int]models.CraftingReagent{
			244604: {Kind: models.ReagentEmbellishment, Name: "Blessed Pango Charm", Icon: "charm",
				BonusIDs: []int{12686, 8960}},
			245792: {Kind: models.ReagentRecraft, Name: "Thalassian Missive of the Quickblade", Icon: "missive",
				BonusIDs: []int{8795}, Stats: []int{32, 40}},
			248130: {Kind: models.ReagentEmbellishment, Name: "Lucky Keychain", Icon: "trade_engineering",
				BonusIDs: []int{12715}},
		},
		Conversions: map[int]models.ItemConversion{13: {BonusIDs: []int{13662}, Items: []models.CatalystItem{
			{ID: 271469, Name: "Baleful Grave-Knight's Greatcloak", Class: 6, InventoryType: 16},
			{ID: 271477, Name: "Baleful Grave-Knight's Breastplate", Class: 6, InventoryType: 5}}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("build =\n%+v\nwant\n%+v", got, want)
	}
}
