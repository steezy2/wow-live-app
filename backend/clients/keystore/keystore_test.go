package keystore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

func TestSaveListDeleteClear(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "local-keys"))
	if keys, err := s.List(); err != nil || len(keys) != 0 {
		t.Fatalf("empty store: %v, %v", keys, err)
	}
	for _, start := range []int64{100, 300, 200} {
		if err := s.Save(models.Key{Start: start, Dungeon: "Voidscar Arena"}); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := s.List()
	if err != nil || len(keys) != 3 || keys[0].Start != 300 || keys[2].Start != 100 {
		t.Fatalf("list = %+v, %v; want newest first", keys, err)
	}
	if err := s.SaveCompare(models.PlayerCompare{Kind: models.FightMPlus, Start: 300,
		Player: "Arnaud-Area 52"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(300); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Compare(models.FightMPlus, 300, "Arnaud-Area 52"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a deleted key keeps its players' reviews: %v", err)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if keys, _ := s.List(); len(keys) != 0 {
		t.Fatalf("after clear: %+v", keys)
	}
}

func TestTracksAreGzippedAndPlainOnesStillRead(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "local-keys")
	s := NewStore(dir)
	want := models.KeyTrack{Deaths: []models.TrackPoint{{ClockMS: 5, X: 1.5, Y: 2, Map: 3}}}
	plain := filepath.Join(dir, "tracks", "100.json")
	if err := cachefile.WriteJSON(plain, want); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Track(100); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("plain track = %+v, %v", got, err)
	}
	if n, err := s.CompressTracks(); err != nil || n != 1 {
		t.Fatalf("CompressTracks = %d, %v", n, err)
	}
	if _, err := os.Stat(plain); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the plain copy stays after compressing: %v", err)
	}
	if got, err := s.Track(100); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("gzipped track = %+v, %v", got, err)
	}
	if err := s.Delete(100); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Track(100); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a deleted track reads: %v", err)
	}
}

func TestRecentTracksAreReadOnceUntilTheFileChanges(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "local-keys"))
	if err := s.SaveTrack(1, models.KeyTrack{Health: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Track(1); err != nil {
		t.Fatal(err)
	}
	path := s.trackPath(1)
	info, _ := os.Stat(path)
	if err := os.WriteFile(path, []byte("not gzip"), 0o644); err != nil { // a read of the file now would fail
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Track(1); err != nil || len(got.Health) != 1 {
		t.Fatalf("a track read twice = %+v, %v; want the first read's", got, err)
	}
	later := info.ModTime().Add(time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Track(1); err == nil {
		t.Fatal("a track whose file changed is read from the file again")
	}
}
