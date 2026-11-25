package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"wow-live/backend/models"
)

const (
	// maxSuggestions is the most of the top players' changes one Suggest sim tests, each a profileset.
	maxSuggestions = 40
	// suggestMinGain is the least gain, as a share of the DPS, a tested change must show to be suggested (Raidbots'
	// 0.05%).
	suggestMinGain = 0.0005
	// suggestZ is how many standard errors of the difference a change's gain must pass: a smaller one is noise.
	suggestZ = 2.0
	// maxFarmSuggestions is how many drops a Suggest sim names to farm, the best first.
	maxFarmSuggestions = 5
	// suggestDungeons and suggestLogsPerDungeon are what Mythic+ overall reads: that many logs in each of the season's
	// first dungeons.
	suggestDungeons       = 4
	suggestLogsPerDungeon = 5
	// minRankedKeyLevel is the lowest key level Warcraft Logs ranks, which Mythic+ overall reads at for a character
	// with no saved key.
	minRankedKeyLevel = 2
	// logPoints is about what reading one top log costs in Warcraft Logs points (measured for player reviews).
	logPoints = 8
	// suggestSteps is how many steps a Suggest sim's stage text counts (suggestRun.run).
	suggestSteps = 5
)

var (
	// errNoSuggestLogs is why a Suggest sim is refused without Warcraft Logs.
	errNoSuggestLogs = fmt.Errorf("%w: Suggest compares you with the top Warcraft Logs players of your spec: set "+
		"Warcraft Logs up in Settings, or run the stat weights sim, which needs none", ErrBadSimOptions)
	// errNoSuggestTarget is why a Suggest sim is refused without one of SimChoices.SuggestTargets.
	errNoSuggestTarget = fmt.Errorf("%w: pick what Suggest reads the top logs of: Mythic+ overall or a raid boss on "+
		"a difficulty Warcraft Logs ranks", ErrBadSimOptions)
)

// SuggestLogs reads the top players a Suggest sim compares a character with (Comparer.TopBuilds).
type SuggestLogs interface {
	TopBuilds(ctx context.Context, specID int, character string, target models.SuggestTarget) (TopBuilds, error)
}

// TopBuilds are the top players a Suggest sim compares a character with: their builds, which logs they are and the
// Warcraft Logs points reading them spent.
type TopBuilds struct {
	Tops     []models.RankedPlayer
	Baseline string
	Points   float64
}

// TopBuilds reads the top logs of spec specID a Suggest sim compares character (the export's name) with, of target:
// for Mythic+ overall suggestLogsPerDungeon logs in each of the season's first suggestDungeons dungeons (by Warcraft
// Logs encounter ID) at suggestKeyLevel, for a raid boss compareLogs on its difficulty, each picked near the median
// ranked time. It spends Warcraft Logs points: ErrNoPoints with too few left, ErrNoTopLogs when none can be read.
func (c Comparer) TopBuilds(ctx context.Context, specID int, character string, target models.SuggestTarget) (
	TopBuilds, error) {
	spec, ok := specs[specID]
	if !ok {
		return TopBuilds{}, fmt.Errorf("%w: no spec %d is known", ErrNoTopLogs, specID)
	}
	fights := []fightRun{{kind: models.FightRaid, encounter: target.Encounter, boss: target.Difficulty}}
	level := 0
	if target.Kind == models.FightMPlus {
		ids, err := c.Baselines.MPlusEncounters(ctx)
		if err != nil {
			return TopBuilds{}, err
		}
		names := slices.SortedFunc(maps.Keys(ids), func(a, b string) int { return cmp.Compare(ids[a], ids[b]) })
		level, fights = c.suggestKeyLevel(character), nil
		for _, name := range names[:min(len(names), suggestDungeons)] {
			fights = append(fights, fightRun{kind: models.FightMPlus, encounter: name, level: level,
				logs: suggestLogsPerDungeon})
		}
	}
	for i := range fights {
		fights[i].spec, fights[i].specID = spec, specID
	}
	reviewing.Lock()
	defer reviewing.Unlock()
	top, err := c.readTopLogs(ctx, 0, fights...)
	if err != nil {
		return TopBuilds{}, err
	}
	b := TopBuilds{Tops: top.players, Baseline: top.baseline, Points: top.points}
	if target.Kind == models.FightMPlus {
		b.Baseline = fmt.Sprintf("%d ranked %s runs in %d dungeons at +%d", len(top.players), spec.label(),
			len(fights), level)
	}
	return b, nil
}

// suggestKeyLevel is the key level Mythic+ overall reads the top logs at: the character's highest saved key, else
// minRankedKeyLevel.
func (c Comparer) suggestKeyLevel(character string) int {
	level := 0
	keys, _ := c.Keys.History()
	for _, k := range keys {
		for name := range k.Specs {
			if strings.EqualFold(name, character) {
				level = max(level, k.Level)
			}
		}
	}
	return cmp.Or(level, minRankedKeyLevel)
}

// suggestTargets are what a Suggest sim can read the top logs of: Mythic+ overall, then each raid boss on each
// difficulty Warcraft Logs ranks.
func (s *Sims) suggestTargets() []models.SuggestTargetChoice {
	out := []models.SuggestTargetChoice{{Name: "Mythic+ overall", Target: models.SuggestTarget{Kind: models.FightMPlus}}}
	for _, boss := range s.Bosses {
		for _, d := range models.RaidDifficulties {
			if d.Ranked {
				out = append(out, models.SuggestTargetChoice{Name: boss + " (" + d.Name + ")",
					Target: models.SuggestTarget{Kind: models.FightRaid, Encounter: boss, Difficulty: d.WCL}})
			}
		}
	}
	return out
}

// validTarget is whether t is one of suggestTargets.
func (s *Sims) validTarget(t *models.SuggestTarget) bool {
	return t != nil && slices.ContainsFunc(s.suggestTargets(), func(c models.SuggestTargetChoice) bool {
		return c.Target == *t
	})
}

// suggestPoints is about what reading target's top logs costs: Mythic+ overall's dungeons' logs, or a raid boss's
// compareLogs.
func suggestPoints(target *models.SuggestTarget) float64 {
	if target != nil && target.Kind == models.FightRaid {
		return compareLogs * logPoints
	}
	return suggestDungeons * suggestLogsPerDungeon * logPoints
}

// planSuggest counts a Suggest sim's runs into plan: the stat weights, a profileset for each change the top players
// can suggest (maxSuggestions at most), Top Gear over the bags, the Droptimizer and the combined run, each with its
// baseline (the changes counted at their most, as how many the top logs suggest isn't known before they are read);
// with the points the request's target costs, what it leaves out (Notes) and, without Warcraft Logs or a target, why
// Start refuses it. It returns the full-precision runs.
func (s *Sims) planSuggest(p models.SimProfile, req models.SimRequest, plan *models.SimPlan) float64 {
	if plan.NeedsLogs = !s.logsReady(); plan.NeedsLogs && plan.Error == "" {
		plan.Error = errNoSuggestLogs.Error()
	}
	if !s.validTarget(req.Target) && plan.Error == "" {
		plan.Error = errNoSuggestTarget.Error()
	}
	bags, drops, notes := s.suggestGear(p)
	plan.Points, plan.Notes, plan.Combos = suggestPoints(req.Target), notes, maxSuggestions+len(bags)+len(drops)+1
	final := simTargetError(req.Options, true)
	runs := float64(statsRuns + maxSuggestions + 1 + 2)
	for _, n := range []int{len(bags), len(drops)} {
		if n > 0 {
			runs += smartRuns(n, smartStages(n, final), final) + 1
		}
	}
	return runs
}

// logsReady is whether a Suggest sim can read the top logs: Tops is wired and the keys it needs are saved.
func (s *Sims) logsReady() bool { return s.Tops != nil && (s.KeysSet == nil || s.KeysSet()) }

// suggestGear is the gear a Suggest sim tries besides the top players' changes: Top Gear over everything the
// character wears and carries, and the season's drops at the Droptimizer's defaults, each upgraded to the rank of the
// gear worn in its slot. What can't be tried is left out and said in a note.
func (s *Sims) suggestGear(p models.SimProfile) (bags, drops []gearCombo, notes []string) {
	notes = []string{}
	why := func(err error) string { return strings.TrimPrefix(err.Error(), ErrBadSimOptions.Error()+": ") }
	if len(p.Bags) > 0 {
		var err error
		bags, err = topGearCombos(gearRules{ref: s.Ref}, p, indexes(p.Equipped), indexes(p.Bags), nil, false,
			s.twoHanded)
		if err != nil {
			notes = append(notes, "Gear from the bags isn't tried: "+why(err))
		}
	}
	drops, err := dropCombos(p, s.Drops, defaultDropDifficulty, s.Choices().DefaultKeyLevel,
		dropUpgrade{Tracks: s.Ref.Upgrades, Match: true})
	if err != nil {
		notes = append(notes, "Drops to farm aren't tried: "+why(err))
	}
	return bags, drops, notes
}

// indexes are the indexes of items, to pick every one.
func indexes(items []models.SimItem) []int {
	out := make([]int, len(items))
	for i := range out {
		out[i] = i
	}
	return out
}

// tested is a change a Suggest sim runs as one profileset: the suggestion and its combination.
type tested struct {
	s     models.Suggestion
	combo gearCombo
}

// suggestRun is a running Suggest sim: the sims, its ID, work files and simc input head, its options, the character,
// the consumables it uses and what its top logs are of.
type suggestRun struct {
	sims                    *Sims
	ctx                     context.Context
	id, input, output, head string
	o                       models.SimOptions
	p                       models.SimProfile
	consumables             models.SimConsumables
	target                  models.SuggestTarget
}

// suggest runs a Suggest sim for the running sim with this ID and saves what it found (suggestRun.run).
func (s *Sims) suggest(r suggestRun) {
	defer removeWork(r.input, r.output)
	result, complaint, err := r.run()
	s.finish(r.ctx, r.id, result, complaint, err, r.p.Equipped)
}

// race runs combos after head as step of the sim, taking from to to of its progress bar.
func (r suggestRun) race(head string, combos []gearCombo, from, to, step int, what string) (*models.SimResult,
	string, error) {
	text := fmt.Sprintf("Step %d of %d: %s", step, suggestSteps, what)
	r.sims.setStage(r.id, text)
	return r.sims.race(r.ctx, r.id, r.input, r.output, head, combos, r.o, raceSpan{from, to, text + ". "})
}

// run is a Suggest sim: (1) the stat weights, while the top logs are read; (2) one profileset for each change most top
// players make that the character doesn't, against the unchanged character; (3) Top Gear over the bags; (4) the
// Droptimizer's drops; (5) every change shown that fits at once. A tested change the sim doesn't show as a gain is
// left out and counted; a talent swap is kept instead, as "talentOther". The result is the stat weights sim's, with
// the changes in Suggest.
func (r suggestRun) run() (*models.SimResult, string, error) {
	type read struct {
		builds TopBuilds
		err    error
	}
	logs := make(chan read, 1)
	go func() {
		b, err := r.sims.topBuilds(r.ctx, r.p, r.target)
		logs <- read{b, err}
	}()
	result, complaint, err := r.race(r.head+scaleLine("stats"), nil, 0, 30, 1, "stat weights")
	if err != nil {
		return nil, complaint, err
	}
	out, err := readSimJSON(r.output)
	if err != nil {
		return nil, "", err
	}
	r.sims.setStage(r.id, fmt.Sprintf("Step 1 of %d: reading the top logs", suggestSteps))
	var got read
	select { // the read waits on any player review, whatever the context
	case got = <-logs:
	case <-r.ctx.Done():
		return nil, "", r.ctx.Err()
	}
	report := &models.SimSuggestReport{Suggestions: []models.Suggestion{}, Notes: []string{}}
	var shown []tested
	var untested []models.Suggestion
	if got.err != nil {
		report.Notes = append(report.Notes, "No changes from the top players: "+got.err.Error())
	} else {
		report.Baseline, report.Points = got.builds.Baseline, got.builds.Points
		in := suggestInput{ref: r.sims.Ref, you: r.p, consumables: r.consumables, tops: got.builds.Tops,
			weights: result.Weights, talentTree: loadoutTree(r.sims.Ref.Talents, r.p.Talents)}
		if st := out.Sim.Players[0].Collected.Buffed.Stats; st.Crit+st.Haste+st.Mastery+st.Vers > 0 {
			in.yourStats = &models.RankedStats{Crit: int(st.Crit), Haste: int(st.Haste), Mastery: int(st.Mastery),
				Vers: int(st.Vers)}
		}
		changes, notes := buildSuggestions(in)
		report.Notes = append(report.Notes, notes...)
		if shown, untested, complaint, err = r.testChanges(changes, report); err != nil {
			return nil, complaint, err
		}
	}
	bags, drops, notes := r.sims.suggestGear(r.p)
	report.Notes = append(report.Notes, notes...)
	gear, complaint, err := r.bestGear(bags, report)
	if err != nil {
		return nil, complaint, err
	}
	shown = append(shown, gear...)
	farm, complaint, err := r.farm(drops)
	if err != nil {
		return nil, complaint, err
	}
	if report.Combined, complaint, err = r.combined(shown); err != nil {
		return nil, complaint, err
	}
	for _, t := range shown {
		report.Suggestions = append(report.Suggestions, t.s)
	}
	report.Suggestions = append(append(report.Suggestions, farm...), untested...)
	slices.SortStableFunc(report.Suggestions, func(a, b models.Suggestion) int {
		return cmp.Or(cmp.Compare(untestedRank[a.Kind], untestedRank[b.Kind]), cmp.Compare(b.Gain, a.Gain),
			cmp.Compare(b.Expected, a.Expected))
	})
	result.Suggest = report
	return result, "", nil
}

// untestedRank orders the suggestions the sim shows no gain of after those it does (0): the stats, priced by the stat
// weights, then the talent swaps that add no DPS.
var untestedRank = map[string]int{"stat": 1, "talentOther": 2}

// topBuilds reads the top players of the character's spec in target.
func (s *Sims) topBuilds(ctx context.Context, p models.SimProfile, target models.SuggestTarget) (TopBuilds, error) {
	spec := specIDOf(s.Drops, p)
	if spec == 0 {
		return TopBuilds{}, fmt.Errorf("%w: no spec %s %s is known", ErrNoTopLogs, p.Spec, p.Class)
	}
	return s.Tops.TopBuilds(ctx, spec, p.Name, target)
}

// testChanges runs one profileset for each of the first maxSuggestions changes the sim can test (an enchant, a gem, a
// consumable, a talent swap) and returns those it shows as a gain, then the ones it doesn't rank by a gain: a stat,
// which the stat weights price, and a talent swap it shows as no gain or a loss, kept as the top players' evidence
// ("talentOther", with its DPS change when it is a loss). It counts the other changes the sim shows as no gain in
// report.Dropped and notes the ones it doesn't try.
func (r suggestRun) testChanges(all []models.Suggestion, report *models.SimSuggestReport) (shown []tested,
	untested []models.Suggestion, complaint string, err error) {
	var changes []tested
	misfits, over := 0, 0
	for _, sg := range all {
		if sg.Variant == nil && sg.Consumables == nil && sg.Talent == nil {
			untested = append(untested, sg)
			continue
		}
		combo, ok := changeCombo(r.sims.Ref, r.p, sg)
		switch {
		case !ok:
			misfits++
		case len(changes) == maxSuggestions:
			over++
		default:
			combo.name = "change" + strconv.Itoa(len(changes)+1)
			changes = append(changes, tested{sg, combo})
		}
	}
	if misfits > 0 {
		report.Notes = append(report.Notes, fmt.Sprintf("%d of the top players' changes weren't tried: they don't fit "+
			"your gear or the sim's options", misfits))
	}
	if over > 0 {
		report.Notes = append(report.Notes, fmt.Sprintf("%d of the top players' changes weren't tried: one sim tries "+
			"%d at most", over, maxSuggestions))
	}
	if len(changes) == 0 {
		return nil, untested, "", nil
	}
	combos := make([]gearCombo, len(changes))
	for i, t := range changes {
		combos[i] = t.combo
	}
	what := fmt.Sprintf("testing %d changes", len(changes))
	base, complaint, err := r.race(r.head, combos, 30, 50, 2, what)
	if err != nil {
		return nil, nil, complaint, err
	}
	sets, err := readSets(r.output)
	if err != nil {
		return nil, nil, "", err
	}
	for _, t := range changes {
		i := slices.IndexFunc(sets, func(s simSetJSON) bool { return s.Name == t.combo.name })
		switch {
		case i >= 0 && gained(&t.s, sets[i], base):
			shown = append(shown, t)
		case t.s.Talent != nil:
			if t.s.Kind = "talentOther"; t.s.Gain >= 0 {
				t.s.Gain, t.s.DPSError = 0, 0
			}
			untested = append(untested, t.s)
		default:
			report.Dropped++
		}
	}
	return shown, untested, "", nil
}

// gained sets s's gain from its profileset over base and says whether the sim shows it as one: at least
// suggestMinGain of the DPS and more than suggestZ standard errors of the difference.
func gained(s *models.Suggestion, set simSetJSON, base *models.SimResult) bool {
	s.Gain, s.DPSError = set.Mean-base.DPS, math.Hypot(set.MeanStdDev, base.DPSError)
	return s.Gain >= suggestMinGain*base.DPS && s.Gain > suggestZ*s.DPSError
}

// changeCombo is a change as a profileset: an enchant or gem put on the worn item it names (false when the item can't
// take it), the character's talents with a swap, or the consumable's line. The change is built from the reference and
// the character's own items and talents.
func changeCombo(ref models.SimReference, p models.SimProfile, s models.Suggestion) (gearCombo, bool) {
	if s.Talent != nil {
		line, err := talentLine(loadoutTree(ref.Talents, p.Talents), swapTalent(p.Picks, s))
		return gearCombo{line: line}, err == nil
	}
	if v := s.Variant; v != nil {
		if !v.Worn || v.Index < 0 || v.Index >= len(p.Equipped) {
			return gearCombo{}, false
		}
		worn := &p.Equipped[v.Index]
		in, err := applyVariant(ref, *worn, *v)
		return gearCombo{swaps: []models.SimSwap{{Slot: worn.Slot, Out: worn, In: &in}}}, err == nil
	}
	picks, err := consumablesPicked(ref, s.Consumables)
	if err != nil || len(picks) != 1 || len(picks[0].picks) != 1 {
		return gearCombo{}, false
	}
	c := picks[0].picks[0]
	return gearCombo{line: consumableLine(picks[0].kind, c), consumable: c.Name}, true
}

// bestGear runs Top Gear over bags and returns its best combination when the sim shows it as a gain, else counts it
// in report.Dropped.
func (r suggestRun) bestGear(bags []gearCombo, report *models.SimSuggestReport) ([]tested, string, error) {
	if len(bags) == 0 {
		return nil, "", nil
	}
	base, complaint, err := r.race(r.head, bags, 50, 70, 3, "gear from the bags")
	if err != nil {
		return nil, complaint, err
	}
	sets, err := readSets(r.output)
	if err != nil {
		return nil, "", err
	}
	best, combo := simSetJSON{}, -1
	for _, set := range sets {
		if i := slices.IndexFunc(bags, func(c gearCombo) bool { return c.name == set.Name }); i >= 0 &&
			(combo < 0 || set.Mean > best.Mean) {
			best, combo = set, i
		}
	}
	if combo < 0 {
		return nil, "", errors.New("simc ran none of the gear combinations")
	}
	swaps := bags[combo].swaps
	s := models.Suggestion{Kind: "gear", What: "Wear " + swapsText(swaps), Swaps: slimmed(swaps),
		Evidence: fmt.Sprintf("the best of %d combinations of your gear", len(bags))}
	if !gained(&s, best, base) {
		report.Dropped++
		return nil, "", nil
	}
	return []tested{{s, bags[combo]}}, "", nil
}

// swapsText names what swaps put on: "Ring of Ages in finger2, nothing in off_hand".
func swapsText(swaps []models.SimSwap) string {
	parts := make([]string, len(swaps))
	for i, s := range swaps {
		name := "nothing"
		if s.In != nil {
			name = cmp.Or(s.In.Name, "item "+strconv.Itoa(s.In.ID))
		}
		parts[i] = name + " in " + s.Slot
	}
	return strings.Join(parts, ", ")
}

// farm runs the Droptimizer's drops and returns the best maxFarmSuggestions items the sim shows as a gain, each
// once, as drops to farm.
func (r suggestRun) farm(drops []gearCombo) ([]models.Suggestion, string, error) {
	if len(drops) == 0 {
		return nil, "", nil
	}
	base, complaint, err := r.race(r.head, drops, 70, 95, 4, "drops to farm")
	if err != nil {
		return nil, complaint, err
	}
	var out []models.Suggestion
	seen := map[int]bool{}
	for _, row := range base.Combos {
		in := row.Swaps[0].In
		s := models.Suggestion{Kind: "farm", What: fmt.Sprintf("Farm %s (%d) from %s", in.Name, in.Level, row.Boss),
			Evidence: fmt.Sprintf("the Droptimizer at %s and +%d, at the rank of the gear you wear",
				defaultDropDifficulty, r.sims.Choices().DefaultKeyLevel),
			Swaps: row.Swaps, Source: row.Source, Boss: row.Boss}
		if len(out) < maxFarmSuggestions && !seen[in.ID] &&
			gained(&s, simSetJSON{Mean: row.DPS, MeanStdDev: row.DPSError}, base) {
			seen[in.ID] = true
			out = append(out, s)
		}
	}
	return out, "", nil
}

// combined runs every shown change that fits at once (the gear first, then each enchant and gem on what is in its
// slot then, each talent swap the tree still allows after the ones before, and the consumables) as one profileset
// and marks the changes in it: nil with fewer than two.
func (r suggestRun) combined(shown []tested) (*models.SimCombo, string, error) {
	worn := map[string]*models.SimItem{}
	for i := range r.p.Equipped {
		worn[r.p.Equipped[i].Slot] = &r.p.Equipped[i]
	}
	now := maps.Clone(worn)
	combo := gearCombo{name: "combined"}
	tree, picks, swapped := loadoutTree(r.sims.Ref.Talents, r.p.Talents), r.p.Picks, false
	var names []string
	var in []int
	for _, gearFirst := range []bool{true, false} {
		for i, t := range shown {
			if (t.s.Kind == "gear") != gearFirst {
				continue
			}
			switch {
			case t.s.Variant != nil:
				item := now[t.s.Slot]
				if item == nil {
					continue
				}
				changed, err := applyVariant(r.sims.Ref, *item, *t.s.Variant)
				if err != nil {
					continue
				}
				now[t.s.Slot] = &changed
			case gearFirst:
				for _, sw := range t.combo.swaps {
					now[sw.Slot] = sw.In
				}
			case t.s.Talent != nil:
				next := swapTalent(picks, t.s)
				if validTalents(tree, next) != nil {
					continue
				}
				picks, swapped = next, true
			default:
				combo.line += t.combo.line
				names = append(names, t.combo.consumable)
			}
			in = append(in, i)
		}
	}
	if len(in) < 2 {
		return nil, "", nil
	}
	if swapped {
		line, err := talentLine(tree, picks)
		if err != nil {
			return nil, "", fmt.Errorf("writing the combined talents: %w", err)
		}
		combo.line += line
	}
	for _, slot := range simSlots {
		if now[slot] != worn[slot] {
			combo.swaps = append(combo.swaps, models.SimSwap{Slot: slot, Out: worn[slot], In: now[slot]})
		}
	}
	base, complaint, err := r.race(r.head, []gearCombo{combo}, 95, 100, 5, "all the changes together")
	if err != nil {
		return nil, complaint, err
	}
	sets, err := readSets(r.output)
	if err != nil {
		return nil, "", err
	}
	i := slices.IndexFunc(sets, func(s simSetJSON) bool { return s.Name == combo.name })
	if i < 0 {
		return nil, "", errors.New("simc ran no combined change")
	}
	for _, j := range in {
		shown[j].s.Combined = true
	}
	return &models.SimCombo{DPS: sets[i].Mean, DPSError: sets[i].MeanStdDev, Gain: sets[i].Mean - base.DPS,
		Swaps: slimmed(combo.swaps), Consumable: strings.Join(names, " + ")}, "", nil
}
