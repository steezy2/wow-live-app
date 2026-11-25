package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"wow-live/backend/models"
)

// fakeTops is Warcraft Logs for a Suggest sim: the same builds (or err) for any spec, and the specs and targets it
// was asked for. With block it waits for block to close first, whatever the context, as a read waits on a player
// review.
type fakeTops struct {
	builds  TopBuilds
	err     error
	block   chan struct{}
	asked   []int
	targets []models.SuggestTarget
}

func (f *fakeTops) TopBuilds(_ context.Context, specID int, _ string, target models.SuggestTarget) (TopBuilds,
	error) {
	if f.block != nil {
		<-f.block
	}
	f.asked, f.targets = append(f.asked, specID), append(f.targets, target)
	return f.builds, f.err
}

// suggestRequest is a Suggest sim of the test export on Mythic+ overall.
func suggestRequest(t *testing.T) models.SimRequest {
	t.Helper()
	req := simRequest(t)
	req.Kind, req.Target = "suggest", &models.SuggestTarget{Kind: models.FightMPlus}
	return req
}

// oneHanders says no weapon is two-handed.
type oneHanders struct{}

func (oneHanders) TwoHanded(context.Context, int) (bool, error) { return false, nil }

// suggestSims are sims of the test export (an elemental shaman, spec 262) over testRef and the reference talent trees,
// with ten top players who all put gem 240883 in their head, enchant 7964 on a ring, have Flask A up, took the other
// entry of the character's first choice talent and have half their secondary stats in crit (the character a
// quarter). Its engine echoes profilesets over a baseline of 99950 DPS (1 DPS sure) with stat weights.
func suggestSims(t *testing.T) (*Sims, *fakeTops) {
	t.Helper()
	s := newTestSims(t, "echo")
	result := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(result, []byte(`{"sim": {"players": [{"collected_data": {
		"dps": {"mean": 99950, "mean_std_dev": 1},
		"buffed_stats": {"stats": {"crit_rating": 1000, "haste_rating": 1000, "mastery_rating": 1000,
			"versatility_rating": 1000}}},
		"scale_factors": {"Int": 1, "Crit": 0.5, "Haste": 0.6, "Mastery": 0.4, "Vers": 0.45},
		"stats": [], "buffs": []}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Engine.(*fakeEngine).result = result
	s.Ref, s.Items, s.Bosses = testRef(), oneHanders{}, []string{"Sszorak"}
	s.Ref.Talents = referenceTrees(t)
	s.Drops = models.SimDrops{Specs: map[int]models.SimSpec{262: {Class: "shaman", Spec: "elemental"}}}
	tree := s.Ref.Talents[262]
	var other models.TalentPick
	for _, p := range profileTalents(s.Ref.Talents, fixtureTalents(t)[0]) {
		if n := slices.IndexFunc(tree.Nodes, func(n models.TalentNode) bool {
			return n.ID == p.NodeID && n.Choice && !n.Free
		}); n >= 0 {
			e := tree.Nodes[n].Entries
			other = models.TalentPick{NodeID: p.NodeID, EntryID: e[0].ID + e[1].ID - p.EntryID, Rank: 1}
			break
		}
	}
	tops := make([]models.RankedPlayer, 10)
	for i := range tops {
		tops[i] = models.RankedPlayer{Buffs: []models.RankedBuff{{Name: "Flask A"}},
			Gear:    []models.RankedGear{{Slot: "head", Gems: []int{240883}}, {Slot: "finger1", Enchant: 7964}},
			Stats:   &models.RankedStats{Crit: 2000, Haste: 1000, Mastery: 500, Vers: 500},
			Talents: []models.TalentPick{other}}
	}
	logs := &fakeTops{builds: TopBuilds{Tops: tops, Baseline: "10 ranked runs", Points: 80}}
	s.Tops = logs
	return s, logs
}

func TestSuggesterDropsWhatTheSimDoesNotConfirm(t *testing.T) {
	s, logs := suggestSims(t)
	// change1 (the head's gem) gains 50 DPS, over 0.05% of 99950 and its error; the ring's enchant gains 250 but within
	// its error of 150; the flask loses
	s.Engine.(*fakeEngine).sets = map[string]simSetJSON{"change2": {Mean: 100200, MeanStdDev: 150},
		"change3": {Mean: 99000, MeanStdDev: 1}}
	started, err := s.Start(suggestRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	sim := waitSim(t, s, started.ID)
	if sim.State != models.SimDone || sim.Result.Suggest == nil {
		t.Fatalf("sim = %+v", sim)
	}
	report := sim.Result.Suggest
	var got []string
	for _, sg := range report.Suggestions {
		got = append(got, sg.Kind)
	}
	// the gem and the best gear from the bags (combo1, 50 DPS) as tested, then more crit (priced at 500 DPS by the
	// weights, but untested), then the talent swap (change4, 20 DPS: no gain) as another talent, not counted as dropped
	if !slices.Equal(got, []string{"gem", "gear", "stat", "talentOther"}) || report.Suggestions[0].Gain != 50 ||
		report.Suggestions[2].Expected != 500 || report.Suggestions[3].Gain != 0 {
		t.Errorf("suggestions = %+v, want the gem and gear gaining 50 DPS, then the stat, then the talent",
			report.Suggestions)
	}
	if report.Dropped != 2 || report.Points != 80 || report.Baseline != "10 ranked runs" {
		t.Errorf("dropped %d, points %v, baseline %q; want the enchant and flask dropped and the logs' numbers",
			report.Dropped, report.Points, report.Baseline)
	}
	if c := report.Combined; c == nil || c.Gain != 50 || !report.Suggestions[0].Combined ||
		!report.Suggestions[1].Combined || report.Suggestions[2].Combined {
		t.Errorf("combined = %+v, want the gem and gear together", report.Combined)
	}
	if len(sim.Result.Weights) == 0 || sim.Result.Pawn == "" || !slices.Equal(logs.asked, []int{262}) ||
		!slices.Equal(logs.targets, []models.SuggestTarget{{Kind: models.FightMPlus}}) {
		t.Errorf("weights %+v, pawn %q, asked %v of %v; want the stat weights and spec 262's Mythic+ logs",
			sim.Result.Weights, sim.Result.Pawn, logs.asked, logs.targets)
	}
	if len(report.Notes) != 1 || !strings.HasPrefix(report.Notes[0], "Drops to farm aren't tried") {
		t.Errorf("notes = %q, want why no drops were tried", report.Notes)
	}
	inputs := strings.Join(s.Engine.(*fakeEngine).inputs, "\n")
	for _, want := range []string{"calculate_scale_factors=1\n", "\nprofileset.change1+=head=,id=271483,",
		"gem_id=240883", "\nprofileset.change3+=flask=flask_a_2\n", "\nprofileset.change4+=talents=",
		"\nprofileset.combined+="} {
		if !strings.Contains(inputs, want) {
			t.Errorf("no sim input has %q", want)
		}
	}
}

func TestSuggestSortsATalentSwapByItsGain(t *testing.T) {
	tests := []struct {
		name string
		mean float64 // the talent swap's (change4) DPS against the baseline's 99950
		kind string
		gain float64
	}{
		{"a gain", 100950, "talent", 1000},
		{"under the least gain", 99960, "talentOther", 0},
		{"a loss", 99000, "talentOther", -950},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := suggestSims(t)
			s.Engine.(*fakeEngine).sets = map[string]simSetJSON{"change4": {Mean: tt.mean, MeanStdDev: 1}}
			started, err := s.Start(suggestRequest(t))
			if err != nil {
				t.Fatal(err)
			}
			report := waitSim(t, s, started.ID).Result.Suggest
			i := slices.IndexFunc(report.Suggestions, func(s models.Suggestion) bool { return s.Talent != nil })
			// the enchant and flask, under the least gain, are dropped; the talent swap never is, and joins the
			// combined row only as a gain
			if i < 0 || report.Suggestions[i].Kind != tt.kind || report.Suggestions[i].Gain != tt.gain ||
				report.Suggestions[i].Combined != (tt.kind == "talent") || report.Dropped != 2 {
				t.Fatalf("suggestions %+v, dropped %d; want a %s gaining %v and 2 dropped", report.Suggestions,
					report.Dropped, tt.kind, tt.gain)
			}
		})
	}
}

func TestSuggestShowsAChangeThatGainsOnEitherFight(t *testing.T) {
	// the head's gem (change1) against the baseline of 99950 DPS on one target and on AoE; the enchant and flask never
	// gain; the best gear from the bags gains 50 DPS on both and the gem's gain on the fight it gains on is 1000
	gem := func(mean float64) map[string]simSetJSON {
		return map[string]simSetJSON{"change1": {Mean: mean, MeanStdDev: 1}, "combined": {Mean: 101000, MeanStdDev: 1}}
	}
	tests := []struct {
		name          string
		single, aoe   map[string]simSetJSON
		gain, aoeGain float64
		shown         bool
		dropped       int
	}{
		{"only on AoE", gem(99950), gem(100950), 0, 1000, true, 2},
		{"only on one target", gem(100950), gem(99950), 1000, 0, true, 2},
		{"on neither", gem(99950), gem(99950), 0, 0, false, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := suggestSims(t)
			engine := s.Engine.(*fakeEngine)
			engine.sets, engine.aoeSets = tt.single, tt.aoe
			started, err := s.Start(suggestRequest(t))
			if err != nil {
				t.Fatal(err)
			}
			report := waitSim(t, s, started.ID).Result.Suggest
			i := slices.IndexFunc(report.Suggestions, func(s models.Suggestion) bool { return s.Kind == "gem" })
			if (i >= 0) != tt.shown || report.Dropped != tt.dropped {
				t.Fatalf("suggestions %+v, dropped %d; want the gem shown %v and %d dropped", report.Suggestions,
					report.Dropped, tt.shown, tt.dropped)
			}
			if tt.shown {
				got := report.Suggestions[i]
				if got.Gain != tt.gain || got.AoEGain != tt.aoeGain || i != 0 || got.Combined != true ||
					report.Combined == nil || report.Combined.AoEGain == 0 {
					t.Errorf("gem = %+v, combined %+v; want the gains %v and %v, first, in the combined row", got,
						report.Combined, tt.gain, tt.aoeGain)
				}
			}
		})
	}
}

func TestSuggestPlanCountsBothFightsForEverythingButTheStatWeights(t *testing.T) {
	s, _ := suggestSims(t)
	req := suggestRequest(t)
	p, err := s.parse(req.Export)
	if err != nil {
		t.Fatal(err)
	}
	bags, _, _ := s.suggestGear(p)
	final := simTargetError(req.Options, true)
	oneFight := float64(maxSuggestions+1+2) + smartRuns(len(bags), smartStages(len(bags), final), final) + 1
	if got := s.planSuggest(p, req, &models.SimPlan{}); got != statsRuns+2*oneFight {
		t.Errorf("runs = %v, want the stat weights' %d plus %v for each of the two fights", got, statsRuns, oneFight)
	}
}

func TestSuggestPlanCostsWCLPoints(t *testing.T) {
	s, _ := suggestSims(t)
	req := suggestRequest(t)
	plan, err := s.Plan(req)
	if err != nil || plan.Error != "" || plan.NeedsLogs || plan.Points != 160 || len(plan.Notes) != 1 {
		t.Errorf("plan = %+v, %v; want 160 points and the drops it leaves out", plan, err)
	}
	req.Kind = "stats"
	if stats, _ := s.Plan(req); stats.Seconds >= plan.Seconds || stats.Points != 0 {
		t.Errorf("a stat weights plan (%+v) should cost no points and less time than Suggest's (%+v)", stats, plan)
	}
	req.Kind = "suggest"
	s.KeysSet = func() bool { return false }
	if plan, _ := s.Plan(req); plan.Error != errNoSuggestLogs.Error() || !plan.NeedsLogs {
		t.Errorf("without Warcraft Logs keys the plan says %q (needs logs %v), want %q", plan.Error, plan.NeedsLogs,
			errNoSuggestLogs)
	}
	if _, err := s.Start(req); !errors.Is(err, ErrBadSimOptions) {
		t.Errorf("Start without Warcraft Logs keys: %v, want it refused", err)
	}
}

func TestSuggestRefusesAMissingOrUnknownTarget(t *testing.T) {
	s, _ := suggestSims(t)
	for name, target := range map[string]*models.SuggestTarget{
		"none":                    nil,
		"a boss not in the raid":  {Kind: models.FightRaid, Encounter: "Nobody", Difficulty: 4},
		"a difficulty not ranked": {Kind: models.FightRaid, Encounter: "Sszorak", Difficulty: 1},
	} {
		req := suggestRequest(t)
		req.Target = target
		if plan, err := s.Plan(req); err != nil || plan.Error != errNoSuggestTarget.Error() {
			t.Errorf("%s: plan error %q (%v), want %q", name, plan.Error, err, errNoSuggestTarget)
		}
		if _, err := s.Start(req); !errors.Is(err, ErrBadSimOptions) {
			t.Errorf("%s: Start = %v, want it refused", name, err)
		}
	}
}

func TestTopBuildsRanksTheTarget(t *testing.T) {
	runs := make([]models.RankedRun, 8)
	for i := range runs {
		runs[i] = models.RankedRun{DurationMS: 300_000 + int64(i)*1000, Code: "r", FightID: i + 1, Name: "P"}
	}
	mplus := func(encounter, level int) models.RankQuery {
		return models.RankQuery{Encounter: encounter, Level: level, Class: "Mage", Spec: "Arcane", Metric: "dps"}
	}
	tests := []struct {
		name      string
		character string
		target    models.SuggestTarget
		queries   []models.RankQuery
		reads     int
		baseline  string
	}{
		{"Mythic+ at the character's highest key", "testmage", models.SuggestTarget{Kind: models.FightMPlus},
			[]models.RankQuery{mplus(1, 12), mplus(3, 12), mplus(5, 12), mplus(7, 12)},
			suggestDungeons * suggestLogsPerDungeon, "20 ranked Arcane Mage runs in 4 dungeons at +12"},
		{"Mythic+ for a character with no key", "nobody", models.SuggestTarget{Kind: models.FightMPlus},
			[]models.RankQuery{mplus(1, 2), mplus(3, 2), mplus(5, 2), mplus(7, 2)},
			suggestDungeons * suggestLogsPerDungeon, "20 ranked Arcane Mage runs in 4 dungeons at +2"},
		{"a raid boss on its difficulty", "testmage",
			models.SuggestTarget{Kind: models.FightRaid, Encounter: "Sszorak", Difficulty: 4},
			[]models.RankQuery{{Encounter: 100, Difficulty: 4, Class: "Mage", Spec: "Arcane", Metric: "dps"}},
			len(runs), "8 ranked Arcane Mage runs within 15% of 5:03"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baselines := &fakeLogs{left: 1000, runs: runs}
			c, _, _ := comparerOf(t, baselines)
			b, err := c.TopBuilds(context.Background(), 62, tt.character, tt.target)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(baselines.queries, tt.queries) || len(b.Tops) != tt.reads || b.Baseline != tt.baseline {
				t.Errorf("queries %+v, %d tops, baseline %q; want %+v, %d and %q",
					baselines.queries, len(b.Tops), b.Baseline, tt.queries, tt.reads, tt.baseline)
			}
		})
	}
}

func TestSuggestFarmsTheBestDropsEachOnce(t *testing.T) {
	s, _ := suggestSims(t)
	// a ring and a trinket (each tried in both slots) and four more drops, each a gain, the first the best
	var items []models.DropItem
	for i, slot := range []string{"finger", "trinket", "neck", "back", "wrist", "waist"} {
		items = append(items, models.DropItem{ID: 901 + i, Name: slot, Slot: slot})
	}
	s.Drops.Raids = []models.DropSource{{Name: "Raid", Bosses: []models.DropBoss{{Name: "Boss",
		Levels: &models.DifficultyLevels{Heroic: 250}, Items: items}}}}
	s.Drops.Keys = []models.KeyLevels{{Level: 10, Loot: 250}}
	sets := map[string]simSetJSON{}
	for i := range 8 {
		sets["drop"+strconv.Itoa(i+1)] = simSetJSON{Mean: 100500 - 10*float64(i), MeanStdDev: 1}
	}
	s.Engine.(*fakeEngine).sets = sets
	started, err := s.Start(suggestRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	sim := waitSim(t, s, started.ID)
	if sim.State != models.SimDone {
		t.Fatalf("sim = %+v", sim)
	}
	var farmed []int
	for _, sg := range sim.Result.Suggest.Suggestions {
		if sg.Kind == "farm" {
			farmed = append(farmed, sg.Swaps[0].In.ID)
		}
	}
	if !slices.Equal(farmed, []int{901, 902, 903, 904, 905}) {
		t.Errorf("farmed %v, want the best %d items, the ring and trinket once each", farmed, maxFarmSuggestions)
	}
}

func TestSuggestGoesOnWithoutTheTopLogs(t *testing.T) {
	s, logs := suggestSims(t)
	logs.err = fmt.Errorf("reading: %w", models.ErrRateLimited)
	started, err := s.Start(suggestRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	sim := waitSim(t, s, started.ID)
	if sim.State != models.SimDone || len(sim.Result.Weights) == 0 {
		t.Fatalf("sim = %+v, want it done with the stat weights", sim)
	}
	report := sim.Result.Suggest
	if len(report.Suggestions) != 1 || report.Suggestions[0].Kind != "gear" {
		t.Errorf("suggestions = %+v, want the gear from the bags still tried", report.Suggestions)
	}
	if len(report.Notes) == 0 || !strings.HasPrefix(report.Notes[0], "No changes from the top players: ") {
		t.Errorf("notes = %q, want why the top players' changes are missing", report.Notes)
	}
}

func TestCancellingASuggestSimDoesNotWaitForTheTopLogs(t *testing.T) {
	s, logs := suggestSims(t)
	logs.block = make(chan struct{})
	t.Cleanup(func() { close(logs.block) })
	started, err := s.Start(suggestRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if sim, _ := s.Get(started.ID); strings.HasSuffix(sim.Stage, "reading the top logs") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the sim never waited on the top logs")
		}
	}
	if err := s.Remove(started.ID); err != nil {
		t.Fatal(err)
	}
	if sim := waitSim(t, s, started.ID); sim.State != models.SimCancelled {
		t.Errorf("state = %s, want cancelled while the top logs were read", sim.State)
	}
}
