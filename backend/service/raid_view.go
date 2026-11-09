package service

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"wow-live/backend/models"
)

// RaidLive is the raid as of now (ms since the epoch): whether a pull is on, and the last pull that ended with the
// pulls it's compared with.
func (k *LogTracker) RaidLive(now int64) models.RaidLive {
	k.mu.RLock()
	defer k.mu.RUnlock()
	var live models.RaidLive
	if run := k.current; run != nil && run.raid != nil {
		live.Pulling, live.Boss, live.Since = true, run.key.Dungeon, run.key.Start
	}
	if len(k.pulls) > 0 {
		last := k.compared(k.pulls[0])
		live.Last = &last
	}
	live.BetweenPulls = k.betweenPulls(now)
	return live
}

// BetweenPulls is whether the overlay shows the last raid pull at now (ms since the epoch).
func (k *LogTracker) BetweenPulls(now int64) bool {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.betweenPulls(now)
}

// betweenPulls is whether a raid pull ended less than RaidLinger before now, no pull or key is on, and no key
// ended since.
func (k *LogTracker) betweenPulls(now int64) bool {
	if k.current != nil || len(k.pulls) == 0 {
		return false
	}
	end := k.pulls[0].End
	return now-end <= RaidLinger.Milliseconds() && (k.last == nil || k.last.End < end)
}

// RaidNights is every raid night's pulls in order, newest night first.
func (k *LogTracker) RaidNights() []models.RaidNight {
	k.mu.RLock()
	defer k.mu.RUnlock()
	nights := []models.RaidNight{}
	for i := len(k.pulls) - 1; i >= 0; i-- { // oldest pull first
		p := k.pulls[i]
		if len(nights) == 0 || nights[0].Night != p.Night {
			nights = append([]models.RaidNight{{Night: p.Night, Pulls: []models.RaidPullSummary{}}}, nights...)
		}
		nights[0].Pulls = append(nights[0].Pulls, pullSummary(k.numbered(p)))
	}
	return nights
}

// RaidPull is one saved pull, compared with tonight's best and the best ever.
func (k *LogTracker) RaidPull(start int64) (models.RaidPull, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	i := slices.IndexFunc(k.pulls, func(p models.RaidPull) bool { return p.Start == start })
	if i < 0 {
		return models.RaidPull{}, ErrNoSuchPull
	}
	return k.compared(k.pulls[i]), nil
}

// numbered is a pull with its number among the night's pulls on its boss and difficulty, from the pulls known now
// (so pulls read later from an older log of the night still count).
func (k *LogTracker) numbered(p models.RaidPull) models.RaidPull {
	p.Number = 1 + countFunc(k.pulls, func(o models.RaidPull) bool {
		return o.Night == p.Night && o.Boss == p.Boss && o.Difficulty == p.Difficulty && o.Start < p.Start
	})
	return p
}

// compared is a pull, numbered, with tonight's best and the best ever on its boss and difficulty, each left out
// when it's the pull itself.
func (k *LogTracker) compared(p models.RaidPull) models.RaidPull {
	p = k.numbered(p)
	var tonight, ever *models.RaidPull
	for i := range k.pulls {
		o := &k.pulls[i]
		if o.Boss != p.Boss || o.Difficulty != p.Difficulty {
			continue
		}
		if ever == nil || better(*o, *ever) {
			ever = o
		}
		if o.Night == p.Night && (tonight == nil || better(*o, *tonight)) {
			tonight = o
		}
	}
	c := models.RaidComparisons{}
	if tonight != nil && tonight.Start != p.Start {
		s := pullSummary(k.numbered(*tonight))
		c.Tonight = &s
	}
	if ever != nil && ever.Start != p.Start {
		s := pullSummary(k.numbered(*ever))
		c.Ever = &s
	}
	if c.Tonight != nil || c.Ever != nil {
		p.Comparisons = &c
	}
	return p
}

// better is whether pull a got further than b: a kill, else a later phase, else less boss health left, else the
// earlier pull.
func better(a, b models.RaidPull) bool {
	switch {
	case a.Kill != b.Kill:
		return a.Kill
	case phaseOf(a) != phaseOf(b):
		return phaseOf(a) > phaseOf(b)
	case left(a) != left(b):
		return left(a) < left(b)
	}
	return a.Start < b.Start
}

// phaseOf is a pull's phase number (1 without phases).
func phaseOf(p models.RaidPull) int {
	if n, err := strconv.Atoi(strings.TrimPrefix(p.Phase, "P")); err == nil {
		return n
	}
	return 1
}

// left is the boss health a pull left, an unknown amount counting as all of it.
func left(p models.RaidPull) float64 {
	if p.BossPct < 0 {
		return 100
	}
	return p.BossPct
}

// pullSummary is a pull as a list shows it.
func pullSummary(p models.RaidPull) models.RaidPullSummary {
	return models.RaidPullSummary{Start: p.Start, Night: p.Night, Boss: p.Boss, Difficulty: p.Difficulty,
		Number: p.Number, DurationMS: p.DurationMS, Kill: p.Kill, BossPct: p.BossPct, Phase: p.Phase}
}

// savePulls saves the raid pulls that ended since the last call, keeping the failed ones for the next.
func (k *LogTracker) savePulls() error {
	k.mu.Lock()
	done := k.unsavedPulls
	k.unsavedPulls = nil
	k.mu.Unlock()
	for i, p := range done {
		if err := k.raid.store.SavePull(p); err != nil {
			k.mu.Lock()
			k.unsavedPulls = append(done[i:], k.unsavedPulls...)
			k.mu.Unlock()
			return fmt.Errorf("saving the %s pull (retried after the next read): %w", p.Boss, err)
		}
	}
	return nil
}
