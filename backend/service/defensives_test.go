package service

import (
	"testing"

	"wow-live/backend/models"
)

func TestClassifyDeathsSaysWhetherThePlayerUsedTheirCover(t *testing.T) {
	for _, tc := range []struct {
		name          string
		casts         []models.Cast
		buff          string
		wantDefensive string
		wantUsed      bool
	}{
		{"cast just before", []models.Cast{{Player: "A", Ability: "Ice Cold", At: 95_000}}, "", "Ice Cold", true},
		{"their own buff, cast before the window", []models.Cast{{Player: "A", Ability: "Ice Cold", At: 80_000}},
			"Ice Cold", "Ice Cold", true},
		{"an external on them", nil, "Pain Suppression", "Pain Suppression", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buffs []models.BuffUp
			if tc.buff != "" {
				buffs = []models.BuffUp{{Player: "A", Ability: tc.buff, Start: 80_000, End: 110_000}}
			}
			deaths := []models.Death{{Player: "A", At: 100_000}}
			ClassifyDeaths(deaths, tc.casts, buffs, LearnKit(tc.casts), 10_000)
			if d := deaths[0]; d.Defensive != tc.wantDefensive || d.Used != tc.wantUsed {
				t.Fatalf("defensive %q, used %v; want %q, %v", d.Defensive, d.Used, tc.wantDefensive, tc.wantUsed)
			}
		})
	}
}
