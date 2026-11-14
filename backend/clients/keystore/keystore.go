// Package keystore keeps finished Mythic+ keys as JSON files in a folder (logs/cache/local-keys/), one per key,
// named by its start time, and their tracks for the map in its tracks/ folder (read only when the map opens).
package keystore

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

// Store is a folder of saved keys.
type Store struct {
	Dir string
}

// Save writes a key, replacing an earlier copy of it.
func (s Store) Save(key models.Key) error {
	return cachefile.WriteJSON(s.path(key.Start), key)
}

// List is every saved key, newest first; a missing folder is no keys.
func (s Store) List() ([]models.Key, error) {
	keys, err := readAll[models.Key](s.Dir)
	slices.SortFunc(keys, func(a, b models.Key) int { return cmp.Compare(b.Start, a.Start) })
	return keys, err
}

// SaveTrack writes a key's track, replacing an earlier copy.
func (s Store) SaveTrack(start int64, track models.KeyTrack) error {
	return cachefile.WriteJSON(s.trackPath(start), track)
}

// Track reads a key's (or a raid pull's) track; the error wraps os.ErrNotExist when it has none.
func (s Store) Track(start int64) (models.KeyTrack, error) {
	var t models.KeyTrack
	data, err := os.ReadFile(s.trackPath(start))
	if err != nil {
		return t, fmt.Errorf("reading a track: %w", err)
	}
	if err := json.Unmarshal(data, &t); err != nil {
		return t, fmt.Errorf("reading a track: %w", err)
	}
	return t, nil
}

// SaveReview writes a key's review.
func (s Store) SaveReview(review models.KeyReview) error {
	return cachefile.WriteJSON(s.reviewPath(review.Start), review)
}

// Review reads a key's review; the error wraps os.ErrNotExist when there's none.
func (s Store) Review(start int64) (models.KeyReview, error) {
	var r models.KeyReview
	data, err := os.ReadFile(s.reviewPath(start))
	if err != nil {
		return r, fmt.Errorf("reading a key's review: %w", err)
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("reading a key's review: %w", err)
	}
	return r, nil
}

// SaveCompare writes a player's review of a key or raid pull.
func (s Store) SaveCompare(c models.PlayerCompare) error {
	return cachefile.WriteJSON(s.comparePath(c.Kind, c.Start, c.Player), c)
}

// Compare reads a player's review of a key or raid pull; the error wraps os.ErrNotExist when there's none.
func (s Store) Compare(kind models.FightKind, start int64, player string) (models.PlayerCompare, error) {
	var c models.PlayerCompare
	data, err := os.ReadFile(s.comparePath(kind, start, player))
	if err != nil {
		return c, fmt.Errorf("reading a player's review: %w", err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("reading a player's review: %w", err)
	}
	return c, nil
}

// Delete removes one saved key, its track, its review and its players' reviews.
func (s Store) Delete(start int64) error {
	compares, err := filepath.Glob(filepath.Join(s.Dir, "compares", fmt.Sprintf("%s-%d-*.json", models.FightMPlus, start)))
	if err != nil {
		return fmt.Errorf("listing a saved key's players' reviews: %w", err)
	}
	for _, path := range append(compares, s.path(start), s.trackPath(start), s.reviewPath(start)) {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("deleting a saved key: %w", err)
		}
	}
	return nil
}

// Clear removes every saved key.
func (s Store) Clear() error {
	keys, err := s.List()
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := s.Delete(key.Start); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) path(start int64) string {
	return filepath.Join(s.Dir, strconv.FormatInt(start, 10)+".json")
}

// comparePath is where a player's review of a fight is kept; the player's name is escaped for the file name.
func (s Store) comparePath(kind models.FightKind, start int64, player string) string {
	name := fmt.Sprintf("%s-%d-%s.json", kind, start, url.PathEscape(player))
	return filepath.Join(s.Dir, "compares", strings.ReplaceAll(name, "%", "_"))
}

func (s Store) reviewPath(start int64) string {
	return filepath.Join(s.Dir, "reviews", strconv.FormatInt(start, 10)+".json")
}

func (s Store) trackPath(start int64) string {
	return filepath.Join(s.Dir, "tracks", strconv.FormatInt(start, 10)+".json")
}
