package service

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
	"time"

	"wow-live/backend/models"
)

// recapTopRuns is the fewest ranked Warcraft Logs runs a top-runs median is worth showing over.
const recapTopRuns = 10

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
	Logs   models.MPlusBaselines // nil without Warcraft Logs
}

// TopTimes is, for each dungeon character ran from from to to (as Recap), the median time of its top speed-ranked
// Warcraft Logs runs at the highest level they ran it, in the recap's order; models.ErrNoCredentials without Logs.
func (r Recaps) TopTimes(ctx context.Context, from, to int64, character string) ([]models.RecapTopTime, error) {
	if r.Logs == nil {
		return nil, models.ErrNoCredentials
	}
	encounters, err := r.Logs.MPlusEncounters(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the season's dungeons: %w", err)
	}
	out := []models.RecapTopTime{}
	for _, d := range r.Recap(from, to, character).MPlus.Dungeons {
		encounter, ok := encounters[d.Dungeon]
		if !ok {
			continue
		}
		times, err := r.Logs.MPlusTopTimes(ctx, encounter, d.TopLevel)
		if err != nil {
			return nil, fmt.Errorf("reading %s +%d's top runs: %w", d.Dungeon, d.TopLevel, err)
		}
		top := models.RecapTopTime{Dungeon: d.Dungeon, Level: d.TopLevel, Runs: len(times)}
		if len(times) >= recapTopRuns {
			top.MedianMS = median(times)
		}
		out = append(out, top)
	}
	return out, nil
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

// Recap is the keys and raid pulls character played ("" for every character, see loggedBy) that started from from to
// to (ms since the epoch, to excluded).
func (r Recaps) Recap(from, to int64, character string) models.Recap {
	keys, pulls := r.Saved()
	return buildRecap(keys, pulls, from, to, character)
}

// buildRecap is Recap on the keys and pulls given. The recap is for character, or with none for the character who
// played most of the period's runs.
func buildRecap(keys []models.Key, pulls []models.RaidPull, from, to int64, character string) models.Recap {
	out := func(start int64) bool { return start < from || start >= to }
	keys = slices.DeleteFunc(slices.Clone(keys), func(k models.Key) bool {
		return out(k.Start) || !loggedBy(k.Logger, k.Realms, character)
	})
	slices.SortFunc(keys, func(a, b models.Key) int { return cmp.Compare(b.Start, a.Start) }) // a tie names the newest
	pulls = slices.DeleteFunc(slices.Clone(pulls), func(p models.RaidPull) bool {
		return out(p.Start) || !loggedBy(p.Logger, p.Realms, character)
	})
	slices.SortFunc(pulls, func(a, b models.RaidPull) int { return cmp.Compare(a.Start, b.Start) })
	you, _, _ := strings.Cut(character, "-")
	if played := playedCharacters(keys, pulls, ""); you == "" && len(played) > 0 {
		you = played[0].Name
	}
	return models.Recap{From: from, To: to, MPlus: recapMPlus(keys, you), Raid: recapRaid(pulls, you)}
}

// recapMPlus is the keys' half of a recap (keys newest first), for you.
func recapMPlus(keys []models.Key, you string) models.RecapMPlus {
	rc := models.RecapMPlus{Dungeons: []models.RecapDungeon{}, Facts: recapFacts(keys), You: recapYou(keys, you),
		Averages: recapAverages(keys, you), Party: recapParty(keys, you)}
	dungeons := map[string][]models.Key{}
	for _, k := range keys {
		rc.Keys, rc.TimeMS, rc.Deaths = rc.Keys+1, rc.TimeMS+keyLength(k), rc.Deaths+len(k.Deaths)
		if k.Status == "timed" {
			rc.Timed, rc.HighestTimed = rc.Timed+1, max(rc.HighestTimed, k.Level)
		}
		dungeons[k.Dungeon] = append(dungeons[k.Dungeon], k)
	}
	rc.SuccessPct = pct(rc.Timed, rc.Keys)
	for name, runs := range dungeons {
		rc.Dungeons = append(rc.Dungeons, recapDungeon(name, runs))
	}
	slices.SortFunc(rc.Dungeons, func(a, b models.RecapDungeon) int {
		return cmp.Or(cmp.Compare(b.Runs, a.Runs), cmp.Compare(a.Dungeon, b.Dungeon))
	})
	return rc
}

// pct is part's share of all in %, 0 for none.
func pct(part, all int) float64 {
	if all == 0 {
		return 0
	}
	return 100 * float64(part) / float64(all)
}

// recapDungeon is one dungeon's runs in a recap: counts, the best timed run, and the times at the highest level run.
func recapDungeon(name string, runs []models.Key) models.RecapDungeon {
	d := models.RecapDungeon{Dungeon: name, Runs: len(runs)}
	for _, k := range runs {
		d.TopLevel = max(d.TopLevel, k.Level)
		if k.Status != "timed" {
			continue
		}
		d.Timed++
		if k.Level > d.BestLevel || k.Level == d.BestLevel && k.FinalMS < d.BestMS {
			d.BestLevel, d.BestMS = k.Level, k.FinalMS
		}
	}
	d.SuccessPct = pct(d.Timed, d.Runs)
	var times []int64
	for _, k := range runs {
		if k.Level != d.TopLevel {
			continue
		}
		d.TopRuns++
		if k.Status == "timed" {
			d.TopTimed++
		}
		if k.FinalMS > 0 {
			times = append(times, k.FinalMS)
		}
	}
	if len(times) > 0 {
		var sum int64
		for _, t := range times {
			sum += t
		}
		d.AvgMS, d.MedianMS, d.FastestMS = sum/int64(len(times)), median(times), slices.Min(times)
	}
	return d
}

// recapAverages is keys' kicks, crowd control, dispels and priority damage per key, the party's and you's (in the keys
// you played).
func recapAverages(keys []models.Key, you string) models.RecapAverages {
	type tally struct {
		kicks, cc, dispels, keys int
		priority, damage         float64
	}
	var party, yours tally
	add := func(t *tally, k models.Key, player string) {
		t.keys++
		for _, i := range k.Interrupts {
			if player == "" || i.Player == player {
				t.kicks++
			}
		}
		for _, c := range k.Casts {
			if c.Kind == "cc" && (player == "" || c.Player == player) {
				t.cc++
			}
		}
		for _, row := range k.Overall.Rows {
			if player == "" || row.Player == player {
				t.dispels += row.Dispels
				if row.PriorityDamage > 0 { // 0 in keys saved before it was kept
					t.priority, t.damage = t.priority+float64(row.PriorityDamage), t.damage+float64(row.Damage)
				}
			}
		}
	}
	for _, k := range keys {
		add(&party, k, "")
		if you != "" && slices.Contains(k.Players, you) {
			add(&yours, k, you)
		}
	}
	per := func(t tally) models.RecapPerKey {
		if t.keys == 0 {
			return models.RecapPerKey{Kicks: -1, CC: -1, Dispels: -1, PriorityPct: -1}
		}
		n := float64(t.keys)
		p := models.RecapPerKey{Kicks: float64(t.kicks) / n, CC: float64(t.cc) / n, Dispels: float64(t.dispels) / n,
			PriorityPct: -1}
		if t.damage > 0 {
			p.PriorityPct = 100 * t.priority / t.damage
		}
		return p
	}
	return models.RecapAverages{Party: per(party), You: per(yours)}
}

// recapParty is who you ran keys (newest first) with most, at most recapMates of them, each by name and realm with
// their latest realm and class.
func recapParty(keys []models.Key, you string) []models.RecapMate {
	mates := map[string]*models.RecapMate{}
	for _, k := range keys {
		if you == "" || !slices.Contains(k.Players, you) {
			continue
		}
		for _, p := range k.Players {
			if p == you {
				continue
			}
			id := p + "-" + k.Realms[p]
			m := mates[id]
			if m == nil {
				m = &models.RecapMate{Player: p, Realm: k.Realms[p]}
				if i := slices.IndexFunc(k.Overall.Rows, func(r models.KeyMeterRow) bool { return r.Player == p }); i >= 0 {
					m.Class = k.Overall.Rows[i].Class
				}
				mates[id] = m
			}
			m.Keys++
			if k.Status == "timed" {
				m.Timed++
			}
		}
	}
	party := []models.RecapMate{}
	for _, id := range slices.Sorted(maps.Keys(mates)) {
		party = append(party, *mates[id])
	}
	slices.SortStableFunc(party, func(a, b models.RecapMate) int { return cmp.Compare(b.Keys, a.Keys) })
	return party[:min(len(party), recapMates)]
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
			if known && spec.Healer {
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
	if k, ok := best(func(k models.Key) float64 {
		if k.Status != "timed" {
			return 0
		}
		return float64(len(k.Deaths))
	}); ok {
		add(k, "Most deaths in a timed key: %d in %s", len(k.Deaths), name(k))
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

// recapRaid is the pulls' half of a recap (pulls oldest first), for you: the totals, then each boss and difficulty in
// the order first pulled with its pulls, kills, first kill, best wipe, your deaths and the ability that hit the raid
// hardest.
func recapRaid(pulls []models.RaidPull, you string) models.RecapRaid {
	rc := models.RecapRaid{Bosses: []models.RecapBoss{}, Facts: []models.RecapFact{}}
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
		rc.Pulls, rc.TimeMS, rc.Deaths = rc.Pulls+1, rc.TimeMS+p.DurationMS, rc.Deaths+len(p.Deaths)
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
	for _, b := range bosses {
		abilities := slices.SortedFunc(maps.Keys(taken[b]), func(x, y string) int {
			return cmp.Or(cmp.Compare(taken[b][y], taken[b][x]), cmp.Compare(x, y))
		})
		if len(abilities) > 0 {
			b.MostHit = abilities[0]
		}
		rc.Kills += b.Kills
		if b.Kills > 0 {
			rc.Killed++
		}
		rc.Bosses = append(rc.Bosses, *b)
	}
	return rc
}
