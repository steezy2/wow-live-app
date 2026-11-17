package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"wow-live/backend/models"
	"wow-live/backend/service"
)

// fakeSims fails every call with err.
type fakeSims struct{ err error }

func (f fakeSims) Choices() models.SimChoices {
	return models.SimChoices{FightStyles: []string{"Patchwerk"}}
}
func (f fakeSims) EngineStatus() models.SimEngineStatus { return models.SimEngineStatus{} }
func (f fakeSims) InstallEngine(string) error           { return f.err }
func (f fakeSims) Get(string) (models.Sim, error)       { return models.Sim{}, f.err }
func (f fakeSims) List() ([]models.Sim, error)          { return nil, f.err }
func (f fakeSims) Remove(string) error                  { return f.err }
func (f fakeSims) Start(models.SimRequest) (models.Sim, error) {
	return models.Sim{ID: "0123456789abcdef"}, f.err
}

func TestSimAPIAnswersEachErrorWithItsStatus(t *testing.T) {
	body := `{"kind": "quick", "export": "x", "options": {}}`
	for _, tc := range []struct {
		method, path, body string
		err                error
		code               int
	}{
		{"POST", "/api/sim", body, nil, http.StatusAccepted},
		{"POST", "/api/sim", body, fmt.Errorf("%w: pick again", service.ErrBadSimOptions), http.StatusBadRequest},
		{"POST", "/api/sim", body, service.ErrBadSimC, http.StatusBadRequest},
		{"POST", "/api/sim", body, service.ErrNoEngine, http.StatusConflict},
		{"POST", "/api/sim", body, service.ErrSimBusy, http.StatusConflict},
		{"POST", "/api/sim", body, errors.New(`disk full: C:\secret`), http.StatusInternalServerError},
		{"POST", "/api/sim", `{"kind": "quick"}`, nil, http.StatusBadRequest},
		{"POST", "/api/sim", `{"export": "` + strings.Repeat("x", simBodyBytes) + `"}`, nil, http.StatusBadRequest},
		{"POST", "/api/sim/engine", "", service.ErrSimBusy, http.StatusConflict},
		{"GET", "/api/sim/0123456789abcdef", "", fs.ErrNotExist, http.StatusNotFound},
		{"DELETE", "/api/sim/0123456789abcdef", "", nil, http.StatusNoContent},
		{"GET", "/api/sim/choices", "", fs.ErrNotExist, http.StatusOK}, // a route of its own, not a sim's ID
	} {
		router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, nil, &SettingsAPI{Sim: &SimAPI{Runs: fakeSims{tc.err}}},
			os.DirFS(t.TempDir()), LocalHosts(8080)...)
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Host = "localhost:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.code || strings.Contains(w.Body.String(), "secret") {
			t.Errorf("%s %s with %v: %d %.80s, want %d and no path of the PC",
				tc.method, tc.path, tc.err, w.Code, w.Body, tc.code)
		}
	}
}

func TestSimAPIReadsAnExportWithoutRunningIt(t *testing.T) {
	data, err := os.ReadFile("../service/testdata/simc-export.txt")
	if err != nil {
		t.Fatal(err)
	}
	router := Router(MPlus{Keys: &fakeKeys{}, Now: time.Now}, nil, &SettingsAPI{Sim: &SimAPI{Runs: fakeSims{}}},
		os.DirFS(t.TempDir()), LocalHosts(8080)...)
	body, _ := json.Marshal(SimExport{Export: string(data)})
	r := httptest.NewRequest("POST", "/api/sim/profile", bytes.NewReader(body))
	r.Host = "localhost:8080"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"class":"shaman"`) {
		t.Errorf("%d %.200s", w.Code, w.Body)
	}
}
