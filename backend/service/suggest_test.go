package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"wow-live/backend/models"
)

// fakeTops is Warcraft Logs for a Suggest sim: the same builds for any spec, and the spec it was asked for.
type fakeTops struct {
	builds TopBuilds
	asked  []int
}

func (f *fakeTops) TopBuilds(_ context.Context, specID int, _ string) (TopBuilds, error) {
	f.asked = append(f.asked, specID)
	return f.builds, nil
}

// oneHanders says no weapon is two-handed.
type oneHanders struct{}

func (oneHanders) TwoHanded(context.Context, int) (bool, error) { return false, nil }

// suggestSims are sims of the test export (an elemental shaman, spec 262) over testRef, with ten top players who all
// put gem 240883 in their head, enchant 7964 on a ring, have Flask A up and took one talent the character didn't.
// Its engine echoes profilesets over a baseline of 99950 DPS (1 DPS sure) with stat weights.
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
			Gear: []models.RankedGear{{Slot: "head", Gems: []int{240883}}, {Slot: "finger1", Enchant: 7964}}}
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
	// the gem and the best gear from the bags (combo1, 50 DPS) as tested, then the talent, which the sim can't test
	if !slices.Equal(got, []string{"gem", "gear", "talent"}) || report.Suggestions[0].Gain != 50 ||
		report.Suggestions[2].Gain != 0 {
		t.Errorf("suggestions = %+v, want the gem and gear gaining 50 DPS, then the talent", report.Suggestions)
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
	if err != nil || plan.Error != "" || plan.Points != suggestPoints || len(plan.Notes) != 1 {
		t.Errorf("plan = %+v, %v; want %d points and the drops it leaves out", plan, err, suggestPoints)
	}
	req.Kind = "stats"
	if stats, _ := s.Plan(req); stats.Seconds >= plan.Seconds || stats.Points != 0 {
		t.Errorf("a stat weights plan (%+v) should cost no points and less time than Suggest's (%+v)", stats, plan)
	}
	req.Kind = "suggest"
	s.KeysSet = func() bool { return false }
	if plan, _ := s.Plan(req); plan.Error != errNoSuggestLogs.Error() {
		t.Errorf("without Warcraft Logs keys the plan says %q, want %q", plan.Error, errNoSuggestLogs)
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
