package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
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
	// suggestSteps is how many steps a Suggest sim's stage text counts (suggestRun.run): the stat weights, then the
	// changes, the gear, the drops and the combined row, each on both fights.
	suggestSteps = 1 + 4*2
)

// suggestFight is a fight a Suggest sim tests its changes on: one of simScenarios, and how the stage text names it.
type suggestFight struct {
	scenario models.SimScenario
	label    string
}

// suggestFights are the fights a Suggest sim tests its changes on, in the order of Suggestion.Gain and AoEGain: one
// target and several.
var suggestFights = [2]suggestFight{{scenarioNamed("Patchwerk (1 target)"), "1 target"},
	{scenarioNamed("M+ AoE"), "AoE"}}

// scenarioNamed is the one of simScenarios called name. It panics on none, so a renamed scenario fails every test
// instead of a Suggest sim running the request's own fight.
func scenarioNamed(name string) models.SimScenario {
	i := slices.IndexFunc(simScenarios, func(s models.SimScenario) bool { return s.Name == name })
	if i < 0 {
		panic("no sim scenario " + name)
	}
	return simScenarios[i]
}

// options are o on the fight: its scenario's fight style, length and targets.
func (f suggestFight) options(o models.SimOptions) models.SimOptions {
	o.FightStyle, o.Length, o.Targets = f.scenario.FightStyle, f.scenario.Length, f.scenario.Targets
	return o
}

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

// TopBuilds are the top players a Suggest sim compares a character with: their builds, which logs they are, what the
// read left out and the Warcraft Logs points reading them spent.
type TopBuilds struct {
	Tops     []models.RankedPlayer
	Baseline string
	Notes    []string
	Points   float64
}

// TopBuilds reads the top logs of spec specID a Suggest sim compares character (the export's "Name-Realm") with, of
// target: for Mythic+ overall suggestLogsPerDungeon logs in each of the season's first suggestDungeons dungeons (by
// Warcraft Logs encounter ID) at suggestKeyLevel, for a raid boss compareLogs on its difficulty, each picked near the
// median ranked time. It spends Warcraft Logs points: ErrNoPoints with too few left, ErrNoTopLogs when none can be
// read.
func (c Comparer) TopBuilds(ctx context.Context, specID int, character string, target models.SuggestTarget) (
	TopBuilds, error) {
	spec, ok := specs[specID]
	if !ok {
		return TopBuilds{}, fmt.Errorf("%w: no spec %d is known", ErrNoTopLogs, specID)
	}
	var fights []fightRun
	level := 0
	if target.Kind == models.FightMPlus {
		ids, err := c.Baselines.MPlusEncounters(ctx)
		if err != nil {
			return TopBuilds{}, err
		}
		names := slices.SortedFunc(maps.Keys(ids), func(a, b string) int { return cmp.Compare(ids[a], ids[b]) })
		level = c.suggestKeyLevel(character, ids)
		for _, name := range names[:min(len(names), suggestDungeons)] {
			fights = append(fights, fightRun{kind: models.FightMPlus, encounter: name, level: level,
				logs: suggestLogsPerDungeon})
		}
	} else {
		fights = []fightRun{{kind: models.FightRaid, encounter: target.Encounter, boss: target.Difficulty}}
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
			top.read, level)
	}
	if top.limited {
		b.Notes = append(b.Notes, fmt.Sprintf("Warcraft Logs' rate limit stopped the read after %d of %d dungeons",
			top.read, len(fights)))
	}
	return b, nil
}

// suggestKeyLevel is the key level Mythic+ overall reads the top logs at: the highest saved key of the season's
// dungeons (ids, by name) the character ("Name-Realm") logged, else minRankedKeyLevel.
func (c Comparer) suggestKeyLevel(character string, ids map[string]int) int {
	level := 0
	keys, err := c.Keys.History()
	if err != nil {
		log.Printf("reading the saved keys for the top logs' key level: %v", err)
	}
	for _, k := range keys {
		if _, ok := ids[k.Dungeon]; ok && loggedBy(k.Logger, k.Realms, character) {
			level = max(level, k.Level)
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
// baseline (the changes counted at their most, as how many the top logs suggest isn't known before they are read), all
// of them but the stat weights on both fights; with the points the request's target costs, what it leaves out (Notes)
// and, without Warcraft Logs or a target, why Start refuses it. It returns about how long they take, each fight timed
// with its own options.
func (s *Sims) planSuggest(p models.SimProfile, req models.SimRequest, plan *models.SimPlan) int {
	if plan.NeedsLogs = !s.logsReady(); plan.NeedsLogs && plan.Error == "" {
		plan.Error = errNoSuggestLogs.Error()
	}
	if !s.validTarget(req.Target) && plan.Error == "" {
		plan.Error = errNoSuggestTarget.Error()
	}
	bags, drops, notes := s.suggestGear(p)
	plan.Points, plan.Notes, plan.Combos = suggestPoints(req.Target), notes, maxSuggestions+len(bags)+len(drops)+1
	final := simTargetError(req.Options, true)
	runs := float64(maxSuggestions + 1 + 2)
	for _, n := range []int{len(bags), len(drops)} {
		if n > 0 {
			runs += smartRuns(n, smartStages(n, final), final) + 1
		}
	}
	seconds := planSeconds(statsRuns, req.Options)
	for _, f := range suggestFights {
		seconds += planSeconds(runs, f.options(req.Options))
	}
	return seconds
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

// suggestRun is a running Suggest sim: the sims, its ID, work files and simc input head, its options, the character
// and its spec's talent tree, the consumables it uses and what its top logs are of.
type suggestRun struct {
	sims                    *Sims
	ctx                     context.Context
	id, input, output, head string
	o                       models.SimOptions
	p                       models.SimProfile
	tree                    models.TalentTree
	consumables             models.SimConsumables
	target                  models.SuggestTarget
}

// suggest runs a Suggest sim for the running sim with this ID and saves what it found (suggestRun.run).
func (s *Sims) suggest(r suggestRun) {
	defer removeWork(r.input, r.output)
	result, complaint, err := r.run()
	s.finish(r.ctx, r.id, result, complaint, err, r.p.Equipped)
}

// race runs combos after head under o as step of the sim, taking from to to of its progress bar.
func (r suggestRun) race(head string, combos []gearCombo, o models.SimOptions, from, to, step int, what string) (
	*models.SimResult, string, error) {
	text := fmt.Sprintf("Step %d of %d: %s", step, suggestSteps, what)
	r.sims.setStage(r.id, text)
	return r.sims.race(r.ctx, r.id, r.input, r.output, head, combos, o, raceSpan{from, to, text + ". "})
}

// fightRace is a race on one fight: the unchanged character's result and the profilesets it ran.
type fightRace struct {
	base *models.SimResult
	sets []simSetJSON
}

// races are a race of the same combinations on each of suggestFights.
type races [len(suggestFights)]fightRace

// raceBoth races combos on each fight, as steps step and step+1 taking from to to of the progress bar.
func (r suggestRun) raceBoth(combos []gearCombo, from, to, step int, what string) (out races, complaint string,
	err error) {
	for i, f := range suggestFights {
		span := (to - from) / len(suggestFights)
		if out[i].base, complaint, err = r.race(r.head, combos, f.options(r.o), from+i*span, from+(i+1)*span, step+i,
			what+" on "+f.label); err != nil {
			return out, complaint, err
		}
		if out[i].sets, err = readSets(r.output); err != nil {
			return out, "", err
		}
	}
	return out, "", nil
}

// sets are the profileset called name on each fight (the baseline's DPS where a fight didn't run it) and whether
// either did.
func (r races) sets(name string) (set [len(suggestFights)]simSetJSON, ran bool) {
	for i, f := range r {
		set[i] = simSetJSON{Mean: f.base.DPS}
		if j := slices.IndexFunc(f.sets, func(s simSetJSON) bool { return s.Name == name }); j >= 0 {
			set[i], ran = f.sets[j], true
		}
	}
	return set, ran
}

// run is a Suggest sim: (1) the stat weights, while the top logs are read; (2) one profileset for each change most top
// players make that the character doesn't, against the unchanged character; (3) Top Gear over the bags; (4) the
// Droptimizer's drops; (5) every change shown that fits at once; each of (2) to (5) on one target and on several. A
// tested change the sim doesn't show as a gain on either fight is left out and counted; a talent swap is kept
// instead, as "talentOther". The result is the stat weights sim's, with the changes in Suggest.
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
	result, complaint, err := r.race(r.head+scaleLine("stats"), nil, r.o, 0, 30, 1, "stat weights")
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
		report.Notes = append(report.Notes, got.builds.Notes...)
		in := suggestInput{ref: r.sims.Ref, you: r.p, consumables: r.consumables, tops: got.builds.Tops,
			weights: result.Weights, talentTree: r.tree}
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
	for i := range report.Suggestions {
		report.Suggestions[i].Group = untestedRank[report.Suggestions[i].Kind]
	}
	slices.SortStableFunc(report.Suggestions, func(a, b models.Suggestion) int {
		return cmp.Or(cmp.Compare(a.Group, b.Group), byGain(a, b),
			cmp.Compare(b.Expected, a.Expected))
	})
	result.Suggest = report
	return result, "", nil
}

// untestedRank is each kind's Suggestion.Group: the suggestions the sim shows no gain of after those it does (0): the
// stats, priced by the stat weights, then the talent swaps that add no DPS.
var untestedRank = map[string]int{"stat": 1, "talentOther": 2}

// byGain orders the suggestion with the larger gain on either fight first.
func byGain(a, b models.Suggestion) int {
	return cmp.Compare(max(b.Gain, b.AoEGain), max(a.Gain, a.AoEGain))
}

// topBuilds reads the top players of the character's spec in target.
func (s *Sims) topBuilds(ctx context.Context, p models.SimProfile, target models.SuggestTarget) (TopBuilds, error) {
	spec := specIDOf(s.Drops, p)
	if spec == 0 {
		return TopBuilds{}, fmt.Errorf("%w: no spec %s %s is known", ErrNoTopLogs, p.Spec, p.Class)
	}
	return s.Tops.TopBuilds(ctx, spec, p.Name+"-"+p.Server, target)
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
		combo, ok := changeCombo(r.sims.Ref, r.tree, r.p, sg)
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
	both, complaint, err := r.raceBoth(combos, 30, 50, 2, what)
	if err != nil {
		return nil, nil, complaint, err
	}
	for _, t := range changes {
		set, ran := both.sets(t.combo.name)
		switch {
		case ran && gained(&t.s, set, both):
			shown = append(shown, t)
		case t.s.Talent != nil:
			t.s.Kind = "talentOther"
			untested = append(untested, t.s)
		default:
			report.Dropped++
		}
	}
	return shown, untested, "", nil
}

// gained sets s's gains from its profilesets over the baselines of both fights and says whether the sim shows either
// as one: at least suggestMinGain of the DPS and more than suggestZ standard errors of the difference. A gain it
// doesn't show is 0, and so is a loss within suggestZ standard errors.
func gained(s *models.Suggestion, set [len(suggestFights)]simSetJSON, both races) bool {
	var gain, dpsError [len(suggestFights)]float64
	shown := false
	for i, f := range both {
		gain[i], dpsError[i] = difference(set[i], f.base)
		switch {
		case gain[i] >= suggestMinGain*f.base.DPS && gain[i] > suggestZ*dpsError[i]:
			shown = true
		case gain[i] >= 0 || -gain[i] <= suggestZ*dpsError[i]:
			gain[i], dpsError[i] = 0, 0
		}
	}
	s.Gain, s.DPSError, s.AoEGain, s.AoEError = gain[0], dpsError[0], gain[1], dpsError[1]
	return shown
}

// difference is set's DPS over base's and the standard error of that difference.
func difference(set simSetJSON, base *models.SimResult) (gain, dpsError float64) {
	return set.Mean - base.DPS, math.Hypot(set.MeanStdDev, base.DPSError)
}

// changeCombo is a change as a profileset: an enchant or gem put on the worn item it names (false when the item can't
// take it), the character's talents (of its tree) with a swap, or the consumable's line. The change is built from the
// reference and the character's own items and talents.
func changeCombo(ref models.SimReference, tree models.TalentTree, p models.SimProfile, s models.Suggestion) (
	gearCombo, bool) {
	if s.Talent != nil {
		line, err := talentLine(tree, swapTalent(p.Picks, s))
		return gearCombo{line: line}, err == nil
	}
	if v := s.Variant; v != nil {
		if !v.Worn || v.Index < 0 || v.Index >= len(p.Equipped) {
			return gearCombo{}, false
		}
		worn := &p.Equipped[v.Index]
		in, err := applyVariant(ref, *worn, *v)
		swaps := []models.SimSwap{{Slot: worn.Slot, Out: worn, In: &in}}
		return gearCombo{swaps: swaps}, err == nil && uniqueGemsOK(uniqueGems(ref), p, swaps)
	}
	picks, err := consumablesPicked(ref, s.Consumables)
	if err != nil || len(picks) != 1 || len(picks[0].picks) != 1 {
		return gearCombo{}, false
	}
	c := picks[0].picks[0]
	return gearCombo{line: consumableLine(picks[0].kind, c), consumable: c.Name}, true
}

// bestGear runs Top Gear over bags and returns its combination with the best gain on either fight when the sim shows
// it as a gain, else counts it in report.Dropped.
func (r suggestRun) bestGear(bags []gearCombo, report *models.SimSuggestReport) ([]tested, string, error) {
	if len(bags) == 0 {
		return nil, "", nil
	}
	both, complaint, err := r.raceBoth(bags, 50, 70, 4, "gear from the bags")
	if err != nil {
		return nil, complaint, err
	}
	combo, top := -1, 0.0
	for i, c := range bags {
		set, ran := both.sets(c.name)
		gain := math.Inf(-1)
		for j, f := range both {
			gain = max(gain, set[j].Mean-f.base.DPS)
		}
		if ran && (combo < 0 || gain > top) {
			combo, top = i, gain
		}
	}
	if combo < 0 {
		return nil, "", errors.New("simc ran none of the gear combinations")
	}
	if complaint, err := r.fill(&both, bags[combo:combo+1], 70, 5, "the best gear"); err != nil {
		return nil, complaint, err
	}
	best, _ := both.sets(bags[combo].name)
	swaps := bags[combo].swaps
	s := models.Suggestion{Kind: "gear", What: "Wear " + swapsText(swaps), Swaps: slimmed(swaps),
		Evidence: fmt.Sprintf("the best of %d combinations of your gear", len(bags))}
	if !gained(&s, best, both) {
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

// farm runs the Droptimizer's drops and returns the best maxFarmSuggestions items the sim shows as a gain on either
// fight, each once, as drops to farm.
func (r suggestRun) farm(drops []gearCombo) ([]models.Suggestion, string, error) {
	if len(drops) == 0 {
		return nil, "", nil
	}
	both, complaint, err := r.raceBoth(drops, 70, 95, 6, "drops to farm")
	if err != nil {
		return nil, complaint, err
	}
	type drop struct {
		s models.Suggestion
		d gearCombo
	}
	var found []drop
	for _, d := range drops {
		set, ran := both.sets(d.name)
		if !ran {
			continue
		}
		in := d.swaps[0].In
		s := models.Suggestion{Kind: "farm", What: fmt.Sprintf("Farm %s (%d) from %s", in.Name, in.Level, d.boss),
			Evidence: fmt.Sprintf("the Droptimizer at %s and +%d, at the rank of the gear you wear",
				defaultDropDifficulty, r.sims.Choices().DefaultKeyLevel),
			Swaps: slimmed(d.swaps), Source: d.source, Boss: d.boss}
		if gained(&s, set, both) {
			found = append(found, drop{s, d})
		}
	}
	slices.SortStableFunc(found, func(a, b drop) int { return byGain(a.s, b.s) })
	var picked []drop
	seen := map[int]bool{}
	for _, f := range found {
		if id := f.s.Swaps[0].In.ID; len(picked) < maxFarmSuggestions && !seen[id] {
			seen[id] = true
			picked = append(picked, f)
		}
	}
	combos := make([]gearCombo, len(picked))
	for i, f := range picked {
		combos[i] = f.d
	}
	if complaint, err := r.fill(&both, combos, 95, 7, "drops to farm"); err != nil {
		return nil, complaint, err
	}
	out := make([]models.Suggestion, len(picked))
	for i, f := range picked {
		set, _ := both.sets(f.d.name)
		gained(&f.s, set, both)
		out[i] = f.s
	}
	slices.SortStableFunc(out, byGain)
	return out, "", nil
}

// fill races each of picked on the fights whose smart stages dropped it, so each has a gain on both: a stage drops
// what can't be the best there, which may still beat the unchanged character. The progress bar stays at at.
func (r suggestRun) fill(both *races, picked []gearCombo, at, step int, what string) (string, error) {
	for i, f := range suggestFights {
		missing := slices.DeleteFunc(slices.Clone(picked), func(c gearCombo) bool {
			return slices.ContainsFunc(both[i].sets, func(s simSetJSON) bool { return s.Name == c.name })
		})
		if len(missing) == 0 {
			continue
		}
		base, complaint, err := r.race(r.head, missing, f.options(r.o), at, at, step+i, what+" again on "+f.label)
		if err != nil {
			return complaint, err
		}
		sets, err := readSets(r.output)
		if err != nil {
			return "", err
		}
		for _, set := range sets {
			// gains are read against the fight's first baseline: move this run's gain onto it
			set.Mean += both[i].base.DPS - base.DPS
			both[i].sets = append(both[i].sets, set)
		}
	}
	return "", nil
}

// swapsTo are the swaps from the gear worn to the gear now, by slot.
func swapsTo(worn, now map[string]*models.SimItem) []models.SimSwap {
	var swaps []models.SimSwap
	for _, slot := range simSlots {
		if now[slot] != worn[slot] {
			swaps = append(swaps, models.SimSwap{Slot: slot, Out: worn[slot], In: now[slot]})
		}
	}
	return swaps
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
	unique := uniqueGems(r.sims.Ref)
	combo := gearCombo{name: "combined"}
	picks, swapped := r.p.Picks, false
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
				if !uniqueGemsOK(unique, r.p, swapsTo(worn, now)) {
					now[t.s.Slot] = item
					continue
				}
			case gearFirst:
				for _, sw := range t.combo.swaps {
					now[sw.Slot] = sw.In
				}
			case t.s.Talent != nil:
				next := swapTalent(picks, t.s)
				if validTalents(r.tree, next) != nil {
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
		line, err := talentLine(r.tree, picks)
		if err != nil {
			return nil, "", fmt.Errorf("writing the combined talents: %w", err)
		}
		combo.line += line
	}
	combo.swaps = swapsTo(worn, now)
	both, complaint, err := r.raceBoth([]gearCombo{combo}, 95, 100, 8, "all the changes together")
	if err != nil {
		return nil, complaint, err
	}
	set, ran := both.sets(combo.name)
	if !ran {
		return nil, "", errors.New("simc ran no combined change")
	}
	for _, j := range in {
		shown[j].s.Combined = true
	}
	out := &models.SimCombo{DPS: set[0].Mean, Swaps: slimmed(combo.swaps), Consumable: strings.Join(names, " + ")}
	out.Gain, out.DPSError = difference(set[0], both[0].base)
	out.AoEGain, out.AoEError = difference(set[1], both[1].base)
	return out, "", nil
}
