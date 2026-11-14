package service

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"wow-live/backend/models"
)

// fakeLogs is Warcraft Logs for a player review: three ranked runs, each log read as asked, and a points count
// that drops by 100 after the first reading of it.
type fakeLogs struct {
	left    float64
	windows []int64
	fail    bool
}

func (f *fakeLogs) MPlusEncounters(context.Context) (map[string]int, error) {
	return map[string]int{"Voidscar Arena": 7}, nil
}

func (f *fakeLogs) RaidEncounters(context.Context, []string) (map[string]int, error) { return nil, nil }

func (f *fakeLogs) RankedRuns(context.Context, models.RankQuery, int) ([]models.RankedRun, bool, error) {
	return []models.RankedRun{{DurationMS: 200_000, Code: "a", FightID: 1, Name: "A", Amount: 9e6},
		{DurationMS: 400_000, Code: "b", FightID: 2, Name: "B", Amount: 1e6},
		{DurationMS: 500_000, Code: "c", FightID: 3, Name: "C", Amount: 1e6}}, false, nil
}

func (f *fakeLogs) RunPlayer(_ context.Context, _ string, _ int, _, _ string, window int64) (models.RankedPlayer, error) {
	f.windows = append(f.windows, window)
	if f.fail {
		return models.RankedPlayer{}, errors.New("report gone")
	}
	return models.RankedPlayer{DurationMS: window, Damage: 600_000_000}, nil
}

func (f *fakeLogs) PointsLeft(context.Context) (float64, error) {
	left := f.left
	f.left -= 100
	return left, nil
}

// fakeCompareStore keeps player reviews in memory.
type fakeCompareStore struct {
	reviews map[string]models.PlayerCompare
}

func (s *fakeCompareStore) SaveCompare(c models.PlayerCompare) error {
	s.reviews[c.Player] = c
	return nil
}

func (s *fakeCompareStore) Compare(_ models.FightKind, _ int64, player string) (models.PlayerCompare, error) {
	if c, ok := s.reviews[player]; ok {
		return c, nil
	}
	return models.PlayerCompare{}, fs.ErrNotExist
}

// comparerOf is a Comparer over an abandoned 300 s key of Testmage (Arcane) that the tracker has saved.
func comparerOf(t *testing.T, baselines *fakeLogs) (Comparer, *LogTracker, *fakeCompareStore) {
	t.Helper()
	keys := newTracker(t, nil)
	keys.history = []models.Key{{Start: 1000, End: 5000, FinalMS: 300_000, Status: "abandoned", Dungeon: "Voidscar Arena",
		MapID: 585, Level: 12, Specs: map[string]int{"Testmage": 62},
		Overall:   models.KeyMeter{Rows: []models.KeyMeterRow{{Player: "Testmage", Damage: 300_000_000}}},
		Abilities: []models.KeyAbility{{Player: "Testmage", Ability: "Arcane Blast", Damage: 300_000_000, Casts: 10}}}}
	store := &fakeCompareStore{reviews: map[string]models.PlayerCompare{}}
	return Comparer{Keys: keys, Baselines: baselines, Store: store}, keys, store
}

func TestRunReadsAnAbandonedKeyOverTheSameStretchAndKeepsIt(t *testing.T) {
	baselines := &fakeLogs{left: 1000}
	c, _, store := comparerOf(t, baselines)
	review, err := c.Run(context.Background(), models.FightMPlus, 1000, "Testmage")
	if err != nil {
		t.Fatal(err)
	}
	if len(baselines.windows) != 2 || baselines.windows[0] != 300_000 {
		t.Errorf("windows = %v, want the 2 runs at least as long as the key (not the 200 s one) read over its first 300000 ms", baselines.windows)
	}
	if len(review.Logs) != 2 || review.Logs[0].Amount != 2e6 || review.Points != 99 {
		t.Errorf("logs %+v, points %v; want the logs' rates over that stretch (2M/s, not the ranking's 1M) and 99 points",
			review.Logs, review.Points)
	}
	if _, ok := store.reviews["Testmage"]; !ok {
		t.Error("the review wasn't stored")
	}
	if again, err := c.Run(context.Background(), models.FightMPlus, 1000, "Testmage"); err != nil || again.Points != 99 ||
		len(baselines.windows) != 2 {
		t.Errorf("second run = %+v, %v after %d reads; want the stored review with no new reads", again, err,
			len(baselines.windows))
	}
}

func TestRunStopsForTooFewPointsAndForLogsThatCantBeRead(t *testing.T) {
	baselines := &fakeLogs{left: minComparePoints - 1}
	c, _, store := comparerOf(t, baselines)
	if _, err := c.Run(context.Background(), models.FightMPlus, 1000, "Testmage"); !errors.Is(err, ErrNoPoints) ||
		len(baselines.windows) != 0 {
		t.Errorf("with too few points: %v after %d reads, want ErrNoPoints and none", err, len(baselines.windows))
	}
	baselines = &fakeLogs{left: 1000, fail: true}
	c.Baselines = baselines
	if _, err := c.Run(context.Background(), models.FightMPlus, 1000, "Testmage"); !errors.Is(err, ErrNoTopLogs) ||
		len(store.reviews) != 0 {
		t.Errorf("with every log failing: %v, %d stored; want ErrNoTopLogs and nothing stored", err, len(store.reviews))
	}
	if _, err := c.Run(context.Background(), models.FightMPlus, 1000, "Nobody"); !errors.Is(err, ErrNoSuchPlayer) {
		t.Errorf("an unknown player: %v, want ErrNoSuchPlayer", err)
	}
}

func TestCachedIsReadAgainWhenTheKeyWentOn(t *testing.T) {
	c, keys, _ := comparerOf(t, &fakeLogs{left: 1000})
	if _, err := c.Run(context.Background(), models.FightMPlus, 1000, "Testmage"); err != nil {
		t.Fatal(err)
	}
	keys.history[0].End = 9000 // an abandoned key the log went on in
	if _, err := c.Cached(models.FightMPlus, 1000, "Testmage"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("cached after the key went on: %v, want ErrNoSuchKey", err)
	}
}
