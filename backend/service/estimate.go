package service

import (
	"cmp"
	"fmt"
	"slices"

	"wow-live/backend/models"
)

const (
	// nearWeight and farWeight: an earlier run within levelSpread counts fully, any other run of the dungeon half.
	nearWeight, farWeight = 1.0, 0.5
	// minPace and maxPace keep one odd stretch from moving the estimate more than this either way.
	minPace, maxPace = 0.5, 2.0
	// forcesCheckPct: a finished key whose kills add up to less than this says the dungeon file needs checking (the
	// game needs 100%, and Mythic Dungeon Tools' counts can be a point or two off).
	forcesCheckPct = 98.0
	// minPacePct: under this share of forces, the pace against the best run is noise and isn't shown.
	minPacePct = 5.0
)

// projection is the expected final time from earlier runs of the dungeon: each run
// gives an estimate, and the page gets their weighted median, the 20th to 80th percentile, the odds of each result and
// the best timed run at the key's level as boss-by-boss marks; nil without earlier runs.
//
// A run with a forces timeline (and a key with one) estimates bosses and trash apart: the trash time the run still
// had after reaching the key's forces and the time it spent on the bosses the key hasn't killed, each scaled by the
// key's own pace on trash and on bosses so far, plus the deaths its death rate predicts. Older runs fall back to
// scaling the last boss kill by the share of the run's time it came at.
func projection(key models.Key, clockMS int64, history []models.Key) *models.KeyProjection {
	var guesses []guess
	withForces := 0
	for _, r := range history {
		if !usable(r, key.MapID) {
			continue
		}
		weight := farWeight
		if max(r.Level-key.Level, key.Level-r.Level) <= levelSpread {
			weight = nearWeight
		}
		ms, ok := fromForces(key, clockMS, r)
		if ok {
			withForces++
		} else if ms, ok = fromKills(key, r); !ok {
			continue
		}
		guesses = append(guesses, guess{max(ms, clockMS), weight})
	}
	if len(guesses) == 0 {
		return nil
	}
	p := summary(key, guesses)
	p.Source = fmt.Sprintf("%d of your runs, boss kills only", len(guesses))
	if withForces > 0 {
		p.Source = fmt.Sprintf("%d of your runs, forces and bosses", len(guesses))
	}
	if best := bestRun(key, history); best != nil && key.TimerMS > 0 {
		for _, b := range best.Bosses {
			if b.Kill {
				p.Best = append(p.Best, models.KeyGhost{Name: b.Name, KillMS: b.EndMS,
					NeedByMS: key.TimerMS - (best.FinalMS - b.EndMS)})
			}
		}
	}
	return p
}

// usable says whether an earlier run can estimate a key in the dungeon: one of it that finished (an abandoned run
// has no end to go by).
func usable(r models.Key, mapID int) bool {
	return r.MapID == mapID && r.FinalMS > 0 && r.Status != "abandoned"
}

// fromTopRuns is the estimate for a dungeon's first run here, from the times of its top Warcraft Logs runs at the
// key's level: each run's time for the share of the key still to go (by forces), at the key's pace against their
// median once a tenth is done (before that, theirs); nil without them.
func fromTopRuns(key models.Key, clockMS int64, tops []int64) *models.KeyProjection {
	if len(tops) == 0 {
		return nil
	}
	done, p := 0.0, 1.0
	if key.Forces != nil {
		done = min(1, key.Forces.Pct/100)
	}
	if done >= 0.1 {
		p = pace(clockMS, int64(done*float64(median(tops))))
	}
	guesses := make([]guess, len(tops))
	for i, ms := range tops {
		guesses[i] = guess{clockMS + int64((1-done)*float64(ms)*p), 1}
	}
	proj := summary(key, guesses)
	proj.Source = fmt.Sprintf("%d top Warcraft Logs runs at +%d, at your pace", len(tops), key.Level)
	return proj
}

// guess is one run's estimate of the key's final time, and how much it counts.
type guess struct {
	ms     int64
	weight float64
}

// summary is the estimate from the guesses: their weighted median and 20th to 80th percentile, the result the median
// gives and the share of guesses pointing at each.
func summary(key models.Key, guesses []guess) *models.KeyProjection {
	slices.SortFunc(guesses, func(a, b guess) int { return cmp.Compare(a.ms, b.ms) })
	var total float64
	for _, g := range guesses {
		total += g.weight
	}
	at := func(q float64) int64 {
		sum := 0.0
		for _, g := range guesses {
			if sum += g.weight; sum >= q*total {
				return g.ms
			}
		}
		return guesses[len(guesses)-1].ms
	}
	p := &models.KeyProjection{FinalMS: at(0.5), LowMS: at(0.2), HighMS: at(0.8), Runs: len(guesses),
		Odds: []float64{0, 0, 0, 0}, Best: []models.KeyGhost{}}
	p.Chests = chests(p.FinalMS, key.TimerMS)
	for _, g := range guesses {
		if c := chests(g.ms, key.TimerMS); c >= 0 {
			p.Odds[c] += g.weight / total
		}
	}
	return p
}

// fromForces is one earlier run's estimate from forces and bosses; false when either key lacks a forces timeline.
func fromForces(key models.Key, clockMS int64, r models.Key) (int64, bool) {
	if key.Forces == nil || r.Forces == nil || len(r.Forces.At) == 0 {
		return 0, false
	}
	killed := map[string]bool{}
	for _, b := range key.Bosses {
		killed[b.Name] = killed[b.Name] || b.Kill
	}
	yours, theirs := bossTimes(key, clockMS), bossTimes(r, r.FinalMS)
	var yourBoss, theirBoss int64
	for name := range theirs {
		if killed[name] {
			yourBoss, theirBoss = yourBoss+yours[name], theirBoss+theirs[name]
		}
	}
	bossPace := pace(yourBoss, theirBoss)
	left := 0.0
	for name := range theirs {
		if !killed[name] { // a boss being fought now has some of its time behind it already
			left += max(0, float64(theirs[name])*bossPace-float64(yours[name]))
		}
	}
	// Paces compare the moment you reached your forces with the moment the run did; trash since then is on the way
	// to the next kill, so it comes off what's left.
	yourReach := int64(0)
	if n := len(key.Forces.At); n > 0 {
		yourReach = key.Forces.At[n-1].ClockMS
	}
	theirTrashThen := trashTime(r, forcesClock(r, min(key.Forces.Pct, r.Forces.At[len(r.Forces.At)-1].Pct)))
	yourTrashThen := trashTime(key, yourReach)
	trashLeft := float64(trashTime(r, r.FinalMS)-theirTrashThen)*pace(yourTrashThen, theirTrashThen) -
		float64(trashTime(key, clockMS)-yourTrashThen)
	left += max(0, trashLeft)
	deathsLeft := left * float64(len(r.Deaths)) / float64(max(1, elapsed(r, r.FinalMS))) * deathPenaltyMS
	return clockMS + int64(left+deathsLeft), true
}

// fromKills is one earlier run's estimate from boss kills alone: the key's last kill scaled by the share of the run's
// time its matching kill came at, or the run's time before any kill; false when the run killed fewer bosses.
func fromKills(key models.Key, r models.Key) (int64, bool) {
	yours, theirs := killTimes(key.Bosses), killTimes(r.Bosses)
	if len(theirs) < len(yours) {
		return 0, false
	}
	if len(yours) == 0 {
		return r.FinalMS, true
	}
	share := float64(theirs[len(yours)-1]) / float64(r.FinalMS)
	return int64(float64(yours[len(yours)-1]) / share), true
}

// pace is how long the key took for a stretch against the earlier run (1 when the run has too little of it to say),
// kept between minPace and maxPace.
func pace(yours, theirs int64) float64 {
	if theirs < 30_000 || yours <= 0 {
		return 1
	}
	return min(maxPace, max(minPace, float64(yours)/float64(theirs)))
}

// elapsed is a key's fighting time at a clock reading: the clock without the death penalties so far.
func elapsed(k models.Key, clockMS int64) int64 {
	n := 0
	for _, d := range k.Deaths {
		if d.ClockMS < clockMS {
			n++
		}
	}
	return clockMS - int64(n)*deathPenaltyMS
}

// bossTimes is the fighting time spent on each boss up to a clock reading, wipes included.
func bossTimes(k models.Key, clockMS int64) map[string]int64 {
	times := map[string]int64{}
	for _, b := range k.Bosses {
		if b.StartMS >= clockMS {
			continue
		}
		end := clockMS
		if b.EndMS > 0 {
			end = min(b.EndMS, clockMS)
		}
		times[b.Name] += elapsed(k, end) - elapsed(k, b.StartMS)
	}
	return times
}

// trashTime is the fighting time not spent on bosses up to a clock reading (walking included).
func trashTime(k models.Key, clockMS int64) int64 {
	t := elapsed(k, clockMS)
	for _, boss := range bossTimes(k, clockMS) {
		t -= boss
	}
	return t
}

// forcesClock is when a key first reached a forces share (its start for none), or its end when it never did.
func forcesClock(k models.Key, pct float64) int64 {
	if pct <= 0 {
		return 0
	}
	for _, p := range k.Forces.At {
		if p.Pct >= pct {
			return p.ClockMS
		}
	}
	return k.FinalMS
}

// forcesAt is a key's forces share at a clock reading.
func forcesAt(k models.Key, clockMS int64) float64 {
	pct := 0.0
	for _, p := range k.Forces.At {
		if p.ClockMS > clockMS {
			break
		}
		pct = p.Pct
	}
	return pct
}

// bestRun is the fastest timed other run of a key's dungeon at its level, nil without one.
func bestRun(key models.Key, history []models.Key) *models.Key {
	var best *models.Key
	for i, r := range history {
		if usable(r, key.MapID) && r.Status == "timed" && r.Level == key.Level && r.Start != key.Start &&
			(best == nil || r.FinalMS < best.FinalMS) {
			best = &history[i]
		}
	}
	return best
}

// forcesPace compares a key's forces (pct at a clock reading) with its best run's: nil without a best run with a
// forces timeline, or under minPacePct.
func forcesPace(best *models.Key, pct float64, clockMS int64) *models.ForcesPace {
	if best == nil || best.Forces == nil || len(best.Forces.At) == 0 || pct < minPacePct {
		return nil
	}
	bestMS := forcesClock(*best, pct)
	if bestMS <= 0 {
		return nil
	}
	return &models.ForcesPace{BestPct: forcesAt(*best, clockMS), FasterPct: 100 * float64(bestMS-clockMS) / float64(bestMS)}
}

// median is the middle value, or the mean of the two middle ones for an even count (sorting a copy).
func median[T ~int64 | ~float64](values []T) T {
	sorted := slices.Sorted(slices.Values(values))
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}
