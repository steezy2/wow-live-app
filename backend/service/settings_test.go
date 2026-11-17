package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"wow-live/backend/models"
)

func TestCleanSettingsTrimsTheCharacterAndCleansThePath(t *testing.T) {
	got := cleanSettings(models.Settings{LogsDir: "logs/../logs/", Character: "  Arnaud-Area52 "})
	if got.LogsDir != "logs" || got.Character != "Arnaud-Area52" {
		t.Fatalf("cleanSettings = %+v", got)
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
		if got := characterNeedsCheck(before, tc.next); got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

func TestValidateSettingsRejectsADuplicateColumn(t *testing.T) {
	s := models.Settings{LogsDir: t.TempDir(), Region: "us", Overlay: models.OverlaySettings{Show: "keys", Opacity: 100,
		Columns: []string{"dps", "hps"}}}
	if err := validateSettings(s); err != nil {
		t.Fatalf("two different columns: %v", err)
	}
	s.Overlay.Columns = []string{"dps", "dps"}
	if err := validateSettings(s); err == nil {
		t.Error("a column twice was accepted")
	}
}

type memSettings struct {
	current models.Settings
	saved   *models.Settings
	failure error
}

func (m *memSettings) Get() models.SettingsView { return models.SettingsView{Settings: m.current} }
func (m *memSettings) Save(s models.Settings) error {
	m.saved = &s
	return m.failure
}

type resolveFunc func(ctx context.Context, region, character string) (string, string, error)

func (f resolveFunc) Resolve(ctx context.Context, region, character string) (string, string, error) {
	return f(ctx, region, character)
}

func TestSaveSettings(t *testing.T) {
	good := models.Settings{LogsDir: filepath.Join(t.TempDir(), "Logs"), Region: "us", Overlay: models.OverlaySettings{Show: "keys", Opacity: 50}}
	known := resolveFunc(func(context.Context, string, string) (string, string, error) { return "Arnaud", "area-52", nil })
	unknown := resolveFunc(func(context.Context, string, string) (string, string, error) { return "", "", ErrBadCharacter })
	withCharacter := func(s models.Settings, c string) models.Settings { s.Character = c; return s }
	for _, tc := range []struct {
		name      string
		next      models.Settings
		resolver  CharacterResolver
		failure   error
		wantErr   error
		wantSaved bool
		wantMoved bool
	}{
		{"a plain save", good, nil, nil, nil, true, false},
		{"settings that break the rules", models.Settings{Region: "xx"}, nil, nil, ErrBadSettings, false, false},
		{"a new character is checked and moves the import", withCharacter(good, "Arnaud-Area52"), known, nil, nil, true, true},
		{"a character Warcraft Logs doesn't know", withCharacter(good, "Arnaud-Nowhere"), unknown, nil, ErrBadCharacter, false, false},
		{"a save that fails", good, nil, errors.New("disk full"), ErrSavingSettings, true, false},
	} {
		store := &memSettings{current: good, failure: tc.failure}
		moved, err := SaveSettings(context.Background(), store, tc.resolver, tc.next)
		if !errors.Is(err, tc.wantErr) || moved != tc.wantMoved || (store.saved != nil) != tc.wantSaved {
			t.Errorf("%s: moved %v, err %v, saved %v; want moved %v, err %v, saved %v",
				tc.name, moved, err, store.saved != nil, tc.wantMoved, tc.wantErr, tc.wantSaved)
		}
	}
}
