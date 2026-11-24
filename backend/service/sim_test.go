package service

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

// fakeEngine stands in for simc: mode "ok" writes testdata/sim-result.json as the result, "echo" writes it with a
// result for each profileset in the input (the first the best, each 10 DPS below the one before), "fail" prints why
// it failed, and "hang" runs until it is stopped.
type fakeEngine struct {
	installed bool
	built     time.Time
	mode      string
	args      []string // the last run's
	input     string   // the last run's input file
	inputs    []string // every run's input file
	result    string   // the result file to write; testdata/sim-result.json when empty
}

func (f *fakeEngine) Run(ctx context.Context, args []string, out io.Writer) error {
	f.args = args
	if data, err := os.ReadFile(args[0]); err == nil {
		f.input = string(data)
		f.inputs = append(f.inputs, f.input)
	}
	fmt.Fprint(out, "Generating Baseline: 1/1 [====>...] 50/100 1.0 Mean=1 Error=1%\r")
	switch f.mode {
	case "fail":
		fmt.Fprintln(out, "Error: Setup failure: bad thing")
		return errors.New("exit status 1")
	case "hang":
		<-ctx.Done()
		return ctx.Err()
	}
	data, err := os.ReadFile(cmp.Or(f.result, "testdata/sim-result.json"))
	if err != nil {
		return err
	}
	if f.mode == "echo" {
		if data, err = withProfilesets(data, f.input); err != nil {
			return err
		}
	}
	return os.WriteFile(strings.TrimPrefix(args[len(args)-1], "json2="), data, 0o644)
}

// profilesetName is a profileset line's name in a simc input.
var profilesetName = regexp.MustCompile(`(?m)^profileset\.(\w+)\+=`)

// withProfilesets is the result file data with a result for each profileset the input names, in its order, 10 DPS
// apart and each 1 DPS sure.
func withProfilesets(data []byte, input string) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	var results []simSetJSON
	seen := map[string]bool{}
	for _, m := range profilesetName.FindAllStringSubmatch(input, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			results = append(results, simSetJSON{Name: m[1], Mean: 100000 - 10*float64(len(results)), MeanStdDev: 1})
		}
	}
	doc["sim"].(map[string]any)["profilesets"] = map[string]any{"results": results}
	return json.Marshal(doc)
}

func (f *fakeEngine) Installed() models.SimEngine {
	return models.SimEngine{Installed: f.installed, BuiltMS: f.built.UnixMilli()}
}
func (f *fakeEngine) Install(context.Context) (models.SimEngine, error) { return f.Installed(), nil }
func (f *fakeEngine) UseFile(context.Context, string) (models.SimEngine, error) {
	return f.Installed(), nil
}

func newTestSims(t *testing.T, mode string) *Sims {
	t.Helper()
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	return &Sims{Engine: &fakeEngine{installed: true, built: now.Add(-24 * time.Hour), mode: mode}, Dir: t.TempDir(),
		Now: func() time.Time { return now }, Ctx: context.Background()}
}

func simRequest(t *testing.T) models.SimRequest {
	return models.SimRequest{Kind: "quick", Export: fixtureSimC(t), Options: simDefaults}
}

// waitSim waits for the sim to leave the running state.
func waitSim(t *testing.T, s *Sims, id string) models.Sim {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		sim, err := s.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if sim.State != models.SimRunning {
			return sim
		}
	}
	t.Fatal("the sim never finished")
	return models.Sim{}
}

func TestSimsRunsAndSavesAQuickSim(t *testing.T) {
	s := newTestSims(t, "ok")
	req := simRequest(t)
	req.Options = models.SimOptions{FightStyle: "HecticAddCleave", Length: 120, Targets: 3, Accuracy: "high"}
	started, err := s.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	if started.State != models.SimRunning || started.Character != "Testshaman" || started.Spec != "elemental" {
		t.Errorf("started = %+v", started)
	}
	sim := waitSim(t, s, started.ID)
	if sim.State != models.SimDone || sim.Progress != 100 || sim.Result == nil || sim.Result.DPS < 100000 ||
		len(sim.Result.Abilities) == 0 ||
		sim.Result.Abilities[0].DPS < sim.Result.Abilities[len(sim.Result.Abilities)-1].DPS {
		t.Errorf("sim = %+v", sim)
	}
	want := "fight_style=HecticAddCleave\nmax_time=120\ndesired_targets=3\ntarget_error=0.05\n"
	if got := strings.Join(s.Engine.(*fakeEngine).args, "\n"); !strings.Contains(got, want) ||
		strings.Contains(got, "html=") {
		t.Errorf("simc got %q", got)
	}
	// a new Sims on the same folder, as after a restart, still has it, and the list leaves its result out but its DPS
	again := &Sims{Engine: s.Engine, Dir: s.Dir, Now: s.Now}
	if saved, err := again.Get(started.ID); err != nil || saved.Result == nil || saved.Result.DPS != sim.Result.DPS {
		t.Errorf("saved = %+v, %v", saved, err)
	}
	if list, err := again.List(); err != nil || len(list) != 1 || list[0].Result != nil ||
		list[0].DPS != sim.Result.DPS {
		t.Errorf("list = %+v, %v", list, err)
	}
	if left, _ := filepath.Glob(filepath.Join(s.Dir, "work", "*")); len(left) != 0 {
		t.Errorf("the work folder still holds %v", left)
	}
	if err := s.Remove(started.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Errorf("a removed sim is still listed: %+v", list)
	}
}

func TestSimsStartRefuses(t *testing.T) {
	tests := []struct {
		name   string
		change func(*models.SimRequest, *Sims)
		want   error
	}{
		{"unknown kind", func(r *models.SimRequest, _ *Sims) { r.Kind = "stat_weights" }, ErrBadSimOptions},
		{"unknown fight style", func(r *models.SimRequest, _ *Sims) { r.Options.FightStyle = "Route" }, ErrBadSimOptions},
		{"too long", func(r *models.SimRequest, _ *Sims) { r.Options.Length = maxSimLength + 1 }, ErrBadSimOptions},
		{"no options", func(r *models.SimRequest, _ *Sims) { r.Options = models.SimOptions{} }, ErrBadSimOptions},
		{"not an export", func(r *models.SimRequest, _ *Sims) { r.Export = "hello" }, ErrBadSimC},
		{"no engine", func(_ *models.SimRequest, s *Sims) { s.Engine.(*fakeEngine).installed = false }, ErrNoEngine},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestSims(t, "ok")
			req := simRequest(t)
			tt.change(&req, s)
			if _, err := s.Start(req); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSimsRunInParallelUpToTheMaxAndCancel(t *testing.T) {
	s := newTestSims(t, "hang")
	first, err := s.Start(simRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	var others []models.Sim
	for range maxParallelSims - 1 {
		other, err := s.Start(simRequest(t))
		if err != nil {
			t.Fatalf("a sim beside the first: %v", err)
		}
		others = append(others, other)
	}
	if _, err := s.Start(simRequest(t)); !errors.Is(err, ErrSimBusy) {
		t.Errorf("one sim too many: err = %v, want ErrSimBusy", err)
	}
	if list, _ := s.List(); len(list) != maxParallelSims {
		t.Errorf("the list has %d sims, want the %d running", len(list), maxParallelSims)
	}
	if err := s.InstallEngine(""); !errors.Is(err, ErrSimBusy) {
		t.Errorf("a download during a sim: err = %v, want ErrSimBusy", err)
	}
	for _, other := range others {
		if err := s.Remove(other.ID); err != nil {
			t.Fatal(err)
		}
		waitSim(t, s, other.ID)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if sim, _ := s.Get(first.ID); sim.Progress == 50 {
			break
		}
	}
	if sim, _ := s.Get(first.ID); sim.Progress != 50 {
		t.Errorf("progress = %d, want 50 from 'Generating Baseline: 1/1 ... 50/100'", sim.Progress)
	}
	if err := s.Remove(first.ID); err != nil {
		t.Fatal(err)
	}
	if sim := waitSim(t, s, first.ID); sim.State != models.SimCancelled {
		t.Errorf("state = %q, want cancelled", sim.State)
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Errorf("a cancelled sim is not saved: %+v", list)
	}
}

func TestSimsStopWhenTheAppQuits(t *testing.T) {
	s := newTestSims(t, "hang")
	ctx, quit := context.WithCancel(context.Background())
	s.Ctx = ctx
	started, err := s.Start(simRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	quit()
	if sim := waitSim(t, s, started.ID); sim.State != models.SimCancelled {
		t.Errorf("state = %q, want cancelled", sim.State)
	}
}

func TestSimsShowWhySimcFailed(t *testing.T) {
	s := newTestSims(t, "fail")
	started, err := s.Start(simRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	sim := waitSim(t, s, started.ID)
	if sim.State != models.SimFailed || !strings.Contains(sim.Error, "Setup failure: bad thing") {
		t.Errorf("sim = %+v", sim)
	}
}

func TestSimsRefuseIDsThatAreNotTheirs(t *testing.T) {
	s := newTestSims(t, "ok")
	if err := os.WriteFile(filepath.Join(filepath.Dir(s.Dir), "secret.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../secret", "..\\secret", "", "0123456789abcdeg", "0123456789abcdef"} {
		if _, err := s.Get(id); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Get(%q) = %v, want ErrNotExist", id, err)
		}
		if err := s.Remove(id); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Remove(%q) = %v, want ErrNotExist", id, err)
		}
	}
}

func TestSimsDeleteTheOnesOlderThanKeepDays(t *testing.T) {
	s := newTestSims(t, "ok")
	for i, age := range []int{1, 10, 29, 31, 60} { // days old; some saved plain, as an earlier version did
		id := fmt.Sprintf("%016x", i+1)
		sim := models.Sim{ID: id, State: models.SimDone, StartedMS: s.Now().AddDate(0, 0, -age).UnixMilli()}
		name := id + ".json.gz"
		if i%2 == 1 {
			name = id + ".json"
		}
		if err := cachefile.WriteJSON(filepath.Join(s.Dir, name), sim); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ days, want int }{{0, 5}, {30, 3}, {7, 1}} {
		s.KeepDays = func() int { return c.days }
		s.Prune()
		if list, err := s.List(); err != nil || len(list) != c.want {
			t.Errorf("kept %d of the sims with %d days, %v, want %d", len(list), c.days, err, c.want)
		}
	}
}

func TestEngineStatusWarnsOfAnOldBuild(t *testing.T) {
	s := newTestSims(t, "ok")
	engine := s.Engine.(*fakeEngine)
	if st := s.EngineStatus(); !st.Installed || st.Stale {
		t.Errorf("a day-old build: %+v", st)
	}
	engine.built = s.Now().Add(-(engineStaleDays + 1) * 24 * time.Hour)
	if st := s.EngineStatus(); !st.Stale {
		t.Errorf("a build %d days old should be stale: %+v", engineStaleDays+1, st)
	}
	engine.installed = false
	if st := s.EngineStatus(); st.Installed || st.Stale {
		t.Errorf("no engine is not a stale one: %+v", st)
	}
}

func TestChoicesDefaultKeyLevel(t *testing.T) {
	tests := []struct {
		name   string
		levels []int
		want   int
	}{
		{"10 present", []int{2, 10, 12}, 10},
		{"10 absent", []int{2, 7, 12}, 12},
	}
	for _, tt := range tests {
		s := Sims{}
		for _, level := range tt.levels {
			s.Drops.Keys = append(s.Drops.Keys, models.KeyLevels{Level: level})
		}
		if got := s.Choices().DefaultKeyLevel; got != tt.want {
			t.Errorf("%s: DefaultKeyLevel = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestParseSimProgress(t *testing.T) {
	tests := []struct {
		line string
		want int
		ok   bool
	}{
		{"Generating Baseline: 1/1 [====>.......] 1142/4400 132.2 Mean=234199 Error=0.199% 1sec", 25, true},
		{"Generating Profileset: t50 2/76 [===================>] 1122/1122 146.2 Mean=235783", 2, true},
		{"Generating Profileset: t50 76/76 [===================>] 1122/1122 146.2 Mean=235783", 99, true},
		{"Simulating... ( iterations=1000000, threads=15 )", 0, false},
	}
	for _, tt := range tests {
		if got, ok := parseSimProgress(tt.line); got != tt.want || ok != tt.ok {
			t.Errorf("parseSimProgress(%q) = %d, %v, want %d, %v", tt.line, got, ok, tt.want, tt.ok)
		}
	}
}

func TestReadSimResult(t *testing.T) {
	result, err := readSimResult("testdata/sim-result.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.DPS < 230000 || result.DPSError <= 0 {
		t.Errorf("DPS %v ± %v", result.DPS, result.DPSError)
	}
	for _, a := range result.Abilities {
		if a.DPS <= 0 || a.Name == "" || a.ID == 0 || a.Casts > 0 && a.PerCast*a.Casts/a.DPS > 600 {
			t.Errorf("ability %+v: a row with no damage or no name should be left out, and a cast hits for what the "+
				"fight's total over its casts says", a)
		}
	}
	for i := 1; i < len(result.Buffs); i++ {
		if result.Buffs[i].Uptime > result.Buffs[i-1].Uptime || result.Buffs[i].Uptime < 1 {
			t.Errorf("buffs %+v: want longest first and at least 1%%", result.Buffs)
		}
	}
	if _, err := readSimResult("testdata/simc-export.txt", nil); err == nil {
		t.Error("a file that isn't simc's JSON should fail")
	}
}

func TestABigSimIsRacedInStages(t *testing.T) {
	s := newTestSims(t, "echo")
	s.Ref = testRef()
	var flasks, foods []string
	for i := range 12 {
		for _, kind := range []string{"flask", "food"} {
			v := fmt.Sprintf("%s_%d", kind, i)
			s.Ref.Consumables[kind] = append(s.Ref.Consumables[kind], models.SimConsumable{Value: v, Name: v})
		}
		flasks, foods = append(flasks, fmt.Sprintf("flask_%d", i)), append(foods, fmt.Sprintf("food_%d", i))
	}
	req := simRequest(t)
	req.Consumables = models.SimConsumables{"flask": flasks, "food": foods}
	started, err := s.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	sim := waitSim(t, s, started.ID)
	if sim.State != models.SimDone {
		t.Fatalf("sim = %+v", sim)
	}
	engine := s.Engine.(*fakeEngine)
	counts := make([]int, len(engine.inputs))
	for i, in := range engine.inputs {
		names := map[string]bool{}
		for _, m := range profilesetName.FindAllStringSubmatch(in, -1) {
			names[m[1]] = true
		}
		counts[i] = len(names)
	}
	// 144 combinations run at 1%, then the 25 best of them (the rest are 3 errors below the 25th) at 0.5% and last.
	if fmt.Sprint(counts) != "[144 25 25]" {
		t.Errorf("combinations in each run = %v, want [144 25 25]", counts)
	}
	if got := fmt.Sprint(sim.Result.Stages); got != "[{144 1} {25 0.5} {25 0.2}]" {
		t.Errorf("stages = %s", got)
	}
	if len(sim.Result.Options) != 25 || sim.Result.Options[0].Name != "flask_0 + food_0" {
		t.Errorf("rows = %d, first %+v, want the 25 survivors, best first", len(sim.Result.Options), sim.Result.Options[0])
	}
}

func TestASmallSimRunsOnce(t *testing.T) {
	s := newTestSims(t, "echo")
	s.Ref = testRef()
	req := simRequest(t)
	req.Consumables = models.SimConsumables{"flask": {"flask_a_2", "flask_b_2"}}
	started, err := s.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	sim := waitSim(t, s, started.ID)
	if n := len(s.Engine.(*fakeEngine).inputs); n != 1 || sim.Result.Stages != nil {
		t.Errorf("%d runs, stages %v, want one run and no stages", n, sim.Result.Stages)
	}
}
