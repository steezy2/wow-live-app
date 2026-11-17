// Package keystore keeps finished Mythic+ keys and raid pulls as JSON files, one folder for each (logs/cache/
// local-keys/, raid-pulls/), one file per fight named by its start time, and their tracks for the map in its tracks/
// folder (read only when the map opens).
package keystore

import (
	"cmp"
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

// Folder is a folder of saved fights (keys or raid pulls).
type Folder[T models.SavedFight[T]] struct {
	Dir string
}

// Pulls is the folder of finished raid pulls.
type Pulls = Folder[models.RaidPull]

// Save writes a fight, replacing an earlier copy of it.
func (s Folder[T]) Save(fight T) error {
	return cachefile.WriteJSON(s.path(fight.FightStart()), fight)
}

// List is every saved fight, newest first; a missing folder is none.
func (s Folder[T]) List() ([]T, error) {
	all, err := cachefile.ReadAll[T](s.Dir)
	slices.SortFunc(all, func(a, b T) int { return cmp.Compare(b.FightStart(), a.FightStart()) })
	return all, err
}

// SaveTrack writes a fight's track, replacing an earlier copy.
func (s Folder[T]) SaveTrack(start int64, track models.KeyTrack) error {
	return cachefile.WriteJSON(s.trackPath(start), track)
}

// Track reads a fight's track; the error wraps os.ErrNotExist when it has none.
func (s Folder[T]) Track(start int64) (models.KeyTrack, error) {
	return cachefile.ReadJSON[models.KeyTrack](s.trackPath(start))
}

// Delete removes one saved fight and its track.
func (s Folder[T]) Delete(start int64) error {
	return remove(s.path(start), s.trackPath(start))
}

func (s Folder[T]) path(start int64) string {
	return filepath.Join(s.Dir, strconv.FormatInt(start, 10)+".json")
}

func (s Folder[T]) trackPath(start int64) string {
	return filepath.Join(s.Dir, "tracks", strconv.FormatInt(start, 10)+".json")
}

// remove deletes the files; one that isn't there is fine.
func remove(paths ...string) error {
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("deleting a saved fight: %w", err)
		}
	}
	return nil
}

// Store is the folder of saved keys, which also keeps their reviews and the reviews of their players (and of raid
// pulls' players).
type Store struct {
	Folder[models.Key]
}

// NewStore is the store of the saved keys in dir.
func NewStore(dir string) Store {
	return Store{Folder[models.Key]{Dir: dir}}
}

// SaveReview writes a key's review.
func (s Store) SaveReview(review models.KeyReview) error {
	return cachefile.WriteJSON(s.reviewPath(review.Start), review)
}

// Review reads a key's review; the error wraps os.ErrNotExist when there's none.
func (s Store) Review(start int64) (models.KeyReview, error) {
	return cachefile.ReadJSON[models.KeyReview](s.reviewPath(start))
}

// SaveCompare writes a player's review of a key or raid pull.
func (s Store) SaveCompare(c models.PlayerCompare) error {
	return cachefile.WriteJSON(s.comparePath(c.Kind, c.Start, c.Player), c)
}

// Compare reads a player's review of a key or raid pull; the error wraps os.ErrNotExist when there's none.
func (s Store) Compare(kind models.FightKind, start int64, player string) (models.PlayerCompare, error) {
	return cachefile.ReadJSON[models.PlayerCompare](s.comparePath(kind, start, player))
}

// Delete removes one saved key, its track, its review and its players' reviews.
func (s Store) Delete(start int64) error {
	compares, err := filepath.Glob(filepath.Join(s.Dir, "compares", fmt.Sprintf("%s-%d-*.json", models.FightMPlus, start)))
	if err != nil {
		return fmt.Errorf("listing a saved key's players' reviews: %w", err)
	}
	return errors.Join(remove(append(compares, s.reviewPath(start))...), s.Folder.Delete(start))
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

// comparePath is where a player's review of a fight is kept; the player's name is escaped for the file name.
func (s Store) comparePath(kind models.FightKind, start int64, player string) string {
	name := fmt.Sprintf("%s-%d-%s.json", kind, start, url.PathEscape(player))
	return filepath.Join(s.Dir, "compares", strings.ReplaceAll(name, "%", "_"))
}

func (s Store) reviewPath(start int64) string {
	return filepath.Join(s.Dir, "reviews", strconv.FormatInt(start, 10)+".json")
}
