// Package keystore keeps finished Mythic+ keys and raid pulls as JSON files, one folder for each (logs/cache/
// local-keys/, raid-pulls/), one file per fight named by its start time, and their tracks for the map, gzipped, in its
// tracks/ folder (read only when the map opens; tracks saved before gzip are plain .json until CompressTracks).
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
	"sync"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

// Folder is a folder of saved fights (keys or raid pulls).
type Folder[T models.SavedFight[T]] struct {
	Dir string
	// Recent keeps the last tracks read, so opening a saved key's map, route and review doesn't decode a few MB each
	// time; nil keeps none.
	Recent *Recent
}

// Pulls is the folder of finished raid pulls.
type Pulls = Folder[models.RaidPull]

// recentTracks is how many decoded tracks a Recent keeps.
const recentTracks = 2

// Recent is the last few decoded tracks of a folder, by fight start, each good while its file keeps the time it had
// (another process may write the cache). A track read from it is shared: callers must not modify it.
type Recent struct {
	mu      sync.Mutex
	starts  []int64 // newest first
	written []time.Time
	tracks  []models.KeyTrack
}

func (r *Recent) get(start int64, written time.Time) (models.KeyTrack, bool) {
	if r == nil {
		return models.KeyTrack{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if i := slices.Index(r.starts, start); i >= 0 && r.written[i].Equal(written) {
		return r.tracks[i], true
	}
	return models.KeyTrack{}, false
}

func (r *Recent) put(start int64, written time.Time, track models.KeyTrack) {
	if r == nil {
		return
	}
	r.drop(start)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts = append([]int64{start}, r.starts...)
	r.written = append([]time.Time{written}, r.written...)
	r.tracks = append([]models.KeyTrack{track}, r.tracks...)
	if len(r.starts) > recentTracks {
		r.starts, r.written, r.tracks = r.starts[:recentTracks], r.written[:recentTracks], r.tracks[:recentTracks]
	}
}

func (r *Recent) drop(start int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if i := slices.Index(r.starts, start); i >= 0 {
		r.starts, r.written = slices.Delete(r.starts, i, i+1), slices.Delete(r.written, i, i+1)
		r.tracks = slices.Delete(r.tracks, i, i+1)
	}
}

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

// SaveTrack writes a fight's track, replacing an earlier copy (and a plain one saved before gzip).
func (s Folder[T]) SaveTrack(start int64, track models.KeyTrack) error {
	s.Recent.drop(start)
	if err := cachefile.WriteJSON(s.trackPath(start), track); err != nil {
		return err
	}
	return remove(s.plainTrackPath(start))
}

// Track reads a fight's track, which a caller must not modify; the error wraps os.ErrNotExist when it has none.
func (s Folder[T]) Track(start int64) (models.KeyTrack, error) {
	path := s.trackPath(start)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		path = s.plainTrackPath(start)
		info, err = os.Stat(path)
	}
	if err != nil {
		return models.KeyTrack{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if track, ok := s.Recent.get(start, info.ModTime()); ok {
		return track, nil
	}
	track, err := cachefile.ReadJSON[models.KeyTrack](path)
	if err == nil {
		s.Recent.put(start, info.ModTime(), track)
	}
	return track, err
}

// CompressTracks gzips the tracks saved plain before gzip, and says how many (one another process just did is
// skipped).
func (s Folder[T]) CompressTracks() (int, error) {
	paths, err := filepath.Glob(filepath.Join(s.Dir, "tracks", "*.json"))
	if err != nil {
		return 0, fmt.Errorf("listing %s: %w", s.Dir, err)
	}
	for i, path := range paths {
		start, err := strconv.ParseInt(strings.TrimSuffix(filepath.Base(path), ".json"), 10, 64)
		if err != nil {
			return i, fmt.Errorf("reading the name of %s: %w", path, err)
		}
		track, err := cachefile.ReadJSON[models.KeyTrack](path)
		if err == nil {
			err = s.SaveTrack(start, track)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return i, err
		}
	}
	return len(paths), nil
}

// Delete removes one saved fight and its track.
func (s Folder[T]) Delete(start int64) error {
	s.Recent.drop(start)
	return remove(s.path(start), s.trackPath(start), s.plainTrackPath(start))
}

func (s Folder[T]) path(start int64) string {
	return filepath.Join(s.Dir, strconv.FormatInt(start, 10)+".json")
}

func (s Folder[T]) trackPath(start int64) string { return s.plainTrackPath(start) + ".gz" }

func (s Folder[T]) plainTrackPath(start int64) string {
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
	return Store{Folder[models.Key]{Dir: dir, Recent: &Recent{}}}
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
