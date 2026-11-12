package service

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

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
		v := view(k.details(run, now), clock(run.key.Start, now, len(run.deaths)), k.history)
		v.SilentMS = max(0, now-k.lastAt)
		v.Projection = projection(v.Key, v.ClockMS, k.history)
		if v.Projection == nil {
			v.Projection = fromTopRuns(v.Key, v.ClockMS, k.top[[2]int{v.MapID, v.Level}])
		}
		if v.Forces != nil {
			v.Forces.Pace = forcesPace(bestRun(v.Key, k.history), v.Forces.Pct, v.ClockMS)
		}
		return v, true
	case k.last != nil:
		return finished(*k.last, k.history), true
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
	var saved []int64
	err := saveQueued(&k.mu, &k.unsaved, func(run savedRun) error {
		skip, err := keepLocal(&k.mu, &k.history, run.key, sameKey, func(key models.Key) bool { return key.Imported },
			func(key models.Key) int64 { return key.Start }, k.store.Delete)
		if skip || err != nil {
			return err
		}
		err = k.store.Save(run.key)
		if err == nil {
			err = k.store.SaveTrack(run.key.Start, run.track)
		}
		if err != nil {
			return fmt.Errorf("saving the %s key (retried after the next read): %w", run.key.Dungeon, err)
		}
		saved = append(saved, run.key.Start)
		return nil
	})
	if err == nil && k.raid != nil {
		err = k.savePulls()
	}
	return saved, err
}

// saveQueued saves queue's items in order with save, then takes the saved ones off it; the first that fails stays
// queued with those after it, for the next call. Items stay queued while they're written, so readers still find them.
// Only the tracker appends to queue (under mu), so the saved ones are still its first.
func saveQueued[T any](mu *sync.RWMutex, queue *[]T, save func(T) error) error {
	mu.RLock()
	done := slices.Clone(*queue)
	mu.RUnlock()
	saved := 0
	defer func() {
		mu.Lock()
		*queue = (*queue)[saved:]
		mu.Unlock()
	}()
	for _, item := range done {
		if err := save(item); err != nil {
			return err
		}
		saved++
	}
	return nil
}

// Saved is every saved key, newest first.
func (k *LogTracker) Saved() []models.KeySummary {
	k.mu.RLock()
	defer k.mu.RUnlock()
	saved := []models.KeySummary{}
	for _, key := range k.history {
		summary := models.KeySummary{Start: key.Start, Dungeon: key.Dungeon, Level: key.Level, Status: key.Status,
			FinalMS: key.FinalMS}
		if key.Imported {
			summary.Imported = key.Report.URL()
		}
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
	return finished(k.history[i], k.history), nil
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
// casts, must-stops, dispels, Bloodlust, the whole run's meter, spikes, activity and who lacked a buff at the start;
// and for a key, its pulls, last pull's meter and forces (a raid pull shows none of those).
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
	if run.raid != nil {
		addKit(kit, k.raid.kit)
	}
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
	key.Interrupts, key.Casts = list(key.Interrupts), append(k.prepots(run), key.Casts...)
	key.Stops, key.WastedKicks, key.Debuffs = list(key.Stops), list(key.WastedKicks), list(key.Debuffs)
	slices.SortStableFunc(key.Stops, func(a, b models.KeyStop) int { return cmp.Compare(a.ClockMS, b.ClockMS) })
	key.Overall = meter(run.total, k.classes(run), 0, run.clock(at), run.elapsed(at))
	key.Lusts = list(key.Lusts)
	key.Spikes, key.Activity = k.spikes(run, key.Casts, buffs, kit), map[string]float64{}
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
	if run.raid != nil {
		return key
	}
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
	key.BetweenPulls = []int64{}
	for i := 1; i < len(run.track.Pulls); i++ {
		key.BetweenPulls = append(key.BetweenPulls, run.track.Pulls[i].StartMS-run.track.Pulls[i-1].EndMS)
	}
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
	if key.Status != "running" {
		key.Lanes = keyLanes(key, run.track.Pulls)
		key.Suggestions = keySuggestions(key, bestRun(key, k.history), k.rules.Suggestions)
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

// buffCheck is who lacked a flask, food (in a raid, their tier set's bonuses too, and a Vantus Rune where the raid's
// rune works) or a party buff a class in the group gives (augment runes are rare in M+), from their auras and gear at
// the key's or pull's start. A buff the log never named counts as missing unless a consumable lists its ID, so a
// flask drunk before logging started shows.
func (k *LogTracker) buffCheck(run *keyRun) []models.KeyBuffCheck {
	classes, difficulty := map[string]bool{}, ""
	for _, guid := range run.party {
		classes[specs[k.specs[guid]].Class] = true
	}
	if run.raid != nil {
		difficulty = run.raid.difficulty
	}
	buffs := k.applicableBuffs(classes)
	checks := []models.KeyBuffCheck{}
	for _, guid := range run.party {
		have := map[int]string{}
		for _, id := range run.auras[guid] {
			have[id] = k.spells[id]
		}
		var missing []string
		for kind, c := range k.consumables(difficulty) {
			if !c.had(have) {
				missing = append(missing, kind)
			}
		}
		names := slices.Collect(maps.Values(have))
		for _, buff := range buffs {
			if !slices.Contains(names, buff) {
				missing = append(missing, buff)
			}
		}
		if run.raid != nil {
			missing = append(missing, k.tierMissing(guid, run.gear[guid])...)
		}
		if len(missing) > 0 {
			slices.Sort(missing)
			checks = append(checks, models.KeyBuffCheck{Player: k.names[guid], Missing: missing})
		}
	}
	return checks
}

// tierMissing is the tier bonus a raider (by GUID) lacks with their equipped items: "2-set" under 2 of their class's
// tier pieces, "4-set" under 4; nothing for a class without a tier list.
func (k *LogTracker) tierMissing(guid string, gear []int) []string {
	tier := k.raid.tier[specs[k.specs[guid]].Class]
	if len(tier) == 0 {
		return nil
	}
	switch n := len(slices.DeleteFunc(slices.Clone(gear), func(id int) bool { return !slices.Contains(tier, id) })); {
	case n < 2:
		return []string{"2-set"}
	case n < 4:
		return []string{"4-set"}
	}
	return nil
}

// castBy is whether player has a cast of ability in casts.
func castBy(casts []models.KeyCast, player, ability string) bool {
	return slices.ContainsFunc(casts, func(c models.KeyCast) bool { return c.Player == player && c.Ability == ability })
}

// prepots are the combat potions drunk before the run started, at 0: a potion's buff in a member's pull-start auras,
// unless the run already has that member's potion of that name (drunk in the run, or its buff seen leaving).
func (k *LogTracker) prepots(run *keyRun) []models.KeyCast {
	out := []models.KeyCast{}
	for _, guid := range run.party {
		player := k.names[guid]
		for _, id := range run.auras[guid] {
			name := k.spells[id]
			if k.kinds[name] != "combat potion" && !slices.Contains(k.rules.Consumables.CombatPotionIDs, id) {
				continue
			}
			if name == "" {
				name = combatPotion
			}
			if !castBy(run.key.Casts, player, name) {
				out = append(out, models.KeyCast{Player: player, Ability: name, Kind: "combat potion", Prepot: true})
			}
		}
	}
	return out
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

// view is a key at a clock reading, with the +3 and +2 marks, the chests still in reach and each boss's length in the
// best other run at the level (from history). Keys saved before a list existed get it empty, so the page always gets
// lists.
func view(key models.Key, clockMS int64, history []models.Key) models.KeyView {
	key.Stops, key.WastedKicks, key.Debuffs = list(key.Stops), list(key.WastedKicks), list(key.Debuffs)
	key.Spikes, key.AvoidableHits, key.Pulls = list(key.Spikes), list(key.AvoidableHits), list(key.Pulls)
	v := models.KeyView{Key: key, ClockMS: clockMS, PenaltyMS: int64(len(key.Deaths)) * deathPenaltyMS,
		Chests: chests(clockMS, key.TimerMS)}
	if key.TimerMS > 0 {
		v.Plus2MS = int64(float64(key.TimerMS) * plus2Share)
		v.Plus3MS = int64(float64(key.TimerMS) * plus3Share)
	}
	if best := bestRun(key, history); best != nil {
		v.BestFinalMS = best.FinalMS
		v.BestBosses = map[string]int64{}
		for _, b := range best.Bosses {
			if b.Kill {
				v.BestBosses[b.Name] = b.EndMS - b.StartMS
			}
		}
	}
	return v
}

// finished is an ended key at its time, against the other saved runs (history).
func finished(key models.Key, history []models.Key) models.KeyView {
	return view(key, lengthMS(key), history)
}

// lengthMS is an ended key's time: the game's own, or for an abandoned key (which has none) its clock when it stopped.
func lengthMS(key models.Key) int64 {
	if key.FinalMS > 0 {
		return key.FinalMS
	}
	return clock(key.Start, key.End, len(key.Deaths))
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
