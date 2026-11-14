package simc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const listing = `<tr><td><a href="simc-1210.01.73a372b-win64.7z">simc-1210.01.73a372b-win64.7z</a></td><td align="right">2026-10-23 05:13  </td><td align="right">115M</td></tr>
<tr><td><a href="simc-1205.01.ac79c0f-win64.7z">simc-1205.01.ac79c0f-win64.7z</a></td><td align="right">2026-08-11 04:16  </td><td align="right">115M</td></tr>`

func TestLatestReadsTheNewestWindowsBuild(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "C=M;O=D" {
			t.Errorf("query = %q, want the list sorted newest first", r.URL.RawQuery)
		}
		w.Write([]byte(listing))
	}))
	defer srv.Close()
	got, err := Client{Base: srv.URL + "/"}.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 23, 5, 13, 0, 0, time.UTC).UnixMilli()
	if got.File != "simc-1210.01.73a372b-win64.7z" || got.URL != srv.URL+"/simc-1210.01.73a372b-win64.7z" || got.BuiltMS != want {
		t.Errorf("Latest = %+v", got)
	}
}

func TestLatestFailsWithoutAWindowsBuild(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html></html>")) }))
	defer srv.Close()
	if _, err := (Client{Base: srv.URL + "/"}).Latest(context.Background()); err == nil {
		t.Error("an empty list should fail")
	}
}

func TestCopyExe(t *testing.T) {
	program := &fstest.MapFile{Data: []byte("MZ\x90\x00program")}
	tests := []struct {
		name    string
		fsys    fstest.MapFS
		wantErr string
	}{
		{"in the folder the nightly uses", fstest.MapFS{"simc-1210/simc.exe": program, "simc-1210/Qt6Core.dll": program}, ""},
		{"at the top", fstest.MapFS{"simc.exe": program}, ""},
		{"not a program", fstest.MapFS{"simc-1210/simc.exe": {Data: []byte("<html>")}}, "not a Windows program"},
		{"missing", fstest.MapFS{"simc-1210/other.exe": program}, "no simc.exe"},
		{"too deep to be the engine", fstest.MapFS{"a/b/c/simc.exe": program}, "no simc.exe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "out.exe")
			err := copyExe(tt.fsys, dest)
			if tt.wantErr == "" {
				if data, readErr := os.ReadFile(dest); err != nil || readErr != nil || !strings.HasPrefix(string(data), "MZ") {
					t.Errorf("copyExe = %v, %v, want the program copied", err, readErr)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestInstallRefusesAFileThatIsNotA7z(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".7z") {
			w.Write([]byte("<html>captive portal</html>"))
			return
		}
		w.Write([]byte(listing))
	}))
	defer srv.Close()
	dir := t.TempDir()
	c := Client{Dir: dir, Base: srv.URL + "/"}
	if _, err := c.Install(context.Background()); err == nil {
		t.Fatal("Install should fail")
	}
	if c.Installed().Installed {
		t.Error("a failed install should leave no engine")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*")); len(left) != 0 {
		t.Errorf("a failed install left %v", left)
	}
}

func TestParseBanner(t *testing.T) {
	version, patch, ok := parseBanner("Nothing to sim! SimulationCraft 1210-01 for World of Warcraft 12.1.5.70529 Live (hotfix 2026-10-22/70529)")
	if !ok || version != "1210-01" || patch != "12.1.5.70529" {
		t.Errorf("parseBanner = %q, %q, %v", version, patch, ok)
	}
	if _, _, ok := parseBanner("not simc"); ok {
		t.Error("another program's output should not parse")
	}
}
