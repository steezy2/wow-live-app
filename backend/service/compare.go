package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"slices"
	"strings"

	"wow-live/backend/models"
)

const (
	// compareLogs is how many top logs a player review reads: about 8 Warcraft Logs points each (measured Nov 13).
	compareLogs = 20
	// comparePages is how many pages of 100 rankings are read looking for compareLogs similar ones.
	comparePages = 3
	// yourKeysLevels is how far a key's level may be from this key's for it to be one of the player's own similar runs.
	yourKeysLevels = 2
	// minComparePoints: a review isn't run with fewer points left this hour: its measured cost (compareLogs times 8,
	// and the rankings) plus 20%.
	minComparePoints = 200
)

// ErrNoSuchPlayer is returned when asked to review a player who wasn't in the fight, or whose spec isn't known.
var ErrNoSuchPlayer = errors.New("no such player in that fight")

// raidDifficulty is Warcraft Logs' number for a raid difficulty; LFR isn't ranked.
var raidDifficulty = map[string]int{"Mythic": 5, "Heroic": 4, "Normal": 3}

// Comparer reviews one player of a finished key or raid pull against their own similar runs, the top Warcraft Logs
// logs of their spec and their own parses, keeping each review in the store.
type Comparer struct {
	Keys       *LogTracker
	Baselines  models.CompareBaselines
	Store      models.CompareStore
	Characters Characters
	Region     string
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
	// partial: the key was abandoned or the pull wiped, so the top logs are read over the same stretch.
	partial bool
	local   models.RankedPlayer
	yours   []models.RankedPlayer
}

// Cached is a player's review if one was made, and the fight didn't go on after it.
func (c Comparer) Cached(kind models.FightKind, start int64, player string) (models.PlayerCompare, error) {
	review, err := c.Store.Compare(kind, start, player)
	if errors.Is(err, fs.ErrNotExist) {
		return review, ErrNoSuchKey
	}
	if err == nil {
		if f, ferr := c.load(kind, start, player); ferr == nil && review.End != 0 && review.End != f.end {
			return review, ErrNoSuchKey
		}
	}
	return review, err
}

// Run is a player's review: the stored one, or a new one, which spends Warcraft Logs points.
func (c Comparer) Run(ctx context.Context, kind models.FightKind, start int64, player string) (models.PlayerCompare, error) {
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
	var top []models.RankedPlayer
	var logs []models.CompareLog
	for _, r := range runs {
		p, err := c.Baselines.RunPlayer(ctx, r.Code, r.FightID, r.Name, r.Server, window)
		if errors.Is(err, models.ErrRateLimited) || errors.Is(err, models.ErrNoCredentials) ||
			errors.Is(err, models.ErrBadCredentials) || ctx.Err() != nil {
			return models.PlayerCompare{}, err
		}
		if err != nil {
			log.Printf("reading %s in %s#%d for a player review: %v", r.Name, r.Code, r.FightID, err)
			continue
		}
		top = append(top, p)
		logs = append(logs, models.CompareLog{Name: r.Name, Server: r.Server, Code: r.Code, FightID: r.FightID,
			DurationMS: r.DurationMS, Amount: r.Amount})
	}
	in := compareInput{you: f.local, yours: f.yours, top: top, logs: logs, metric: mainMetric(f.spec), spec: f.spec}
	parses, err := c.Characters.Parses(ctx, c.Region, f.character)
	if err == nil {
		in.parse = parseOf(parses, f.encounter, f.kind)
	}
	review := buildCompare(in)
	review.Kind, review.Start, review.End, review.Player, review.Spec = f.kind, f.start, f.end, player, f.spec.label()
	review.WindowMS, review.Baseline = window, baseline
	if err == nil {
		review.Parses = &parses
	}
	if after, err := c.Baselines.PointsLeft(ctx); err == nil {
		review.Points = left - after - 1 // the second points check costs about 1
	}
	return review, c.Store.SaveCompare(review)
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
		_, pulls := c.Keys.History()
		f := fightRun{kind: kind, start: start, end: pull.End, encounter: pull.Boss, boss: raidDifficulty[pull.Difficulty],
			character: character(player, pull.Realms), spec: spec, partial: !pull.Kill,
			local: c.pullPlayer(pull, player)}
		for _, p := range similarPulls(pulls, pull, player) {
			f.yours = append(f.yours, c.pullPlayer(p, player))
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
	keys, _ := c.Keys.History()
	f := fightRun{kind: kind, start: start, end: key.End, encounter: key.Dungeon, level: key.Level,
		character: character(player, key.Realms), spec: spec, partial: key.Status == "abandoned",
		local: c.keyPlayer(key, player)}
	for _, k := range similarKeys(keys, key, player) {
		f.yours = append(f.yours, c.keyPlayer(k, player))
	}
	return f, nil
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
	return slices.DeleteFunc(slices.Clone(keys), func(k models.Key) bool {
		return k.Start == key.Start || k.MapID != key.MapID || abs(int64(k.Level-key.Level)) > yourKeysLevels ||
			k.Specs[player] == 0 || k.Specs[player] != key.Specs[player] || len(k.Abilities) == 0
	})
}

// similarPulls are a raider's other pulls of the same boss and difficulty in the same spec, with abilities saved.
func similarPulls(pulls []models.RaidPull, pull models.RaidPull, player string) []models.RaidPull {
	return slices.DeleteFunc(slices.Clone(pulls), func(p models.RaidPull) bool {
		return p.Start == pull.Start || p.Boss != pull.Boss || p.Difficulty != pull.Difficulty ||
			p.Specs[player] == 0 || p.Specs[player] != pull.Specs[player] || len(p.Abilities) == 0
	})
}

// keyPlayer is a player's numbers in a key, and pullPlayer in a raid pull.
func (c Comparer) keyPlayer(key models.Key, player string) models.RankedPlayer {
	track, _ := c.Keys.Track(key.Start)
	return localPlayer(lengthMS(key), key.Overall, key.Abilities, key.Activity, key.Deaths, track, player)
}

func (c Comparer) pullPlayer(pull models.RaidPull, player string) models.RankedPlayer {
	track, _ := c.Keys.RaidTrack(pull.Start)
	deaths := make([]models.KeyDeath, len(pull.Deaths))
	for i, d := range pull.Deaths {
		deaths[i] = d.KeyDeath
	}
	return localPlayer(pull.DurationMS, pull.Meter, pull.Abilities, pull.Activity, deaths, track, player)
}

// localPlayer is a player's numbers in a saved fight in the shape the Warcraft Logs side comes in.
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
	for _, s := range track.Buffs[player] {
		p.Buffs = append(p.Buffs, models.RankedBuff{ID: s.ID, Name: s.Name, UptimeMS: s.ToMS - s.FromMS})
	}
	return p
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
		return nil, "", fmt.Errorf("no Warcraft Logs encounter for %s", f.encounter)
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
