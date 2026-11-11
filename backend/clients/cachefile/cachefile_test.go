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
)

func TestFreshTooBig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(strings.Repeat("x", MaxDownload+1)))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "big")
	if err := Fresh(context.Background(), nil, server.URL, path, 0); !errors.Is(err, ErrTooBig) {
		t.Fatalf("err = %v, want ErrTooBig", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a too-big download was written: %v", err)
	}
}
