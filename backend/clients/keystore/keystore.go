// Package keystore keeps finished Mythic+ keys as JSON files in a folder (logs/cache/local-keys/), one per key,
// named by its start time, and their tracks for the map in its tracks/ folder (read only when the map opens).
package keystore

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"

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

// Delete removes one saved key, its track and its review.
func (s Store) Delete(start int64) error {
	for _, path := range []string{s.path(start), s.trackPath(start), s.reviewPath(start)} {
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

func (s Store) reviewPath(start int64) string {
	return filepath.Join(s.Dir, "reviews", strconv.FormatInt(start, 10)+".json")
}

func (s Store) trackPath(start int64) string {
	return filepath.Join(s.Dir, "tracks", strconv.FormatInt(start, 10)+".json")
}
