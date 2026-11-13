package controller

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wow-live/backend/models"
	"wow-live/backend/service"
)

type fakeKeys struct {
	key models.KeyView
	ok  bool
	at  int64
}

func (f *fakeKeys) Latest(now int64) (models.KeyView, bool) {
	f.at = now
	return f.key, f.ok
}

func (f *fakeKeys) Saved(string) []models.KeySummary {
	return []models.KeySummary{{Start: 7, Dungeon: "Voidscar Arena"}}
}

func (f *fakeKeys) Played(string) []models.Character { return []models.Character{} }

func (f *fakeKeys) SavedKey(start int64) (models.KeyView, error) {
	if start != 7 {
		return models.KeyView{}, service.ErrNoSuchKey
	}
	return f.key, nil
}

func (f *fakeKeys) Track(int64) (models.KeyTrack, error) {
	return models.KeyTrack{}, service.ErrNoSuchKey
}

func (f *fakeKeys) DungeonMap(int) (models.DungeonMap, error) { return models.DungeonMap{}, nil }
func (f *fakeKeys) Abandon() bool                             { return f.ok }

func (f *fakeKeys) DeleteSaved(start int64) error {
	if start != 7 && start != 0 {
		return service.ErrNoSuchKey
	}
	return nil
}

// fakeRaid has one raid pull, starting at 1.
type fakeRaid struct{}

func (fakeRaid) RaidLive(int64) models.RaidLive       { return models.RaidLive{} }
func (fakeRaid) RaidNights(string) []models.RaidNight { return []models.RaidNight{} }
func (fakeRaid) RaidPull(start int64) (models.RaidPull, error) {
	if start != 1 {
		return models.RaidPull{}, service.ErrNoSuchPull
	}
	return models.RaidPull{Start: 1}, nil
}
func (f fakeRaid) RaidTrack(start int64) (models.KeyTrack, error) {
	_, err := f.RaidPull(start)
	return models.KeyTrack{}, err
}

// fakeMaps has one raid room's map, 2610.
type fakeMaps struct{}

func (fakeMaps) Image(_ context.Context, uiMapID int) ([]byte, error) {
	if uiMapID != 2610 {
		return nil, service.ErrNoSuchMap
	}
	return []byte("png"), nil
}

func TestSavedKeysAndRaidPulls(t *testing.T) {
	router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, &Raid{Pulls: fakeRaid{}, Maps: fakeMaps{}, Now: time.Now}, nil,
		os.DirFS(t.TempDir()), LocalHosts(port)...)
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{http.MethodGet, "/api/mplus/keys", http.StatusOK},
		{http.MethodGet, "/api/mplus/keys/7", http.StatusOK},
		{http.MethodGet, "/api/mplus/keys/8", http.StatusNotFound},
		{http.MethodGet, "/api/mplus/keys/x", http.StatusBadRequest},
		{http.MethodDelete, "/api/mplus/keys/7", http.StatusNoContent},
		{http.MethodDelete, "/api/mplus/keys", http.StatusNoContent},
		{http.MethodGet, "/api/raid/live", http.StatusOK},
		{http.MethodGet, "/api/raid/nights", http.StatusOK},
		{http.MethodGet, "/api/raid/pulls/1", http.StatusOK},
		{http.MethodGet, "/api/raid/pulls/2", http.StatusNotFound},
		{http.MethodGet, "/api/raid/pulls/x", http.StatusBadRequest},
		{http.MethodGet, "/api/raid/pulls/1/track", http.StatusOK},
		{http.MethodGet, "/api/raid/pulls/2/track", http.StatusNotFound},
		{http.MethodGet, "/api/raid/maps/2610", http.StatusOK},
		{http.MethodGet, "/api/raid/maps/1", http.StatusNotFound},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Errorf("%s %s: %d, want %d", tc.method, tc.path, w.Code, tc.code)
		}
	}
}

const port = 8080

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Host = "localhost:8080"
	handler.ServeHTTP(w, r)
	return w
}

func TestLiveKeyIsReadAtNowOrNoContentBeforeAnyKey(t *testing.T) {
	keys := &fakeKeys{key: models.KeyView{Key: models.Key{Dungeon: "Voidscar Arena", Level: 17}, ClockMS: 75_000},
		ok: true}
	now := time.UnixMilli(1_793_800_000_000)
	router := Router(MPlus{Keys: keys, Now: func() time.Time { return now }}, nil, nil, os.DirFS(t.TempDir()), LocalHosts(port)...)
	w := get(t, router, "/api/mplus/live")
	var got models.KeyView
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK ||
		got.Dungeon != "Voidscar Arena" || got.ClockMS != 75_000 || keys.at != now.UnixMilli() {
		t.Fatalf("got %d %s (%v), read at %d", w.Code, w.Body, err, keys.at)
	}
	keys.ok = false
	if w := get(t, router, "/api/mplus/live"); w.Code != http.StatusNoContent {
		t.Fatalf("before any key: %d, want 204", w.Code)
	}
}

func TestAbandonEndsTheRunningKeyOrConflictsWithoutOne(t *testing.T) {
	keys := &fakeKeys{}
	router := Router(MPlus{Keys: keys, Now: time.Now}, nil, nil, os.DirFS(t.TempDir()), LocalHosts(port)...)
	for _, tc := range []struct {
		running bool
		code    int
	}{{true, http.StatusNoContent}, {false, http.StatusConflict}} {
		keys.ok = tc.running
		r := httptest.NewRequest(http.MethodPost, "/api/mplus/live/abandon", nil)
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Errorf("abandon with a key running %v: %d, want %d", tc.running, w.Code, tc.code)
		}
	}
}

func TestPagesAndUnknownAPIPaths(t *testing.T) {
	pages := t.TempDir()
	if err := os.WriteFile(filepath.Join(pages, "index.html"), []byte("page"), 0o644); err != nil {
		t.Fatal(err)
	}
	router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, nil, nil, os.DirFS(pages), LocalHosts(port)...)
	if w := get(t, router, "/mplus"); w.Code != http.StatusOK || w.Body.String() != "page" {
		t.Fatalf("a page route: %d %q, want index.html", w.Code, w.Body)
	}
	if w := get(t, router, "/../../secret"); w.Code != http.StatusBadRequest && w.Body.String() != "page" {
		t.Fatalf("a path out of the pages folder: %d %q, want refused or index.html", w.Code, w.Body)
	}
	if w := get(t, router, "/api/nope"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown API path: %d, want 404", w.Code)
	}
	for _, tc := range []struct {
		name, host, origin string
		code               int
	}{
		{"another host name", "evil.example:8080", "", http.StatusMisdirectedRequest},
		{"another website's page", "localhost:8080", "https://evil.example", http.StatusForbidden},
		{"this site's page", "localhost:8080", "http://localhost:8080", http.StatusNoContent},
	} {
		r := httptest.NewRequest(http.MethodDelete, "/api/mplus/keys", nil)
		r.Host = tc.host
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Errorf("a request from %s: %d, want %d", tc.name, w.Code, tc.code)
		}
	}
}

func TestEventsStreamAChangeNotice(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	changes := &service.Changes{}
	router := Router(MPlus{Keys: &fakeKeys{}, Changes: changes, Now: time.Now}, nil, nil, os.DirFS(t.TempDir()),
		LocalHosts(listener.Addr().(*net.TCPAddr).Port)...)
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: router}}
	server.Start()
	defer server.Close()
	resp, err := http.Get(server.URL + "/api/mplus/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	go func() {
		time.Sleep(50 * time.Millisecond)
		changes.Publish(4)
	}()
	lines := bufio.NewScanner(resp.Body)
	for lines.Scan() {
		if line := lines.Text(); strings.HasPrefix(line, "data:") {
			if resp.Header.Get("Content-Type") != "text/event-stream" || line != `data:{"version":4}` {
				t.Fatalf("got %q (%s), want the version 4 notice as an event stream", line,
					resp.Header.Get("Content-Type"))
			}
			return
		}
	}
	t.Fatalf("the stream ended without a notice: %v", lines.Err())
}

type fakeWCLKeys struct{ err error }

func (f fakeWCLKeys) Status() models.KeyStatus { return models.KeyStatus{Set: true, IDEnd: "1234"} }
func (f fakeWCLKeys) Save(context.Context, models.Credentials, bool) (models.KeyStatus, error) {
	return f.Status(), f.err
}
func (f fakeWCLKeys) Retest(context.Context) (models.KeyStatus, error) { return f.Status(), f.err }
func (f fakeWCLKeys) Remove() error                                    { return f.err }

func TestSavingWCLKeysAnswersWithTheTestNeverTheSecret(t *testing.T) {
	for _, tc := range []struct {
		body string
		err  error
		code int
	}{
		{`{"id": "abcd-1234", "secret": "s3cret"}`, nil, http.StatusOK},
		{`{"id": " ", "secret": "s3cret"}`, nil, http.StatusBadRequest},
		{`{"id": "abcd-1234", "secret": "s3cret"}`, models.ErrBadCredentials, http.StatusUnprocessableEntity},
		{`{"id": "abcd-1234", "secret": "s3cret"}`, errors.New("dial tcp: s3cret"), http.StatusBadGateway},
		{`{"id": "abcd-1234", "secret": "s3cret"}`, fmt.Errorf("%w: access denied", service.ErrKeyStore),
			http.StatusInternalServerError},
	} {
		router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, nil, &SettingsAPI{Keys: fakeWCLKeys{tc.err}},
			os.DirFS(t.TempDir()), LocalHosts(port)...)
		r := httptest.NewRequest(http.MethodPut, "/api/settings/warcraftlogs", strings.NewReader(tc.body))
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.code || strings.Contains(w.Body.String(), "s3cret") {
			t.Errorf("%s with %v: %d %s, want %d and no secret", tc.body, tc.err, w.Code, w.Body, tc.code)
		}
	}
}

// fakeSettings keeps what's saved.
type fakeSettings struct{ saved *models.Settings }

func (f fakeSettings) Get() models.SettingsView     { return models.SettingsView{Settings: *f.saved} }
func (f fakeSettings) Save(s models.Settings) error { *f.saved = s; return nil }

// fakeCharacters knows one character, Testmage-Somerealm, and guesses Testrogue.
type fakeCharacters struct{ err error }

func (f fakeCharacters) Resolve(_ context.Context, _, character string) (string, string, error) {
	if f.err == nil && character != "Testmage-Somerealm" {
		return "", "", service.ErrBadCharacter
	}
	return "Testmage", "some-realm", f.err
}
func (fakeCharacters) Guess() string { return "Testrogue" }
func (f fakeCharacters) Parses(context.Context, string, string) (models.CharacterParses, error) {
	return models.CharacterParses{}, f.err
}

func TestCharacterIsSavedOnlyWhenWarcraftLogsKnowsItsRealm(t *testing.T) {
	for _, tc := range []struct {
		character string
		err       error
		code      int
	}{
		{"Testmage-Somerealm", nil, http.StatusOK},
		{" Testmage-Somerealm ", nil, http.StatusOK},
		{"", nil, http.StatusOK},
		{"Testmage-Nowhere", nil, http.StatusBadRequest},
		{"Testmage-Somerealm", models.ErrNoCredentials, http.StatusNotFound},
		{"Testmage-Somerealm", errors.New("down"), http.StatusBadGateway},
	} {
		var saved models.Settings
		router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, nil, &SettingsAPI{Settings: fakeSettings{&saved},
			Characters: fakeCharacters{tc.err}}, os.DirFS(t.TempDir()), LocalHosts(port)...)
		body, _ := json.Marshal(map[string]any{"logsDir": t.TempDir(), "region": "us", "character": tc.character,
			"overlay": map[string]any{"show": "keys", "opacity": 100}})
		r := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(string(body)))
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		var got models.SettingsView
		json.Unmarshal(w.Body.Bytes(), &got)
		if w.Code != tc.code || (tc.code == http.StatusOK) != (saved.Region == "us") ||
			tc.code == http.StatusOK && (got.CharacterGuess != "Testrogue" || saved.Character != strings.TrimSpace(tc.character)) {
			t.Errorf("%q (%v): %d %s, saved %+v; want %d", tc.character, tc.err, w.Code, w.Body, saved, tc.code)
		}
	}
}

func TestSettingsAreSavedOnlyWithChoicesTheAPIOffers(t *testing.T) {
	full, _ := json.Marshal(t.TempDir()) // a full path on any OS
	for _, tc := range []struct {
		logsDir, overlay string
		code             int
	}{
		{string(full), `{"show": "always", "opacity": 70}`, http.StatusOK},
		{string(full), `{"show": "sometimes", "opacity": 70}`, http.StatusBadRequest},
		{string(full), `{"show": "keys", "opacity": 120}`, http.StatusBadRequest},
		{string(full), `{"show": "keys", "opacity": 90, "rows": 5, "columns": ["dps", "taken"]}`, http.StatusOK},
		{string(full), `{"show": "keys", "opacity": 90, "columns": ["dps", "mana"]}`, http.StatusBadRequest},
		{string(full), `{"show": "keys", "opacity": 90, "rows": -1}`, http.StatusBadRequest},
		{string(full), `{"show": "keys", "opacity": 90}, "auto": {"archiveGameLogsDays": 365, "deleteImportsDays": 0}`,
			http.StatusOK},
		{string(full), `{"show": "keys", "opacity": 90}, "auto": {"archiveGameLogsDays": 366}`, http.StatusBadRequest},
		{string(full), `{"show": "keys", "opacity": 90}, "auto": {"deleteImportsDays": -1}`, http.StatusBadRequest},
		{string(full), `{}`, http.StatusBadRequest},
		{`"Logs"`, `{"show": "always", "opacity": 70}`, http.StatusBadRequest},
		{`""`, `{"show": "always", "opacity": 70}`, http.StatusBadRequest},
	} {
		var saved models.Settings
		router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, nil, &SettingsAPI{Settings: fakeSettings{&saved},
			Characters: fakeCharacters{}}, os.DirFS(t.TempDir()), LocalHosts(port)...)
		body := `{"logsDir": ` + tc.logsDir + `, "region": "eu", "overlay": ` + tc.overlay + `}`
		r := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.code || (tc.code == http.StatusOK) != (saved.Region == "eu") {
			t.Errorf("%s, overlay %s: %d %s, saved %+v; want %d", tc.logsDir, tc.overlay, w.Code, w.Body, saved, tc.code)
		}
	}
}

func TestAllAndForceAreBooleans(t *testing.T) {
	for _, tc := range []struct {
		method, url string
		code        int
		all         bool // what Read old logs was asked for
	}{
		{http.MethodPost, "/api/settings/read-keys", http.StatusOK, false},
		{http.MethodPost, "/api/settings/read-keys?all=1", http.StatusOK, true},
		{http.MethodPost, "/api/settings/read-keys?all=yes", http.StatusBadRequest, false},
		{http.MethodPut, "/api/settings/warcraftlogs", http.StatusOK, false},
		{http.MethodPut, "/api/settings/warcraftlogs?force=true", http.StatusOK, false},
		{http.MethodPut, "/api/settings/warcraftlogs?force=maybe", http.StatusBadRequest, false},
	} {
		var all bool
		api := &SettingsAPI{Keys: fakeWCLKeys{}, Logs: &fakeLogs{all: &all}}
		router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, nil, api, os.DirFS(t.TempDir()), LocalHosts(port)...)
		r := httptest.NewRequest(tc.method, tc.url, strings.NewReader(`{"id": "abcd-1234", "secret": "s3cret"}`))
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.code || all != tc.all {
			t.Errorf("%s %s: %d, all=%v; want %d, %v", tc.method, tc.url, w.Code, all, tc.code, tc.all)
		}
	}
}

// fakeLogs is a LogFolder that notes what it was asked and fails with err.
type fakeLogs struct {
	all    *bool
	source string
	names  []string
	err    error
}

func (f *fakeLogs) Learn(all bool) (int, int, error) {
	if f.all != nil {
		*f.all = all
	}
	return 0, 0, f.err
}
func (f *fakeLogs) LogFiles() ([]models.LogFile, error) { return []models.LogFile{}, f.err }
func (f *fakeLogs) ArchiveLogs(source string, names []string) ([]models.LogFile, error) {
	f.source, f.names = source, names
	return []models.LogFile{}, f.err
}
func (f *fakeLogs) DeleteLogs(source string, names []string) ([]models.LogFile, error) {
	return f.ArchiveLogs(source, names)
}
func (f *fakeLogs) Storage() (models.Storage, error) { return models.Storage{}, f.err }

func TestLogFolderAnswers(t *testing.T) {
	for _, tc := range []struct {
		method, url, body string
		err               error
		code              int
		source            string // the source the folder was asked for
	}{
		{http.MethodGet, "/api/settings/logs", "", nil, http.StatusOK, ""},
		{http.MethodGet, "/api/storage", "", nil, http.StatusOK, ""},
		{http.MethodGet, "/api/storage", "", errors.New("disk"), http.StatusInternalServerError, ""},
		{http.MethodPost, "/api/settings/logs/archive", `{"names": ["a.txt"]}`, nil, http.StatusOK, "game"},
		{http.MethodDelete, "/api/settings/logs/archive", `{"names": ["a.txt"]}`, nil, http.StatusOK, "game"},
		{http.MethodDelete, "/api/settings/logs/archive", `{"names": ["a.txt"], "source": "wcl"}`, nil, http.StatusOK,
			"wcl"},
		{http.MethodDelete, "/api/settings/logs/archive", `{"names": ["a.txt"], "source": "disk"}`, nil,
			http.StatusBadRequest, ""},
		{http.MethodPost, "/api/settings/logs/archive", `{"names": []}`, nil, http.StatusBadRequest, ""},
		{http.MethodPost, "/api/settings/logs/archive", `{}`, nil, http.StatusBadRequest, ""},
		{http.MethodPost, "/api/settings/logs/archive", `{"names": ["../x"]}`, service.ErrBadLog,
			http.StatusBadRequest, "game"},
		{http.MethodPost, "/api/settings/logs/archive", `{"names": ["a.txt"]}`, service.ErrBusy,
			http.StatusConflict, "game"},
		{http.MethodPost, "/api/settings/read-keys", "", service.ErrBusy, http.StatusConflict, ""},
		{http.MethodGet, "/api/settings/logs", "", errors.New("disk"), http.StatusInternalServerError, ""},
	} {
		logs := &fakeLogs{err: tc.err}
		api := &SettingsAPI{Keys: fakeWCLKeys{}, Logs: logs}
		router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, nil, api, os.DirFS(t.TempDir()), LocalHosts(port)...)
		r := httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.code || logs.source != tc.source {
			t.Errorf("%s %s %s (%v): %d %s, source %q; want %d, %q", tc.method, tc.url, tc.body, tc.err, w.Code, w.Body,
				logs.source, tc.code, tc.source)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	pages := t.TempDir()
	if err := os.WriteFile(filepath.Join(pages, "index.html"), []byte("<p>page</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, nil, nil, os.DirFS(pages), LocalHosts(port)...)
	for _, tc := range []struct {
		url    string
		policy bool
	}{{"/", true}, {"/raid", true}, {"/api/mplus/keys", false}} {
		r := httptest.NewRequest(http.MethodGet, tc.url, nil)
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		h := w.Header()
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") != "no-referrer" ||
			(h.Get("Content-Security-Policy") != "") != tc.policy {
			t.Errorf("%s: headers %v", tc.url, h)
		}
	}
}

// fakeImport is an import whose Start and Find fail with err.
type fakeImport struct{ err error }

func (f fakeImport) Start(context.Context, string, string, []models.ImportKind) error { return f.err }
func (f fakeImport) Find(context.Context, string, string) error                       { return f.err }
func (f fakeImport) Stop()                                                            {}
func (f fakeImport) Status() models.ImportStatus                                      { return models.ImportStatus{State: "listing"} }
func (f fakeImport) Remove() error                                                    { return f.err }

func TestImportNeedsACharacterKindsAndOneImportAtATime(t *testing.T) {
	const keys = `{"kinds": ["keys", "raidKills"]}`
	for _, tc := range []struct {
		path, body, character string
		err                   error
		code                  int
	}{
		{"", keys, "Testmage-Somerealm", nil, http.StatusAccepted},
		{"", keys, "", nil, http.StatusBadRequest},
		{"", keys, "Testmage-Somerealm", service.ErrBusy, http.StatusConflict},
		{"", keys, "Testmage-Somerealm", service.ErrBadCharacter, http.StatusBadRequest},
		{"", keys, "Testmage-Somerealm", models.ErrNoCredentials, http.StatusNotFound},
		{"", "", "Testmage-Somerealm", nil, http.StatusBadRequest},
		{"", `{"kinds": []}`, "Testmage-Somerealm", nil, http.StatusBadRequest},
		{"", `{"kinds": ["keys", "trash"]}`, "Testmage-Somerealm", nil, http.StatusBadRequest},
		{"", `{"kinds": ["keys", "keys"]}`, "Testmage-Somerealm", nil, http.StatusBadRequest},
		{"/plan", "", "Testmage-Somerealm", nil, http.StatusAccepted},
		{"/plan", "", "", nil, http.StatusBadRequest},
		{"/plan", "", "Testmage-Somerealm", service.ErrBusy, http.StatusConflict},
		{"/plan", "", "Testmage-Somerealm", errors.New("down"), http.StatusBadGateway},
	} {
		saved := models.Settings{Region: "us", Character: tc.character}
		router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, nil, &SettingsAPI{Settings: fakeSettings{&saved},
			Import: fakeImport{tc.err}}, os.DirFS(t.TempDir()), LocalHosts(port)...)
		r := httptest.NewRequest(http.MethodPost, "/api/settings/warcraftlogs/import"+tc.path, strings.NewReader(tc.body))
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Errorf("%s %s %q (%v): %d %s, want %d", tc.path, tc.body, tc.character, tc.err, w.Code, w.Body, tc.code)
		}
	}
}

func TestAnUnchangedCharacterIsntCheckedAgain(t *testing.T) {
	saved := models.Settings{Region: "us", Character: "Testmage-Somerealm"}
	router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, nil, &SettingsAPI{Settings: fakeSettings{&saved},
		Characters: fakeCharacters{errors.New("Warcraft Logs is down")}}, os.DirFS(t.TempDir()), LocalHosts(port)...)
	body, _ := json.Marshal(map[string]any{"logsDir": t.TempDir(), "region": "us", "character": saved.Character,
		"overlay": map[string]any{"show": "keys", "opacity": 50}})
	r := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(string(body)))
	r.Host = "localhost:8080"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusOK || saved.Overlay.Opacity != 50 {
		t.Errorf("%d %s: want the overlay saved without asking Warcraft Logs", w.Code, w.Body)
	}
}

// fakeRecaps answers a recap of the span it was asked for.
type fakeRecaps struct{}

func (fakeRecaps) TopTimes(context.Context, int64, int64, string) ([]models.RecapTopTime, error) {
	return nil, models.ErrNoCredentials
}
func (fakeRecaps) Periods(context.Context) []models.RecapPeriod { return []models.RecapPeriod{} }
func (fakeRecaps) Recap(from, to int64, _ string) models.Recap {
	return models.Recap{From: from, To: to}
}

func TestRecapNeedsASpan(t *testing.T) {
	for _, tc := range []struct {
		url  string
		code int
	}{
		{"/api/recap?from=0&to=100", http.StatusOK},
		{"/api/recap?from=100&to=100", http.StatusBadRequest},
		{"/api/recap?from=5", http.StatusBadRequest},
		{"/api/recap?from=-1&to=100", http.StatusBadRequest},
		{"/api/recap?from=x&to=100", http.StatusBadRequest},
	} {
		router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now, Recaps: fakeRecaps{}}, nil, nil,
			os.DirFS(t.TempDir()), LocalHosts(port)...)
		r := httptest.NewRequest(http.MethodGet, tc.url, nil)
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Errorf("%s: %d %s; want %d", tc.url, w.Code, w.Body, tc.code)
		}
	}
}

type fakeSpells struct{}

func (fakeSpells) Info(_ context.Context, name string) (models.SpellInfo, error) {
	if name == "Icebound Fortitude" {
		return models.SpellInfo{ID: 48792, Name: name, Cooldown: "2 min cooldown"}, nil
	}
	return models.SpellInfo{}, service.ErrUnknownSpell
}

func TestSpellAnswersByAbilityName(t *testing.T) {
	for _, tc := range []struct {
		url  string
		code int
	}{
		{"/api/spells?name=Icebound+Fortitude", http.StatusOK},
		{"/api/spells?name=Nothing", http.StatusNotFound},
		{"/api/spells", http.StatusBadRequest},
	} {
		router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now, Spells: fakeSpells{}}, nil, nil,
			os.DirFS(t.TempDir()), LocalHosts(port)...)
		r := httptest.NewRequest(http.MethodGet, tc.url, nil)
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Errorf("%s: %d %s; want %d", tc.url, w.Code, w.Body, tc.code)
		}
	}
}
