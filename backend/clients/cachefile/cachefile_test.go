package cachefile

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFreshTooBig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(strings.Repeat("x", MaxDownload+1)))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "big")
	if _, err := Fresh(context.Background(), nil, server.URL, path, 0); !errors.Is(err, ErrTooBig) {
		t.Fatalf("err = %v, want ErrTooBig", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a too-big download was written: %v", err)
	}
}

func TestFreshKeepsStaleCopyWhenDownloadFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "cached")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if data, err := Fresh(context.Background(), nil, server.URL, path, time.Hour); err != nil || string(data) != "old" {
		t.Fatalf("a stale copy should stand in while the site is down: %q, %v", data, err)
	}
	if _, err := Fresh(context.Background(), nil, server.URL, filepath.Join(t.TempDir(), "none"), time.Hour); err == nil {
		t.Fatal("with no copy at all the failure must surface")
	}
}

func TestFreshReturnsTheDownloadThenTheKeptFile(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Write([]byte("body"))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "kept")
	for range 2 {
		data, err := Fresh(context.Background(), nil, server.URL, path, Forever)
		if err != nil || string(data) != "body" {
			t.Fatalf("Fresh = %q, %v; want body", data, err)
		}
	}
	if hits != 1 {
		t.Errorf("downloaded %d times, want once", hits)
	}
}

func TestReadJSONMissingWrapsNotExist(t *testing.T) {
	if _, err := ReadJSON[map[string]int](filepath.Join(t.TempDir(), "none.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want os.ErrNotExist", err)
	}
}

func TestReadAllReadsPlainAndGzippedFiles(t *testing.T) {
	type row struct{ N int }
	dir := t.TempDir()
	for name, n := range map[string]int{"a.json": 1, "b.json.gz": 2} {
		if err := WriteJSON(filepath.Join(dir, name), row{n}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ReadAll[row](dir)
	if err != nil || len(got) != 2 {
		t.Fatalf("ReadAll = %v, %v, want both files", got, err)
	}
	if got[0].N+got[1].N != 3 {
		t.Errorf("rows = %v, want 1 and 2", got)
	}
}
