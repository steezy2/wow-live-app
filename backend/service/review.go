package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"sync"

	"wow-live/backend/models"
)

const (
	// minReviewPoints: a review isn't run (automatically or asked for) with fewer Warcraft Logs points left this hour.
	minReviewPoints = 100
	// similarTime: the spec baseline is the top runs within this share of the key's time, when there are any.
	similarTime = 0.15
	// partyRuns: how many similar top runs the party's damage taken is compared with.
	partyRuns = 3
)

var (
	// ErrKeyRunning is returned when asked to review a key that hasn't ended.
	ErrKeyRunning = errors.New("the key is still running")
	// ErrNoPoints is returned when too few Warcraft Logs points are left this hour for a review.
	ErrNoPoints = errors.New("too few Warcraft Logs points left this hour")
)

// Reviewer reviews finished keys against Warcraft Logs' top runs and the players' own earlier runs, keeping each
// review in the store.
type Reviewer struct {
	Keys      *KeyTracker
	Baselines models.MPlusBaselines
	Store     models.ReviewStore
}

// Cached is a key's review if one was made, and the key didn't go on after it (a key closed as abandoned can).
func (r Reviewer) Cached(start int64) (models.KeyReview, error) {
	review, err := r.Store.Review(start)
	if errors.Is(err, fs.ErrNotExist) {
		return review, ErrNoSuchKey
	}
	if key, _, kerr := r.Keys.finished(start); err == nil && kerr == nil && review.End != 0 && review.End != key.End {
		return review, ErrNoSuchKey
	}
	return review, err
}

// reviewing lets one review run at a time: a second ask for the same key then finds the first's stored review.
var reviewing sync.Mutex

// Review is a key's review: the stored one, or a new one, which spends Warcraft Logs points.
func (r Reviewer) Review(ctx context.Context, start int64) (models.KeyReview, error) {
	reviewing.Lock()
	defer reviewing.Unlock()
	if review, err := r.Cached(start); err == nil {
		return review, nil
	}
	key, history, err := r.Keys.finished(start)
	if err != nil {
		return models.KeyReview{}, err
	}
	left, err := r.Baselines.PointsLeft(ctx)
	if err != nil {
		return models.KeyReview{}, fmt.Errorf("reading the points left: %w", err)
	}
	if left < minReviewPoints {
		return models.KeyReview{}, ErrNoPoints
	}
	encounters, err := r.Baselines.MPlusEncounters(ctx)
	if err != nil {
		return models.KeyReview{}, err
	}
	encounter := encounters[key.Dungeon]
	length := lengthMS(key)
	baselines := map[string]map[string][]models.MPlusRanking{}
	var similar []models.MPlusRanking
	for player, specID := range key.Specs {
		spec, ok := specs[specID]
		if !ok || encounter == 0 {
			continue
		}
		baselines[player] = map[string][]models.MPlusRanking{}
		for _, metric := range []string{dpsMetric.name, hpsMetric.name} {
			runs, err := r.Baselines.MPlusRankings(ctx, encounter, key.Level, spec.Class, spec.Name,
				strings.ToLower(metric))
			if err != nil {
				return models.KeyReview{}, err
			}
			baselines[player][metric] = nearTime(runs, length)
		}
		similar = append(similar, baselines[player][mainMetric(spec)]...)
	}
	var taken []float64 // the similar runs' party damage taken per second
	slices.SortFunc(similar, func(a, b models.MPlusRanking) int {
		return cmp.Compare(abs(a.DurationMS-length), abs(b.DurationMS-length))
	})
	seen := map[string]bool{}
	for _, run := range similar {
		id := fmt.Sprint(run.Code, run.FightID)
		if seen[id] || len(taken) == partyRuns {
			continue
		}
		seen[id] = true
		total, err := r.Baselines.RunDamageTaken(ctx, run.Code, run.FightID)
		if err != nil {
			return models.KeyReview{}, err
		}
		taken = append(taken, float64(total)/float64(max(1, run.DurationMS))*1000)
	}
	review := buildReview(key, history, baselines, taken)
	if after, err := r.Baselines.PointsLeft(ctx); err == nil {
		review.Points = left - after - 1 // the second points check costs about 1
	}
	return review, r.Store.SaveReview(review)
}

// TopRuns is the times of a dungeon's top Warcraft Logs runs at the key's level, for its first run's estimate: one of
// the party's specs' rankings (cached, and read again by the key's review anyway).
func (r Reviewer) TopRuns(ctx context.Context, key models.Key) ([]int64, error) {
	encounters, err := r.Baselines.MPlusEncounters(ctx)
	if err != nil {
		return nil, err
	}
	encounter := encounters[key.Dungeon]
	if encounter == 0 {
		return nil, fmt.Errorf("no Warcraft Logs encounter for %s", key.Dungeon)
	}
	for _, player := range slices.Sorted(maps.Keys(key.Specs)) {
		spec, ok := specs[key.Specs[player]]
		if !ok {
			continue
		}
		runs, err := r.Baselines.MPlusRankings(ctx, encounter, key.Level, spec.Class, spec.Name,
			strings.ToLower(mainMetric(spec)))
		if err != nil {
			return nil, err
		}
		ms := make([]int64, len(runs))
		for i, run := range runs {
			ms[i] = run.DurationMS
		}
		return ms, nil
	}
	return nil, fmt.Errorf("no known spec in the %s party", key.Dungeon)
}

// finished is a finished key by its start time and the earlier saved runs of its dungeon.
func (k *KeyTracker) finished(start int64) (models.Key, []models.Key, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.current != nil && k.current.key.Start == start {
		return models.Key{}, nil, ErrKeyRunning
	}
	i := slices.IndexFunc(k.history, func(key models.Key) bool { return key.Start == start })
	if i < 0 {
		return models.Key{}, nil, ErrNoSuchKey
	}
	key := k.history[i]
	var earlier []models.Key
	for _, h := range k.history {
		if h.MapID == key.MapID && h.Start < start {
			earlier = append(earlier, h)
		}
	}
	return key, earlier, nil
}

// nearTime is the runs within similarTime of a key's time, or all of them when none are.
func nearTime(runs []models.MPlusRanking, ms int64) []models.MPlusRanking {
	var near []models.MPlusRanking
	for _, r := range runs {
		if float64(abs(r.DurationMS-ms)) <= similarTime*float64(ms) {
			near = append(near, r)
		}
	}
	if len(near) == 0 {
		return runs
	}
	return near
}
