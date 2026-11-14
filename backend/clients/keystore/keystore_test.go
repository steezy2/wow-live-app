package keystore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"wow-live/backend/models"
)

func TestSaveListDeleteClear(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "local-keys")}
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
	if err := s.SaveCompare(models.PlayerCompare{Kind: models.FightMPlus, Start: 300, Player: "Arnaud-Area 52"}); err != nil {
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
