package service

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"wow-live/backend/models"
)

// lane is a timeline lane: its kind and name.
type lane struct{ kind, name string }

// laneKinds is the order the timeline shows its lanes in, by kind (then by name).
var laneKinds = []string{"phases", "deaths", "lust", "cooldowns", "potions", "spike", "avoidable", "soak", "stacks",
	"adds"}

// namesShown: a suggestion names at most this many raiders, then says how many more.
const namesShown = 4

// mark puts a mark on the lane of that kind and name.
func (r *raidRun) mark(kind, name string, m models.RaidMark) {
	r.marks[lane{kind, name}] = append(r.marks[lane{kind, name}], m)
}

// lanes is a finished pull's timeline: the marks seen as they happened (avoidable hits, stacks past the limit, phase
// changes, made stretches here) and the deaths, Bloodlust and cooldowns (stretches while their buff lasted), potions,
// damage spike, soaks and adds from the pull's facts, each lane's marks in order.
func (k *LogTracker) lanes(run *keyRun, p models.RaidPull, end int64) []models.RaidLane {
	r := run.raid
	marks := maps.Clone(r.marks)
	add := func(kind, name string, m models.RaidMark) {
		marks[lane{kind, name}] = append(marks[lane{kind, name}], m)
	}
	if phases := marks[lane{"phases", "Phases"}]; len(r.phases) > 0 {
		phases = append([]models.RaidMark{{Text: "P1"}}, phases...)
		for i := range phases {
			phases[i].EndMS = p.DurationMS
			if i+1 < len(phases) {
				phases[i].EndMS = phases[i+1].ClockMS
			}
		}
		marks[lane{"phases", "Phases"}] = phases
	}
	for _, d := range p.Deaths {
		add("deaths", "Deaths", models.RaidMark{ClockMS: d.ClockMS, Text: d.Player + " · " + cmp.Or(d.Ability, "?"),
			Bad: d.Early})
	}
	for _, t := range p.Lusts {
		add("lust", "Bloodlust", models.RaidMark{ClockMS: t, EndMS: r.lustEnds[t], Text: "Bloodlust"})
	}
	for _, c := range p.Cooldowns {
		if c.Kind == "combat potion" {
			add("potions", "Potions", models.RaidMark{ClockMS: c.ClockMS, Text: c.Player})
		} else {
			add("cooldowns", "Raid cooldowns", models.RaidMark{ClockMS: c.ClockMS, EndMS: c.EndMS,
				Text: c.Player + " · " + c.Ability})
		}
	}
	if s := p.Spike; s != nil {
		add("spike", "Damage spike", models.RaidMark{ClockMS: max(0, s.ClockMS-k.spikeMS()), EndMS: s.ClockMS,
			Text: short(float64(s.PerSecond)) + "/s", Bad: len(s.Covered) == 0})
	}
	for id, t := range r.soaks {
		most := 0
		for _, c := range t.casts {
			most = max(most, len(c))
		}
		for i, c := range t.casts {
			add("soak", k.spells[id], models.RaidMark{ClockMS: run.clock(t.at[i]), Text: fmt.Sprintf("%d soaked", len(c)),
				Bad: len(c) < most})
		}
	}
	boss := r.bossUnits()
	for guid, u := range r.units {
		if u.hit && boss[guid] == nil && slices.ContainsFunc(p.Adds, func(a models.RaidAdd) bool { return a.Name == u.name }) {
			died := cmp.Or(u.died, end)
			add("adds", u.name, models.RaidMark{ClockMS: run.clock(u.seen), EndMS: run.clock(died), Text: u.name,
				Bad: u.died == 0})
		}
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

// spikeMS is raid-rules.toml's damage spike window.
func (k *LogTracker) spikeMS() int64 {
	return int64(k.raid.rules.Suggestions.SpikeSeconds * 1000)
}

// spike is the raid's worst damage taken over window ms, from the track's meter samples (each raider's damage taken
// so far), with the raid cooldowns used in it or up to window before; nil when nobody took damage.
func spike(track models.KeyTrack, cooldowns []models.KeyCast, window int64) *models.RaidSpike {
	taken := map[int64]int64{}
	for _, points := range track.Meters {
		for _, pt := range points {
			taken[pt.ClockMS] += pt.Taken
		}
	}
	at := slices.Sorted(maps.Keys(taken))
	var worst *models.RaidSpike
	for i, j := 0, 0; i < len(at); i++ {
		for at[j] < at[i]-window {
			j++
		}
		if j == i {
			continue
		}
		perS := (taken[at[i]] - taken[at[j]]) * 1000 / (at[i] - at[j])
		if perS > 0 && (worst == nil || perS > worst.PerSecond) {
			worst = &models.RaidSpike{ClockMS: at[i], PerSecond: perS}
		}
	}
	if worst == nil {
		return nil
	}
	worst.Covered = []string{}
	for _, c := range cooldowns {
		if c.Kind == "raid cooldown" && c.ClockMS >= worst.ClockMS-2*window && c.ClockMS <= worst.ClockMS {
			worst.Covered = append(worst.Covered, c.Player+" · "+c.Ability)
		}
	}
	return worst
}

// suggestions is what to fix on the next pull, most important first, at most rules.Most: deaths to what can be dodged
// or soaked, early deaths with a defensive ready, mass kills, several early deaths to one other ability, an uncovered
// damage spike, stacks past the limit, casts that went off, slow dispels, adds that lived longer than in the best
// pull, the most avoidable hits, less damage into the target to burst than the best pull, and in a pull long enough
// for them, no Bloodlust and raiders without a combat potion; last, raid cooldowns stacked. best is nil for the best
// pull itself.
func suggestions(p models.RaidPull, best *models.RaidPull, rules models.SuggestionRules) []string {
	var out []string
	say := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	avoidable := map[string]bool{}
	for _, a := range p.Avoidable {
		avoidable[a.Ability] = true
	}
	said := map[string]bool{} // abilities whose deaths have a line of their own
	for _, s := range p.Soaks {
		said[s.Ability] = true
	}
	for _, m := range p.MassKills {
		said[m.Ability] = true
	}
	byAbility := map[string][]string{}
	var abilities, ready []string
	for _, d := range p.Deaths {
		if avoidable[d.Ability] || (d.Early && d.Ability != "" && !said[d.Ability]) {
			if byAbility[d.Ability] == nil {
				abilities = append(abilities, d.Ability)
			}
			byAbility[d.Ability] = append(byAbility[d.Ability], d.Player)
		}
		if d.Early && len(d.Ready) > 0 {
			ready = append(ready, d.Player+" ("+d.Ready[0]+")")
		}
	}
	for _, a := range abilities {
		if avoidable[a] {
			say("%s to %s, which can be dodged: %s", deaths(len(byAbility[a])), a, nameList(byAbility[a]))
		}
	}
	for _, s := range p.Soaks {
		if len(s.Died) > 0 {
			say("%s soaking %s: %s", deaths(len(s.Died)), s.Ability, nameList(s.Died))
		}
	}
	if len(ready) > 0 {
		say("%s with a defensive ready: %s", counted(len(ready), "early death"), nameList(ready))
	}
	for _, m := range p.MassKills {
		say("%s killed %d raiders at %s", m.Ability, m.Killed, clockText(m.ClockMS))
	}
	for _, a := range abilities {
		if !avoidable[a] && len(byAbility[a]) > 1 {
			say("%s to %s: %s", deaths(len(byAbility[a])), a, nameList(byAbility[a]))
		}
	}
	if s := p.Spike; s != nil && len(s.Covered) == 0 {
		say("No raid cooldown for the worst damage (%s/s at %s)", short(float64(s.PerSecond)), clockText(s.ClockMS))
	}
	for _, s := range p.Stacks {
		var over []string
		for _, c := range s.Players {
			if s.Limit > 0 && c.Count > s.Limit {
				over = append(over, c.Player)
			}
		}
		if len(over) > 0 {
			say("%s went past %d stacks of %s: %s", raiders(len(over)), s.Limit, s.Ability, nameList(over))
		}
	}
	wentOff := map[string]int{}
	for _, s := range p.WentOff {
		wentOff[s.Spell]++
	}
	for _, spell := range slices.Sorted(maps.Keys(wentOff)) {
		say("%s went off %s", spell, times(wentOff[spell]))
	}
	slow := map[string][]string{}
	for _, d := range p.Dispels {
		switch {
		case d.By == "":
			slow[d.Spell] = append(slow[d.Spell], d.Player+" (ran out)")
		case float64(d.AfterMS) > rules.DispelSlowSeconds*1000:
			slow[d.Spell] = append(slow[d.Spell], fmt.Sprintf("%s (%.1f s)", d.Player, float64(d.AfterMS)/1000))
		}
	}
	for _, spell := range slices.Sorted(maps.Keys(slow)) {
		say("%s sat too long on %s: %s", spell, raiders(len(slow[spell])), nameList(slow[spell]))
	}
	if best != nil {
		for _, a := range p.Adds {
			i := slices.IndexFunc(best.Adds, func(b models.RaidAdd) bool { return b.Name == a.Name })
			if i >= 0 && float64(a.LongestMS) > rules.AddsLonger*float64(best.Adds[i].LongestMS) {
				say("%s lived up to %s (%s in pull %d)", a.Name, clockText(a.LongestMS), clockText(best.Adds[i].LongestMS),
					best.Number)
			}
		}
	}
	var top models.RaidHits // the most hit ability nobody died to (those are said above)
	for _, a := range p.Avoidable {
		if byAbility[a.Ability] == nil && total(a.Players) > total(top.Players) {
			top = a
		}
	}
	if len(top.Players) > 0 {
		say("%s hit %s %s", top.Ability, raiders(len(top.Players)), times(total(top.Players)))
	}
	if best != nil {
		for _, t := range p.Priority {
			i := slices.IndexFunc(best.Priority, func(b models.RaidPriority) bool { return b.Target == t.Target })
			if i >= 0 && float64(t.Damage) < rules.PriorityBelow*float64(best.Priority[i].Damage) {
				say("Damage into %s: %s (%s in pull %d)", t.Target, short(float64(t.Damage)),
					short(float64(best.Priority[i].Damage)), best.Number)
			}
		}
	}
	if float64(p.DurationMS) > rules.LustAfterSeconds*1000 {
		if len(p.Lusts) == 0 {
			say("No Bloodlust this pull")
		}
		var without []string
		for _, r := range p.Meter.Rows {
			if !slices.ContainsFunc(p.Cooldowns, func(c models.KeyCast) bool {
				return c.Kind == "combat potion" && c.Player == r.Player
			}) {
				without = append(without, r.Player)
			}
		}
		slices.Sort(without)
		if len(without) > 0 {
			say("%s used no combat potion: %s", raiders(len(without)), nameList(without))
		}
	}
	var last *models.KeyCast // the raid cooldown before this one
	for i := range p.Cooldowns {
		c := &p.Cooldowns[i]
		if c.Kind != "raid cooldown" {
			continue
		}
		if last != nil && float64(c.ClockMS-last.ClockMS) <= rules.CooldownOverlapSeconds*1000 {
			say("Raid cooldowns stacked at %s: %s, %s", clockText(last.ClockMS), last.Ability, c.Ability)
			break
		}
		last = c
	}
	return out[:min(len(out), rules.Most)]
}

// nameList is raiders' names joined, the ones past namesShown counted.
func nameList(names []string) string {
	if len(names) <= namesShown {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:namesShown], ", ") + fmt.Sprintf(" and %d more", len(names)-namesShown)
}

// deaths, raiders and times are a count with its word: "1 death", "3 raiders", "2 times".
func deaths(n int) string  { return counted(n, "death") }
func raiders(n int) string { return counted(n, "raider") }
func times(n int) string   { return counted(n, "time") }

func counted(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
