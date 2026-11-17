package service

import (
	"testing"

	"wow-live/backend/models"
)

func TestCleanSettingsTrimsTheCharacterAndCleansThePath(t *testing.T) {
	got := CleanSettings(models.Settings{LogsDir: "logs/../logs/", Character: "  Arnaud-Area52 "})
	if got.LogsDir != "logs" || got.Character != "Arnaud-Area52" {
		t.Fatalf("CleanSettings = %+v", got)
	}
}

func TestCharacterNeedsCheck(t *testing.T) {
	before := models.Settings{Character: "Arnaud-Area52", Region: "us"}
	for name, tc := range map[string]struct {
		next models.Settings
		want bool
	}{
		"unchanged":      {before, false},
		"cleared":        {models.Settings{Region: "us"}, false},
		"new character":  {models.Settings{Character: "Bob-Area52", Region: "us"}, true},
		"another region": {models.Settings{Character: "Arnaud-Area52", Region: "eu"}, true},
	} {
		if got := CharacterNeedsCheck(before, tc.next); got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

func TestValidateSettingsRejectsADuplicateColumn(t *testing.T) {
	s := models.Settings{LogsDir: t.TempDir(), Region: "us", Overlay: models.OverlaySettings{Show: "keys", Opacity: 100,
		Columns: []string{"dps", "hps"}}}
	if err := ValidateSettings(s); err != nil {
		t.Fatalf("two different columns: %v", err)
	}
	s.Overlay.Columns = []string{"dps", "dps"}
	if err := ValidateSettings(s); err == nil {
		t.Error("a column twice was accepted")
	}
}
