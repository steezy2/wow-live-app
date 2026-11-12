package service

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"wow-live/backend/models"
)

// fakeReports is Warcraft Logs with one report of two keys, and the points the budget answers in turn (the last
// repeats).
type fakeReports struct {
	mu      sync.Mutex
	budgets []models.RateLimit
}

func (f *fakeReports) CurrentZones(context.Context, []string) (models.WCLZones, error) {
	return models.WCLZones{MPlusID: 55, RaidID: 53}, nil
}

func (f *fakeReports) RecentReports(context.Context, string, string, string, int) ([]models.CharacterReport, bool,
	error) {
	fight := func(id int, start int64) models.ReportFight {
		return models.ReportFight{ID: id, KeystoneLevel: 12, StartTime: start, EndTime: start + 1_800_000,
			Players: []string{"Testmage"}}
	}
	return []models.CharacterReport{{Code: "abc", ZoneID: 55, Fights: []models.ReportFight{fight(1, 0),
		fight(2, 3_600_000)}}}, false, nil
}

func (f *fakeReports) RateLimit(context.Context) (models.RateLimit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.budgets[0]
	if len(f.budgets) > 1 {
		f.budgets = f.budgets[1:]
	}
	return b, nil
}

// testImporter is an Importer on fake Warcraft Logs whose Fetch fails with fail's errors in turn, recording the
// fights read and the waits.
type testImporter struct {
	*Importer
	mu    sync.Mutex
	read  []models.ReportLink
	waits []time.Duration
}

func newTestImporter(t *testing.T, budgets []models.RateLimit, fail ...error) *testImporter {
	ti := &testImporter{}
	ti.Importer = &Importer{Source: &fakeReports{budgets: budgets}, Characters: Characters{Realms: fakeRealms{}},
		Fetch: func(_ context.Context, ref models.FightRef, _ string) (string, error) {
			ti.mu.Lock()
			defer ti.mu.Unlock()
			if len(fail) > 0 {
				err := fail[0]
				fail = fail[1:]
				return "", err
			}
			return fmt.Sprintf("%s-%d.zip", ref.Code, ref.FightID), nil
		},
		Read: func(_ string, link models.ReportLink) error {
			ti.mu.Lock()
			defer ti.mu.Unlock()
			ti.read = append(ti.read, link)
			return nil
		},
		Saved:   func() ([]models.Key, []models.RaidPull, error) { return nil, nil, nil },
		Prepare: func() error { return nil },
		Count:   func() int { return 0 },
		Dir:     t.TempDir(),
		Now:     func() time.Time { return time.UnixMilli(1_000_000) },
		Wait: func(_ context.Context, d time.Duration) error {
			ti.mu.Lock()
			defer ti.mu.Unlock()
			ti.waits = append(ti.waits, d)
			return nil
		},
	}
	return ti
}

// finish waits for the running import to end.
func (ti *testImporter) finish() {
	ti.Importer.mu.Lock()
	done := ti.done
	ti.Importer.mu.Unlock()
	if done != nil {
		<-done
	}
}

var plenty = models.RateLimit{LimitPerHour: 3600, PointsSpentThisHour: 100}

func TestImportFetchesEachFightNewestFirst(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty, {LimitPerHour: 3600, PointsSpentThisHour: 140}})
	if err := ti.Start(context.Background(), "us", "Testmage-Area52"); err != nil {
		t.Fatal(err)
	}
	ti.finish()
	want := []models.ReportLink{{Code: "abc", FightID: 2}, {Code: "abc", FightID: 1}}
	if !slices.Equal(ti.read, want) {
		t.Errorf("read %v, want %v", ti.read, want)
	}
	if s := ti.Status(); s.State != importDone || s.FightsDone != 2 || s.PointsSpent != 40 {
		t.Errorf("status %+v, want done with 2 fights and 40 points", s)
	}
}

func TestImportWaitsForThePointsToReset(t *testing.T) {
	low := models.RateLimit{LimitPerHour: 3600, PointsSpentThisHour: 3500, PointsResetIn: 600}
	ti := newTestImporter(t, []models.RateLimit{low, plenty})
	if err := ti.Start(context.Background(), "us", "Testmage-Area52"); err != nil {
		t.Fatal(err)
	}
	ti.finish()
	if len(ti.waits) != 1 || ti.waits[0] != 601*time.Second {
		t.Errorf("waited %v, want once for the 600 s to the reset", ti.waits)
	}
	if len(ti.read) != 2 {
		t.Errorf("read %d fights after the wait, want 2", len(ti.read))
	}
}

func TestImportRetriesAFightThePointsStopped(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty}, fmt.Errorf("page 3: %w", models.ErrRateLimited))
	if err := ti.Start(context.Background(), "us", "Testmage-Area52"); err != nil {
		t.Fatal(err)
	}
	ti.finish()
	if s := ti.Status(); len(ti.read) != 2 || s.FightsFailed != 0 || len(ti.waits) != 1 {
		t.Errorf("read %v, %d failed, waits %v: want both read after one wait", ti.read, s.FightsFailed, ti.waits)
	}
}

func TestImportMarksAFightFailedAfterItsTries(t *testing.T) {
	boom := fmt.Errorf("boom")
	ti := newTestImporter(t, []models.RateLimit{plenty}, boom, boom)
	if err := ti.Start(context.Background(), "us", "Testmage-Area52"); err != nil {
		t.Fatal(err)
	}
	ti.finish()
	if s := ti.Status(); s.FightsFailed != 1 || s.FightsDone != 1 || s.Problem != "boom" || s.State != importDone {
		t.Errorf("status %+v, want one failed (boom) and one done", s)
	}
}

func TestImportResumesFromItsState(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty})
	st := importState{Region: "us", Character: "Testmage-Area52", Done: []models.ReportLink{{Code: "abc", FightID: 1}},
		Fights: []models.FightRef{{ReportLink: models.ReportLink{Code: "abc", FightID: 1}, Kind: models.FightMPlus},
			{ReportLink: models.ReportLink{Code: "abc", FightID: 2}, Kind: models.FightMPlus}}}
	if err := ti.writeState(st); err != nil {
		t.Fatal(err)
	}
	if err := ti.Resume(); err != nil {
		t.Fatal(err)
	}
	ti.finish()
	if want := []models.ReportLink{{Code: "abc", FightID: 2}}; !slices.Equal(ti.read, want) {
		t.Errorf("read %v, want only the fight left %v", ti.read, want)
	}
}

func TestStartTurnsDownABadCharacter(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty})
	if err := ti.Start(context.Background(), "us", "Testmage-Nowhere"); err == nil {
		t.Fatal("started with an unknown realm")
	}
	if s := ti.Status(); s.State != importIdle {
		t.Errorf("state %q, want idle", s.State)
	}
}

func TestRemoveImportedKeepsLocalKeysAndPulls(t *testing.T) {
	k, pulls := newRaidTracker(t)
	keys := k.store.(*memStore)
	keys.keys = []models.Key{{Start: 1}, {Start: 2, Imported: true}}
	pulls.pulls = []models.RaidPull{{Start: 3, Imported: true, Meter: fightingMeter()}, {Start: 4,
		Meter: fightingMeter()}}
	if err := k.Reload(); err != nil {
		t.Fatal(err)
	}
	if n := k.ImportedCount(); n != 2 {
		t.Errorf("%d imported, want 2", n)
	}
	if err := k.RemoveImported(); err != nil {
		t.Fatal(err)
	}
	if len(keys.keys) != 1 || keys.keys[0].Start != 1 || len(pulls.pulls) != 1 || pulls.pulls[0].Start != 4 {
		t.Errorf("left keys %v and pulls %v, want only the local ones", keys.keys, pulls.pulls)
	}
	if n := k.ImportedCount(); n != 0 {
		t.Errorf("%d imported after removing, want 0", n)
	}
}

// fightingMeter is a meter with a row, so the pull counts as fought.
func fightingMeter() models.KeyMeter {
	return models.KeyMeter{Rows: []models.KeyMeterRow{{Player: "Testmage"}}}
}

func TestTheLocalCopyOfAKeyWins(t *testing.T) {
	local := models.Key{Start: 60_000, Dungeon: "Testvault", Level: 12}
	imported := models.Key{Start: 118_400, Dungeon: "Testvault", Level: 12, Imported: true}
	other := models.Key{Start: 120_000, Dungeon: "Testvault", Level: 13, Imported: true}
	isImported := func(k models.Key) bool { return k.Imported }
	start := func(k models.Key) int64 { return k.Start }
	for _, tc := range []struct {
		name     string
		saving   models.Key
		skip     bool
		deleted  []int64
		leftKeys []int64
	}{
		{"an imported copy of a local key isn't saved", imported, true, nil, []int64{60_000, 120_000}},
		{"a local key deletes its imported copy", local, false, []int64{118_400}, []int64{60_000, 120_000}},
	} {
		var mu sync.RWMutex
		list := []models.Key{local, imported, other} // the tracker lists a key it's saving
		var deleted []int64
		skip, err := keepLocal(&mu, &list, tc.saving, sameKey, isImported, start, func(s int64) error {
			deleted = append(deleted, s)
			return nil
		})
		var left []int64
		for _, k := range list {
			left = append(left, k.Start)
		}
		if err != nil || skip != tc.skip || !slices.Equal(deleted, tc.deleted) || !slices.Equal(left, tc.leftKeys) {
			t.Errorf("%s: skip %v, deleted %v, left %v (%v)", tc.name, skip, deleted, left, err)
		}
	}
}

func TestAnImportedPullNeverReplacesTheLocalOneAtItsStart(t *testing.T) {
	keys, store := newRaidTracker(t)
	feed(t, keys, pullLines(3, 0, 120, 40, false)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	keys.SetSource(&models.ReportLink{Code: "abc", FightID: 7}) // the same pull, uploaded from the same log
	feed(t, keys, pullLines(3, 0, 120, 40, false)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	if len(store.pulls) != 1 || store.pulls[0].Imported {
		t.Errorf("saved %d pulls (imported %v), want only the local one", len(store.pulls), store.pulls[0].Imported)
	}
}

func TestAStoppedImportDoesntResumeAfterARestart(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty})
	ti.Fetch = func(ctx context.Context, _ models.FightRef, _ string) (string, error) {
		ti.Stop()
		return "", ctx.Err()
	}
	if err := ti.Start(context.Background(), "us", "Testmage-Area52"); err != nil {
		t.Fatal(err)
	}
	ti.finish()
	if st, err := ti.readState(); err != nil || len(st.left()) != 0 {
		t.Errorf("state %+v (%v): want no fights left to resume", st, err)
	}
}
