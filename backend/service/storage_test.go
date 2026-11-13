package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFolderBytes(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, size := range map[string]int{"a.zip": 100, "state.json": 20, filepath.Join("pages", "p.json"): 3} {
		if err := os.WriteFile(filepath.Join(dir, path), make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		dir  string
		want int64
	}{{dir, 123}, {filepath.Join(dir, "pages"), 3}, {filepath.Join(dir, "missing"), 0}} {
		if got, err := FolderBytes(tc.dir); err != nil || got != tc.want {
			t.Errorf("FolderBytes(%s) = %d, %v; want %d", tc.dir, got, err, tc.want)
		}
	}
}
