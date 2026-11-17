package importstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreKeepsFilesAndForgivesMissingOnes(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "wcl-import")}
	got := map[string]int{"kept": 1}
	if err := s.Read("state.json", &got); err != nil || got["kept"] != 1 {
		t.Fatalf("reading a missing file = %v, %v; want it to leave out as it was", got, err)
	}
	if err := s.Write("state.json", map[string]int{"n": 2}); err != nil {
		t.Fatal(err)
	}
	var back map[string]int
	if err := s.Read("state.json", &back); err != nil || back["n"] != 2 {
		t.Fatalf("read back = %v, %v; want n=2", back, err)
	}
	if err := s.Remove("state.json"); err != nil || s.Remove("state.json") != nil {
		t.Fatalf("removing twice: %v", err)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Dir); !os.IsNotExist(err) {
		t.Errorf("folder after Clear: %v, want it gone", err)
	}
}
