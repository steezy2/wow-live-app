// Package importstore keeps a Warcraft Logs import's state and plan as JSON files in its folder
// (<cache>\wcl-import), which also holds the fights' zips.
package importstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"wow-live/backend/clients/cachefile"
)

// Store is the import's folder.
type Store struct {
	Dir string
}

// Read decodes the file name into out, leaving out as it is when there's none.
func (s Store) Read(name string, out any) error {
	data, err := os.ReadFile(filepath.Join(s.Dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err == nil {
		err = json.Unmarshal(data, out)
	}
	if err != nil {
		return fmt.Errorf("reading the import's %s: %w", name, err)
	}
	return nil
}

// Write saves v as the file name, never leaving half a file.
func (s Store) Write(name string, v any) error {
	if err := cachefile.WriteJSON(filepath.Join(s.Dir, name), v); err != nil {
		return fmt.Errorf("saving the import's %s: %w", name, err)
	}
	return nil
}

// Remove deletes the file name; one that isn't there is fine.
func (s Store) Remove(name string) error {
	if err := os.Remove(filepath.Join(s.Dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing the import's %s: %w", name, err)
	}
	return nil
}

// Clear deletes the whole folder: the files and the fights' zips.
func (s Store) Clear() error {
	if err := os.RemoveAll(s.Dir); err != nil {
		return fmt.Errorf("removing the imported fights: %w", err)
	}
	return nil
}
