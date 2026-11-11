package app

import (
	"os"
	"path/filepath"
	"testing"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/clients/keystore"
	"wow-live/backend/models"
	"wow-live/backend/service"
)

func TestLearnSkipsReadLogs(t *testing.T) {
	logs, cache := t.TempDir(), t.TempDir()
	path := filepath.Join(logs, "WoWCombatLog-111026_190211.txt")
	if err := os.WriteFile(path, []byte("11/10/2026 19:02:11.000-7  ZONE_CHANGE,2805,\"Ula'tek\",16\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := keystore.Store{Dir: filepath.Join(cache, "local-keys")}
	newKeys := func() (*service.LogTracker, error) {
		return service.NewLogTracker(nil, nil, models.MPlusRules{}, nil, store)
	}
	keys, err := newKeys()
	if err != nil {
		t.Fatal(err)
	}
	s := &Site{Keys: keys, Changes: &service.Changes{}, opts: Options{Cache: cache, LogsDir: logs}, newKeys: newKeys}
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
