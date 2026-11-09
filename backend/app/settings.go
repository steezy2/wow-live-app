package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"sync"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/clients/combatlog"
	"wow-live/backend/models"
)

// Settings are the desktop app's choices, and where its windows were, kept in settings.json in its data folder.
type Settings struct {
	path string
	mu   sync.Mutex
	file settingsFile
	// OnSave runs after each save with the new settings (the app follows the new log folder); it mustn't call back.
	OnSave func(models.Settings)
}

// LoadSettings reads path, or starts from defaults when there's no file yet.
func LoadSettings(path string, defaults models.Settings) (*Settings, error) {
	s := &Settings{path: path, file: settingsFile{Settings: defaults}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &s.file); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return s, nil
}

// Current is the settings as they are.
func (s *Settings) Current() models.Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file.Settings
}

// Get is the settings with whether the log folder exists and when the game last wrote a log there.
func (s *Settings) Get() models.SettingsView {
	view := models.SettingsView{Settings: s.Current()}
	info, err := os.Stat(view.LogsDir)
	view.LogsDirFound = err == nil && info.IsDir()
	if newest, err := combatlog.Newest(view.LogsDir); err == nil && newest != "" {
		if info, err := os.Stat(newest); err == nil {
			view.LastLogMS = info.ModTime().UnixMilli()
		}
	}
	return view
}

// Save writes next and runs OnSave, one save at a time.
func (s *Settings) Save(next models.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	file := s.file
	file.Settings = next
	if err := s.write(file); err != nil {
		return err
	}
	if s.OnSave != nil {
		s.OnSave(next)
	}
	return nil
}

// Place is where a window was last (in Windows' scaled pixels), and whether it's been saved.
func (s *Settings) Place(window string) (Place, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.file.Windows[window]
	return p, ok
}

// SavePlace saves where a window is; a zero Place forgets it.
func (s *Settings) SavePlace(window string, p Place) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	file := s.file
	file.Windows = maps.Clone(file.Windows)
	if p == (Place{}) {
		delete(file.Windows, window)
	} else {
		if file.Windows == nil {
			file.Windows = map[string]Place{}
		}
		file.Windows[window] = p
	}
	return s.write(file)
}

// write saves file and keeps it as the current one.
func (s *Settings) write(file settingsFile) error {
	if err := cachefile.WriteJSON(s.path, file); err != nil {
		return err
	}
	s.file = file
	return nil
}

// Place is a window's place and size.
type Place struct{ X, Y, W, H int }

// settingsFile is settings.json: the settings the page edits, and the windows' places only the app writes.
type settingsFile struct {
	models.Settings
	Windows map[string]Place `json:"windows,omitempty"`
}
