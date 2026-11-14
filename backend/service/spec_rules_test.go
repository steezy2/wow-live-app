package service

import (
	"testing"

	"wow-live/backend/models"
)

func TestFailedRules(t *testing.T) {
	cast := func(at int64, spell string) models.TrackCast { return models.TrackCast{ClockMS: at, Spell: spell} }
	casts := []models.TrackCast{cast(1000, "Barrage"), cast(5000, "Barrage"), cast(9000, "Barrage"), cast(2000, "Surge"),
		cast(30_000, "Surge"), cast(3000, "Touch"), {ClockMS: 31_000, Spell: "Barrage", Cut: true}}
	buffs := []models.BuffSpan{{Name: "Soul", FromMS: 0, ToMS: 6000}, {Name: "Tempo", FromMS: 0, ToMS: 40_000}}
	const fight = 100_000
	tests := []struct {
		name string
		rule models.SpecRule
		want []models.RuleResult
	}{
		{"with: 2 of 3 casts in the buff", models.SpecRule{Kind: "with", Cast: "Barrage", Buff: "Soul", AtLeast: 0.9, Why: "free"},
			[]models.RuleResult{{Text: "Barrage while Soul was up", Share: 2.0 / 3, AtLeast: 0.9, Why: "free"}}},
		{"with: enough", models.SpecRule{Kind: "with", Cast: "Barrage", Buff: "Soul", AtLeast: 0.6}, []models.RuleResult{}},
		{"together: 1 of 2 Surges within 3 s of Touch", models.SpecRule{Kind: "together", Cast: "Surge", Other: "Touch", Seconds: 3, AtLeast: 1},
			[]models.RuleResult{{Text: "Surge within 3 s of Touch", Share: 0.5, AtLeast: 1}}},
		{"uptime: 40% of the fight", models.SpecRule{Kind: "uptime", Buff: "Tempo", AtLeast: 0.95},
			[]models.RuleResult{{Text: "Tempo up", Share: 0.4, AtLeast: 0.95}}},
		{"nothing to measure: no such cast", models.SpecRule{Kind: "with", Cast: "Missing", Buff: "Soul", AtLeast: 1}, []models.RuleResult{}},
	}
	for _, tc := range tests {
		got := failedRules([]models.SpecRule{tc.rule}, casts, buffs, fight)
		if len(got) != len(tc.want) || len(got) == 1 && got[0] != tc.want[0] {
			t.Errorf("%s: failed = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
