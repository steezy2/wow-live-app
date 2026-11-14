package service

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"wow-live/backend/models"
)

func TestSimilarKeysAreTheSameDungeonSpecAndNearLevels(t *testing.T) {
	arcane, frost := map[string]int{"Testmage": 62}, map[string]int{"Testmage": 64}
	abilities := []models.KeyAbility{{Player: "Testmage", Ability: "Arcane Blast"}}
	key := models.Key{Start: 100, MapID: 1, Level: 10, Specs: arcane, Abilities: abilities}
	keys := []models.Key{
		key, // the key itself
		{Start: 101, MapID: 1, Level: 12, Specs: arcane, Abilities: abilities},
		{Start: 102, MapID: 1, Level: 8, Specs: arcane, Abilities: abilities},
		{Start: 103, MapID: 1, Level: 13, Specs: arcane, Abilities: abilities}, // 3 levels up
		{Start: 104, MapID: 2, Level: 10, Specs: arcane, Abilities: abilities}, // another dungeon
		{Start: 105, MapID: 1, Level: 10, Specs: frost, Abilities: abilities},  // another spec
		{Start: 106, MapID: 1, Level: 10, Specs: arcane},                       // saved before abilities were kept
	}
	var got []int64
	for _, k := range similarKeys(keys, key, "Testmage") {
		got = append(got, k.Start)
	}
	if want := []int64{101, 102}; !slices.Equal(got, want) {
		t.Errorf("similar keys = %v, want %v", got, want)
	}
}

func TestPickRunsByTime(t *testing.T) {
	run := func(ms int64) models.RankedRun { return models.RankedRun{DurationMS: ms, Code: fmt.Sprint(ms)} }
	many := make([]models.RankedRun, 30)
	for i := range many {
		many[i] = run(1_000_000 + int64(i)*1000)
	}
	tests := []struct {
		name    string
		ranked  []models.RankedRun
		ms      int64
		partial bool
		want    []int64
		how     string
	}{
		{"closest within 15% first", []models.RankedRun{run(900_000), run(1_040_000), run(1_500_000), run(990_000)},
			1_000_000, false, []int64{990_000, 1_040_000, 900_000}, "near"},
		{"none near: the closest", []models.RankedRun{run(2_000_000), run(1_500_000)}, 1_000_000, false,
			[]int64{1_500_000, 2_000_000}, "closest"},
		{"a wipe: runs at least as long, shortest first", []models.RankedRun{run(100), run(200), run(150)}, 150, true,
			[]int64{150, 200}, "longer"},
	}
	for _, tc := range tests {
		p := pickRuns(tc.ranked, tc.ms, tc.partial)
		var got []int64
		for _, r := range p.runs {
			got = append(got, r.DurationMS)
		}
		if !slices.Equal(got, tc.want) || p.how != tc.how {
			t.Errorf("%s: picked %v (%s), want %v (%s)", tc.name, got, p.how, tc.want, tc.how)
		}
	}
	if got := len(pickRuns(many, 1_010_000, false).runs); got != compareLogs {
		t.Errorf("picked %d of 30 runs, want %d", got, compareLogs)
	}
}

// comparison is a 5-minute fight where the player cast Arcane Surge 3 times against the top logs' 5.
func comparison(yourUsual ...models.RankedPlayer) models.PlayerCompare {
	run := func(surges int, damage int64, haste int64) models.RankedPlayer {
		return models.RankedPlayer{DurationMS: 300_000, Damage: damage, ActiveMS: 270_000,
			Abilities: []models.RankedAbility{{ID: 365350, Name: "Arcane Surge", Damage: damage / 10, Casts: surges},
				{ID: 30451, Name: "Arcane Blast", Damage: damage * 9 / 10, Casts: 100}},
			Buffs: []models.RankedBuff{{ID: 1, Name: "Arcane Tempo", UptimeMS: haste}}}
	}
	top := []models.RankedPlayer{run(5, 450_000_000, 285_000), run(5, 420_000_000, 285_000), run(5, 480_000_000, 285_000)}
	logs := []models.CompareLog{{Name: "A", Amount: 1_500_000}, {Name: "B", Amount: 1_400_000}, {Name: "C", Amount: 1_600_000}}
	return buildCompare(compareInput{you: run(3, 300_000_000, 150_000), yours: yourUsual, top: top, logs: logs,
		metric: "DPS", spec: specs[62]})
}

func TestBuildCompareRowsAndAdvice(t *testing.T) {
	c := comparison()
	i := slices.IndexFunc(c.Abilities, func(r models.CompareRow) bool { return r.Name == "Arcane Surge" })
	if i < 0 {
		t.Fatalf("abilities = %+v, want Arcane Surge", c.Abilities)
	}
	if r := c.Abilities[i]; r.Unit != "casts" || r.You != 3 || r.Top != 5 || r.Best != 5 || r.Yours != nil {
		t.Errorf("Arcane Surge row = %+v, want 3 casts against the top's 5 and no usual", r)
	}
	if len(c.Advice) < 2 || !strings.HasPrefix(c.Advice[0], "Arcane Surge: 3 casts, top Arcane Mages 5 in the same time (about ") ||
		!strings.Contains(strings.Join(c.Advice, "|"), "Arcane Tempo up 50% of the time, top Arcane Mages 95%") {
		t.Errorf("advice = %q, want the Arcane Surge casts first and the buff uptime", c.Advice)
	}
	if want := "67% of the top 3's median DPS (4th of 4)"; !strings.HasPrefix(c.Verdict, want) ||
		!strings.Contains(c.Verdict, "Most lost: Arcane Surge") {
		t.Errorf("verdict = %q, want it to start %q and name the costliest line", c.Verdict, want)
	}
}

func TestVerdictAgainstYourUsualNeedsTwoRuns(t *testing.T) {
	usual := func() models.RankedPlayer {
		return models.RankedPlayer{DurationMS: 300_000, Damage: 270_000_000, ActiveMS: 270_000}
	}
	if v := comparison(usual()).Verdict; strings.Contains(v, "your usual") {
		t.Errorf("verdict with one earlier run = %q, want no usual", v)
	}
	if v := comparison(usual(), usual()).Verdict; !strings.Contains(v, "11% above your usual here") {
		t.Errorf("verdict with two earlier runs = %q, want 11%% above their usual", v)
	}
}

func TestAbilitiesOnlyOneSideUsedAreListedApart(t *testing.T) {
	you := models.RankedPlayer{DurationMS: 60_000, Damage: 100, Abilities: []models.RankedAbility{{ID: 1, Name: "Mine", Casts: 4, Damage: 50}}}
	top := models.RankedPlayer{DurationMS: 60_000, Damage: 100, Abilities: []models.RankedAbility{{ID: 2, Name: "Theirs", Casts: 4, Damage: 50}}}
	c := buildCompare(compareInput{you: you, top: []models.RankedPlayer{top, top}, logs: []models.CompareLog{{Amount: 1}, {Amount: 1}},
		metric: "DPS", spec: specs[62]})
	if !slices.Equal(c.OnlyYou, []string{"Mine"}) || !slices.Equal(c.OnlyTop, []string{"Theirs"}) || len(c.Abilities) != 0 {
		t.Errorf("only you %v, only top %v, rows %+v; want each side's own ability listed apart and not scored", c.OnlyYou,
			c.OnlyTop, c.Abilities)
	}
}

func TestBaselineSaysWhichLogsTheyAre(t *testing.T) {
	run := models.RankedRun{}
	tests := []struct {
		p    picked
		want string
	}{
		{picked{runs: []models.RankedRun{run, run}, how: "near"}, "2 ranked Arcane Mage runs within 15% of 5:00"},
		{picked{runs: []models.RankedRun{run}, how: "closest"}, "closest 1 ranked Arcane Mage runs; none within 15% of 5:00"},
		{picked{runs: []models.RankedRun{run}, how: "longer"},
			"1 ranked Arcane Mage runs at least as long as 5:00, read over their first 5:00"},
		{picked{how: "near"}, "no ranked Arcane Mage runs found"},
	}
	for _, tc := range tests {
		if got := tc.p.baseline("Arcane Mage", 300_000); got != tc.want {
			t.Errorf("baseline = %q, want %q", got, tc.want)
		}
	}
}
