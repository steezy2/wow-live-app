package service

import (
	"context"
	"math"
	"testing"

	"wow-live/backend/models"
)

func TestStatSheet(t *testing.T) {
	for _, c := range []struct {
		name  string
		stats models.RankedStats
		want  healSheet
	}{
		{"nothing", models.RankedStats{}, healSheet{Crit: 5, Mastery: 8}},
		{"under the first step", models.RankedStats{Main: 3000, Crit: 460, Haste: 440, Mastery: 460, Vers: 540},
			healSheet{Int: 3000, Crit: 15, Haste: 10, Mastery: 18, Vers: 10}},
		// 35% of haste is 1540 rating: the 220 above 30% (1320) lose 10%, 22 rating, half a percent.
		{"past 30%", models.RankedStats{Haste: 1540}, healSheet{Crit: 5, Haste: 34.5, Mastery: 8}},
		// 45%: 10% of the 660 above 30% and of the 220 above 40%.
		{"past 40%", models.RankedStats{Haste: 1980}, healSheet{Crit: 5, Haste: 43, Mastery: 8}},
	} {
		got := sheetOf(c.stats)
		if math.Abs(got.Haste-c.want.Haste) > 1e-9 || got.Int != c.want.Int || got.Crit != c.want.Crit ||
			got.Mastery != c.want.Mastery || got.Vers != c.want.Vers {
			t.Errorf("%s: sheet = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestGearStatsAddsGemsEnchantsAndBase(t *testing.T) {
	ref := models.SimReference{Gems: []models.SimGem{{ID: 1, Stats: "23 Main stat"}, {ID: 2, Stats: "11 Vers & 4 Haste"}},
		Enchants: []models.SimEnchant{{ID: 3, Stats: "41 Int & 115 Sta"}}}
	gear := []*models.SimItem{{Slot: "head", ID: 10, Level: 272, Opts: map[string][]int{"gem_id": {1, 2}}},
		{Slot: "legs", ID: 11, Level: 272, Opts: map[string][]int{"enchant_id": {3}}}}
	got, items, err := gearStats(context.Background(), oneHanders{}, ref, gear)
	want := models.RankedStats{Main: baseIntellect + 200 + 23 + 41, Crit: 100, Haste: 104, Mastery: 100, Vers: 111}
	if err != nil || got != want || len(items) != 2 {
		t.Errorf("gear stats = %+v, %d items (%v); want %+v", got, len(items), err, want)
	}
}
