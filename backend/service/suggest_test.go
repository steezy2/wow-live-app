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

// fakeTops is Warcraft Logs for a Suggest sim: the same builds (or err) for any spec, and the spec it was asked for.
// With block it waits for block to close first, whatever the context, as a read waits on a player review.
type fakeTops struct {
	builds TopBuilds
	err    error
	block  chan struct{}
	asked  []int
}

func (f *fakeTops) TopBuilds(_ context.Context, specID int, _ string) (TopBuilds, error) {
	if f.block != nil {
		<-f.block
	}
	f.asked = append(f.asked, specID)
	return f.builds, f.err
}

// oneHanders says no weapon is two-handed.
type oneHanders struct{}

func (oneHanders) TwoHanded(context.Context, int) (bool, error) { return false, nil }

// suggestSims are sims of the test export (an elemental shaman, spec 262) over testRef, with ten top players who all
// put gem 240883 in their head, enchant 7964 on a ring, have Flask A up, took one talent the character didn't and
// have half their secondary stats in crit (the character a quarter). Its engine echoes profilesets over a baseline of
// 99950 DPS (1 DPS sure) with stat weights.
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
	s.Ref, s.Items = testRef(), oneHanders{}
	s.Drops = models.SimDrops{Specs: map[int]models.SimSpec{262: {Class: "shaman", Spec: "elemental"}}}
	tops := make([]models.RankedPlayer, 10)
	for i := range tops {
		tops[i] = models.RankedPlayer{Buffs: []models.RankedBuff{{Name: "Flask A"}},
			Gear:  []models.RankedGear{{Slot: "head", Gems: []int{240883}}, {Slot: "finger1", Enchant: 7964}},
			Stats: &models.RankedStats{Crit: 2000, Haste: 1000, Mastery: 500, Vers: 500}}
	}
	logs := &fakeTops{builds: TopBuilds{Tops: tops, Baseline: "10 ranked runs", Points: 80,
		Talents: &models.CompareTalents{Of: 10, Top: []models.TalentShare{{NodeID: 1, EntryID: 11, Taken: 10}}}}}
	s.Tops = logs
	return s, logs
}

func TestSuggesterDropsWhatTheSimDoesNotConfirm(t *testing.T) {
	s, logs := suggestSims(t)
	// change1 (the head's gem) gains 50 DPS, over 0.05% of 99950 and its error; the ring's enchant gains 250 but within
	// its error of 150; the flask loses
	s.Engine.(*fakeEngine).sets = map[string]simSetJSON{"change2": {Mean: 100200, MeanStdDev: 150},
		"change3": {Mean: 99000, MeanStdDev: 1}}
	req := simRequest(t)
	req.Kind = "suggest"
	started, err := s.Start(req)
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
	// weights, but untested), then the talent, which the sim can't test
	if !slices.Equal(got, []string{"gem", "gear", "stat", "talent"}) || report.Suggestions[0].Gain != 50 ||
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
	if len(sim.Result.Weights) == 0 || sim.Result.Pawn == "" || !slices.Equal(logs.asked, []int{262}) {
		t.Errorf("weights %+v, pawn %q, asked %v; want the stat weights and spec 262's logs", sim.Result.Weights,
			sim.Result.Pawn, logs.asked)
	}
	if len(report.Notes) != 1 || !strings.HasPrefix(report.Notes[0], "Drops to farm aren't tried") {
		t.Errorf("notes = %q, want why no drops were tried", report.Notes)
	}
	inputs := strings.Join(s.Engine.(*fakeEngine).inputs, "\n")
	for _, want := range []string{"calculate_scale_factors=1\n", "\nprofileset.change1+=head=,id=271483,",
		"gem_id=240883", "\nprofileset.change3+=flask=flask_a_2\n", "\nprofileset.combined+="} {
		if !strings.Contains(inputs, want) {
			t.Errorf("no sim input has %q", want)
		}
	}
}

func TestSuggestPlanCostsWCLPoints(t *testing.T) {
	s, _ := suggestSims(t)
	req := simRequest(t)
	req.Kind = "suggest"
	plan, err := s.Plan(req)
	if err != nil || plan.Error != "" || plan.NeedsLogs || plan.Points != suggestPoints || len(plan.Notes) != 1 {
		t.Errorf("plan = %+v, %v; want %d points and the drops it leaves out", plan, err, suggestPoints)
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

func TestTopBuildsReadsTheLogsOfWhatWasLastPlayed(t *testing.T) {
	baselines := &fakeLogs{left: 1000}
	c, keys, _ := comparerOf(t, baselines)
	b, err := c.TopBuilds(context.Background(), 62, "testmage")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Tops) != 3 || !slices.Equal(baselines.windows, []int64{0, 0, 0}) || b.Points != 99 {
		t.Errorf("tops %d, windows %v, points %v; want the 3 ranked runs read whole and 99 points", len(b.Tops),
			baselines.windows, b.Points)
	}
	keys.history = nil
	if _, err := c.TopBuilds(context.Background(), 62, "testmage"); !errors.Is(err, ErrNoTopLogs) {
		t.Errorf("with nothing saved: %v, want ErrNoTopLogs", err)
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
	req := simRequest(t)
	req.Kind = "suggest"
	started, err := s.Start(req)
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
	req := simRequest(t)
	req.Kind = "suggest"
	started, err := s.Start(req)
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
	req := simRequest(t)
	req.Kind = "suggest"
	started, err := s.Start(req)
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

func TestLastPlayedPrefersTheCharactersOwnFight(t *testing.T) {
	c, keys, _ := comparerOf(t, &fakeLogs{})
	keys.history = append(keys.history, models.Key{Start: 9000, Dungeon: "Voidscar Arena", Level: 15,
		Specs: map[string]int{"Someone": 62}})
	for character, want := range map[string]int64{"testmage": 1000, "nobody": 9000} {
		if f, ok := c.lastPlayed(62, character); !ok || f.start != want {
			t.Errorf("%s: start %d (found %v), want %d: their own key, else the newest", character, f.start, ok, want)
		}
	}
}
