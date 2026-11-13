package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"wow-live/backend/models"
)

// recapKey is a key of dungeon at level started at start (minutes), timed when finalMS is under the 30-minute timer,
// with the mage logging it and the party given.
func recapKey(dungeon string, level int, start, finalMS int64, party ...string) models.Key {
	status := "depleted"
	if finalMS < 30*minute {
		status = "timed"
	}
	return models.Key{Dungeon: dungeon, Level: level, Start: start * minute, End: start*minute + finalMS,
		FinalMS: finalMS, TimerMS: 30 * minute, Status: status, Logger: "Testmage",
		Players: append([]string{"Testmage"}, party...), Specs: map[string]int{"Testmage": 63}}
}

func TestRecapCountsKeysDungeonsAndParty(t *testing.T) {
	keys := []models.Key{
		recapKey("Skyreach", 10, 0, 25*minute, "Testpriest"),
		recapKey("Skyreach", 12, 100, 28*minute, "Testpriest", "Testrogue"),
		recapKey("Skyreach", 12, 200, 27*minute, "Testpriest"),
		recapKey("Ara-Kara", 14, 300, 35*minute, "Testrogue"),
		recapKey("Ara-Kara", 15, 999, 20*minute), // after the period
	}
	keys[1].Realms = map[string]string{"Testrogue": "Sargeras"}
	keys[1].Overall.Rows = []models.KeyMeterRow{{Player: "Testrogue", Class: "Rogue"}}
	keys[3].Deaths = []models.KeyDeath{{Player: "Testmage", Ability: "Slam"}, {Player: "Testrogue", Ability: "Slam"}}
	got := buildRecap(keys, nil, 0, 999*minute, "").MPlus
	if got.Keys != 4 || got.Timed != 3 || got.SuccessPct != 75 || got.HighestTimed != 12 || got.Deaths != 2 ||
		got.TimeMS != (25+28+27+35)*minute {
		t.Errorf("tiles = %d keys, %d timed (%v%%), +%d, %d ms, %d deaths; want 4, 3 (75%%), +12, 115 min, 2",
			got.Keys, got.Timed, got.SuccessPct, got.HighestTimed, got.TimeMS, got.Deaths)
	}
	wantDungeons := []models.RecapDungeon{ // the depleted Ara-Kara +14 sets no level and no time
		{Dungeon: "Skyreach", Runs: 3, Timed: 3, SuccessPct: 100, BestLevel: 12, BestRuns: 2,
			AvgMS: 27*minute + 30_000, MedianMS: 27*minute + 30_000, BestMS: 27 * minute},
		{Dungeon: "Ara-Kara", Runs: 1}}
	if !reflect.DeepEqual(got.Dungeons, wantDungeons) {
		t.Errorf("dungeons = %+v\nwant %+v", got.Dungeons, wantDungeons)
	}
	wantParty := []models.RecapMate{{Player: "Testpriest", Keys: 3, Timed: 3},
		{Player: "Testrogue", Realm: "Sargeras", Class: "Rogue", Keys: 2, Timed: 1}}
	if !reflect.DeepEqual(got.Party, wantParty) {
		t.Errorf("party = %+v\nwant %+v (one rogue, though only one key knows the realm)", got.Party, wantParty)
	}
	if y := got.You; y == nil || y.Character != "Testmage" || y.Deaths != 1 || len(y.Specs) != 1 ||
		y.Specs[0] != "Fire Mage" || !reflect.DeepEqual(y.Killers, []models.RecapCount{{Name: "Slam", Count: 1}}) {
		t.Errorf("you = %+v; want the mage, a Fire Mage, killed once by Slam", got.You)
	}
}

func TestRecapKeepsMPlusAndRaidApart(t *testing.T) {
	pull := models.RaidPull{Boss: "Test Boss", Difficulty: "Mythic", Start: 50 * minute, DurationMS: 5 * minute,
		Kill: true, Logger: "Testmage", Deaths: []models.RaidDeath{{}, {}}}
	got := buildRecap([]models.Key{recapKey("Skyreach", 10, 0, 25*minute)}, []models.RaidPull{pull}, 0, 999*minute, "")
	if got.MPlus.Keys != 1 || got.MPlus.Deaths != 0 || got.MPlus.TimeMS != 25*minute {
		t.Errorf("mplus = %d keys, %d deaths, %d ms; want the key only", got.MPlus.Keys, got.MPlus.Deaths,
			got.MPlus.TimeMS)
	}
	if r := got.Raid; r.Pulls != 1 || r.Kills != 1 || r.Killed != 1 || r.Deaths != 2 || r.TimeMS != 5*minute ||
		r.YourPulls != 0 {
		t.Errorf("raid = %+v; want the pull only, without the mage in it", r)
	}
}

func TestRecapIsOneCharactersRuns(t *testing.T) {
	alt := recapKey("Ara-Kara", 12, 10, 25*minute)
	alt.Logger, alt.Players, alt.Realms = "Testrogue", []string{"Testrogue"}, map[string]string{"Testrogue": "Sargeras"}
	keys := []models.Key{recapKey("Skyreach", 10, 0, 25*minute), alt}
	got := buildRecap(keys, nil, 0, 999*minute, "Testrogue-Sargeras").MPlus
	if got.Keys != 1 || got.Dungeons[0].Dungeon != "Ara-Kara" || got.You == nil || got.You.Character != "Testrogue" {
		t.Errorf("rogue's recap = %d keys, %+v, you %+v; want the rogue's key only", got.Keys, got.Dungeons, got.You)
	}
}

func TestRecapKeyRows(t *testing.T) {
	keys := []models.Key{recapKey("Skyreach", 10, 0, 25*minute, "Testpriest"), recapKey("Skyreach", 11, 50, 35*minute),
		recapKey("Ara-Kara", 12, 100, 25*minute, "Testpriest")}
	keys[0].Interrupts = []models.KeyInterrupt{{Player: "Testmage"}, {Player: "Testpriest"}, {Player: "Testpriest"}}
	keys[0].Casts = []models.KeyCast{{Player: "Testmage", Kind: "cc"}, {Player: "Testmage", Kind: "defensive"}}
	keys[0].Overall.Rows = []models.KeyMeterRow{
		{Player: "Testmage", DPS: 900, HPS: 50, Damage: 100, PriorityDamage: 40, Dispels: 2},
		{Player: "Testpriest", Damage: 300, PriorityDamage: 20, Dispels: 1}}
	keys[1].Players = []string{"Testpriest"} // the mage logged it but wasn't in it; saved before priority damage
	keys[1].Overall.Rows = []models.KeyMeterRow{{Player: "Testpriest", Damage: 100, Dispels: 3}}
	for i := range recapKeyRows {
		keys = append(keys, recapKey("Old", 2, int64(-1-i), 25*minute))
	}
	got := buildRecap(keys, nil, -999*minute, 999*minute, "").MPlus.KeyRows
	if len(got) != recapKeyRows {
		t.Fatalf("%d rows; want the newest %d", len(got), recapKeyRows)
	}
	want := []models.RecapKeyRow{
		{Start: 100 * minute, Dungeon: "Ara-Kara", Level: 12, Status: "timed",
			You:   models.RecapKeyYou{DPS: -1, HPS: -1, RecapKeyCounts: models.RecapKeyCounts{PriorityPct: -1}}, // no meter row
			Party: models.RecapKeyCounts{PriorityPct: -1}},
		{Start: 50 * minute, Dungeon: "Skyreach", Level: 11, Status: "depleted",
			You:   models.RecapKeyYou{DPS: -1, HPS: -1, RecapKeyCounts: models.RecapKeyCounts{Kicks: -1, CC: -1, Dispels: -1, PriorityPct: -1}},
			Party: models.RecapKeyCounts{Dispels: 3, PriorityPct: -1}},
		{Start: 0, Dungeon: "Skyreach", Level: 10, Status: "timed",
			You:   models.RecapKeyYou{DPS: 900, HPS: 50, RecapKeyCounts: models.RecapKeyCounts{Kicks: 1, CC: 1, Dispels: 2, PriorityPct: 40}},
			Party: models.RecapKeyCounts{Kicks: 3, CC: 1, Dispels: 3, PriorityPct: 15}}}
	if !reflect.DeepEqual(got[:3], want) {
		t.Errorf("rows = %+v\nwant %+v", got[:3], want)
	}
}

func TestRecapFacts(t *testing.T) {
	keys := []models.Key{
		recapKey("Skyreach", 10, 0, 20*minute),
		recapKey("Skyreach", 13, 100, 29*minute+50_000),
		recapKey("Ara-Kara", 12, 200, 40*minute),
	}
	keys[2].Deaths = []models.KeyDeath{{Ability: "Slam"}, {Ability: "Slam"}, {Ability: "Bolt"}} // depleted
	keys[0].Deaths = []models.KeyDeath{{}}
	keys[1].Interrupts = []models.KeyInterrupt{{Player: "Testshaman"}, {Player: "Testshaman"}, {Player: "Testmage"}}
	var got []string
	starts := map[string]int64{}
	for _, f := range buildRecap(keys, nil, 0, 999*minute, "").MPlus.Facts {
		got = append(got, f.Text)
		starts[f.Text] = f.Start
	}
	want := []string{
		"Fastest timed key: Skyreach +10 in 20:00, 67% of the timer",
		"Closest call: Skyreach +13 with 0:10 to spare",
		"Most deaths in a timed key: 1 in Skyreach +10",
		"Deadliest ability: Slam, 2 deaths",
		"Most kicks in a key: Testshaman, 2 in Skyreach +13",
		"Longest key: Ara-Kara +12, 40:00",
		"Most improved: Skyreach, from +10 to +13 timed",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("facts =\n%q\nwant\n%q", got, want)
	}
	if starts[want[2]] != 0 || starts[want[0]] != 0 {
		t.Errorf("facts open %v; want each its key", starts)
	}
	if facts := buildRecap(nil, nil, 0, 1, "").MPlus.Facts; len(facts) != 0 {
		t.Errorf("facts with no keys = %v; want none", facts)
	}
}

func TestRecapRaidByBoss(t *testing.T) {
	pull := func(start int64, kill bool, pct float64) models.RaidPull {
		return models.RaidPull{Boss: "Test Boss", Difficulty: "Mythic", Start: start, Kill: kill, BossPct: pct,
			Deaths: []models.RaidDeath{{KeyDeath: models.KeyDeath{Player: "Testmage"}}},
			Taken:  []models.KeyHit{{Ability: "Slam", Damage: 10}, {Ability: "Bolt", Damage: 4}}}
	}
	got := buildRecap([]models.Key{recapKey("Skyreach", 10, 0, 20*minute)},
		[]models.RaidPull{pull(30, true, 0), pull(10, false, 42), pull(20, false, 18.5), pull(2_000_000, true, 0)},
		0, 1_000_000, "").Raid.Bosses
	want := []models.RecapBoss{{Boss: "Test Boss", Difficulty: "Mythic", Pulls: 3, Kills: 1, FirstKill: 30,
		FirstKillPull: 3, BestWipePct: 18.5, YourDeaths: 3, MostHit: "Slam"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("raid = %+v; want %+v", got, want)
	}
}

type fakeSeason struct{ err error }

func (f fakeSeason) SeasonDates(context.Context, string) (time.Time, time.Time, error) {
	return time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC), time.Time{}, f.err
}

func TestRecapPeriods(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	labels := func(err error) []string {
		var got []string
		for _, p := range (Recaps{Season: fakeSeason{err}, Now: func() time.Time { return now }}).Periods(context.Background()) {
			got = append(got, p.Label)
			if p.Label == "Last month" && (p.From != time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).UnixMilli() ||
				p.To != time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).UnixMilli()) {
				t.Errorf("last month = %+v; want October", p)
			}
		}
		return got
	}
	if got := labels(nil); !reflect.DeepEqual(got, []string{"This month", "Last month", "This season", "This year", "All time"}) {
		t.Errorf("periods = %v", got)
	}
	if got := labels(errors.New("down")); len(got) != 4 {
		t.Errorf("periods without Raider.IO = %v; want no season", got)
	}
}

func TestRecapTopTimes(t *testing.T) {
	keys := []models.Key{recapKey("Voidscar Arena", 12, 0, 25*minute), recapKey("Voidscar Arena", 14, 50, 32*minute),
		recapKey("Skyreach", 10, 100, 20*minute), // Skyreach isn't a Warcraft Logs dungeon in the fake
		recapKey("Voidscar Arena", 16, 150, 0)}
	keys[3].Status = "abandoned"
	saved := func() ([]models.Key, []models.RaidPull) { return keys, nil }
	times := make([]int64, recapTopRuns)
	for i := range times {
		times[i] = int64(20+i) * minute
	}
	got, err := Recaps{Saved: saved, Logs: &fakeBaselines{topTimes: times}}.TopTimes(context.Background(), 0, 999*minute, "")
	// At +12, the highest level timed: the depleted +14 and the abandoned +16 don't count.
	want := []models.RecapTopTime{{Dungeon: "Voidscar Arena", Level: 12, MedianMS: 24*minute + 30_000, Runs: 10}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("top times = %+v, %v; want %+v", got, err, want)
	}
	few, _ := Recaps{Saved: saved, Logs: &fakeBaselines{topTimes: times[:3]}}.TopTimes(context.Background(), 0, 999*minute, "")
	if len(few) != 1 || few[0].MedianMS != 0 || few[0].Runs != 3 {
		t.Errorf("top times over 3 runs = %+v; want no median", few)
	}
	failed, err := Recaps{Saved: saved, Logs: &fakeBaselines{topErr: errors.New("down")}}.TopTimes(context.Background(),
		0, 999*minute, "")
	if err != nil || len(failed) != 0 {
		t.Errorf("top times when Warcraft Logs fails = %+v, %v; want the dungeon left out", failed, err)
	}
	if _, err := (Recaps{Saved: saved}).TopTimes(context.Background(), 0, 1, ""); !errors.Is(err, models.ErrNoCredentials) {
		t.Errorf("without Logs: %v; want ErrNoCredentials", err)
	}
}
