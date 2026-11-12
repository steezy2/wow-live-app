package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"wow-live/backend/models"
)

// fakeReports is Warcraft Logs with a report of two keys and a raid report (an hour older) of a kill, a wipe and a
// wipe without the character, and the points the budget answers in turn (the last repeats).
type fakeReports struct {
	mu      sync.Mutex
	budgets []models.RateLimit
}

func (f *fakeReports) CurrentZones(context.Context, []string) (models.WCLZones, error) {
	return models.WCLZones{MPlusID: 55, RaidID: 53}, nil
}

func (f *fakeReports) RecentReports(context.Context, string, string, string, int) ([]models.CharacterReport, bool,
	error) {
	party := []string{"Testmage", "Testrogue", "Testpriest", "Testhunter", "Testwarrior"}
	raid := append(slices.Clone(party), make([]string, 15)...)
	fight := func(id, level int, kill bool, start, length int64, players []string) models.ReportFight {
		return models.ReportFight{ID: id, KeystoneLevel: level, EncounterID: 3000 + id, Kill: kill, StartTime: start,
			EndTime: start + length, Players: players}
	}
	return []models.CharacterReport{
		{Code: "abc", ZoneID: 55, Fights: []models.ReportFight{fight(1, 12, false, 0, 30*60_000, party),
			fight(2, 12, false, 3_600_000, 30*60_000, party)}},
		{Code: "raid", ZoneID: 53, StartTime: -3_600_000, Fights: []models.ReportFight{
			fight(1, 0, true, 0, 5*60_000, raid), fight(2, 0, false, 600_000, 2*60_000, raid),
			fight(3, 0, false, 900_000, 3*60_000, []string{"Testrogue"})}},
	}, false, nil
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

var keysOnly = []models.ImportKind{models.ImportKeys}

func TestImportFetchesEachFightNewestFirst(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty, {LimitPerHour: 3600, PointsSpentThisHour: 140}})
	if err := ti.Start(context.Background(), "us", "Testmage-Area52", keysOnly); err != nil {
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
	if err := ti.Start(context.Background(), "us", "Testmage-Area52", keysOnly); err != nil {
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
	if err := ti.Start(context.Background(), "us", "Testmage-Area52", keysOnly); err != nil {
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
	if err := ti.Start(context.Background(), "us", "Testmage-Area52", keysOnly); err != nil {
		t.Fatal(err)
	}
	ti.finish()
	if s := ti.Status(); s.FightsFailed != 1 || s.FightsDone != 1 || s.Problem != "boom" || s.State != importDone {
		t.Errorf("status %+v, want one failed (boom) and one done", s)
	}
}

func TestImportStartsWithTheKindsPicked(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty})
	if err := ti.Start(context.Background(), "us", "Testmage-Area52",
		[]models.ImportKind{models.ImportRaidKills, models.ImportRaidWipes}); err != nil {
		t.Fatal(err)
	}
	ti.finish()
	want := []models.ReportLink{{Code: "raid", FightID: 2}, {Code: "raid", FightID: 1}}
	if s := ti.Status(); !slices.Equal(ti.read, want) || s.Fights != 2 {
		t.Errorf("read %v of %d fights, want only the raid's wipe and kill %v", ti.read, s.Fights, want)
	}
}

func TestImportResumesFromItsState(t *testing.T) {
	key := func(id int) models.FightRef {
		return models.FightRef{ReportLink: models.ReportLink{Code: "abc", FightID: id}, Kind: models.FightMPlus}
	}
	kill := models.FightRef{ReportLink: models.ReportLink{Code: "raid", FightID: 1}, Kind: models.FightRaid, Kill: true}
	for _, tc := range []struct {
		name  string
		kinds []models.ImportKind
		want  []models.ReportLink
	}{
		{"a state from before kinds resumes every fight left", nil, []models.ReportLink{key(2).ReportLink,
			kill.ReportLink}},
		{"the saved kinds stay", []models.ImportKind{models.ImportRaidKills}, []models.ReportLink{kill.ReportLink}},
	} {
		ti := newTestImporter(t, []models.RateLimit{plenty})
		st := importState{Region: "us", Character: "Testmage-Area52", Kinds: tc.kinds,
			Done: []models.ReportLink{key(1).ReportLink}, Fights: []models.FightRef{key(1), key(2), kill}}
		if err := ti.writeFile(importStateFile, st); err != nil {
			t.Fatal(err)
		}
		if err := ti.Resume(); err != nil {
			t.Fatal(err)
		}
		ti.finish()
		if !slices.Equal(ti.read, tc.want) {
			t.Errorf("%s: read %v, want %v", tc.name, ti.read, tc.want)
		}
	}
}

func TestFindCountsEachKindInTheBackground(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty, {LimitPerHour: 3600, PointsSpentThisHour: 104}})
	if err := ti.Find(context.Background(), "us", "Testmage-Area52"); err != nil {
		t.Fatal(err)
	}
	ti.finish()
	s := ti.Status()
	if s.State != importIdle || s.Plan == nil {
		t.Fatalf("status %+v, want idle with a plan", s)
	}
	plan := *s.Plan
	if plan.Character != "Testmage-Area52" || plan.PointsPerHour != 3400 || plan.ListedPoints != 4 {
		t.Errorf("plan %+v, want Testmage's, 3,400 points an hour and 4 spent listing", plan)
	}
	want := []models.ImportKindPlan{
		{Kind: models.ImportKeys, Fights: 2, Minutes: 60, Points: 81, Default: true}, // 60 × 5 × 0.27
		{Kind: models.ImportRaidKills, Fights: 1, Minutes: 5, Points: 27, Default: true},
		{Kind: models.ImportRaidWipes, Fights: 1, Minutes: 2, Points: 11}, // not Testrogue's
	}
	if !slices.Equal(plan.Kinds, want) {
		t.Errorf("kinds %+v, want %+v", plan.Kinds, want)
	}
}

func TestFindKeepsWhatItFoundUntilAnImportStarts(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty})
	if err := ti.Find(context.Background(), "us", "Testmage-Area52"); err != nil {
		t.Fatal(err)
	}
	ti.finish()
	restarted := newTestImporter(t, []models.RateLimit{plenty})
	restarted.Dir = ti.Dir
	if err := restarted.Resume(); err != nil {
		t.Fatal(err)
	}
	if s := restarted.Status(); s.Plan == nil || s.Plan.Kinds[0].Fights != 2 {
		t.Fatalf("after a restart: %+v, want the plan found", s)
	}
	if err := restarted.Start(context.Background(), "us", "Testmage-Area52", keysOnly); err != nil {
		t.Fatal(err)
	}
	restarted.finish()
	if s := restarted.Status(); s.Plan != nil || len(restarted.read) != 2 {
		t.Errorf("plan %+v and %d fights read, want no plan once the import started and both keys", s.Plan,
			len(restarted.read))
	}
}

func TestStopEndsAFindWithoutAPlan(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty})
	ti.Prepare = func() error { ti.Stop(); return nil }
	if err := ti.Find(context.Background(), "us", "Testmage-Area52"); err != nil {
		t.Fatal(err)
	}
	ti.finish()
	if s := ti.Status(); s.State != importIdle || s.Plan != nil {
		t.Errorf("status %+v, want idle without a plan", s)
	}
}

func TestFindIsBusyWhileAnImportRuns(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty})
	release := make(chan struct{})
	ti.Prepare = func() error { <-release; return nil }
	if err := ti.Start(context.Background(), "us", "Testmage-Area52", keysOnly); err != nil {
		t.Fatal(err)
	}
	err := ti.Find(context.Background(), "us", "Testmage-Area52")
	close(release)
	ti.finish()
	if !errors.Is(err, ErrBusy) {
		t.Errorf("found fights during an import: %v, want ErrBusy", err)
	}
}

func TestABusyStartKeepsTheFightsFound(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty})
	if err := ti.writeFile(importPlanFile, models.ImportPlan{Character: "Testmage-Area52"}); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	ti.Prepare = func() error { <-release; return nil }
	if err := ti.Find(context.Background(), "us", "Testmage-Area52"); err != nil {
		t.Fatal(err)
	}
	err := ti.Start(context.Background(), "us", "Testmage-Area52", keysOnly)
	_, statErr := os.Stat(filepath.Join(ti.Dir, importPlanFile))
	close(release)
	ti.finish()
	if !errors.Is(err, ErrBusy) || statErr != nil {
		t.Errorf("start during a Find: %v, plan file: %v; want ErrBusy and the plan kept", err, statErr)
	}
}

func TestStartTurnsDownABadCharacter(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty})
	if err := ti.Start(context.Background(), "us", "Testmage-Nowhere", keysOnly); err == nil {
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
	if err := ti.Start(context.Background(), "us", "Testmage-Area52", keysOnly); err != nil {
		t.Fatal(err)
	}
	ti.finish()
	var st importState
	if err := ti.readFile(importStateFile, &st); err != nil || len(st.left()) != 0 {
		t.Errorf("state %+v (%v): want no fights left to resume", st, err)
	}
}

func TestASecondImportCountsOnlyItsOwnFights(t *testing.T) {
	ti := newTestImporter(t, []models.RateLimit{plenty})
	for _, kinds := range [][]models.ImportKind{keysOnly, {models.ImportRaidKills}} {
		if err := ti.Start(context.Background(), "us", "Testmage-Area52", kinds); err != nil {
			t.Fatal(err)
		}
		ti.finish()
	}
	if s := ti.Status(); s.Fights != 1 || s.FightsDone != 1 {
		t.Errorf("status %+v, want the raid kill as 1 of 1 fight, not the keys done before", s)
	}
}
