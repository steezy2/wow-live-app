package keystore

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

// Pulls is a folder of finished raid pulls (raid-pulls/ beside the saved keys), one JSON file per pull named by its
// start time, and their tracks for the replay in its tracks/ folder, as the saved keys keep theirs.
type Pulls struct {
	Dir string
}

// SaveTrack writes a pull's track, replacing an earlier copy.
func (s Pulls) SaveTrack(start int64, track models.KeyTrack) error {
	return Store(s).SaveTrack(start, track)
}

// Track reads a pull's track; the error wraps os.ErrNotExist when it has none.
func (s Pulls) Track(start int64) (models.KeyTrack, error) {
	return Store(s).Track(start)
}

// SavePull writes a pull, replacing an earlier copy of it.
func (s Pulls) SavePull(pull models.RaidPull) error {
	return cachefile.WriteJSON(filepath.Join(s.Dir, strconv.FormatInt(pull.Start, 10)+".json"), pull)
}

// DeletePull removes one saved pull and its track.
func (s Pulls) DeletePull(start int64) error {
	return Store(s).Delete(start)
}

// ListPulls is every saved pull, newest first; a missing folder is no pulls.
func (s Pulls) ListPulls() ([]models.RaidPull, error) {
	pulls, err := readAll[models.RaidPull](s.Dir)
	slices.SortFunc(pulls, func(a, b models.RaidPull) int { return cmp.Compare(b.Start, a.Start) })
	return pulls, err
}

// readAll reads every JSON file in dir; a missing folder is none.
func readAll[T any](dir string) ([]T, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", dir, err)
	}
	all := []T{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		var v T
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		all = append(all, v)
	}
	return all, nil
}
