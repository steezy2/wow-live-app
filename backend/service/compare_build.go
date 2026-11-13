package service

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"wow-live/backend/models"
)

const (
	// topShare: an ability or buff the top logs use under this share of their median is advice.
	topShare = 0.8
	// buffGapPoints and activeGapPoints: a buff's uptime this many percentage points under the top logs' median, or
	// active time this many under, is advice.
	buffGapPoints, activeGapPoints = 10.0, 5.0
	// oncePerMinute: an ability the top logs cast at most this often per minute (median) is shown as counts.
	oncePerMinute = 1.0
	// maxCompareAdvice is how many advice lines a review keeps, costliest first.
	maxCompareAdvice = 5
	// maxBuffRows is how many buff rows a review lists, most up in the top logs first.
	maxBuffRows = 25
	// minBuffPct: a buff under this uptime in the player and the top logs alike isn't listed.
	minBuffPct = 5.0
	// presentShare: an ability counts as one the top logs use when this share of them cast it.
	presentShare = 0.5
)

// compareInput is everything buildCompare needs, read already: the player's run, their own similar runs, the top logs
// (best first as ranked, with the runs they came from) and the metric ("DPS" or "HPS") the spec is judged by.
type compareInput struct {
	you    models.RankedPlayer
	yours  []models.RankedPlayer
	top    []models.RankedPlayer
	logs   []models.CompareLog
	metric string
	parse  *models.CharacterParse
	spec   specInfo
}

// buildCompare is the player review's rows, advice and verdict from the runs read.
func buildCompare(in compareInput) models.PlayerCompare {
	c := models.PlayerCompare{Logs: in.logs, YourRuns: len(in.yours), Stats: []models.CompareRow{},
		Abilities: []models.CompareRow{}, Buffs: []models.CompareRow{}, OnlyYou: []string{}, OnlyTop: []string{},
		Advice: []string{}, Parse: in.parse}
	best := 0
	for i, l := range in.logs {
		if l.Amount > in.logs[best].Amount {
			best = i
		}
	}
	c.Stats = statRows(in, best)
	abilities, onlyYou, onlyTop, lines := abilityRows(in, best)
	c.Abilities, c.OnlyYou, c.OnlyTop = abilities, onlyYou, onlyTop
	buffs, buffLines := buffRows(in, best)
	c.Buffs = buffs
	c.Advice = adviceOf(append(lines, append(buffLines, activeAdvice(c.Stats, in)...)...))
	c.Verdict = verdict(c, in.metric)
	return c
}

// costedLine is one line of advice with what it costs, in % of the player's damage or healing, to order them by.
type costedLine struct {
	text string
	cost float64
}

// adviceOf is the costliest lines, at most maxCompareAdvice.
func adviceOf(lines []costedLine) []string {
	slices.SortStableFunc(lines, func(a, b costedLine) int { return cmp.Compare(b.cost, a.cost) })
	out := []string{}
	for _, l := range lines[:min(len(lines), maxCompareAdvice)] {
		out = append(out, l.text)
	}
	return out
}

// perMinute is a count over a stretch as a rate.
func perMinute(count int, durationMS int64) float64 {
	return float64(count) / (float64(max(1, durationMS)) / 60_000)
}

// rateOf is a player's damage per second, or healing for "HPS".
func rateOf(p models.RankedPlayer, metric string) float64 {
	total := p.Damage
	if metric == hpsMetric.name {
		total = p.Healing
	}
	return float64(total) / (float64(max(1, p.DurationMS)) / 1000)
}

// med is the median of values, 0 for none.
func med(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return median(values)
}

// pointer is a pointer to v.
func pointer(v float64) *float64 { return &v }

// medianOf is the median of the values by f over runs, or 0 for none.
func medianOf(runs []models.RankedPlayer, f func(models.RankedPlayer) float64) float64 {
	values := make([]float64, len(runs))
	for i, r := range runs {
		values[i] = f(r)
	}
	return med(values)
}

// statRow is one of a player's numbers against their other runs and the top logs by f.
func statRow(in compareInput, best int, name, unit string, f func(models.RankedPlayer) float64) models.CompareRow {
	row := models.CompareRow{Name: name, Unit: unit, You: f(in.you), Top: medianOf(in.top, f)}
	if len(in.yours) > 0 {
		row.Yours = pointer(medianOf(in.yours, f))
	}
	if best < len(in.top) {
		row.Best = f(in.top[best])
	}
	return row
}

// statRows are the key stats: DPS or HPS, damage taken per second, active time and deaths.
func statRows(in compareInput, best int) []models.CompareRow {
	rows := []models.CompareRow{
		statRow(in, best, in.metric, "/s", func(p models.RankedPlayer) float64 { return rateOf(p, in.metric) }),
		statRow(in, best, "damage taken", "/s", func(p models.RankedPlayer) float64 {
			return float64(p.Taken) / (float64(max(1, p.DurationMS)) / 1000)
		}),
		statRow(in, best, "deaths", "", func(p models.RankedPlayer) float64 { return float64(p.Deaths) }),
	}
	active := func(p models.RankedPlayer) float64 { return 100 * float64(p.ActiveMS) / float64(max(1, p.DurationMS)) }
	if in.you.ActiveMS > 0 && medianOf(in.top, active) > 0 {
		rows = slices.Insert(rows, 2, statRow(in, best, "active time", "%", active))
	}
	return rows
}

// activeAdvice is a line when the player was active activeGapPoints under the top logs' median.
func activeAdvice(stats []models.CompareRow, in compareInput) []costedLine {
	i := slices.IndexFunc(stats, func(r models.CompareRow) bool { return r.Name == "active time" })
	if i < 0 || stats[i].Top-stats[i].You < activeGapPoints {
		return nil
	}
	return []costedLine{{fmt.Sprintf("Active time %.0f%%, top %ss %.0f%%", stats[i].You, in.spec.label(), stats[i].Top),
		(stats[i].Top - stats[i].You) / 10}}
}

// abilityID is the key an ability matches across runs by: its spell ID, else (a key saved before IDs were kept) the
// ID the same name has elsewhere, else the name.
func abilityID(a models.RankedAbility, byName map[string]int) string {
	if a.ID == 0 {
		a.ID = byName[a.Name]
	}
	if a.ID != 0 {
		return fmt.Sprint(a.ID)
	}
	return a.Name
}

// abilityRun is what one run did with one ability.
type abilityRun struct {
	casts int
	done  int64 // damage or healing
	name  string
	id    int
}

// abilitiesOf is a run's abilities by abilityID, with the run's total damage and healing.
func abilitiesOf(p models.RankedPlayer, byName map[string]int) (map[string]abilityRun, int64) {
	out := map[string]abilityRun{}
	for _, a := range p.Abilities {
		k := abilityID(a, byName)
		r := out[k]
		r.casts += a.Casts
		r.done += a.Damage + a.Healing
		r.name, r.id = a.Name, max(a.ID, byName[a.Name])
		out[k] = r
	}
	return out, p.Damage + p.Healing
}

// abilityRows are one row per ability the player or the top logs used, with the advice for the ones under topShare of
// the top logs' median, and the abilities only one side used (a talent difference), unscored.
func abilityRows(in compareInput, best int) (rows []models.CompareRow, onlyYou, onlyTop []string, lines []costedLine) {
	byName := map[string]int{}
	for _, p := range slices.Concat(in.yours, in.top, []models.RankedPlayer{in.you}) {
		for _, a := range p.Abilities {
			if a.ID != 0 {
				byName[a.Name] = a.ID
			}
		}
	}
	you, youTotal := abilitiesOf(in.you, byName)
	mine, top := make([]map[string]abilityRun, len(in.yours)), make([]map[string]abilityRun, len(in.top))
	names := map[string]abilityRun{}
	for k, r := range you {
		names[k] = r
	}
	for i, p := range in.yours {
		mine[i], _ = abilitiesOf(p, byName)
	}
	for i, p := range in.top {
		var runs map[string]abilityRun
		runs, _ = abilitiesOf(p, byName)
		top[i] = runs
		for k, r := range runs {
			if _, ok := names[k]; !ok {
				names[k] = r
			}
		}
	}
	for _, k := range slices.Sorted(maps.Keys(names)) {
		name := names[k]
		castRate := func(runs []map[string]abilityRun, durations func(int) int64) []float64 {
			out := make([]float64, len(runs))
			for i, r := range runs {
				out[i] = perMinute(r[k].casts, durations(i))
			}
			return out
		}
		topRates := castRate(top, func(i int) int64 { return in.top[i].DurationMS })
		used := 0
		for _, r := range top {
			if r[k].casts > 0 || r[k].done > 0 {
				used++
			}
		}
		youRate := perMinute(you[k].casts, in.you.DurationMS)
		topMed := med(topRates)
		youUsed, topUsed := you[k].casts > 0 || you[k].done > 0, len(top) > 0 && float64(used) >= presentShare*float64(len(top))
		switch {
		case !youUsed && topUsed:
			onlyTop = append(onlyTop, name.name)
			continue
		case youUsed && !topUsed:
			onlyYou = append(onlyYou, name.name)
			continue
		case !youUsed:
			continue
		}
		row := models.CompareRow{Name: name.name, ID: name.id, Unit: "per min"}
		scale := 1.0 // per minute to a count over the player's own stretch
		if topMed <= oncePerMinute {
			row.Unit, scale = "casts", float64(in.you.DurationMS)/60_000
		}
		row.You, row.Top = youRate*scale, topMed*scale
		if len(mine) > 0 {
			row.Yours = pointer(med(castRate(mine, func(i int) int64 { return in.yours[i].DurationMS })) * scale)
		}
		if best < len(top) {
			row.Best = perMinute(top[best][k].casts, in.top[best].DurationMS) * scale
		}
		if youTotal > 0 {
			row.Share = 100 * float64(you[k].done) / float64(youTotal)
		}
		rows = append(rows, row)
		if topMed > 0 && youRate < topShare*topMed {
			lines = append(lines, castAdvice(row, you[k], youTotal, in))
		}
	}
	slices.SortStableFunc(rows, func(a, b models.CompareRow) int { return cmp.Compare(b.Share, a.Share) })
	return rows, nilToEmpty(onlyYou), nilToEmpty(onlyTop), lines
}

// castAdvice is the advice for an ability cast fewer times than the top logs cast it in the same time: the casts
// missed times the player's damage (or healing) per cast, as a share of what they did. A ready-made line adds what
// the player's own usual run managed when that was better.
func castAdvice(row models.CompareRow, you abilityRun, youTotal int64, in compareInput) costedLine {
	missed := row.Top - row.You
	if row.Unit == "per min" {
		missed = (row.Top - row.You) * float64(in.you.DurationMS) / 60_000
	}
	perCast := 0.0
	if you.casts > 0 {
		perCast = float64(you.done) / float64(you.casts)
	}
	cost := 0.0
	if youTotal > 0 {
		cost = 100 * missed * perCast / float64(youTotal)
	}
	text := fmt.Sprintf("%s: %.0f casts, top %ss %.0f in the same time", row.Name, row.You, in.spec.label(), row.Top)
	usual := fmt.Sprintf("; %.0f in your usual run", pv(row.Yours))
	if row.Unit == "per min" {
		text = fmt.Sprintf("%s: %.1f a minute, top %ss %.1f", row.Name, row.You, in.spec.label(), row.Top)
		usual = fmt.Sprintf("; %.1f in your usual run", pv(row.Yours))
	}
	if cost >= 0.5 {
		text += fmt.Sprintf(" (about %.0f%% of your %s)", cost, doneWord(in.metric))
	}
	if row.Yours != nil && *row.Yours > row.You*1.1 {
		text += usual
	}
	return costedLine{text, max(cost, 0.01)}
}

// pv is the value of an optional number, 0 for none.
func pv(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

// doneWord is what a metric measures.
func doneWord(metric string) string {
	if metric == hpsMetric.name {
		return "healing"
	}
	return "damage"
}

// buffRows are the player's own buffs' uptime against theirs and the top logs', and a line for each the top logs
// keep up buffGapPoints more than the player did.
func buffRows(in compareInput, best int) ([]models.CompareRow, []costedLine) {
	up := func(p models.RankedPlayer) map[string]float64 {
		out := map[string]float64{}
		for _, b := range p.Buffs {
			out[b.Name] += 100 * float64(b.UptimeMS) / float64(max(1, p.DurationMS))
		}
		return out
	}
	you, mine, top := up(in.you), make([]map[string]float64, len(in.yours)), make([]map[string]float64, len(in.top))
	names := map[string]bool{}
	for k := range you {
		names[k] = true
	}
	for i, p := range in.yours {
		mine[i] = up(p)
	}
	for i, p := range in.top {
		top[i] = up(p)
		for k := range top[i] {
			names[k] = true
		}
	}
	var rows []models.CompareRow
	var lines []costedLine
	for k := range names {
		row := models.CompareRow{Name: k, You: you[k], Unit: "%"}
		tops := make([]float64, len(top))
		for i, t := range top {
			tops[i] = t[k]
		}
		if len(tops) > 0 {
			row.Top = med(tops)
		}
		if len(mine) > 0 {
			ys := make([]float64, len(mine))
			for i, m := range mine {
				ys[i] = m[k]
			}
			row.Yours = pointer(med(ys))
		}
		if best < len(top) {
			row.Best = top[best][k]
		}
		if max(row.You, row.Top) < minBuffPct {
			continue
		}
		rows = append(rows, row)
		if row.Top-row.You >= buffGapPoints {
			lines = append(lines, costedLine{fmt.Sprintf("%s up %.0f%% of the time, top %ss %.0f%%", k, row.You,
				in.spec.label(), row.Top), (row.Top - row.You) / 10})
		}
	}
	slices.SortFunc(rows, func(a, b models.CompareRow) int {
		return cmp.Or(cmp.Compare(b.Top, a.Top), strings.Compare(a.Name, b.Name))
	})
	return rows[:min(len(rows), maxBuffRows)], lines
}

// verdict is one or two sentences from the numbers, never a score: the player's DPS or HPS as a share of the top
// logs' median and where it falls among them, against their own usual, their parse here, and what cost them most.
func verdict(c models.PlayerCompare, metric string) string {
	if len(c.Stats) == 0 || c.Stats[0].Top == 0 {
		return "No top logs to compare with."
	}
	main := c.Stats[0]
	rank := 1
	for _, l := range c.Logs {
		if l.Amount > main.You {
			rank++
		}
	}
	parts := []string{fmt.Sprintf("%.0f%% of the top %d's median %s (%s of %d)", 100*main.You/main.Top, len(c.Logs), metric,
		ordinal(rank), len(c.Logs)+1)}
	if main.Yours != nil && c.YourRuns >= 2 && *main.Yours > 0 {
		switch d := 100 * (main.You / *main.Yours - 1); {
		case d >= 1:
			parts = append(parts, fmt.Sprintf("%.0f%% above your usual here", d))
		case d <= -1:
			parts = append(parts, fmt.Sprintf("%.0f%% below your usual here", -d))
		default:
			parts = append(parts, "on your usual here")
		}
	}
	text := strings.Join(parts, ", ")
	if c.Parse != nil {
		text += fmt.Sprintf("; Warcraft Logs median parse %.0f here (their usual)", c.Parse.Median)
	}
	text += "."
	if len(c.Advice) > 0 {
		text += " Most lost: " + c.Advice[0] + "."
	}
	return text
}

// nilToEmpty is list, or an empty list for nil, so it encodes as [].
func nilToEmpty(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}
