package service

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"wow-live/backend/models"
)

// ErrNoSuchKey is returned for a saved key that isn't there.
var ErrNoSuchKey = errors.New("no saved key starts then")

// Latest is the running key as of now (ms since the epoch), with its projected result, or else the last one that
// ended; false before any key.
func (k *LogTracker) Latest(now int64) (models.KeyView, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	switch {
	case k.current != nil && k.current.raid == nil:
		run := k.current
		quiet := now-k.lastAt > quietMS
		if quiet {
			now = k.lastAt
		}
		v := view(k.details(run, now), clock(run.key.Start, now, len(run.deaths)))
		v.Quiet = quiet
		v.Projection = projection(v.Key, v.ClockMS, k.history)
		if v.Projection == nil {
			v.Projection = fromTopRuns(v.Key, v.ClockMS, k.top[[2]int{v.MapID, v.Level}])
		}
		if v.Forces != nil {
			v.Forces.ReferencePct = referenceForces(v.Key, v.ClockMS, k.history)
		}
		return v, true
	case k.last != nil:
		return finished(*k.last), true
	}
	return models.KeyView{}, false
}

// FirstRun is the running key when it's the first of its dungeon here (no finished runs saved) and its top Warcraft Logs runs
// haven't been asked for: asking marks them, so they're fetched once a dungeon and level. False until COMBATANT_INFO
// names the party's specs.
func (k *LogTracker) FirstRun() (models.Key, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	run := k.current
	if run == nil || run.raid != nil || len(run.party) == 0 {
		return models.Key{}, false
	}
	id := [2]int{run.key.MapID, run.key.Level}
	if k.asked[id] || slices.ContainsFunc(k.history, func(h models.Key) bool { return usable(h, id[0]) }) {
		return models.Key{}, false
	}
	k.asked[id] = true
	key := run.key
	key.Specs = map[string]int{}
	for _, guid := range run.party {
		key.Specs[k.names[guid]] = k.specs[guid]
	}
	return key, true
}

// SetTopRuns keeps the times of a dungeon's top Warcraft Logs runs at a level, for a first run's estimate.
func (k *LogTracker) SetTopRuns(mapID, level int, ms []int64) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.top[[2]int{mapID, level}] = ms
	k.version++
}

// SaveFinished saves the keys that ended since the last call (the log follower calls it after each read), and says
// which (by start time).
func (k *LogTracker) SaveFinished() ([]int64, error) {
	k.mu.Lock()
	done := k.unsaved
	k.unsaved = nil
	k.mu.Unlock()
	var saved []int64
	for i, run := range done {
		err := k.store.Save(run.key)
		if err == nil {
			err = k.store.SaveTrack(run.key.Start, run.track)
		}
		if err != nil {
			k.mu.Lock()
			k.unsaved = append(done[i:], k.unsaved...)
			k.mu.Unlock()
			return saved, fmt.Errorf("saving the %s key (retried after the next read): %w", run.key.Dungeon, err)
		}
		saved = append(saved, run.key.Start)
	}
	if k.raid != nil {
		return saved, k.savePulls()
	}
	return saved, nil
}

// Saved is every saved key, newest first.
func (k *LogTracker) Saved() []models.KeySummary {
	k.mu.RLock()
	defer k.mu.RUnlock()
	saved := []models.KeySummary{}
	for _, key := range k.history {
		summary := models.KeySummary{Start: key.Start, Dungeon: key.Dungeon, Level: key.Level, Status: key.Status,
			FinalMS: key.FinalMS}
		if key.Status == "timed" {
			summary.Chests = chests(key.FinalMS, key.TimerMS)
		}
		saved = append(saved, summary)
	}
	return saved
}

// SavedKey is one saved key as the page shows it.
func (k *LogTracker) SavedKey(start int64) (models.KeyView, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	i := slices.IndexFunc(k.history, func(key models.Key) bool { return key.Start == start })
	if i < 0 {
		return models.KeyView{}, ErrNoSuchKey
	}
	return finished(k.history[i]), nil
}

// DeleteSaved removes one saved key, or every one when start is 0.
func (k *LogTracker) DeleteSaved(start int64) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if start == 0 {
		if err := k.store.Clear(); err != nil {
			return fmt.Errorf("clearing saved keys: %w", err)
		}
		k.history = nil
		return nil
	}
	i := slices.IndexFunc(k.history, func(key models.Key) bool { return key.Start == start })
	if i < 0 {
		return ErrNoSuchKey
	}
	if err := k.store.Delete(start); err != nil {
		return fmt.Errorf("deleting a saved key: %w", err)
	}
	k.history = slices.Delete(k.history, i, i+1)
	return nil
}

// details is a run as the page shows it at a log time: deaths with what covered them or was ready, avoidable hits,
// casts, must-stops, dispels, Bloodlust and the whole run's meter; and for a key, its spikes, pulls, last pull's
// meter, activity, forces and who lacked a buff at the start (a raid pull shows none of those).
func (k *LogTracker) details(run *keyRun, at int64) models.Key {
	key := run.key
	buffs := slices.Clone(run.buffs)
	for open, from := range k.up {
		if slices.Contains(run.party, open[0]) {
			buffs = append(buffs, models.BuffUp{Fight: run.pull, Start: from, End: at, Player: k.names[open[0]],
				Ability: open[1]})
		}
	}
	deaths := slices.Clone(run.deaths)
	kit := LearnKit(run.casts)
	ClassifyDeaths(deaths, run.casts, buffs, kit, int64(k.rules.Defensives.WindowSeconds*1000))
	key.Deaths = list(run.key.Deaths)
	for i := range key.Deaths {
		key.Deaths[i].Defensive, key.Deaths[i].Ready = deaths[i].Defensive, list(deaths[i].Ready)
	}
	key.Players, key.Specs = []string{}, map[string]int{}
	for _, guid := range run.party {
		key.Players = append(key.Players, k.names[guid])
		key.Specs[k.names[guid]] = k.specs[guid]
	}
	key.AvoidableHits = []models.KeyHit{}
	for _, hit := range run.avoidable {
		key.AvoidableHits = append(key.AvoidableHits, *hit)
	}
	slices.SortFunc(key.AvoidableHits, func(a, b models.KeyHit) int { return cmp.Compare(b.Damage, a.Damage) })
	key.Bosses, key.Affixes = list(key.Bosses), list(key.Affixes)
	key.Interrupts, key.Casts = list(key.Interrupts), list(key.Casts)
	key.Stops, key.WastedKicks, key.Debuffs = list(key.Stops), list(key.WastedKicks), list(key.Debuffs)
	slices.SortStableFunc(key.Stops, func(a, b models.KeyStop) int { return cmp.Compare(a.ClockMS, b.ClockMS) })
	key.Overall = meter(run.total, k.classes(run), 0, run.clock(at), run.elapsed(at))
	key.Lusts = list(key.Lusts)
	if run.raid != nil {
		return key
	}
	key.Spikes = k.spikes(run, key.Casts, buffs, kit)
	key.Pulls = priorityPulls(run)
	key.LastPull = run.lastPull
	if len(run.inPull) > 0 {
		key.LastPull = k.pullMeter(run)
		key.LastPull.Ongoing = key.Status == "running" && at-run.pullLast <= pullGapMS
	}
	key.LastPull.Rows = list(key.LastPull.Rows)
	all := make([]int, 0, len(run.pulls))
	for n := range run.pulls {
		all = append(all, n)
	}
	lastPull := run.pull
	if len(run.inPull) == 0 {
		lastPull--
	}
	withPriority(key.Overall.Rows, run, all...)
	withPriority(key.LastPull.Rows, run, lastPull)
	key.BetweenPulls, key.Activity = []int64{}, map[string]float64{}
	for i := 1; i < len(run.track.Pulls); i++ {
		key.BetweenPulls = append(key.BetweenPulls, run.track.Pulls[i].StartMS-run.track.Pulls[i-1].EndMS)
	}
	for player, seconds := range run.fighting {
		active := 0
		for s := range seconds {
			if run.acting[player][s] {
				active++
			}
		}
		key.Activity[player] = 100 * float64(active) / float64(len(seconds))
	}
	key.Buffs = k.buffCheck(run)
	if run.key.Forces != nil {
		forces := *run.key.Forces
		forces.At = list(forces.At)
		finished := key.Status == "timed" || key.Status == "depleted"
		forces.NeedsCheck = finished && forces.Pct < forcesCheckPct
		if finished && forces.Pct < 100 { // the game finishes a key only at 100%: what's missing wasn't counted here
			forces.Pct = 100
			forces.At = append(forces.At, models.ForcesPoint{ClockMS: key.FinalMS, Pct: 100})
		}
		forces.Uncounted = []models.UncountedNPC{}
		for _, u := range run.uncounted {
			forces.Uncounted = append(forces.Uncounted, *u)
		}
		slices.SortFunc(forces.Uncounted, func(a, b models.UncountedNPC) int {
			return cmp.Or(cmp.Compare(b.Deaths, a.Deaths), cmp.Compare(a.NPC, b.NPC))
		})
		key.Forces = &forces
	}
	return key
}

// pullMeter is the meter of the run's current pull.
func (k *LogTracker) pullMeter(run *keyRun) models.KeyMeter {
	return meter(run.inPull, k.classes(run), run.pullAt, run.clock(run.pullLast), run.pullLast-run.pullFrom)
}

// meter is each player's row over a stretch shown from startMS to endMS on the key's clock, with their class, per
// second of the fighting's own length (no death penalty), highest damage first.
func meter(rows map[string]*models.KeyMeterRow, classes map[string]string, startMS, endMS,
	lengthMS int64) models.KeyMeter {
	m := models.KeyMeter{StartMS: startMS, EndMS: endMS, Rows: []models.KeyMeterRow{}}
	seconds := max(float64(lengthMS)/1000, 1)
	for player, r := range rows {
		row := *r
		row.Class, row.DPS, row.HPS = classes[player], float64(r.Damage)/seconds, float64(r.Healing)/seconds
		m.Rows = append(m.Rows, row)
	}
	slices.SortFunc(m.Rows, func(a, b models.KeyMeterRow) int {
		return cmp.Or(cmp.Compare(b.Damage, a.Damage), cmp.Compare(a.Player, b.Player))
	})
	return m
}

// withPriority sets each meter row's damage into priority mobs in the given pulls.
func withPriority(rows []models.KeyMeterRow, run *keyRun, pulls ...int) {
	priority := priorityDamage(run, pulls...)
	for i := range rows {
		rows[i].PriorityDamage = priority[rows[i].Player]
	}
}

// classes is each party member's class by name.
func (k *LogTracker) classes(run *keyRun) map[string]string {
	classes := map[string]string{}
	for _, guid := range run.party {
		classes[k.names[guid]] = specs[k.specs[guid]].Class
	}
	return classes
}

// buffCheck is who lacked a flask, food or a party buff a class in the group gives (augment runes are rare in M+),
// from their auras at the key's start. A buff the log never named counts as missing, so a flask drunk before logging
// started shows.
func (k *LogTracker) buffCheck(run *keyRun) []models.KeyBuffCheck {
	classes := map[string]bool{}
	for _, guid := range run.party {
		classes[specs[k.specs[guid]].Class] = true
	}
	checks := []models.KeyBuffCheck{}
	c := k.rules.Consumables
	for _, guid := range run.party {
		ids, player := run.auras[guid], k.names[guid]
		var have []string
		for _, id := range ids {
			have = append(have, k.spells[id])
		}
		var missing []string
		for kind, patterns := range map[string][]string{"flask": c.Flask, "food": c.Food} {
			if !slices.ContainsFunc(have, func(name string) bool { return named(name, patterns) }) {
				missing = append(missing, kind)
			}
		}
		for buff, class := range k.rules.Buffs {
			if classes[class] && !slices.Contains(have, buff) {
				missing = append(missing, buff)
			}
		}
		if len(missing) > 0 {
			slices.Sort(missing)
			checks = append(checks, models.KeyBuffCheck{Player: player, Missing: missing})
		}
	}
	return checks
}

// killTimes is when each boss died on the key's clock, wipes left out.
func killTimes(bosses []models.KeyBoss) []int64 {
	var at []int64
	for _, b := range bosses {
		if b.Kill {
			at = append(at, b.EndMS)
		}
	}
	return at
}

// view is a key at a clock reading, with the +3 and +2 marks and the chests still in reach. Keys saved before a
// list existed get it empty, so the page always gets lists.
func view(key models.Key, clockMS int64) models.KeyView {
	key.Stops, key.WastedKicks, key.Debuffs = list(key.Stops), list(key.WastedKicks), list(key.Debuffs)
	key.Spikes, key.AvoidableHits, key.Pulls = list(key.Spikes), list(key.AvoidableHits), list(key.Pulls)
	v := models.KeyView{Key: key, ClockMS: clockMS, PenaltyMS: int64(len(key.Deaths)) * deathPenaltyMS,
		Chests: chests(clockMS, key.TimerMS)}
	if key.TimerMS > 0 {
		v.Plus2MS = int64(float64(key.TimerMS) * plus2Share)
		v.Plus3MS = int64(float64(key.TimerMS) * plus3Share)
	}
	return v
}

// finished is an ended key at its final time; an abandoned key has none, so it's when it stopped.
func finished(key models.Key) models.KeyView {
	ms := key.FinalMS
	if ms == 0 {
		ms = clock(key.Start, key.End, len(key.Deaths))
	}
	return view(key, ms)
}

// chests is the upgrade a key's time gives: 3, 2, 1, or 0 when over time; -1 without a timer.
func chests(ms, timerMS int64) int {
	switch {
	case timerMS == 0:
		return -1
	case float64(ms) <= float64(timerMS)*plus3Share:
		return 3
	case float64(ms) <= float64(timerMS)*plus2Share:
		return 2
	case ms <= timerMS:
		return 1
	}
	return 0
}

// list is a copy of s that's never nil, so the page always gets a list.
func list[T any](s []T) []T {
	return append([]T{}, s...)
}
