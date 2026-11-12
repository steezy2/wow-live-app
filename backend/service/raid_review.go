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
var laneKinds = []string{"pulls", "bosses", "phases", "deaths", "lust", "cooldowns", "potions", "spike", "stops", "cc",
	"avoidable", "soak", "pickups", "stacks", "adds"}

// namesShown: a suggestion names at most this many raiders, then says how many more.
const namesShown = 4

// mark puts a mark on the lane of that kind and name.
func (r *raidRun) mark(kind, name string, m models.RaidMark) {
	r.marks[lane{kind, name}] = append(r.marks[lane{kind, name}], m)
}

// lanes is a finished pull's timeline: the marks seen as they happened (avoidable hits, stacks past the limit,
// expected pickups, phase changes, made stretches here) and the deaths, Bloodlust and cooldowns (stretches while their
// buff lasted), potions,
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
	if len(p.Spikes) > 0 {
		s, span := p.Spikes[0], k.spikeWindow(run)
		add("spike", "Damage spike", models.RaidMark{ClockMS: s.ClockMS, EndMS: s.ClockMS + span,
			Text: spikeRate(s, k.raid.rules.Suggestions) + "/s", Bad: len(s.PartyCDs) == 0})
	}
	for _, id := range slices.Sorted(maps.Keys(r.soaks)) { // key order: marks at one moment keep one order
		t := r.soaks[id]
		want := r.boss.Soakers[id] // else as many as the best cast
		if want == 0 {
			for _, c := range t.casts {
				want = max(want, len(c))
			}
		}
		for i, c := range t.casts {
			add("soak", k.spells[id], models.RaidMark{ClockMS: run.clock(t.at[i]), Text: fmt.Sprintf("%d soaked", len(c)),
				Bad: len(c) < want})
		}
	}
	boss := r.bossUnits()
	for _, guid := range slices.Sorted(maps.Keys(r.units)) {
		u := r.units[guid]
		if u.hit && boss[guid] == nil && slices.ContainsFunc(p.Adds, func(a models.RaidAdd) bool { return a.Name == u.name }) {
			died := cmp.Or(u.died, end)
			add("adds", u.name, models.RaidMark{ClockMS: run.clock(u.seen), EndMS: run.clock(died), Text: u.name,
				Bad: u.died == 0})
		}
	}
	return packLanes(marks)
}

// packLanes is the marks of each lane in time order, the lanes by kind (laneKinds) and name.
func packLanes(marks map[lane][]models.RaidMark) []models.RaidLane {
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

// coversSpike is whether a raid cooldown covers a spike from start to end: cast by its end, and its buff still up at
// its start or, when none was seen leaving, cast no more than window before it.
func coversSpike(c models.KeyCast, start, end, window int64) bool {
	return c.ClockMS <= end && (c.EndMS >= start || c.EndMS == 0 && c.ClockMS >= start-window)
}

// spikeRate is a spike's damage taken per second over the raid's spike window, shortened.
func spikeRate(s models.KeySpike, rules models.SuggestionRules) string {
	return short(float64(s.Damage) / rules.SpikeSeconds)
}

// suggestions is what to fix on the next pull, most important first, at most rules.Most: adds that come from a carrier
// (a Viper from an egg, close to a wipe) with who spawned them or how many weren't picked up, deaths to what can be
// dodged
// or soaked, raiders who didn't pick up what everyone should, early deaths with a defensive ready, mass kills,
// several early deaths to one other ability, an uncovered damage spike, stacks past the limit, casts that went off, slow dispels, adds that lived longer than
// in the best pull, the most avoidable hits, less damage into the target to burst than the best pull, and in a pull long enough
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
	for _, a := range p.Adds {
		var who []string
		var loose []int64 // when the ones nobody picked up spawned, in order
		for _, s := range a.Spawns {
			if s.Spawner {
				who = append(who, fmt.Sprintf("%s (%s at %s)", strings.Join(s.Players, ", "), s.Cause, clockText(s.ClockMS)))
			} else if s.Cause == notPickedUp {
				loose = append(loose, s.ClockMS)
			}
		}
		if n := len(who) + len(loose); n > 0 {
			if len(loose) > 0 {
				who = append(who, fmt.Sprintf("%d %s (first at %s)", len(loose), strings.ToLower(notPickedUp),
					clockText(loose[0])))
			}
			say("%s spawned: %s", counted(n, a.Name), nameList(who))
		}
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
	var things []string
	missed := map[string][]string{} // what everyone should pick up -> the raiders who missed one, each once
	for _, w := range p.Pickups {
		for _, name := range w.Missed {
			if missed[w.Name] == nil {
				things = append(things, w.Name)
			}
			if !slices.Contains(missed[w.Name], name) {
				missed[w.Name] = append(missed[w.Name], name)
			}
		}
	}
	for _, t := range things {
		say("Didn't pick up %s: %s", t, nameList(missed[t]))
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
	if len(p.Spikes) > 0 && len(p.Spikes[0].PartyCDs) == 0 {
		s := p.Spikes[0]
		say("No raid cooldown for the worst damage (%s/s at %s)", spikeRate(s, rules), clockText(s.ClockMS))
	}
	for _, s := range p.Stacks {
		if len(s.Over) > 0 {
			say("%s went past %d stacks of %s: %s", raiders(len(s.Over)), s.Limit, s.Ability, nameList(s.Over))
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
