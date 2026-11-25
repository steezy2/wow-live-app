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
	// suggestPoints is about what reading the top logs costs, as for a player review: compareLogs logs at about 8.
	suggestPoints = compareLogs * 8
	// suggestSteps is how many steps a Suggest sim's stage text counts.
	suggestSteps = 4
)

// errNoSuggestLogs is why a Suggest sim is refused without Warcraft Logs.
var errNoSuggestLogs = fmt.Errorf("%w: Suggest compares you with the top Warcraft Logs players of your spec: set "+
	"Warcraft Logs up in Settings, or run the stat weights sim, which needs none", ErrBadSimOptions)

// SuggestLogs reads the top players a Suggest sim compares a character with (Comparer.TopBuilds).
type SuggestLogs interface {
	TopBuilds(ctx context.Context, specID int, character string) (TopBuilds, error)
}

// TopBuilds are the top players a Suggest sim compares a character with: their builds, the character's talents
// against theirs (nil when no saved key or pull holds the character's) with the spec's tree to name them, which logs
// they are and the Warcraft Logs points reading them spent.
type TopBuilds struct {
	Tops     []models.RankedPlayer
	Talents  *models.CompareTalents
	Tree     models.TalentTree
	Baseline string
	Points   float64
}

// TopBuilds reads the top logs a Suggest sim compares character (the export's name) in spec specID with, picked as a
// player review picks them for the content last played: the character's latest saved key or pull in that spec (whose
// talents are compared too), else the latest one saved. It spends Warcraft Logs points: ErrNoPoints with too few
// left, ErrNoTopLogs with nothing saved or no log read.
func (c Comparer) TopBuilds(ctx context.Context, specID int, character string) (TopBuilds, error) {
	spec, ok := specs[specID]
	f, played := c.lastPlayed(specID, character)
	if !ok || !played {
		return TopBuilds{}, fmt.Errorf("%w: play a key or a raid pull first, so the top logs are of what you play",
			ErrNoTopLogs)
	}
	f.spec, f.specID = spec, specID
	reviewing.Lock()
	defer reviewing.Unlock()
	top, err := c.readTopLogs(ctx, f, 0)
	if err != nil {
		return TopBuilds{}, err
	}
	b := TopBuilds{Tops: top.players, Tree: c.Talents[specID], Baseline: top.baseline, Points: top.points}
	if len(f.local.Talents) > 0 {
		b.Talents = talentCompare(compareInput{you: f.local, top: top.players, specID: specID})
	}
	return b, nil
}

// lastPlayed is the fight whose top logs a Suggest sim reads, with the character's talents in it: their newest saved
// key or pull in spec specID, else the newest saved one (a pull only on a difficulty Warcraft Logs ranks); false with
// none.
func (c Comparer) lastPlayed(specID int, character string) (fightRun, bool) {
	var best fightRun
	found, mine := false, false
	consider := func(run fightRun, player string) {
		if !found || player != "" && !mine || (player != "") == mine && run.start > best.start {
			best, found, mine = run, true, player != ""
		}
	}
	keys, pulls := c.Keys.History()
	for _, k := range keys {
		player := playerIn(k.Specs, specID, character)
		consider(fightRun{kind: models.FightMPlus, start: k.Start, encounter: k.Dungeon, level: k.Level,
			local: models.RankedPlayer{DurationMS: lengthMS(k), Talents: k.Talents[player]}}, player)
	}
	for _, p := range pulls {
		if d := models.RankedDifficultyWCL(p.Difficulty); d != 0 {
			player := playerIn(p.Specs, specID, character)
			consider(fightRun{kind: models.FightRaid, start: p.Start, encounter: p.Boss, boss: d,
				local: models.RankedPlayer{DurationMS: p.DurationMS, Talents: p.Talents[player]}}, player)
		}
	}
	return best, found
}

// playerIn is the name character played spec specID under in a fight's specs, "" when they didn't.
func playerIn(specs map[string]int, specID int, character string) string {
	for name, id := range specs {
		if id == specID && strings.EqualFold(name, character) {
			return name
		}
	}
	return ""
}

// planSuggest counts a Suggest sim's runs into plan: the stat weights, a profileset for each change the top players
// can suggest (maxSuggestions at most), Top Gear over the bags, the Droptimizer and the combined run, each with its
// baseline (the changes counted at their most, as how many the top logs suggest isn't known before they are read);
// with the points the top logs cost, what it leaves out (Notes) and, without Warcraft Logs, why Start refuses it. It
// returns the full-precision runs.
func (s *Sims) planSuggest(p models.SimProfile, o models.SimOptions, plan *models.SimPlan) float64 {
	if !s.logsReady() && plan.Error == "" {
		plan.Error = errNoSuggestLogs.Error()
	}
	bags, drops, notes := s.suggestGear(p)
	plan.Points, plan.Notes, plan.Combos = suggestPoints, notes, maxSuggestions+len(bags)+len(drops)+1
	final := simTargetError(o, true)
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

// suggestRun is a running Suggest sim: the sims, its ID, work files and simc input head, its options, the character
// and the consumables it uses.
type suggestRun struct {
	sims                    *Sims
	ctx                     context.Context
	id, input, output, head string
	o                       models.SimOptions
	p                       models.SimProfile
	consumables             models.SimConsumables
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
// players make that the character doesn't, against the unchanged character; (3) Top Gear over the bags and the
// Droptimizer's drops; (4) every change shown that fits at once. A tested change the sim doesn't show as a gain is
// left out and counted. The result is the stat weights sim's, with the changes in Suggest.
func (r suggestRun) run() (*models.SimResult, string, error) {
	type read struct {
		builds TopBuilds
		err    error
	}
	logs := make(chan read, 1)
	go func() {
		b, err := r.sims.topBuilds(r.ctx, r.p)
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
	got := <-logs
	report := &models.SimSuggestReport{Suggestions: []models.Suggestion{}, Notes: []string{}}
	var shown []tested
	var untested []models.Suggestion
	if got.err != nil {
		report.Notes = append(report.Notes, "No changes from the top players: "+got.err.Error())
	} else {
		report.Baseline, report.Points = got.builds.Baseline, got.builds.Points
		in := suggestInput{ref: r.sims.Ref, you: r.p, consumables: r.consumables, tops: got.builds.Tops,
			weights: result.Weights, talents: got.builds.Talents, talentTree: got.builds.Tree}
		if st := out.Sim.Players[0].Collected.Buffed.Stats; st.Crit+st.Haste+st.Mastery+st.Vers > 0 {
			in.yourStats = &models.RankedStats{Crit: int(st.Crit), Haste: int(st.Haste), Mastery: int(st.Mastery),
				Vers: int(st.Vers)}
		}
		var dropped int
		if shown, untested, dropped, complaint, err = r.testChanges(buildSuggestions(in)); err != nil {
			return nil, complaint, err
		}
		report.Dropped += dropped
	}
	bags, drops, notes := r.sims.suggestGear(r.p)
	report.Notes = append(report.Notes, notes...)
	gear, dropped, complaint, err := r.bestGear(bags)
	if err != nil {
		return nil, complaint, err
	}
	shown, report.Dropped = append(shown, gear...), report.Dropped+dropped
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
		return cmp.Compare(b.Gain+b.Expected, a.Gain+a.Expected) // one of them is 0; a talent's both are
	})
	result.Suggest = report
	return result, "", nil
}

// topBuilds reads the top players of the character's spec.
func (s *Sims) topBuilds(ctx context.Context, p models.SimProfile) (TopBuilds, error) {
	spec := specIDOf(s.Drops, p)
	if spec == 0 {
		return TopBuilds{}, fmt.Errorf("%w: no spec %s %s is known", ErrNoTopLogs, p.Spec, p.Class)
	}
	return s.Tops.TopBuilds(ctx, spec, p.Name)
}

// testChanges runs one profileset for each of the first maxSuggestions changes the sim can test (an enchant, a gem, a
// consumable) and returns those it shows as a gain, the ones it can't test (a stat, which the stat weights price, and
// a talent: the sim would need the talent string the export's talents are written in, and nothing here encodes one,
// so a talent stays the top players' evidence only) and how many it dropped.
func (r suggestRun) testChanges(all []models.Suggestion) (shown []tested, untested []models.Suggestion, dropped int,
	complaint string, err error) {
	var changes []tested
	for _, sg := range all {
		if sg.Variant == nil && sg.Consumables == nil {
			untested = append(untested, sg)
			continue
		}
		combo, ok := changeCombo(r.sims.Ref, r.p, sg)
		if !ok || len(changes) == maxSuggestions {
			dropped++
			continue
		}
		combo.name = "change" + strconv.Itoa(len(changes)+1)
		changes = append(changes, tested{sg, combo})
	}
	if len(changes) == 0 {
		return nil, untested, dropped, "", nil
	}
	combos := make([]gearCombo, len(changes))
	for i, t := range changes {
		combos[i] = t.combo
	}
	what := fmt.Sprintf("testing %d changes", len(changes))
	base, complaint, err := r.race(r.head, combos, 30, 50, 2, what)
	if err != nil {
		return nil, nil, 0, complaint, err
	}
	sets, err := readSets(r.output)
	if err != nil {
		return nil, nil, 0, "", err
	}
	for _, t := range changes {
		i := slices.IndexFunc(sets, func(s simSetJSON) bool { return s.Name == t.combo.name })
		if i < 0 || !gained(&t.s, sets[i], base) {
			dropped++
			continue
		}
		shown = append(shown, t)
	}
	return shown, untested, dropped, "", nil
}

// gained sets s's gain from its profileset over base and says whether the sim shows it as one: at least
// suggestMinGain of the DPS and more than suggestZ standard errors of the difference.
func gained(s *models.Suggestion, set simSetJSON, base *models.SimResult) bool {
	s.Gain, s.DPSError = set.Mean-base.DPS, math.Hypot(set.MeanStdDev, base.DPSError)
	return s.Gain >= suggestMinGain*base.DPS && s.Gain > suggestZ*s.DPSError
}

// changeCombo is a change as a profileset: an enchant or gem put on the worn item it names (false when the item can't
// take it), or the consumable's line. The change is built from the reference and the character's own items.
func changeCombo(ref models.SimReference, p models.SimProfile, s models.Suggestion) (gearCombo, bool) {
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
// dropped.
func (r suggestRun) bestGear(bags []gearCombo) ([]tested, int, string, error) {
	if len(bags) == 0 {
		return nil, 0, "", nil
	}
	base, complaint, err := r.race(r.head, bags, 50, 70, 3, "gear from the bags")
	if err != nil {
		return nil, 0, complaint, err
	}
	sets, err := readSets(r.output)
	if err != nil {
		return nil, 0, "", err
	}
	best, combo := simSetJSON{}, -1
	for _, set := range sets {
		if i := slices.IndexFunc(bags, func(c gearCombo) bool { return c.name == set.Name }); i >= 0 &&
			(combo < 0 || set.Mean > best.Mean) {
			best, combo = set, i
		}
	}
	if combo < 0 {
		return nil, 0, "", errors.New("simc ran none of the gear combinations")
	}
	swaps := bags[combo].swaps
	s := models.Suggestion{Kind: "gear", What: "Wear " + swapsText(swaps), Swaps: slimmed(swaps),
		Evidence: fmt.Sprintf("the best of %d combinations of your gear", len(bags))}
	if !gained(&s, best, base) {
		return nil, 1, "", nil
	}
	return []tested{{s, bags[combo]}}, 0, "", nil
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
	base, complaint, err := r.race(r.head, drops, 70, 95, 3, "drops to farm")
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
// slot then, and the consumables) as one profileset and marks the changes in it: nil with fewer than two.
func (r suggestRun) combined(shown []tested) (*models.SimCombo, string, error) {
	worn := map[string]*models.SimItem{}
	for i := range r.p.Equipped {
		worn[r.p.Equipped[i].Slot] = &r.p.Equipped[i]
	}
	now := maps.Clone(worn)
	combo := gearCombo{name: "combined"}
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
	for _, slot := range simSlots {
		if now[slot] != worn[slot] {
			combo.swaps = append(combo.swaps, models.SimSwap{Slot: slot, Out: worn[slot], In: now[slot]})
		}
	}
	base, complaint, err := r.race(r.head, []gearCombo{combo}, 95, 100, 4, "all the changes together")
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
