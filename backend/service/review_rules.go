package service

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"wow-live/backend/models"
)

const (
	// maxAdvice is how many things to work on a player is given.
	maxAdvice = 4
	// lowShare and highShare: DPS or HPS under this share of the spec's top runs is something to work on, over it a
	// thing done well.
	lowShare, highShare = 0.7, 0.9
	// lustPotionMS: a combat potion this close to Bloodlust counts as used with it.
	lustPotionMS = 10_000
	// belowUsual: DPS or HPS under this share of the player's usual in the dungeon is something to work on.
	belowUsual = 0.85
	// lowActive: active time under this (in %) that is also the group's lowest is something to work on.
	lowActive = 85
	// lustEveryMS is how long after a Bloodlust the party can have the next.
	lustEveryMS = 600_000
	// lustLastMS: a Bloodlust needs this much of the key left to be worth one.
	lustLastMS = 60_000
	// lateDebuffs: this many dispellable debuffs running their full time is worth a line.
	lateDebuffs = 3
)

// reviewMetric is a reviewed number: how to read it for a player from the key, and whether more is better.
type reviewMetric struct {
	name   string
	value  func(key models.Key, r models.KeyMeterRow) float64
	higher bool
	amount bool // shown shortened ("2.1M"), else as a count
	plain  bool // a count shown for reference: unranked, and never advice or a callout
}

var (
	dpsMetric = reviewMetric{"DPS", func(_ models.Key, r models.KeyMeterRow) float64 { return r.DPS }, true, true, false}
	hpsMetric = reviewMetric{"HPS", func(_ models.Key, r models.KeyMeterRow) float64 { return r.HPS }, true, true, false}
	// otherMetrics are reviewed for everyone, after DPS or HPS.
	otherMetrics = []reviewMetric{
		{"kicks", func(_ models.Key, r models.KeyMeterRow) float64 { return float64(r.Interrupts) }, true, false, false},
		{"dispels", func(_ models.Key, r models.KeyMeterRow) float64 { return float64(r.Dispels + r.Purges) }, true,
			false, false},
		{"damage taken", func(_ models.Key, r models.KeyMeterRow) float64 { return float64(r.DamageTaken) }, false,
			true, false},
		{"avoidable damage", func(_ models.Key, r models.KeyMeterRow) float64 { return float64(r.Avoidable) }, false,
			true, false},
		{"deaths", func(k models.Key, r models.KeyMeterRow) float64 {
			return float64(countFunc(k.Deaths, func(d models.KeyDeath) bool { return d.Player == r.Player }))
		}, false, false, false},
		{"wasted kicks", func(k models.Key, r models.KeyMeterRow) float64 {
			return float64(countFunc(k.WastedKicks, func(c models.KeyCast) bool { return c.Player == r.Player }))
		}, false, false, false},
		{"priority %", since(func(_ models.Key, r models.KeyMeterRow) float64 {
			return share(r.PriorityDamage, r.Damage)
		}), true, false, false},
		{"active %", since(func(k models.Key, r models.KeyMeterRow) float64 { return k.Activity[r.Player] }), true,
			false, false},
		{"big hits, defensive ready", func(k models.Key, r models.KeyMeterRow) float64 {
			n, _ := unusedDefensives(k, r.Player)
			return float64(n)
		}, false, false, false},
		{"crowd control", since(castCount("cc")), true, false, false},
		{"combat potions", since(castCount("combat potion")), true, false, false},
		{"health potions", since(castCount("health potion")), true, false, true},
		{"healthstones", since(castCount("healthstone")), true, false, true},
	}
)

// buildReview reviews a finished key: each player's DPS and HPS against the top runs of their spec (baselines,
// by player then metric name), their own earlier runs of the dungeon (history) and the party, with advice and one positive callout; and
// the party's damage taken per second against similar top runs (taken).
func buildReview(key models.Key, history []models.Key, baselines map[string]map[string][]models.MPlusRanking,
	taken []float64) models.KeyReview {
	review := models.KeyReview{Start: key.Start, End: key.End, Players: []models.PlayerReview{},
		Baseline: "No Warcraft Logs baseline for this key"}
	switch {
	case len(baselines) == 0:
	case key.Status == "abandoned":
		review.Baseline = fmt.Sprintf("Top runs of each spec at +%d from Warcraft Logs: whole keys, against the %s of "+
			"this one", key.Level, clockText(lengthMS(key)))
	default:
		review.Baseline = fmt.Sprintf("Top runs of each spec at +%d within %.0f%% of this time, from Warcraft Logs",
			key.Level, similarTime*100)
	}
	rows := map[string]models.KeyMeterRow{}
	for _, p := range key.Players {
		rows[p] = models.KeyMeterRow{Player: p}
	}
	for _, r := range key.Overall.Rows {
		rows[r.Player] = r
	}
	for _, player := range key.Players {
		spec := specs[key.Specs[player]]
		row := rows[player]
		pr := models.PlayerReview{Player: player, Spec: spec.label(), Advice: []string{}}
		for _, m := range reviewed {
			value := m.value(key, row)
			if math.IsNaN(value) { // a key saved before the metric existed
				continue
			}
			metric := models.ReviewMetric{Name: m.name, Value: value, Usual: -1, Amount: m.amount,
				Help: metricHelp[m.name]}
			if !m.plain {
				metric.PartyRank = partyRank(key, rows, m, value)
			}
			switch m.name {
			case "DPS":
				metric.Total, metric.TotalName = float64(row.Damage), "damage"
			case "HPS":
				metric.Total, metric.TotalName = float64(row.Healing), "healing"
			}
			if top := baselines[player][m.name]; len(top) > 0 {
				amounts := make([]float64, len(top))
				for j, r := range top {
					amounts[j] = r.Amount
				}
				metric.Top = median(amounts)
			}
			var mine []float64
			for _, h := range history {
				i := slices.IndexFunc(h.Overall.Rows, func(r models.KeyMeterRow) bool { return r.Player == player })
				if i < 0 {
					continue
				}
				if v := m.value(h, h.Overall.Rows[i]); !math.IsNaN(v) {
					mine = append(mine, v)
				}
			}
			if len(mine) > 0 {
				metric.Usual = median(mine)
			}
			pr.Metrics = append(pr.Metrics, metric)
		}
		pr.Advice = advice(key, player, spec, pr.Metrics, len(key.Players))
		pr.Positive = positive(key, spec, pr.Metrics)
		review.Players = append(review.Players, pr)
	}
	review.Notes = partyNotes(key)
	if len(taken) > 0 {
		var ours float64
		for _, r := range rows {
			ours += float64(r.DamageTaken)
		}
		ratio := ours / float64(max(1, lengthMS(key))) * 1000 / median(taken)
		review.Party = fmt.Sprintf("The party took %.1f× the damage per second of %d top +%d runs%s",
			ratio, len(taken), key.Level, unlessAbandoned(key, " of similar time"))
	}
	return review
}

// partyRank is a value's rank among the party for a metric (1 is best).
func partyRank(key models.Key, rows map[string]models.KeyMeterRow, m reviewMetric, value float64) int {
	rank := 1
	for _, p := range key.Players {
		other := m.value(key, rows[p])
		if (m.higher && other > value) || (!m.higher && other < value) {
			rank++
		}
	}
	return rank
}

// advice is up to maxAdvice things for a player to work on, costliest first, each naming its evidence.
func advice(key models.Key, player string, spec specInfo, metrics []models.ReviewMetric, party int) []string {
	out := []string{}
	byName := map[string]models.ReviewMetric{}
	for _, m := range metrics {
		byName[m.Name] = m
	}
	killers := map[string]int{}
	for _, d := range key.Deaths {
		if d.Player != player {
			continue
		}
		if len(d.Ready) > 0 {
			out = append(out, fmt.Sprintf("Died to %s at %s with %s ready", d.Ability, clockText(d.ClockMS), d.Ready[0]))
		} else if d.Ability != "" {
			killers[d.Ability]++
		}
	}
	for _, ability := range slices.Sorted(maps.Keys(killers)) {
		if n := killers[ability]; n > 1 {
			out = append(out, fmt.Sprintf("Died to %s %s", ability, counted(n, "time")))
		}
	}
	if a := byName["avoidable damage"]; a.Value > 0 && a.PartyRank == party && party > 1 {
		i := slices.IndexFunc(key.AvoidableHits, func(h models.KeyHit) bool { return h.Player == player })
		if i >= 0 {
			out = append(out, fmt.Sprintf("Took %s avoidable damage, the most in the group, most of it from %s",
				short(a.Value), key.AvoidableHits[i].Ability))
		}
	}
	wentOff := countFunc(key.Stops, func(s models.KeyStop) bool { return s.Outcome == "went off" })
	if k := byName["kicks"]; wentOff > 0 && k.PartyRank > (party+1)/2 && !spec.Healer {
		out = append(out, fmt.Sprintf("%d must-stop casts went off; you kicked %.0f", wentOff, k.Value))
	}
	switch main := byName[mainMetric(spec)]; {
	case main.Top > 0 && main.Value < lowShare*main.Top:
		out = append(out, fmt.Sprintf("%s %s, %.0f%% of top %ss at +%d%s (%s)", main.Name,
			short(main.Value), 100*main.Value/main.Top, spec.label(), key.Level,
			unlessAbandoned(key, " in similar time"), short(main.Top)))
	case main.Usual > 0 && main.Value < belowUsual*main.Usual:
		out = append(out, fmt.Sprintf("%s %s, %.0f%% of your usual here (%s)", main.Name, short(main.Value),
			100*main.Value/main.Usual, short(main.Usual)))
	}
	if readyIn, ready := unusedDefensives(key, player); readyIn >= 2 {
		out = append(out, fmt.Sprintf("Took big hits with %s ready in %d damage spikes", ready, readyIn))
	}
	if p, ok := byName["combat potions"]; ok && !spec.Healer && lustsWithoutPotion(key, player) > 0 {
		out = append(out, fmt.Sprintf("Used %s in a key with %s", counted(int(p.Value), "combat potion"),
			counted(len(key.Lusts), "Bloodlust")))
	}
	for _, b := range key.Buffs {
		if b.Player != player {
			continue
		}
		lacks := slices.DeleteFunc(slices.Clone(b.Missing), func(m string) bool { return m != "flask" && m != "food" })
		if len(lacks) > 0 { // a missing party buff is its class's to give
			out = append(out, "Started the key without "+strings.Join(lacks, " or "))
		}
	}
	if a, ok := byName["active %"]; ok && a.Value < lowActive && a.PartyRank == party && party > 1 {
		out = append(out, fmt.Sprintf("Active %.0f%% of the time in pulls, the least in the group", a.Value))
	}
	if c, ok := byName["crowd control"]; ok && c.Value == 0 && wentOff > 0 {
		out = append(out, fmt.Sprintf("Used no crowd control while %s went off: a stun or knock stops a cast too",
			counted(wentOff, "must-stop cast")))
	}
	if w := byName["wasted kicks"]; w.Value >= 3 {
		out = append(out, fmt.Sprintf("%.0f kicks interrupted nothing: call kicks so two don't go on one cast", w.Value))
	}
	return out[:min(maxAdvice, len(out))]
}

// unlessAbandoned is a note about the top runs' time, which holds only for a key that finished: an abandoned key is
// compared with whole runs.
func unlessAbandoned(key models.Key, note string) string {
	if key.Status == "abandoned" {
		return ""
	}
	return note
}

// reviewed is every metric of a review, in the page's column order: DPS and HPS for everyone, then the rest.
var reviewed = append([]reviewMetric{dpsMetric, hpsMetric}, otherMetrics...)

// mainMetric is the metric a spec is ranked and advised by: HPS for healers, DPS for everyone else (tanks too, in M+).
func mainMetric(spec specInfo) string {
	if spec.Healer {
		return hpsMetric.name
	}
	return dpsMetric.name
}

// metricHelp says what each reviewed metric counts.
var metricHelp = map[string]string{
	"DPS":                       "damage per second over the key",
	"HPS":                       "healing per second over the key, absorbs included",
	"kicks":                     "every interrupt, must-stop or not",
	"dispels":                   "debuffs dispelled off the party, and buffs purged off enemies",
	"damage taken":              "all damage taken",
	"avoidable damage":          "damage from the dungeon's avoidable abilities (reference/mplus-dungeons.json)",
	"deaths":                    "times died",
	"wasted kicks":              "interrupts cast that stopped nothing: the cast was already stopped, or the target couldn't be",
	"priority %":                "share of damage into trash pulls' minibosses and into bosses",
	"active %":                  "share of time alive in pulls with a cast or hit of theirs going",
	"big hits, defensive ready": "damage spikes they took the most of with a defensive ready and none used",
	"combat potions":            "combat potions drunk over the key, a pre-pot included",
	"health potions":            "health potions drunk over the key",
	"healthstones":              "healthstones used over the key",
	"crowd control":             "stuns, incapacitates, fears, roots, knocks (up or back) and grips cast ([cc] in mplus.toml; an area stun counts once)",
}

// since reads a metric only from keys saved with it (Nov 8 on; older keys have no activity): NaN for older ones, so a
// player's usual leaves them out.
func since(value func(models.Key, models.KeyMeterRow) float64) func(models.Key, models.KeyMeterRow) float64 {
	return func(k models.Key, r models.KeyMeterRow) float64 {
		if k.Activity == nil {
			return math.NaN()
		}
		return value(k, r)
	}
}

// castCount reads how many tracked buttons of one kind (KeyCast.Kind) a player pressed over the key.
func castCount(kind string) func(models.Key, models.KeyMeterRow) float64 {
	return func(k models.Key, r models.KeyMeterRow) float64 {
		return float64(countFunc(k.Casts, func(c models.KeyCast) bool { return c.Player == r.Player && c.Kind == kind }))
	}
}

// lustsWithoutPotion is how many Bloodlusts passed with no combat potion of the player's within lustPotionMS.
func lustsWithoutPotion(key models.Key, player string) int {
	return countFunc(key.Lusts, func(lust int64) bool {
		return !slices.ContainsFunc(key.Casts, func(c models.KeyCast) bool {
			return c.Player == player && c.Kind == "combat potion" && abs(c.ClockMS-lust) <= lustPotionMS
		})
	})
}

// unusedDefensives is how many damage spikes a player took the most of with a defensive ready and none used, and one
// they had ready.
func unusedDefensives(key models.Key, player string) (int, string) {
	var n int
	var ready string
	for _, s := range key.Spikes {
		if m := s.Members; len(m) > 0 && m[0].Player == player && m[0].Defensive == "" && len(m[0].Ready) > 0 {
			n, ready = n+1, m[0].Ready[0]
		}
	}
	return n, ready
}

// partyNotes is what the review says about the party as a whole: what its deaths cost, the share of must-stop casts
// stopped, the time between pulls with the longest wait, Bloodlusts against how many the key had room for, minibosses
// that died late, dispellable debuffs left to run out, and party cooldowns used on top of each other.
func partyNotes(key models.Key) []string {
	notes := []string{}
	if n := len(key.Deaths); n > 0 {
		notes = append(notes, fmt.Sprintf("%s added %s to the clock", counted(n, "death"),
			clockText(int64(n)*deathPenaltyMS)))
	}
	if n := len(key.Stops); n > 0 {
		stopped := countFunc(key.Stops, func(s models.KeyStop) bool { return s.Outcome != "went off" })
		notes = append(notes, fmt.Sprintf("Stopped %d of %d must-stop casts (%.0f%%)", stopped, n,
			100*float64(stopped)/float64(n)))
	}
	if len(key.BetweenPulls) > 0 {
		longest := slices.Index(key.BetweenPulls, slices.Max(key.BetweenPulls))
		var total int64
		for _, gap := range key.BetweenPulls {
			total += gap
		}
		notes = append(notes, fmt.Sprintf("%s between pulls in all; the longest %s, before pull %d", clockText(total),
			clockText(key.BetweenPulls[longest]), longest+2))
	}
	// A party without a Bloodlust has no class for it.
	if room := int(1 + (lengthMS(key)-lustLastMS)/lustEveryMS); len(key.Lusts) > 0 && len(key.Lusts) < room {
		notes = append(notes, fmt.Sprintf("%s; %s had room for %d", counted(len(key.Lusts), "Bloodlust"),
			clockText(lengthMS(key)), room))
	}
	if late := countFunc(key.Pulls, func(p models.KeyPull) bool { return p.Late }); late > 0 {
		notes = append(notes, fmt.Sprintf("A miniboss died after most of its pack in %d of %d pulls with one", late,
			len(key.Pulls)))
	}
	if ranOut := countFunc(key.Debuffs, func(d models.KeyDispel) bool { return d.By == "" }); ranOut >= lateDebuffs {
		notes = append(notes, fmt.Sprintf("%d of %d dispellable debuffs ran their full time", ranOut, len(key.Debuffs)))
	}
	if overlaps := countFunc(key.Spikes, func(s models.KeySpike) bool { return s.Overlap }); overlaps > 0 {
		notes = append(notes, "Party cooldowns overlapped in "+counted(overlaps, "damage spike"))
	}
	return notes
}

// positive is exactly one thing a player did well, always from the data: DPS or HPS close to their spec's top runs,
// else their biggest gain on their usual here, else their best rank in the party.
func positive(key models.Key, spec specInfo, metrics []models.ReviewMetric) string {
	metrics = slices.DeleteFunc(slices.Clone(metrics), func(m models.ReviewMetric) bool { return m.PartyRank == 0 })
	main := metrics[slices.IndexFunc(metrics, func(m models.ReviewMetric) bool { return m.Name == mainMetric(spec) })]
	if main.Top > 0 && main.Value >= highShare*main.Top {
		return fmt.Sprintf("%s %s, %.0f%% of top %ss at +%d", main.Name, short(main.Value),
			100*main.Value/main.Top, spec.label(), key.Level)
	}
	all := make([]reviewMetric, len(metrics)) // the rules of each metric, in the same order
	for i, m := range metrics {
		all[i] = reviewed[slices.IndexFunc(reviewed, func(r reviewMetric) bool { return r.name == m.Name })]
	}
	bestGain, best := 1.1, -1
	for i, m := range metrics {
		if m.Usual <= 0 {
			continue
		}
		gain := m.Value / m.Usual
		if !all[i].higher {
			gain = m.Usual / max(m.Value, 1)
		}
		if gain > bestGain {
			bestGain, best = gain, i
		}
	}
	if best >= 0 {
		m := metrics[best]
		return fmt.Sprintf("Better %s than your usual here: %s (usually %s)", m.Name, show(all[best], m.Value),
			show(all[best], m.Usual))
	}
	top := slices.MinFunc(metrics, func(a, b models.ReviewMetric) int { return cmp.Compare(a.PartyRank, b.PartyRank) })
	i := slices.Index(metrics, top)
	word := map[bool]string{true: "Most", false: "Least"}[all[i].higher]
	if all[i].higher && all[i].amount {
		word = "Highest"
	}
	if top.PartyRank > 1 {
		word = ordinal(top.PartyRank) + "-" + strings.ToLower(word)
	}
	return fmt.Sprintf("%s %s in the group: %s", word, top.Name, show(all[i], top.Value))
}

func show(m reviewMetric, v float64) string {
	if m.amount {
		return short(v)
	}
	return fmt.Sprintf("%.0f", v)
}

// short is a big number shortened: "2.1M", "640k", "950".
func short(v float64) string {
	switch {
	case v >= 1e6:
		return fmt.Sprintf("%.1fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.0fk", v/1e3)
	}
	return fmt.Sprintf("%.0f", v)
}

// clockText is a time on the key's clock: "12:03".
func clockText(ms int64) string {
	return fmt.Sprintf("%d:%02d", ms/60_000, ms/1000%60)
}

// counted is a number of things: "1 death", "3 deaths".
func counted(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// ordinal is "1st", "2nd", "23rd", "11th".
func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		suffix = map[int]string{1: "st", 2: "nd", 3: "rd"}[n%10]
		if suffix == "" {
			suffix = "th"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

func countFunc[T any](s []T, f func(T) bool) int {
	n := 0
	for _, v := range s {
		if f(v) {
			n++
		}
	}
	return n
}
