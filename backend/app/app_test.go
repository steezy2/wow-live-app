package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/clients/keystore"
	"wow-live/backend/models"
	"wow-live/backend/service"
)

func TestLearnSkipsReadLogs(t *testing.T) {
	logs := t.TempDir()
	path := filepath.Join(logs, "WoWCombatLog-111026_190211.txt")
	writeLog(t, logs, filepath.Base(path), time.Hour)
	s := testSite(t, &logs)
	cache := s.opts.Cache
	learn := func(step string, all bool, want int) {
		t.Helper()
		if read, _, err := s.Learn(all); err != nil || read != want {
			t.Fatalf("%s: read %d logs (%v), want %d", step, read, err, want)
		}
	}
	learn("first", false, 1)
	learn("again", false, 0)
	learn("everything again", true, 1)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("11/10/2026 19:02:12.000-7  ZONE_CHANGE,2805,\"Ula'tek\",16\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	learn("grown", false, 1)
	if err := cachefile.WriteJSON(filepath.Join(cache, "learned.json"),
		learned{ReaderVersion: readerVersion - 1, Logs: readLearned(filepath.Join(cache, "learned.json")).Logs}); err != nil {
		t.Fatal(err)
	}
	learn("older reader", false, 1)
	learn("after the older reader", false, 0)
}

// zoneLine is a log line that parses (and saves nothing).
const zoneLine = "11/10/2026 19:02:11.000-7  ZONE_CHANGE,2805,\"Ula'tek\",16\n"

// testSite is a site on a log folder of its own, whose folder Learn reads through dir (so a test can change it).
func testSite(t *testing.T, dir *string) *Site {
	t.Helper()
	store := keystore.Store{Dir: filepath.Join(t.TempDir(), "local-keys")}
	newKeys := func() (*service.LogTracker, error) {
		return service.NewLogTracker(nil, nil, models.MPlusRules{}, nil, store)
	}
	keys, err := newKeys()
	if err != nil {
		t.Fatal(err)
	}
	return &Site{Keys: keys, Changes: &service.Changes{}, newKeys: newKeys, opts: Options{Cache: t.TempDir(),
		LogsDir: func() string { return *dir }, Now: time.Now}}
}

// writeLog writes a log named name into dir, last written ago.
func writeLog(t *testing.T, dir, name string, ago time.Duration) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(zoneLine), 0o644); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-ago)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestLearnUsesCurrentFolder(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	writeLog(t, second, "WoWCombatLog-111026_190211.txt", time.Hour)
	dir := first
	s := testSite(t, &dir)
	if read, _, err := s.Learn(false); err != nil || read != 0 {
		t.Fatalf("empty folder: read %d (%v)", read, err)
	}
	dir = second
	if read, _, err := s.Learn(false); err != nil || read != 1 {
		t.Fatalf("after the folder changed: read %d (%v), want 1", read, err)
	}
}

func TestLogJobsOneAtATime(t *testing.T) {
	dir := t.TempDir()
	s := testSite(t, &dir)
	s.logJob.Lock()
	if _, _, err := s.Learn(false); !errors.Is(err, service.ErrBusy) {
		t.Errorf("Learn while busy: %v", err)
	}
	if _, err := s.ArchiveLogs([]string{"x"}); !errors.Is(err, service.ErrBusy) {
		t.Errorf("ArchiveLogs while busy: %v", err)
	}
	s.logJob.Unlock()
}

func TestArchiveThenReadEverythingAgain(t *testing.T) {
	dir := t.TempDir()
	const old, newest = "WoWCombatLog-110126_190211.txt", "WoWCombatLog-111026_190211.txt"
	writeLog(t, dir, old, 30*24*time.Hour)
	writeLog(t, dir, newest, time.Hour)
	s := testSite(t, &dir)
	if _, err := s.ArchiveLogs([]string{newest}); !errors.Is(err, service.ErrBadLog) {
		t.Fatalf("archiving the newest log: %v, want ErrBadLog", err)
	}
	files, err := s.ArchiveLogs([]string{old}) // not read yet: read first, then zipped
	if err != nil || len(files) != 2 || !files[1].Archived || files[1].Error != "" || !files[1].Read {
		t.Fatalf("after archiving: %+v, %v", files, err)
	}
	for _, tc := range []struct {
		all  bool
		want int
	}{{false, 0}, {true, 2}} {
		if read, _, err := s.Learn(tc.all); err != nil || read != tc.want {
			t.Errorf("Learn(all %v) read %d (%v), want %d", tc.all, read, err, tc.want)
		}
	}
	if files, err = s.DeleteArchivedLogs([]string{old}); err != nil || len(files) != 1 || files[0].Archived {
		t.Fatalf("after deleting: %+v, %v", files, err)
	}
}
