package routestore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"wow-live/backend/models"
)

func TestSaveGetListDelete(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "routes")}
	if routes, err := s.List(); err != nil || len(routes) != 0 {
		t.Fatalf("empty store: %v, %v", routes, err)
	}
	for id, updated := range map[string]int64{"old": 1, "new": 3, "mid(1)": 2} {
		if err := s.Save(models.Route{ID: id, MDT: []byte{1, 2}, Updated: updated}); err != nil {
			t.Fatal(err)
		}
	}
	routes, err := s.List()
	if err != nil || len(routes) != 3 || routes[0].ID != "new" || routes[2].ID != "old" {
		t.Fatalf("list = %+v, %v; want newest first", routes, err)
	}
	if got, err := s.Get("mid(1)"); err != nil || string(got.MDT) != "\x01\x02" {
		t.Fatalf("get = %+v, %v; want the saved route with its MDT bytes", got, err)
	}
	if err := s.Delete("new"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("new"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("get after delete: %v, want ErrNotExist", err)
	}
}

func TestBadIDsNeverBuildAPath(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for _, id := range []string{"", "../x", `a\b`, "a/b", "a.json", "a b", "0123456789012345678901234567890123"} {
		if err := s.Save(models.Route{ID: id}); err == nil {
			t.Errorf("save %q: no error", id)
		}
		if _, err := s.Get(id); err == nil || errors.Is(err, os.ErrNotExist) {
			t.Errorf("get %q: %v, want a bad id error", id, err)
		}
		if err := s.Delete(id); err == nil {
			t.Errorf("delete %q: no error", id)
		}
	}
	if entries, _ := os.ReadDir(s.Dir); len(entries) != 0 {
		t.Errorf("files written: %v", entries)
	}
}
