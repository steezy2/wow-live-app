package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wow-live/backend/models"
)

// TestMain lets the test binary stand in for simc.exe: Sims runs os.Args[0], and WOWLIVE_FAKE_SIMC says how it acts.
func TestMain(m *testing.M) {
	if mode := os.Getenv("WOWLIVE_FAKE_SIMC"); mode != "" {
		fakeSimc(mode)
		return
	}
	os.Exit(m.Run())
}

func fakeSimc(mode string) {
	var output string
	for _, arg := range os.Args[1:] {
		if v, ok := strings.CutPrefix(arg, "json2="); ok {
			output = v
		}
	}
	if path := os.Getenv("WOWLIVE_FAKE_ARGS"); path != "" {
		os.WriteFile(path, []byte(strings.Join(os.Args[1:], "\n")), 0o644)
		input, _ := os.ReadFile(os.Args[1])
		os.WriteFile(path+".simc", input, 0o644)
	}
	fmt.Print("Generating Baseline: 1/1 [====>...] 50/100 1.0 Mean=1 Error=1%\r")
	switch mode {
	case "fail":
		fmt.Println("Error: Setup failure: bad thing")
		os.Exit(1)
	case "hang":
		time.Sleep(time.Minute)
	default:
		data, err := os.ReadFile(os.Getenv("WOWLIVE_FAKE_RESULT"))
		if err == nil {
			err = os.WriteFile(output, data, 0o644)
		}
		if err != nil {
			os.Exit(2)
		}
	}
}

type fakeEngine struct {
	installed bool
	built     time.Time
}

func (f *fakeEngine) Path() string { return os.Args[0] }
func (f *fakeEngine) Installed() models.SimEngine {
	return models.SimEngine{Installed: f.installed, BuiltMS: f.built.UnixMilli()}
}
func (f *fakeEngine) Install(context.Context) (models.SimEngine, error) { return f.Installed(), nil }

func newTestSims(t *testing.T, mode string) *Sims {
	t.Helper()
	result, err := filepath.Abs("testdata/sim-result.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WOWLIVE_FAKE_SIMC", mode)
	t.Setenv("WOWLIVE_FAKE_RESULT", result)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	return &Sims{Engine: &fakeEngine{installed: true, built: now.Add(-24 * time.Hour)}, Dir: t.TempDir(),
		Now: func() time.Time { return now }}
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
	args := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("WOWLIVE_FAKE_ARGS", args)
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
		len(sim.Result.Abilities) == 0 || sim.Result.Abilities[0].DPS < sim.Result.Abilities[len(sim.Result.Abilities)-1].DPS {
		t.Errorf("sim = %+v", sim)
	}
	if got, err := os.ReadFile(args); err != nil || !strings.Contains(string(got), "fight_style=HecticAddCleave\nmax_time=120\n"+
		"desired_targets=3\ntarget_error=0.05\n") || strings.Contains(string(got), "html=") {
		t.Errorf("simc got %q, %v", got, err)
	}
	// a new Sims on the same folder, as after a restart, still has it, and the list leaves its result out
	again := &Sims{Engine: s.Engine, Dir: s.Dir, Now: s.Now}
	if saved, err := again.Get(started.ID); err != nil || saved.Result == nil || saved.Result.DPS != sim.Result.DPS {
		t.Errorf("saved = %+v, %v", saved, err)
	}
	if list, err := again.List(); err != nil || len(list) != 1 || list[0].Result != nil {
		t.Errorf("list = %+v, %v", list, err)
	}
	if left, _ := filepath.Glob(filepath.Join(s.Dir, "work", "*")); len(left) != 0 {
		t.Errorf("the work folder still holds %v", left)
	}
}

func TestSimsStartRefuses(t *testing.T) {
	tests := []struct {
		name   string
		change func(*models.SimRequest, *Sims)
		want   error
	}{
		{"unknown kind", func(r *models.SimRequest, _ *Sims) { r.Kind = "stat_weights" }, ErrBadSimOptions},
		{"unknown fight style", func(r *models.SimRequest, _ *Sims) { r.Options.FightStyle = "DungeonRoute" }, ErrBadSimOptions},
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

func TestSimsRunOneAtATimeAndCancel(t *testing.T) {
	s := newTestSims(t, "hang")
	first, err := s.Start(simRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(simRequest(t)); !errors.Is(err, ErrSimBusy) {
		t.Errorf("a second sim: err = %v, want ErrSimBusy", err)
	}
	if err := s.InstallEngine(); !errors.Is(err, ErrSimBusy) {
		t.Errorf("a download during a sim: err = %v, want ErrSimBusy", err)
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

func TestSimsRemoveDeletesAFinishedSim(t *testing.T) {
	s := newTestSims(t, "ok")
	started, err := s.Start(simRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	waitSim(t, s, started.ID)
	if err := s.Remove(started.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.List(); len(list) != 0 {
		t.Errorf("a removed sim is still listed: %+v", list)
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
	if sim := waitSim(t, s, started.ID); sim.State != models.SimFailed || !strings.Contains(sim.Error, "Setup failure: bad thing") {
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

func TestSimsKeepTheLastTwenty(t *testing.T) {
	s := newTestSims(t, "ok")
	for i := range keepSims + 3 {
		id := fmt.Sprintf("%016x", i+1)
		data := fmt.Sprintf(`{"id":%q,"startedMS":%d,"state":"done"}`, id, i+1)
		if err := os.WriteFile(filepath.Join(s.Dir, id+".json"), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s.prune()
	list, err := s.List()
	if err != nil || len(list) != keepSims || list[0].StartedMS != keepSims+3 || list[keepSims-1].StartedMS != 4 {
		t.Errorf("kept %d, newest %d, oldest %d, %v: want the newest %d", len(list), list[0].StartedMS,
			list[len(list)-1].StartedMS, err, keepSims)
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
			t.Errorf("ability %+v: a row with no damage or no name should be left out, and a cast hits for what the fight's total over its casts says", a)
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
