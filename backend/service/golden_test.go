package service

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"wow-live/backend/clients/cachefile"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// golden checks that value is saved as the bytes in testdata/name (cachefile.Marshal, what the stores write), or
// rewrites that file with -update. A change that should leave saved keys and pulls as they were must not touch these.
func golden(t *testing.T, name string, value any) {
	t.Helper()
	got, err := cachefile.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (go test ./service -run Golden -update writes it)", err)
	}
	if string(append(got, '\n')) != string(want) {
		t.Errorf("%s differs from what is saved now; if the change is meant to save something else, bump readerVersion "+
			"and run go test ./service -run Golden -update", name)
	}
}

func TestGoldenSavedKeyAndTrack(t *testing.T) {
	keys := newTracker(t, nil)
	feed(t, keys, shortKeyLines()...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	store := keys.store.(*memStore)
	golden(t, "key.golden.json", store.keys[0])
	golden(t, "key-track.golden.json", store.tracks[store.keys[0].Start])
}

func TestGoldenSavedPullAndTrack(t *testing.T) {
	keys, store := newRaidTracker(t)
	died := func(s float64, unit string) string { return raidAt(3, s, "UNIT_DIED,"+noUnit+","+unit+",0") }
	feed(t, keys, pullLines(3, 0, 120, 40, false,
		hitOn(3, 9, "SPELL_DAMAGE", shamanUnit, 1001, "Caustic Waves"),
		hitOn(3, 30, "SPELL_DAMAGE", priestUnit, 1002, "Spectral Coils"),
		died(100, priestUnit),
	)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	golden(t, "pull.golden.json", store.pulls[0])
	golden(t, "pull-track.golden.json", store.tracks[store.pulls[0].Start])
}
