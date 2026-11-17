package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"slices"
	"strings"
	"sync"

	"wow-live/backend/models"
)

const (
	// compareLogs is how many top logs a player review reads: about 8 Warcraft Logs points each (measured Nov 13).
	compareLogs = 20
	// comparePages is how many pages of 100 rankings are read looking for compareLogs similar ones.
	comparePages = 3
	// yourKeysLevels is how far a key's level may be from this key's for it to be one of the player's own similar runs.
	yourKeysLevels = 2
	// yourRunsMax is how many of the player's own similar runs (the newest) a review reads, as each is a track decoded.
	yourRunsMax = 10
	// minComparePoints: a review isn't run with fewer points left this hour: its measured cost (compareLogs times 8,
	// and the rankings) plus 20%.
	minComparePoints = 200
)

// ErrNoSuchPlayer is returned when asked to review a player who wasn't in the fight, or whose spec isn't known.
var ErrNoSuchPlayer = errors.New("no such player in that fight")

// ErrNoTopLogs is returned when no top log could be read to compare with (none ranked, or each one failed).
var ErrNoTopLogs = errors.New("no top Warcraft Logs run could be read to compare with")

// Comparer reviews one player of a finished key or raid pull against their own similar runs, the top Warcraft Logs
// logs of their spec and their own parses, keeping each review in the store.
type Comparer struct {
	Keys       *LogTracker
	Baselines  models.CompareBaselines
	Store      models.CompareStore
	Characters Characters
	Region     string
	Rules      models.SpecRules
	// Talents are the spec trees by spec ID, to name the talents a review compares.
	Talents map[int]models.TalentTree
}

// fightRun is the finished key or pull a review is of: what it was, the player's numbers in it and in their own
// similar runs.
type fightRun struct {
	kind      models.FightKind
	start     int64
	end       int64
	encounter string // the dungeon or boss
	level     int    // a key's
	boss      int    // a pull's difficulty in Warcraft Logs' numbers
	character string // "Name-Realm", or the bare name when the realm isn't known
	spec      specInfo
	specID    int
	// partial: the key was abandoned or the pull wiped, so the top logs are read over the same stretch.
	partial bool
	local   models.RankedPlayer
	yours   []models.RankedPlayer
	// casts and buffs are the player's own, from the track, for the spec's rules.
	casts []models.TrackCast
	buffs []models.BuffSpan
}

// compareVersion is the version of what a review holds. Bump it when a review's numbers or fields change, so the
// reviews already stored (which would show the old ones) are read again.
const compareVersion = 2

// Cached is a player's review if one was made by this version, and the fight didn't go on after it.
func (c Comparer) Cached(kind models.FightKind, start int64, player string) (models.PlayerCompare, error) {
	review, err := c.Store.Compare(kind, start, player)
	if errors.Is(err, fs.ErrNotExist) {
		return review, ErrNoSuchKey
	}
	if err == nil && review.Version != compareVersion {
		return review, ErrNoSuchKey
	}
	if end, ferr := c.fightEnd(kind, start); err == nil && ferr == nil && review.End != 0 && review.End != end {
		return review, ErrNoSuchKey
	}
	return review, err
}

// fightEnd is when a saved key or pull ended (a key closed as abandoned can go on after a review was made).
func (c Comparer) fightEnd(kind models.FightKind, start int64) (int64, error) {
	if kind == models.FightRaid {
		pull, err := c.Keys.RaidPull(start)
		return pull.End, err
	}
	key, _, err := c.Keys.finished(start)
	return key.End, err
}

// Run is a player's review: the stored one, or a new one, which spends Warcraft Logs points.
func (c Comparer) Run(ctx context.Context, kind models.FightKind, start int64, player string) (models.PlayerCompare,
	error) {
	reviewing.Lock()
	defer reviewing.Unlock()
	if review, err := c.Cached(kind, start, player); err == nil {
		return review, nil
	}
	f, err := c.load(kind, start, player)
	if err != nil {
		return models.PlayerCompare{}, err
	}
	left, err := c.Baselines.PointsLeft(ctx)
	if err != nil {
		return models.PlayerCompare{}, fmt.Errorf("reading the points left: %w", err)
	}
	if left < minComparePoints {
		return models.PlayerCompare{}, ErrNoPoints
	}
	runs, baseline, err := c.topRuns(ctx, f)
	if err != nil {
		return models.PlayerCompare{}, err
	}
	window := int64(0)
	if f.partial {
		window = f.local.DurationMS
	}
	parsing := make(chan error, 1) // the character's parses are one query that doesn't need the top logs: ask it with them
	var parses models.CharacterParses
	go func() {
		var err error
		parses, err = c.Characters.Parses(ctx, c.Region, f.character)
		parsing <- err
	}()
	top, logs, err := c.readTop(ctx, f, runs, window)
	perr := <-parsing
	if err != nil {
		return models.PlayerCompare{}, err
	}
	in := compareInput{you: f.local, yours: f.yours, top: top, logs: logs, metric: mainMetric(f.spec), spec: f.spec,
		specID: f.specID, tree: c.Talents[f.specID]}
	if f.kind == models.FightMPlus {
		in.raidOnly = c.Keys.RaidOnlyBuff
	}
	if perr == nil {
		in.parse = parseOf(parses, f.encounter, f.kind)
	}
	review := buildCompare(in)
	review.Rules = failedRules(c.Rules[f.spec.label()].Rules, f.casts, f.buffs, f.local.DurationMS)
	review.Version = compareVersion
	review.Kind, review.Start, review.End, review.Player, review.Spec = f.kind, f.start, f.end, player, f.spec.label()
	review.WindowMS, review.Baseline = window, baseline
	if perr == nil {
		review.Parses = &parses
	}
	if after, err := c.Baselines.PointsLeft(ctx); err == nil {
		review.Points = left - after - 1 // the second points check costs about 1
	}
	return review, c.Store.SaveCompare(review)
}

// compareWorkers is how many top logs are read at once: each is a few Warcraft Logs queries, so reading them one after
// another waits on the network for most of a review.
const compareWorkers = 5

// readTop reads each ranked run's player (over window, 0 for all of it), compareWorkers at a time, and returns them
// with their logs in ranking order, leaving out a log that can't be read. A rate limit, missing or bad credentials or
// a stopped context end the review at once.
func (c Comparer) readTop(ctx context.Context, f fightRun, runs []models.RankedRun, window int64) (
	[]models.RankedPlayer, []models.CompareLog, error) {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	players := make([]*models.RankedPlayer, len(runs))
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		fatal error
		slots = make(chan struct{}, compareWorkers)
	)
	for i, r := range runs {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer func() { <-slots; wg.Done() }()
			p, err := c.Baselines.RunPlayer(ctx, r.Code, r.FightID, r.Name, r.Server, window)
			switch {
			case errors.Is(err, models.ErrRateLimited) || errors.Is(err, models.ErrNoCredentials) ||
				errors.Is(err, models.ErrBadCredentials) || ctx.Err() != nil:
				mu.Lock()
				if fatal == nil {
					fatal = err
					if fatal == nil {
						fatal = ctx.Err()
					}
				}
				mu.Unlock()
				stop()
			case err != nil:
				log.Printf("reading %s in %s#%d for a player review: %v", r.Name, r.Code, r.FightID, err)
			default:
				players[i] = &p
			}
		}()
	}
	wg.Wait()
	if fatal != nil {
		return nil, nil, fatal
	}
	var top []models.RankedPlayer
	var logs []models.CompareLog
	for i, p := range players {
		if p == nil {
			continue
		}
		r := runs[i]
		amount := r.Amount
		if window > 0 { // the ranking's is over the whole fight
			amount = rateOf(*p, mainMetric(f.spec))
		}
		top = append(top, *p)
		logs = append(logs, models.CompareLog{Name: r.Name, Server: r.Server, Code: r.Code, FightID: r.FightID,
			DurationMS: r.DurationMS, Amount: amount})
	}
	if len(top) == 0 {
		return nil, nil, ErrNoTopLogs
	}
	return top, logs, nil
}

// parseOf is the character's parse of one dungeon or boss, nil when they have none there.
func parseOf(parses models.CharacterParses, encounter string, kind models.FightKind) *models.CharacterParse {
	i := slices.IndexFunc(parses.Parses, func(p models.CharacterParse) bool {
		return p.Kind == kind && p.Encounter == encounter
	})
	if i < 0 {
		return nil
	}
	return &parses.Parses[i]
}

// load is the fight, the player's numbers in it and their own similar runs; errors are ErrNoSuchKey (or ErrNoSuchPull),
// ErrKeyRunning and ErrNoSuchPlayer.
func (c Comparer) load(kind models.FightKind, start int64, player string) (fightRun, error) {
	if kind == models.FightRaid {
		pull, err := c.Keys.RaidPull(start)
		if err != nil {
			return fightRun{}, err
		}
		spec, ok := specs[pull.Specs[player]]
		if !ok {
			return fightRun{}, ErrNoSuchPlayer
		}
		if models.RankedDifficultyWCL(pull.Difficulty) == 0 {
			return fightRun{}, fmt.Errorf("%w: Warcraft Logs ranks no %s pulls", ErrNoTopLogs, pull.Difficulty)
		}
		track, err := readTrack(c.Keys.RaidTrack, start, ErrNoSuchPull)
		if err != nil {
			return fightRun{}, err
		}
		f := fightRun{kind: kind, start: start, end: pull.End, encounter: pull.Boss,
			boss:      models.RankedDifficultyWCL(pull.Difficulty),
			character: character(player, pull.Realms), spec: spec, specID: pull.Specs[player], partial: !pull.Kill,
			local: pullPlayer(pull, track, player), casts: track.Casts[player], buffs: track.Buffs[player]}
		f.local.Talents = pull.Talents[player]
		_, pulls := c.Keys.History()
		for _, p := range similarPulls(pulls, pull, player) {
			t, err := readTrack(c.Keys.RaidTrack, p.Start, ErrNoSuchPull)
			if err != nil {
				return fightRun{}, err
			}
			f.yours = append(f.yours, pullPlayer(p, t, player))
		}
		return f, nil
	}
	key, _, err := c.Keys.finished(start)
	if err != nil {
		return fightRun{}, err
	}
	spec, ok := specs[key.Specs[player]]
	if !ok {
		return fightRun{}, ErrNoSuchPlayer
	}
	track, err := readTrack(c.Keys.Track, start, ErrNoSuchKey)
	if err != nil {
		return fightRun{}, err
	}
	f := fightRun{kind: kind, start: start, end: key.End, encounter: key.Dungeon, level: key.Level,
		character: character(player, key.Realms), spec: spec, specID: key.Specs[player],
		partial: key.Status == models.StatusAbandoned, local: keyPlayer(key, track, player), casts: track.Casts[player],
		buffs: track.Buffs[player]}
	f.local.Talents = key.Talents[player]
	keys, _ := c.Keys.History()
	for _, k := range similarKeys(keys, key, player) {
		t, err := readTrack(c.Keys.Track, k.Start, ErrNoSuchKey)
		if err != nil {
			return fightRun{}, err
		}
		f.yours = append(f.yours, keyPlayer(k, t, player))
	}
	return f, nil
}

// readTrack reads a saved fight's track with read; a fight saved without one (the error wraps none) has an empty one.
func readTrack(read func(int64) (models.KeyTrack, error), start int64, none error) (models.KeyTrack, error) {
	track, err := read(start)
	if err != nil && !errors.Is(err, none) {
		return track, fmt.Errorf("reading a track: %w", err)
	}
	return track, nil
}

// character is "Name-Realm" for a player whose realm the fight kept, else the bare name.
func character(player string, realms map[string]string) string {
	if realm := realms[player]; realm != "" {
		return player + "-" + realm
	}
	return player
}

// similarKeys are a player's own earlier or later runs like key: its dungeon, a level within yourKeysLevels, the same
// spec for that player, this key excluded, and abilities saved (older keys would read as no casts at all).
func similarKeys(keys []models.Key, key models.Key, player string) []models.Key {
	similar := slices.DeleteFunc(slices.Clone(keys), func(k models.Key) bool {
		return k.MapID != key.MapID || abs(int64(k.Level-key.Level)) > yourKeysLevels ||
			!otherRunSameSpec(player, key.Start, k.Start, key.Specs, k.Specs, len(k.Abilities))
	})
	return similar[:min(len(similar), yourRunsMax)]
}

// otherRunSameSpec is whether a saved run (start, specs, abilities saved) is another run than the one reviewed where
// the player played the same spec; abilities is 0 for runs saved before they were kept, which would read as no casts.
func otherRunSameSpec(player string, start, otherStart int64, specs, otherSpecs map[string]int, abilities int) bool {
	return otherStart != start && otherSpecs[player] != 0 && otherSpecs[player] == specs[player] && abilities > 0
}

// similarPulls are a raider's other pulls of the same boss and difficulty in the same spec, with abilities saved.
func similarPulls(pulls []models.RaidPull, pull models.RaidPull, player string) []models.RaidPull {
	similar := slices.DeleteFunc(slices.Clone(pulls), func(p models.RaidPull) bool {
		return p.Boss != pull.Boss || p.Difficulty != pull.Difficulty ||
			!otherRunSameSpec(player, pull.Start, p.Start, pull.Specs, p.Specs, len(p.Abilities))
	})
	return similar[:min(len(similar), yourRunsMax)]
}

// keyPlayer is a player's numbers in a key (with its track), and pullPlayer in a raid pull.
func keyPlayer(key models.Key, track models.KeyTrack, player string) models.RankedPlayer {
	return localPlayer(lengthMS(key), key.Overall, key.Abilities, nil, key.Deaths, track, player)
}

func pullPlayer(pull models.RaidPull, track models.KeyTrack, player string) models.RankedPlayer {
	deaths := make([]models.KeyDeath, len(pull.Deaths))
	for i, d := range pull.Deaths {
		deaths[i] = d.KeyDeath
	}
	return localPlayer(pull.DurationMS, pull.Meter, pull.Abilities, pull.Activity, deaths, track, player)
}

// localPlayer is a player's numbers in a saved fight in the shape the Warcraft Logs side comes in. activity is each
// player's share of their time alive acting (a raid pull's, which is the whole fight Warcraft Logs measures); a key's
// is over its pulls only, not comparable with Warcraft Logs' whole-key active time, so keys pass nil.
func localPlayer(durationMS int64, meter models.KeyMeter, abilities []models.KeyAbility, activity map[string]float64,
	deaths []models.KeyDeath, track models.KeyTrack, player string) models.RankedPlayer {
	p := models.RankedPlayer{DurationMS: durationMS, Abilities: []models.RankedAbility{}, Buffs: []models.RankedBuff{}}
	for _, r := range meter.Rows {
		if r.Player == player {
			p.Damage, p.Healing, p.Taken = r.Damage, r.Healing, r.DamageTaken
		}
	}
	p.ActiveMS = int64(activity[player] / 100 * float64(durationMS))
	for _, d := range deaths {
		if d.Player == player {
			p.Deaths++
		}
	}
	for _, a := range abilities {
		if a.Player == player {
			p.Abilities = append(p.Abilities, models.RankedAbility{ID: a.ID, Name: a.Ability, Damage: a.Damage,
				Healing: a.Healing, Casts: a.Casts, Hits: a.Hits})
		}
	}
	p.Buffs = buffUptimes(track.Buffs[player])
	return p
}

// buffUptimes is one buff per name with the time any of its stretches was up: a buff with several spell IDs, or a
// refresh while up, would otherwise count the same time twice and read over 100%.
func buffUptimes(spans []models.BuffSpan) []models.RankedBuff {
	byName := map[string][]models.BuffSpan{}
	for _, s := range spans {
		byName[s.Name] = append(byName[s.Name], s)
	}
	out := []models.RankedBuff{}
	for _, name := range slices.Sorted(maps.Keys(byName)) {
		group := byName[name]
		slices.SortFunc(group, func(a, b models.BuffSpan) int { return cmp.Compare(a.FromMS, b.FromMS) })
		up, end := int64(0), int64(0)
		for _, s := range group {
			up += max(0, s.ToMS-max(s.FromMS, end))
			end = max(end, s.ToMS)
		}
		out = append(out, models.RankedBuff{ID: group[0].ID, Name: name, UptimeMS: up})
	}
	return out
}

// topRuns is the ranked runs to compare with: up to compareLogs of the spec's rankings for the dungeon at the key's
// level or the boss on its difficulty, read a page at a time (comparePages at most), and a line saying which they are.
func (c Comparer) topRuns(ctx context.Context, f fightRun) ([]models.RankedRun, string, error) {
	q := models.RankQuery{Level: f.level, Difficulty: f.boss, Class: f.spec.Class, Spec: f.spec.Name,
		Metric: strings.ToLower(mainMetric(f.spec))}
	var ids map[string]int
	var err error
	if f.kind == models.FightRaid {
		ids, err = c.Baselines.RaidEncounters(ctx, []string{f.encounter})
	} else {
		ids, err = c.Baselines.MPlusEncounters(ctx)
	}
	if err != nil {
		return nil, "", err
	}
	if q.Encounter = ids[f.encounter]; q.Encounter == 0 {
		return nil, "", fmt.Errorf("%w: Warcraft Logs ranks no %s", ErrNoTopLogs, f.encounter)
	}
	var ranked []models.RankedRun
	for page := 1; page <= comparePages; page++ {
		runs, more, err := c.Baselines.RankedRuns(ctx, q, page)
		if err != nil {
			return nil, "", err
		}
		ranked = append(ranked, runs...)
		if !more || len(pickRuns(ranked, f.local.DurationMS, f.partial).runs) >= compareLogs {
			break
		}
	}
	pick := pickRuns(ranked, f.local.DurationMS, f.partial)
	return pick.runs, pick.baseline(f.spec.label(), f.local.DurationMS), nil
}

// picked is the ranked runs chosen to compare with and why these.
type picked struct {
	runs []models.RankedRun
	// how: "near" within similarTime of the time, "closest" when none were, "longer" for a stretch of a longer fight.
	how string
}

// pickRuns chooses up to compareLogs runs by time: those within similarTime of ms, closest first; when none are, the
// closest overall; for a partial fight (a wipe, an abandoned key) the shortest runs at least as long as it ran, since
// every number is then read over its length.
func pickRuns(ranked []models.RankedRun, ms int64, partial bool) picked {
	byCloseness := func(a, b models.RankedRun) int { return cmp.Compare(abs(a.DurationMS-ms), abs(b.DurationMS-ms)) }
	var out picked
	switch {
	case partial:
		out.how = "longer"
		out.runs = slices.DeleteFunc(slices.Clone(ranked), func(r models.RankedRun) bool { return r.DurationMS < ms })
		slices.SortStableFunc(out.runs, func(a, b models.RankedRun) int { return cmp.Compare(a.DurationMS, b.DurationMS) })
	default:
		out.how = "near"
		out.runs = slices.DeleteFunc(slices.Clone(ranked), func(r models.RankedRun) bool {
			return float64(abs(r.DurationMS-ms)) > similarTime*float64(ms)
		})
		if len(out.runs) == 0 {
			out.how, out.runs = "closest", slices.Clone(ranked)
		}
		slices.SortStableFunc(out.runs, byCloseness)
	}
	out.runs = out.runs[:min(len(out.runs), compareLogs)]
	return out
}

// baseline says which logs the top column is, as the review shows it.
func (p picked) baseline(spec string, ms int64) string {
	n := len(p.runs)
	switch {
	case n == 0:
		return fmt.Sprintf("no ranked %s runs found", spec)
	case p.how == "longer":
		return fmt.Sprintf("%d ranked %s runs at least as long as %s, read over their first %s", n, spec, clockText(ms),
			clockText(ms))
	case p.how == "closest":
		return fmt.Sprintf("closest %d ranked %s runs; none within %.0f%% of %s", n, spec, similarTime*100, clockText(ms))
	}
	return fmt.Sprintf("%d ranked %s runs within %.0f%% of %s", n, spec, similarTime*100, clockText(ms))
}
