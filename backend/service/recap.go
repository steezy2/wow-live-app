package service

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"maps"
	"slices"
	"time"

	"wow-live/backend/models"
)

// recapMates is how many of the players the owner ran with most a recap names, and recapKillers how many of what
// killed them.
const recapMates, recapKillers = 8, 5

// SeasonSource gives when the current Mythic+ season starts and ends in a region, the end zero while it runs
// (raiderio.Client).
type SeasonSource interface {
	SeasonDates(ctx context.Context, region string) (start, end time.Time, err error)
}

// Recaps builds the Recap tab from the saved keys and raid pulls: the periods to pick from and a period's recap.
type Recaps struct {
	Saved  func() ([]models.Key, []models.RaidPull) // LogTracker.History
	Season SeasonSource
	Region string
	Now    func() time.Time
}

// History is every saved key and raid pull as the tracker holds them, newest first.
func (k *LogTracker) History() ([]models.Key, []models.RaidPull) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return slices.Clone(k.history), slices.Clone(k.pulls)
}

// Periods are the periods a recap covers at now, in the PC's time zone: this month, last month, this Mythic+ season
// (left out when Raider.IO can't say), this year and all time.
func (r Recaps) Periods(ctx context.Context) []models.RecapPeriod {
	now := r.Now()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periods := []models.RecapPeriod{{Label: "This month", From: month.UnixMilli(), To: now.UnixMilli()},
		{Label: "Last month", From: month.AddDate(0, -1, 0).UnixMilli(), To: month.UnixMilli()}}
	if start, end, err := r.Season.SeasonDates(ctx, r.Region); err != nil {
		log.Printf("reading the season's dates: %v", err)
	} else {
		if end.IsZero() || end.After(now) {
			end = now
		}
		periods = append(periods, models.RecapPeriod{Label: "This season", From: start.UnixMilli(), To: end.UnixMilli()})
	}
	year := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	return append(periods, models.RecapPeriod{Label: "This year", From: year.UnixMilli(), To: now.UnixMilli()},
		models.RecapPeriod{Label: "All time", From: 0, To: now.UnixMilli()})
}

// Recap is the keys and raid pulls that started from from to to (ms since the epoch, to excluded).
func (r Recaps) Recap(from, to int64) models.Recap {
	keys, pulls := r.Saved()
	return buildRecap(keys, pulls, from, to)
}

// buildRecap is Recap on the keys and pulls given.
func buildRecap(keys []models.Key, pulls []models.RaidPull, from, to int64) models.Recap {
	out := func(start int64) bool { return start < from || start >= to }
	keys = slices.DeleteFunc(slices.Clone(keys), func(k models.Key) bool { return out(k.Start) })
	slices.SortFunc(keys, func(a, b models.Key) int { return cmp.Compare(b.Start, a.Start) }) // a tie names the newest
	pulls = slices.DeleteFunc(slices.Clone(pulls), func(p models.RaidPull) bool { return out(p.Start) })
	slices.SortFunc(pulls, func(a, b models.RaidPull) int { return cmp.Compare(a.Start, b.Start) })
	you := mostLogged(keys)
	rc := models.Recap{From: from, To: to, Dungeons: []models.RecapDungeon{}, Party: []models.RecapMate{},
		Facts: recapFacts(keys), Raid: recapRaid(pulls, you), You: recapYou(keys, you)}
	dungeons := map[string]*models.RecapDungeon{}
	mates := map[string]*models.RecapMate{}
	for _, k := range keys {
		timed := k.Status == "timed"
		rc.Keys, rc.TimeMS, rc.Deaths = rc.Keys+1, rc.TimeMS+keyLength(k), rc.Deaths+len(k.Deaths)
		d := dungeons[k.Dungeon]
		if d == nil {
			d = &models.RecapDungeon{Dungeon: k.Dungeon}
			dungeons[k.Dungeon] = d
		}
		d.Runs++
		if timed {
			rc.Timed, rc.HighestTimed, d.Timed = rc.Timed+1, max(rc.HighestTimed, k.Level), d.Timed+1
			if k.Level > d.BestLevel || k.Level == d.BestLevel && k.FinalMS < d.BestMS {
				d.BestLevel, d.BestMS = k.Level, k.FinalMS
			}
		}
		if you == "" || !slices.Contains(k.Players, you) {
			continue
		}
		for _, p := range k.Players {
			if p == you {
				continue
			}
			m := mates[p]
			if m == nil {
				m = &models.RecapMate{Player: p}
				mates[p] = m
			}
			m.Keys++
			if timed {
				m.Timed++
			}
		}
	}
	for _, d := range dungeons {
		rc.Dungeons = append(rc.Dungeons, *d)
	}
	slices.SortFunc(rc.Dungeons, func(a, b models.RecapDungeon) int {
		return cmp.Or(cmp.Compare(b.Runs, a.Runs), cmp.Compare(a.Dungeon, b.Dungeon))
	})
	for _, m := range mates {
		rc.Party = append(rc.Party, *m)
	}
	slices.SortFunc(rc.Party, func(a, b models.RecapMate) int {
		return cmp.Or(cmp.Compare(b.Keys, a.Keys), cmp.Compare(a.Player, b.Player))
	})
	rc.Party = rc.Party[:min(len(rc.Party), recapMates)]
	return rc
}

// keyLength is a key's time: the game's own once it ended, else from its start to its end.
func keyLength(k models.Key) int64 {
	if k.FinalMS > 0 {
		return k.FinalMS
	}
	return k.End - k.Start
}

// mostLogged is the player whose game wrote most of keys' logs ("" when none says).
func mostLogged(keys []models.Key) string {
	counts := map[string]int{}
	for _, k := range keys {
		if k.Logger != "" {
			counts[k.Logger]++
		}
	}
	best := ""
	for _, name := range slices.Sorted(maps.Keys(counts)) {
		if counts[name] > counts[best] {
			best = name
		}
	}
	return best
}

// recapYou is you in keys (nil for no one): the specs played, most first; the median DPS, or HPS in a healer spec,
// per dungeon; the deaths and what killed you most.
func recapYou(keys []models.Key, you string) *models.RecapYou {
	if you == "" {
		return nil
	}
	r := &models.RecapYou{Character: you, Specs: []string{}, Dungeons: []models.RecapRate{}, Killers: []models.RecapCount{}}
	specCount, killers := map[string]int{}, map[string]int{}
	rates := map[[2]string][]float64{} // (dungeon, metric) -> per key
	for _, k := range keys {
		if !slices.Contains(k.Players, you) {
			continue
		}
		spec, known := specs[k.Specs[you]]
		if known {
			specCount[spec.label()]++
		}
		for _, row := range k.Overall.Rows {
			if row.Player != you {
				continue
			}
			if known && spec.Role == "healer" {
				rates[[2]string{k.Dungeon, "hps"}] = append(rates[[2]string{k.Dungeon, "hps"}], row.HPS)
			} else {
				rates[[2]string{k.Dungeon, "dps"}] = append(rates[[2]string{k.Dungeon, "dps"}], row.DPS)
			}
		}
		for _, d := range k.Deaths {
			if d.Player == you {
				r.Deaths++
				if d.Ability != "" {
					killers[d.Ability]++
				}
			}
		}
	}
	r.Specs = mostFirst(specCount, len(specCount))
	for _, k := range slices.SortedFunc(maps.Keys(rates), func(a, b [2]string) int {
		return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
	}) {
		r.Dungeons = append(r.Dungeons, models.RecapRate{Dungeon: k[0], Metric: k[1], Median: median(rates[k])})
	}
	for _, name := range mostFirst(killers, recapKillers) {
		r.Killers = append(r.Killers, models.RecapCount{Name: name, Count: killers[name]})
	}
	return r
}

// mostFirst is counts' names, the highest count first (ties by name), at most n of them.
func mostFirst(counts map[string]int, n int) []string {
	names := slices.SortedFunc(maps.Keys(counts), func(a, b string) int {
		return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b))
	})
	return names[:min(len(names), n)]
}

// recapFacts are the lines worth telling about keys (newest first), each opening its key, left out when no key has
// what it needs: the fastest and the closest timed key, the most deaths and kicks in one key, the deadliest ability,
// the longest key and the dungeon improved most. A tie names the newest key.
func recapFacts(keys []models.Key) []models.RecapFact {
	facts := []models.RecapFact{}
	name := func(k models.Key) string { return fmt.Sprintf("%s +%d", k.Dungeon, k.Level) }
	// best is the key with the highest score (newest on a tie), and whether any key scored above zero.
	best := func(score func(k models.Key) float64) (models.Key, bool) {
		var top models.Key
		high := 0.0
		for _, k := range keys {
			if s := score(k); s > high {
				top, high = k, s
			}
		}
		return top, high > 0
	}
	add := func(k models.Key, text string, args ...any) {
		facts = append(facts, models.RecapFact{Text: fmt.Sprintf(text, args...), Kind: models.FightMPlus, Start: k.Start})
	}
	timed := func(k models.Key) bool { return k.Status == "timed" && k.TimerMS > 0 && k.FinalMS > 0 }
	if k, ok := best(func(k models.Key) float64 {
		if !timed(k) {
			return 0
		}
		return 1 - float64(k.FinalMS)/float64(k.TimerMS)
	}); ok {
		add(k, "Fastest timed key: %s in %s, %.0f%% of the timer", name(k), clockText(k.FinalMS),
			100*float64(k.FinalMS)/float64(k.TimerMS))
	}
	if k, ok := best(func(k models.Key) float64 {
		if !timed(k) {
			return 0
		}
		return 1 / float64(k.TimerMS-k.FinalMS+1)
	}); ok {
		add(k, "Closest call: %s with %s to spare", name(k), clockText(k.TimerMS-k.FinalMS))
	}
	if k, ok := best(func(k models.Key) float64 { return float64(len(k.Deaths)) }); ok {
		add(k, "Most deaths in a key: %d in %s", len(k.Deaths), name(k))
	}
	killed := map[string]int{}
	for _, k := range keys {
		for _, d := range k.Deaths {
			if d.Ability != "" {
				killed[d.Ability]++
			}
		}
	}
	if deadliest := mostFirst(killed, 1); len(deadliest) == 1 {
		ability := deadliest[0]
		k, _ := best(func(k models.Key) float64 {
			return float64(len(slices.DeleteFunc(slices.Clone(k.Deaths), func(d models.KeyDeath) bool {
				return d.Ability != ability
			})))
		})
		add(k, "Deadliest ability: %s, %s", ability, counted(killed[ability], "death"))
	}
	kicker := func(k models.Key) (string, int) {
		kicks := map[string]int{}
		for _, i := range k.Interrupts {
			kicks[i.Player]++
		}
		top := mostFirst(kicks, 1)
		if len(top) == 0 {
			return "", 0
		}
		return top[0], kicks[top[0]]
	}
	if k, ok := best(func(k models.Key) float64 { _, n := kicker(k); return float64(n) }); ok {
		player, n := kicker(k)
		add(k, "Most kicks in a key: %s, %d in %s", player, n, name(k))
	}
	if k, ok := best(func(k models.Key) float64 { return float64(keyLength(k)) }); ok {
		add(k, "Longest key: %s, %s", name(k), clockText(keyLength(k)))
	}
	first := map[string]int{} // dungeon -> its oldest key's level
	for _, k := range keys {
		first[k.Dungeon] = k.Level
	}
	if k, ok := best(func(k models.Key) float64 {
		if k.Status != "timed" {
			return 0
		}
		return float64(k.Level - first[k.Dungeon])
	}); ok {
		add(k, "Most improved: %s, from +%d to +%d timed", k.Dungeon, first[k.Dungeon], k.Level)
	}
	return facts
}

// recapRaid is pulls (oldest first) by boss and difficulty, in the order first pulled: pulls, kills, the first kill,
// the best wipe, your deaths and the ability that hit the raid hardest.
func recapRaid(pulls []models.RaidPull, you string) []models.RecapBoss {
	bosses := []*models.RecapBoss{}
	taken := map[*models.RecapBoss]map[string]int64{}
	for _, p := range pulls {
		i := slices.IndexFunc(bosses, func(b *models.RecapBoss) bool { return b.Boss == p.Boss && b.Difficulty == p.Difficulty })
		if i < 0 {
			bosses = append(bosses, &models.RecapBoss{Boss: p.Boss, Difficulty: p.Difficulty, BestWipePct: -1})
			i = len(bosses) - 1
		}
		b := bosses[i]
		b.Pulls++
		switch {
		case p.Kill:
			b.Kills++
			if b.FirstKill == 0 {
				b.FirstKill, b.FirstKillPull = p.Start, b.Pulls
			}
		case p.BossPct >= 0 && (b.BestWipePct < 0 || p.BossPct < b.BestWipePct):
			b.BestWipePct = p.BossPct
		}
		for _, d := range p.Deaths {
			if d.Player == you && you != "" {
				b.YourDeaths++
			}
		}
		if taken[b] == nil {
			taken[b] = map[string]int64{}
		}
		for _, h := range p.Taken {
			taken[b][h.Ability] += h.Damage
		}
	}
	out := []models.RecapBoss{}
	for _, b := range bosses {
		abilities := slices.SortedFunc(maps.Keys(taken[b]), func(x, y string) int {
			return cmp.Or(cmp.Compare(taken[b][y], taken[b][x]), cmp.Compare(x, y))
		})
		if len(abilities) > 0 {
			b.MostHit = abilities[0]
		}
		out = append(out, *b)
	}
	return out
}
