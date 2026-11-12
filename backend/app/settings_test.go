package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wow-live/backend/models"
)

func TestSettingsFromAnOlderFileKeepDefaultsAndPlacesSurviveASave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"logsDir": "D:/Logs", "region": "eu"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// no columns read as the default ones
	overlay := models.OverlaySettings{Show: "keys", Opacity: 100, Columns: []string{"dps", "hps", "kicks"}}
	s, err := LoadSettings(path, models.Settings{Region: "us", Overlay: models.OverlaySettings{Show: "keys", Opacity: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Current(); got.Region != "eu" || !reflect.DeepEqual(got.Overlay, overlay) {
		t.Fatalf("loaded %+v, want region eu and the default overlay", got)
	}
	place := Place{X: 10, Y: 20, W: 420, H: 340}
	if err := s.SavePlace("overlay", place); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(models.Settings{LogsDir: "E:/Logs", Region: "us", Overlay: overlay}); err != nil {
		t.Fatal(err)
	}
	again, err := LoadSettings(path, models.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := again.Place("overlay"); !ok || got != place || again.Current().LogsDir != "E:/Logs" {
		t.Errorf("after a save: place %+v %v, settings %+v", got, ok, again.Current())
	}
}
