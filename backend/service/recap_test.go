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
	keys[3].Deaths = []models.KeyDeath{{Player: "Testmage", Ability: "Slam"}, {Player: "Testrogue", Ability: "Slam"}}
	got := buildRecap(keys, nil, 0, 999*minute)
	if got.Keys != 4 || got.Timed != 3 || got.HighestTimed != 12 || got.Deaths != 2 ||
		got.TimeMS != (25+28+27+35)*minute {
		t.Errorf("tiles = %d keys, %d timed, +%d, %d ms, %d deaths; want 4, 3, +12, 115 min, 2", got.Keys, got.Timed,
			got.HighestTimed, got.TimeMS, got.Deaths)
	}
	wantDungeons := []models.RecapDungeon{{Dungeon: "Skyreach", Runs: 3, Timed: 3, BestLevel: 12, BestMS: 27 * minute},
		{Dungeon: "Ara-Kara", Runs: 1}}
	if !reflect.DeepEqual(got.Dungeons, wantDungeons) {
		t.Errorf("dungeons = %+v; want %+v", got.Dungeons, wantDungeons)
	}
	wantParty := []models.RecapMate{{Player: "Testpriest", Keys: 3, Timed: 3}, {Player: "Testrogue", Keys: 2, Timed: 1}}
	if !reflect.DeepEqual(got.Party, wantParty) {
		t.Errorf("party = %+v; want %+v", got.Party, wantParty)
	}
	if y := got.You; y == nil || y.Character != "Testmage" || y.Deaths != 1 || len(y.Specs) != 1 ||
		y.Specs[0] != "Fire Mage" || !reflect.DeepEqual(y.Killers, []models.RecapCount{{Name: "Slam", Count: 1}}) {
		t.Errorf("you = %+v; want the mage, a Fire Mage, killed once by Slam", got.You)
	}
}

func TestRecapFacts(t *testing.T) {
	keys := []models.Key{
		recapKey("Skyreach", 10, 0, 20*minute),
		recapKey("Skyreach", 13, 100, 29*minute+50_000),
		recapKey("Ara-Kara", 12, 200, 40*minute),
	}
	keys[2].Deaths = []models.KeyDeath{{Ability: "Slam"}, {Ability: "Slam"}, {Ability: "Bolt"}}
	keys[1].Interrupts = []models.KeyInterrupt{{Player: "Testshaman"}, {Player: "Testshaman"}, {Player: "Testmage"}}
	var got []string
	starts := map[string]int64{}
	for _, f := range buildRecap(keys, nil, 0, 999*minute).Facts {
		got = append(got, f.Text)
		starts[f.Text] = f.Start
	}
	want := []string{
		"Fastest timed key: Skyreach +10 in 20:00, 67% of the timer",
		"Closest call: Skyreach +13 with 0:10 to spare",
		"Most deaths in a key: 3 in Ara-Kara +12",
		"Deadliest ability: Slam, 2 deaths",
		"Most kicks in a key: Testshaman, 2 in Skyreach +13",
		"Longest key: Ara-Kara +12, 40:00",
		"Most improved: Skyreach, from +10 to +13 timed",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("facts =\n%q\nwant\n%q", got, want)
	}
	if starts[want[2]] != 200*minute || starts[want[0]] != 0 {
		t.Errorf("facts open %v; want each its key", starts)
	}
	if facts := buildRecap(nil, nil, 0, 1).Facts; len(facts) != 0 {
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
		0, 1_000_000).Raid
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
