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

func (f *fakeKeys) Saved() []models.KeySummary {
	return []models.KeySummary{{Start: 7, Dungeon: "Voidscar Arena"}}
}

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
func (f *fakeKeys) Ready() (models.ReadyCheck, bool)          { return models.ReadyCheck{}, false }

func (f *fakeKeys) DeleteSaved(start int64) error {
	if start != 7 && start != 0 {
		return service.ErrNoSuchKey
	}
	return nil
}

// fakeRaid has one raid pull, starting at 1.
type fakeRaid struct{}

func (fakeRaid) RaidLive(int64) models.RaidLive { return models.RaidLive{} }
func (fakeRaid) RaidNights() []models.RaidNight { return []models.RaidNight{} }
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

func TestSettingsAreSavedOnlyWithChoicesTheAPIOffers(t *testing.T) {
	for _, tc := range []struct {
		overlay string
		code    int
	}{
		{`{"show": "always", "opacity": 70}`, http.StatusOK},
		{`{"show": "sometimes", "opacity": 70}`, http.StatusBadRequest},
		{`{"show": "keys", "opacity": 120}`, http.StatusBadRequest},
		{`{}`, http.StatusBadRequest},
	} {
		var saved models.Settings
		router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, nil, &SettingsAPI{Settings: fakeSettings{&saved}},
			os.DirFS(t.TempDir()), LocalHosts(port)...)
		body := `{"logsDir": "D:/Logs", "region": "eu", "overlay": ` + tc.overlay + `}`
		r := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.code || (tc.code == http.StatusOK) != (saved.Region == "eu") {
			t.Errorf("overlay %s: %d %s, saved %+v; want %d", tc.overlay, w.Code, w.Body, saved, tc.code)
		}
	}
}
