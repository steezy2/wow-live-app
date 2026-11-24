package simc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// engineServer serves a release holding body as simc.exe, with a manifest naming sum as its sha256 and version as the
// program's version and patch ("1210-01 12.1.5.70783", "" for none).
func engineServer(t *testing.T, body, sum, version string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/engine.json":
			v, p, _ := strings.Cut(version, " ")
			fmt.Fprintf(w, `{"file":"simc-1210.01.73a372b-win64.7z","builtMS":1,"sha256":%q,"version":%q,"patch":%q}`, sum, v, p)
		case "/simc.exe":
			w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sumOf(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestInstallRefusesWhatIsNotAsTheManifestSays(t *testing.T) {
	program := "MZ\x90\x00program"
	tests := []struct {
		name, body, sum, wantErr string
	}{
		{"a swapped file", "MZ\x90\x00evil", sumOf(program), "sha256"},
		{"not a program", "<html>captive portal</html>", sumOf("<html>captive portal</html>"), "not a Windows program"},
		{"no checksum", program, "", "no file or checksum"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := engineServer(t, tt.body, tt.sum, "")
			dir := t.TempDir()
			c := Client{Dir: dir, Base: srv.URL + "/"}
			if _, err := c.Install(context.Background()); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if c.Installed().Installed {
				t.Error("a failed install should leave no engine")
			}
			if left, _ := filepath.Glob(filepath.Join(dir, "*")); len(left) != 0 {
				t.Errorf("a failed install left %v", left)
			}
		})
	}
}

func TestInstallTrustsTheManifestsVersionWithoutRunningTheFile(t *testing.T) {
	program := "MZ\x90\x00not really runnable"
	srv := engineServer(t, program, sumOf(program), "1210-01 12.1.5.70783")
	c := Client{Dir: t.TempDir(), Base: srv.URL + "/"}
	got, err := c.Install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Installed || got.Version != "1210-01" || got.Patch != "12.1.5.70783" || c.Installed().Version != "1210-01" {
		t.Errorf("Install = %+v, installed %+v; want the manifest's version kept", got, c.Installed())
	}
	if updated := time.UnixMilli(c.Installed().UpdatedMS); time.Since(updated) > time.Minute {
		t.Errorf("updated %v, want when the file was put in place, which is just now", updated)
	}
}

func TestUseFileRefusesWhatIsNotAProgram(t *testing.T) {
	dir := t.TempDir()
	text := filepath.Join(dir, "notes.txt")
	notProgram := filepath.Join(dir, "simc.exe")
	for _, f := range []string{text, notProgram} {
		if err := os.WriteFile(f, []byte("not a program"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c := Client{Dir: filepath.Join(dir, "engine")}
	for _, src := range []string{text, notProgram, filepath.Join(dir, "missing.exe"), dir} {
		if _, err := c.UseFile(context.Background(), src); err == nil {
			t.Errorf("UseFile(%s) should fail", src)
		}
	}
	if c.Installed().Installed {
		t.Error("a refused file should leave no engine")
	}
}

func TestParseBanner(t *testing.T) {
	version, patch, ok := parseBanner(
		"Nothing to sim! SimulationCraft 1210-01 for World of Warcraft 12.1.5.70529 Live (hotfix 2026-10-22/70529)")
	if !ok || version != "1210-01" || patch != "12.1.5.70529" {
		t.Errorf("parseBanner = %q, %q, %v", version, patch, ok)
	}
	if _, _, ok := parseBanner("not simc"); ok {
		t.Error("another program's output should not parse")
	}
}
