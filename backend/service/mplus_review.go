package service

import (
	"cmp"
	"fmt"
	"slices"

	"wow-live/backend/models"
)

// lustBuffMS is how long a Bloodlust's buff lasts, for the stretch the lanes draw (a key keeps only when it landed).
const lustBuffMS = 40_000

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
	potionNear := func(lust int64) bool {
		return slices.ContainsFunc(key.Casts, func(c models.KeyCast) bool {
			return c.Kind == "combat potion" && abs(c.ClockMS-lust) <= lustPotionMS
		})
	}
	for _, t := range key.Lusts {
		add("lust", "Bloodlust", models.RaidMark{ClockMS: t, EndMS: t + lustBuffMS, Text: "Bloodlust", Bad: !potionNear(t)})
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
	lanes := []models.RaidLane{}
	for l, m := range marks {
		slices.SortStableFunc(m, func(a, b models.RaidMark) int { return cmp.Compare(a.ClockMS, b.ClockMS) })
		lanes = append(lanes, models.RaidLane{Name: l.name, Kind: l.kind, Marks: m})
	}
	slices.SortFunc(lanes, func(a, b models.RaidLane) int {
		return cmp.Or(cmp.Compare(slices.Index(laneKinds, a.Kind), slices.Index(laneKinds, b.Kind)),
			cmp.Compare(a.Name, b.Name))
	})
	return lanes
}

// pullStarts is when each of a key's pulls started, from its lanes (none in a key saved before Nov 11).
func pullStarts(key models.Key) []int64 {
	var starts []int64
	for _, l := range key.Lanes {
		if l.Kind == "pulls" {
			for _, m := range l.Marks {
				starts = append(starts, m.ClockMS)
			}
		}
	}
	return starts
}

// pullLength is how long the pull started at the lane mark i of a key's lanes lasted, 0 without one.
func pullLength(key models.Key, i int) int64 {
	for _, l := range key.Lanes {
		if l.Kind == "pulls" && i < len(l.Marks) {
			return l.Marks[i].EndMS - l.Marks[i].ClockMS
		}
	}
	return 0
}

// slowPullMS: a pull is only worth a line when it took this much longer than in the best run.
const slowPullMS = 20_000

// keySuggestions is what to fix next key, worst first and at most rules.Most: a death with a defensive ready, the
// slowest pull against the same pull in the best timed run of the dungeon at the level (best, nil without one), must-stop
// casts that went off, a Bloodlust few took a combat potion with, damage spikes with no party cooldown, the longest
// wait between pulls, and minibosses that died after most of their pack. Each line names a time on the key's clock.
func keySuggestions(key models.Key, best *models.Key, rules models.KeySuggestionRules) []string {
	out := []string{}
	for _, d := range key.Deaths {
		if d.Defensive == "" && len(d.Ready) > 0 {
			out = append(out, fmt.Sprintf("%s died to %s at %s with %s ready", d.Player, cmp.Or(d.Ability, "?"),
				clockText(d.ClockMS), d.Ready[0]))
		}
	}
	if best != nil {
		worst, over := -1, int64(0)
		for i := range pullStarts(key) {
			mine, theirs := pullLength(key, i), pullLength(*best, i)
			if theirs > 0 && float64(mine) > float64(theirs)*(1+rules.SlowPullPct/100) && mine-theirs > slowPullMS &&
				mine-theirs > over {
				worst, over = i, mine-theirs
			}
		}
		if worst >= 0 {
			out = append(out, fmt.Sprintf("Pull %d took %s against %s in your best +%d", worst+1,
				clockText(pullLength(key, worst)), clockText(pullLength(*best, worst)), best.Level))
		}
	}
	if went := countFunc(key.Stops, func(s models.KeyStop) bool { return s.Outcome == "went off" }); went > 0 {
		first := key.Stops[slices.IndexFunc(key.Stops, func(s models.KeyStop) bool { return s.Outcome == "went off" })]
		out = append(out, fmt.Sprintf("%s went off, the first %s from %s at %s", counted(went, "must-stop cast"), first.Spell,
			first.Mob, clockText(first.ClockMS)))
	}
	for _, lust := range key.Lusts {
		var dps, potions int
		for _, p := range key.Players {
			if specs[key.Specs[p]].Healer {
				continue
			}
			dps++
			if slices.ContainsFunc(key.Casts, func(c models.KeyCast) bool {
				return c.Player == p && c.Kind == "combat potion" && abs(c.ClockMS-lust) <= lustPotionMS
			}) {
				potions++
			}
		}
		if dps > 0 && potions < dps {
			out = append(out, fmt.Sprintf("Bloodlust at %s: %d of %d had a combat potion", clockText(lust), potions, dps))
		}
	}
	if bare := countFunc(key.Spikes, func(s models.KeySpike) bool { return len(s.PartyCDs) == 0 }); bare > 0 {
		out = append(out, fmt.Sprintf("%s with no party cooldown, the worst at %s", counted(bare, "damage spike"),
			clockText(key.Spikes[slices.IndexFunc(key.Spikes, func(s models.KeySpike) bool { return len(s.PartyCDs) == 0 })].ClockMS)))
	}
	if len(key.BetweenPulls) > 0 {
		gap := slices.Max(key.BetweenPulls)
		if float64(gap) > rules.GapSeconds*1000 {
			out = append(out, fmt.Sprintf("%s waiting before pull %d", clockText(gap),
				slices.Index(key.BetweenPulls, gap)+2))
		}
	}
	if late := countFunc(key.Pulls, func(p models.KeyPull) bool { return p.Late }); late > 0 {
		out = append(out, fmt.Sprintf("A miniboss died after most of its pack in %s", counted(late, "pull")))
	}
	return out[:min(len(out), rules.Most)]
}
