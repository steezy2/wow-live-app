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
