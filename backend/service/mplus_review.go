package service

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"

	"wow-live/backend/models"
)

// lustBuffMS is how long a Bloodlust's buff lasts, for the stretch the lanes draw (a key keeps only when it landed).
const lustBuffMS = 40_000

const (
	// lowPriorityPct: a DPS puts less than this share of their damage into priority targets, or...
	lowPriorityPct = 20.0
	// lowPriorityOfAverage: ...less than this fraction of the party's average share, and is told so.
	lowPriorityOfAverage = 0.5
)

// lateText says how many of a key's priority pulls had a target die last of its pack, and how long after the rest of
// it (the worst, and its pull); "" when none was late.
func lateText(pulls []models.KeyPull) string {
	late := countFunc(pulls, func(p models.KeyPull) bool { return p.Late })
	if late == 0 {
		return ""
	}
	var worst models.KeyMiniboss
	var worstPull int
	for _, p := range pulls {
		for _, m := range p.Minibosses {
			if m.AfterMS > worst.AfterMS {
				worst, worstPull = m, p.Pull
			}
		}
	}
	if worst.AfterMS == 0 {
		return fmt.Sprintf("Priority target died last of its pack in %d of %d pulls", late, len(pulls))
	}
	return fmt.Sprintf("Priority target died late in %d of %d pulls, last up to %.0f s after the rest of its pack (pull %d)",
		late, len(pulls), math.Ceil(float64(worst.AfterMS)/1000), worstPull) // up: under a second still reads as 1 s
}

// lowPriorityText names the DPS (not a healer) with the smallest share of their damage into priority targets when it
// is under lowPriorityPct or lowPriorityOfAverage of the party's average; "" when none is, the key has no priority
// pull, or no one hit a priority target.
func lowPriorityText(key models.Key) string {
	if len(key.Pulls) == 0 {
		return ""
	}
	var lowest models.KeyMeterRow
	lowestPct, total, n := 100.0, 0.0, 0
	for _, r := range key.Overall.Rows {
		if r.Damage == 0 || mainMetric(specs[key.Specs[r.Player]]) != dpsMetric.name {
			continue
		}
		pct := share(r.PriorityDamage, r.Damage)
		if total, n = total+pct, n+1; pct <= lowestPct {
			lowest, lowestPct = r, pct
		}
	}
	if n == 0 || total == 0 {
		return ""
	}
	average := total / float64(n)
	if lowestPct >= lowPriorityPct && lowestPct >= lowPriorityOfAverage*average {
		return ""
	}
	return fmt.Sprintf("%s put %.0f%% of their damage into priority targets (party %.0f%%)", lowest.Player, lowestPct, average)
}

// keyLanes is a key's timeline for the replay's lanes, from its facts: its pulls (red when a miniboss died after most
// of its pack) and bosses (red for a wipe) as stretches, deaths, Bloodlust (red when no one drank a combat potion
// with it) and combat potions, defensives and raid cooldowns, the worst damage spikes (red without party cover),
// must-stop casts (red when one went off), wasted kicks and crowd control, each lane's marks in order.
func keyLanes(key models.Key, pulls []models.PullSpan) []models.RaidLane {
	marks := map[lane][]models.RaidMark{}
	add := func(kind, name string, m models.RaidMark) {
		marks[lane{kind, name}] = append(marks[lane{kind, name}], m)
	}
	for i, p := range pulls {
		late := slices.ContainsFunc(key.Pulls, func(kp models.KeyPull) bool {
			return kp.Late && kp.ClockMS >= p.StartMS && kp.ClockMS <= p.EndMS
		})
		add("pulls", "Pulls", models.RaidMark{ClockMS: p.StartMS, EndMS: p.EndMS, Text: fmt.Sprintf("Pull %d", i+1),
			Bad: late})
	}
	for _, b := range key.Bosses {
		add("bosses", "Bosses", models.RaidMark{ClockMS: b.StartMS, EndMS: b.EndMS, Text: b.Name,
			Bad: !b.Kill && b.EndMS > 0})
	}
	for _, d := range key.Deaths {
		add("deaths", "Deaths", models.RaidMark{ClockMS: d.ClockMS, Text: d.Player + " · " + cmp.Or(d.Ability, "?"),
			Bad: true})
	}
	for _, t := range key.Lusts {
		add("lust", "Bloodlust", models.RaidMark{ClockMS: t, EndMS: t + lustBuffMS, Text: "Bloodlust", Bad: !potionWithLust(key, "", t)})
	}
	for _, c := range key.Casts {
		text := c.Player + " · " + c.Ability
		switch c.Kind {
		case "combat potion":
			add("potions", "Potions", models.RaidMark{ClockMS: c.ClockMS, Text: c.Player})
		case "defensive", "raid cooldown":
			add("cooldowns", "Defensives", models.RaidMark{ClockMS: c.ClockMS, EndMS: c.EndMS, Text: text})
		case "cc":
			add("cc", "Crowd control", models.RaidMark{ClockMS: c.ClockMS, Text: text})
		}
	}
	for _, s := range key.Spikes {
		add("spike", "Damage spikes", models.RaidMark{ClockMS: s.ClockMS, EndMS: s.ClockMS + spikeMS,
			Text: short(float64(s.Damage)*1000/spikeMS) + "/s", Bad: len(s.PartyCDs) == 0})
	}
	for _, s := range key.Stops {
		add("stops", "Must-stops", models.RaidMark{ClockMS: s.ClockMS, Text: s.Mob + ": " + s.Spell,
			Bad: s.Outcome == "went off"})
	}
	for _, w := range key.WastedKicks {
		add("stops", "Wasted kicks", models.RaidMark{ClockMS: w.ClockMS, Text: w.Player + " · " + w.Ability, Bad: true})
	}
	return packLanes(marks)
}

// pullsLane is the marks of a key's pulls lane (none in a key saved before Nov 11).
func pullsLane(key models.Key) []models.RaidMark {
	for _, l := range key.Lanes {
		if l.Kind == "pulls" {
			return l.Marks
		}
	}
	return nil
}

// keySuggestions is what to fix next key, in this order and at most rules.Most: deaths to one ability, damage spikes a
// defensive could have covered, the slowest pull against the same pull in the best timed run of the dungeon at the level
// (best, nil without one; only when both runs have the same number of pulls), must-stop casts that went off, a
// Bloodlust few took a combat potion with, damage spikes with no party cooldown, the longest wait between pulls, and
// priority targets that died after most of their pack, and a DPS who put little damage into priority targets. Each
// line names a time on the key's clock where it has one.
func keySuggestions(key models.Key, best *models.Key, rules models.KeySuggestionRules) []string {
	out := []string{}
	byAbility := map[string]int{}
	for _, d := range key.Deaths {
		byAbility[cmp.Or(d.Ability, "?")]++
	}
	for _, ability := range slices.Sorted(maps.Keys(byAbility)) {
		if n := byAbility[ability]; n >= 2 {
			out = append(out, fmt.Sprintf("%s to %s", counted(n, "death"), ability))
		}
	}
	covered := 0
	for _, p := range key.Players {
		n, _ := unusedDefensives(key, p)
		covered += n
	}
	if covered >= 2 {
		out = append(out, "Took big hits with a defensive ready in "+counted(covered, "damage spike"))
	}
	var theirs []models.RaidMark
	if best != nil {
		theirs = pullsLane(*best)
	}
	if mine := pullsLane(key); len(mine) > 0 && len(mine) == len(theirs) {
		worst, over := -1, int64(0)
		for i := range mine {
			m, t := mine[i].EndMS-mine[i].ClockMS, theirs[i].EndMS-theirs[i].ClockMS
			if t > 0 && float64(m) > float64(t)*(1+rules.SlowPullPct/100) && m-t > int64(rules.SlowPullSeconds*1000) &&
				m-t > over {
				worst, over = i, m-t
			}
		}
		if worst >= 0 {
			out = append(out, fmt.Sprintf("Pull %d took %s against %s in your best +%d", worst+1,
				clockText(mine[worst].EndMS-mine[worst].ClockMS), clockText(theirs[worst].EndMS-theirs[worst].ClockMS),
				best.Level))
		}
	}
	wentOff := func(s models.KeyStop) bool { return s.Outcome == "went off" }
	if i := slices.IndexFunc(key.Stops, wentOff); i >= 0 {
		first := key.Stops[i]
		out = append(out, fmt.Sprintf("%s went off, the first %s from %s at %s",
			counted(countFunc(key.Stops, wentOff), "must-stop cast"), first.Spell, first.Mob, clockText(first.ClockMS)))
	}
	for _, lust := range key.Lusts {
		var dps, potions int
		for _, p := range key.Players {
			if specs[key.Specs[p]].Role == "healer" {
				continue
			}
			dps++
			if potionWithLust(key, p, lust) {
				potions++
			}
		}
		if dps > 0 && potions < dps {
			out = append(out, fmt.Sprintf("Bloodlust at %s: %d of %d had a combat potion", clockText(lust), potions, dps))
		}
	}
	uncovered := func(s models.KeySpike) bool { return len(s.PartyCDs) == 0 }
	if i := slices.IndexFunc(key.Spikes, uncovered); i >= 0 {
		out = append(out, fmt.Sprintf("%s with no party cooldown, the worst at %s",
			counted(countFunc(key.Spikes, uncovered), "damage spike"), clockText(key.Spikes[i].ClockMS)))
	}
	if len(key.BetweenPulls) > 0 {
		gap := slices.Max(key.BetweenPulls)
		if float64(gap) > rules.GapSeconds*1000 {
			out = append(out, fmt.Sprintf("%s waiting before pull %d", clockText(gap),
				slices.Index(key.BetweenPulls, gap)+2))
		}
	}
	if late := lateText(key.Pulls); late != "" {
		out = append(out, late)
	}
	if low := lowPriorityText(key); low != "" {
		out = append(out, low)
	}
	return out[:min(len(out), rules.Most)]
}
