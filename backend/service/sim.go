package service

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

const (
	// simTimeout ends a sim that runs this long (a 1000-combination Top Gear takes a quarter of an hour on a fast PC,
	// and several sims at once share it).
	simTimeout = 2 * time.Hour
	// maxParallelSims is how many sims run at once; they share the PC, so each takes longer.
	maxParallelSims = 4
	// keepEnded is how many sims that ended without a result to save (failed or cancelled) the page can still read.
	keepEnded = 10
	// engineStaleDays is how old the installed simc may be before the page warns: each patch makes an older build wrong.
	engineStaleDays = 14
	// maxSimIterations caps a sim whatever its accuracy asks for.
	maxSimIterations = 100000
	minSimLength     = 30
	maxSimLength     = 900
	maxSimTargets    = 20
	// maxSimRows is how many abilities and buffs a result keeps.
	maxSimRows = 40
	// defaultDropDifficulty and defaultDropKey are what the Droptimizer starts on (the key when the drops offer it).
	defaultDropDifficulty = "heroic"
	defaultDropKey        = 10
)

var (
	// ErrSimBusy is a sim or an engine download asked for while another runs.
	ErrSimBusy = errors.New("the engine is downloading, or as many sims as can run at once already are")
	// ErrNoEngine is a sim asked for before the engine was downloaded.
	ErrNoEngine = errors.New("download the SimulationCraft engine first")
	// ErrBadSimOptions is a sim request with a kind or options outside SimChoices.
	ErrBadSimOptions = errors.New("bad sim options")
)

// simAccuracy is an accuracy the page offers, as simc's target error in % of the DPS.
type simAccuracy struct {
	Name        string
	TargetError float64
}

var (
	// simFightStyles are simc's fight styles that need no more than the character (DungeonRoute needs pull events).
	simFightStyles = []string{"Patchwerk", "CastingPatchwerk", "LightMovement", "HeavyMovement", "HecticAddCleave",
		"DungeonSlice"}
	simAccuracies = []simAccuracy{{"quick", 0.3}, {"normal", 0.1}, {"high", 0.05}}
	simDefaults   = models.SimOptions{FightStyle: "Patchwerk", Length: 300, Targets: 1, Accuracy: "normal"}

	simIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)
	// simProgress is simc's progress line: "Generating Profileset: t50 2/76 [====>...] 1122/4300 146.2 Mean=...", the
	// stage (a baseline is 1/1, each profileset a stage) and the iterations in it.
	simProgress = regexp.MustCompile(`(\d+)/(\d+) \[[=>. ]*\] (\d+)/(\d+)`)
)

// SimEngineSource is the program sims run: running it, whether it's installed and downloading it (simc.Client).
type SimEngineSource interface {
	Run(ctx context.Context, args []string, out io.Writer) error
	Installed() models.SimEngine
	Install(ctx context.Context) (models.SimEngine, error)
	UseFile(ctx context.Context, path string) (models.SimEngine, error)
}

// ItemTypes says whether an item is a two-handed weapon, which the SimC export doesn't (wowhead.Client).
type ItemTypes interface {
	TwoHanded(ctx context.Context, id int) (bool, error)
}

// Sims runs sims on this PC, up to maxParallelSims at once, with the downloaded SimulationCraft and keeps the finished
// ones in Dir, gzipped, until they are KeepDays old.
type Sims struct {
	Engine SimEngineSource
	Items  ItemTypes
	Drops  models.SimDrops // reference/sim-drops.json, for a Droptimizer
	// Ref is reference/sim-options.json: the consumables, enchants, gems, upgrade tracks and Omnium Folio a request may
	// use.
	Ref models.SimReference
	Dir string
	// KeepDays is how many days a finished sim is kept (the owner's setting; 0, or nil, keeps them for good).
	KeepDays func() int
	Now      func() time.Time
	Ctx      context.Context // the app's: canceled on Quit, which stops a running sim and download

	work       sync.WaitGroup // the running sims and download, for Wait
	mu         sync.Mutex
	runs       map[string]*simRun    // the sims running now, by ID
	ended      map[string]models.Sim // the last keepEnded sims that ended without a result to save, for the page's poll
	installing bool
	installErr string
}

// simRun is a sim running: what the page sees of it and how to stop it.
type simRun struct {
	sim    models.Sim
	cancel context.CancelFunc
}

// Choices are the options a sim can be started with, and the ones the page starts with.
func (s *Sims) Choices() models.SimChoices {
	accuracies := make([]string, len(simAccuracies))
	for i, a := range simAccuracies {
		accuracies[i] = a.Name
	}
	keyLevels := make([]int, len(s.Drops.Keys))
	for i, k := range s.Drops.Keys {
		keyLevels[i] = k.Level
	}
	defaultKey := slices.Max(append([]int{0}, keyLevels...))
	if slices.Contains(keyLevels, defaultDropKey) {
		defaultKey = defaultDropKey
	}
	groups := make([]string, len(simSlots))
	for i, slot := range simSlots {
		groups[i] = gearKey(slot)
	}
	return models.SimChoices{FightStyles: simFightStyles, Accuracies: accuracies, MinLength: minSimLength,
		MaxLength: maxSimLength, MaxTargets: maxSimTargets, MaxCombos: maxCombos, MaxCompare: maxCompareValues,
		MaxParallel:     maxParallelSims,
		ConsumableKinds: models.SimConsumableKinds, MaxDrops: maxDrops,
		Difficulties: simDifficulties, KeyLevels: keyLevels, Defaults: simDefaults,
		DefaultDifficulty: defaultDropDifficulty, DefaultKeyLevel: defaultKey, GearGroups: slices.Compact(groups),
		MaxItemLookups: maxItemLookups}
}

// simAccuracyOf is the accuracy named name, false when there is none.
func simAccuracyOf(name string) (simAccuracy, bool) {
	i := slices.IndexFunc(simAccuracies, func(a simAccuracy) bool { return a.Name == name })
	if i < 0 {
		return simAccuracy{}, false
	}
	return simAccuracies[i], true
}

// validateSimOptions is nil for a kind and options inside Choices, else an error wrapping ErrBadSimOptions.
func validateSimOptions(kind string, o models.SimOptions) error {
	_, accurate := simAccuracyOf(o.Accuracy)
	if !slices.Contains(models.SimKinds, kind) || !slices.Contains(simFightStyles, o.FightStyle) || !accurate ||
		o.Length < minSimLength || o.Length > maxSimLength || o.Targets < 1 || o.Targets > maxSimTargets {
		return fmt.Errorf("%w: send a kind (%v), a fight style (%v), a length (%d to %d s), targets (1 to %d) and "+
			"an accuracy", ErrBadSimOptions, models.SimKinds, simFightStyles, minSimLength, maxSimLength, maxSimTargets)
	}
	return nil
}

// EngineStatus is the engine, with whether a download runs, how the last one failed, and whether the build is old.
func (s *Sims) EngineStatus() models.SimEngineStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	engine := s.Engine.Installed()
	age := s.Now().Sub(time.UnixMilli(engine.BuiltMS))
	return models.SimEngineStatus{SimEngine: engine, Installing: s.installing, Error: s.installErr,
		Stale: engine.Installed && age > engineStaleDays*24*time.Hour}
}

// InstallEngine installs the engine in the background; poll EngineStatus. With a path it takes the simc.exe there,
// else it downloads the newest release. It is ErrSimBusy while a sim or another install runs.
func (s *Sims) InstallEngine(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.runs) > 0 || s.installing {
		return ErrSimBusy
	}
	s.installing, s.installErr = true, ""
	s.work.Go(func() {
		var err error
		if path != "" {
			_, err = s.Engine.UseFile(s.Ctx, path)
		} else {
			_, err = s.Engine.Install(s.Ctx)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.installing = false
		if err != nil {
			log.Printf("installing the sim engine: %v", err)
			s.installErr = err.Error()
		}
	})
	return nil
}

// Wait returns once the running sim and download, if any, have ended: after Ctx is canceled, once simc is stopped.
func (s *Sims) Wait() { s.work.Wait() }

// Reference is what a request may pick besides its gear (reference/sim-options.json, which the app won't start
// without).
func (s *Sims) Reference() models.SimReference { return s.Ref }

// Plan is what a request would run, counted without starting it, and about how long that takes: the runs besides the
// sim's own (a Top Gear sim's gear combinations, a Droptimizer's drops, a comparison's combinations) and, for Top Gear,
// what each variant changes. A request Start would refuse says why in the plan's Error. ErrBadSimOptions for options
// outside Choices, a bad pick, or items and variants that don't fit the export.
func (s *Sims) Plan(req models.SimRequest) (models.SimPlan, error) {
	if err := validateSimOptions(req.Kind, req.Options); err != nil {
		return models.SimPlan{}, err
	}
	profile, err := ParseSimC(req.Export)
	if err != nil {
		return models.SimPlan{}, err
	}
	dims, err := s.compared(req, &profile)
	if err != nil {
		return models.SimPlan{}, err
	}
	values, err := compareCombos(dims)
	if err != nil {
		return models.SimPlan{}, err
	}
	plan := models.SimPlan{Combos: len(values)}
	switch req.Kind {
	case "topgear":
		err = s.planTopGear(req, profile, len(values), &plan)
	case "drops":
		err = s.planDrops(req, profile, dims, &plan)
	case "stats":
		if len(dims) > 0 {
			plan.Error = errStatsOneSetup.Error()
		}
	}
	if err != nil {
		return models.SimPlan{}, err
	}
	final := simTargetError(req.Options, true)
	runs := smartRuns(plan.Combos, smartStages(plan.Combos, final), final) + 1 // the sim's own run besides its sets
	if req.Kind == "stats" {
		runs = statsRuns
	}
	plan.Seconds = planSeconds(runs, req.Options)
	return plan, nil
}

// planTopGear counts a Top Gear request's gear combinations (each once for every compared value, the equipped gear
// too) into plan, with the notes of its variants and what Start would refuse it for.
func (s *Sims) planTopGear(req models.SimRequest, p models.SimProfile, values int, plan *models.SimPlan) error {
	extra, err := variantCandidates(s.Ref, p, req.Variants)
	if err != nil {
		return err
	}
	groups, err := gearGroups(p, req.Equipped, req.Bags, extra, s.twoHanded)
	if err != nil {
		return err
	}
	plan.Combos = comboCount(groups, maxPlanCombos)
	if values > 0 {
		plan.Combos = (plan.Combos + 1) * values
	}
	if err := comboRefusal(plan.Combos); err != nil {
		plan.Error = err.Error()
	}
	plan.Notes = make([]string, len(extra))
	for i, c := range extra {
		plan.Notes[i] = c.item.Note
	}
	return nil
}

// planDrops counts a Droptimizer request's drops (each once for every compared value) into plan, with what Start
// would refuse it for.
func (s *Sims) planDrops(req models.SimRequest, p models.SimProfile, dims []compareDim, plan *models.SimPlan) error {
	up, err := s.dropUpgrade(req)
	var drops []gearCombo
	if err == nil {
		drops, err = dropCombos(p, s.Drops, req.Difficulty, req.KeyLevel, up)
	}
	if err == nil {
		drops, err = withCompare(dims, false, drops, maxDrops)
	}
	if errors.Is(err, ErrBadSimOptions) {
		plan.Error, err = err.Error(), nil
	}
	plan.Combos = len(drops)
	return err
}

// dropUpgrade is how a Droptimizer request upgrades its drops (SimRequest.Upgrade). ErrBadSimOptions for a value that
// is none of those.
func (s *Sims) dropUpgrade(req models.SimRequest) (dropUpgrade, error) {
	switch req.Upgrade {
	case "":
		return dropUpgrade{}, nil
	case "max":
		return dropUpgrade{Tracks: s.Ref.Upgrades}, nil
	case "match":
		return dropUpgrade{Tracks: s.Ref.Upgrades, Match: true}, nil
	}
	rank, err := strconv.Atoi(req.Upgrade)
	if err != nil || rank < 1 {
		return dropUpgrade{}, fmt.Errorf("%w: upgrade is \"max\", \"match\" or a rank from 1", ErrBadSimOptions)
	}
	return dropUpgrade{Tracks: s.Ref.Upgrades, Rank: rank}, nil
}

// twoHanded asks Items whether the item is a two-handed weapon.
func (s *Sims) twoHanded(id int) (bool, error) { return s.Items.TwoHanded(s.Ctx, id) }

// compared are the things a request compares, the consumables with several values picked and then the Omnium Folio's
// runes, and sets p's own Omnium picks to the ones the sim itself uses. ErrBadSimOptions for a bad pick.
func (s *Sims) compared(req models.SimRequest, p *models.SimProfile) ([]compareDim, error) {
	dims, err := consumableDims(s.Ref, req.Consumables)
	if err != nil {
		return nil, err
	}
	var omnium compareDim
	if p.Omnium, omnium, err = omniumInput(s.Ref, *p, req.Omnium); err != nil {
		return nil, err
	}
	if omnium != nil {
		dims = append(dims, omnium)
	}
	return dims, nil
}

// Start parses the export and starts the sim in the background; poll Get. Errors: ErrBadSimOptions, ErrBadSimC,
// ErrNoEngine, ErrSimBusy.
func (s *Sims) Start(req models.SimRequest) (models.Sim, error) {
	if err := validateSimOptions(req.Kind, req.Options); err != nil {
		return models.Sim{}, err
	}
	profile, err := ParseSimC(req.Export)
	if err != nil {
		return models.Sim{}, err
	}
	consumables, err := consumableLines(s.Ref, req.Consumables)
	if err != nil {
		return models.Sim{}, err
	}
	dims, err := s.compared(req, &profile)
	if err != nil {
		return models.Sim{}, err
	}
	var combos []gearCombo
	limit := maxCombos
	switch req.Kind {
	case "topgear":
		var extra []gearCandidate
		if extra, err = variantCandidates(s.Ref, profile, req.Variants); err == nil {
			combos, err = topGearCombos(profile, req.Equipped, req.Bags, extra, len(dims) > 0, s.twoHanded)
			if len(dims) > 0 { // the equipped gear takes each compared value too
				combos = append([]gearCombo{currentGear()}, combos...)
			}
		}
	case "drops":
		limit = maxDrops
		var up dropUpgrade
		if up, err = s.dropUpgrade(req); err == nil {
			combos, err = dropCombos(profile, s.Drops, req.Difficulty, req.KeyLevel, up)
		}
	}
	if err == nil && req.Kind == "stats" && len(dims) > 0 {
		err = errStatsOneSetup
	}
	if err == nil {
		combos, err = withCompare(dims, req.Kind == "quick", combos, limit)
	}
	if err != nil {
		return models.Sim{}, err
	}
	if !s.Engine.Installed().Installed {
		return models.Sim{}, ErrNoEngine
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.runs) >= maxParallelSims || s.installing {
		return models.Sim{}, ErrSimBusy
	}
	id, err := newSimID()
	if err != nil {
		return models.Sim{}, err
	}
	work := filepath.Join(s.Dir, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return models.Sim{}, fmt.Errorf("making %s: %w", work, err)
	}
	input, output := filepath.Join(work, id+".simc"), filepath.Join(work, id+".json")
	head := SimInput(profile) + consumables + scaleLine(req.Kind)
	ctx, cancel := context.WithTimeout(s.Ctx, simTimeout)
	sim := models.Sim{ID: id, Kind: req.Kind, State: models.SimRunning, Character: profile.Name, Spec: profile.Spec,
		Options: req.Options, StartedMS: s.Now().UnixMilli()}
	if s.runs == nil {
		s.runs = map[string]*simRun{}
	}
	s.runs[id] = &simRun{sim: sim, cancel: cancel}
	s.work.Go(func() { s.execute(ctx, id, input, output, head, combos, profile.Equipped) })
	return sim, nil
}

// simArgs are simc's arguments: the input file, the options, the target error (in % of the DPS) to stop each sim at and
// the JSON file to write (always last). Nothing but those validated values goes in, and no shell reads them. A sim with
// profilesets runs several at once, each on profilesetThreads threads (fewer on a small PC, so at least two run).
func simArgs(input, output string, o models.SimOptions, targetError float64, profilesets bool) []string {
	args := []string{input, "fight_style=" + o.FightStyle, "max_time=" + strconv.Itoa(o.Length),
		"desired_targets=" + strconv.Itoa(o.Targets),
		"target_error=" + strconv.FormatFloat(targetError, 'f', -1, 64),
		"iterations=" + strconv.Itoa(maxSimIterations), "threads=" + strconv.Itoa(simThreads())}
	if profilesets {
		args = append(args, "profileset_work_threads="+strconv.Itoa(min(profilesetThreads, max(1, simThreads()/2))))
	}
	return append(args, "json2="+output)
}

// simTargetError is the target error a sim with o runs at: its accuracy's, no finer than setTargetError with
// profilesets.
func simTargetError(o models.SimOptions, profilesets bool) float64 {
	accuracy, _ := simAccuracyOf(o.Accuracy)
	if profilesets {
		return max(accuracy.TargetError, setTargetError)
	}
	return accuracy.TargetError
}

// simThreads is how many threads a sim runs on: all but one, so the PC stays usable.
func simThreads() int { return max(1, runtime.NumCPU()-1) }

// execute runs simc for the running sim with this ID, following its progress, and saves what it found. A sim of many
// combinations is raced: simc first runs all of them at a coarse error, then only those the results cannot rule out of
// the best (survivors) at a finer one, down to the precision asked for.
func (s *Sims) execute(ctx context.Context, id, input, output, head string, combos []gearCombo,
	equipped []models.SimItem) {
	defer func() {
		os.Remove(input)
		os.Remove(output)
	}()
	s.mu.Lock()
	sim := s.runs[id].sim
	s.mu.Unlock()
	final := simTargetError(sim.Options, len(combos) > 0)
	early := smartStages(len(combos), final)
	var stages []models.SimStage
	var complaint string
	var runErr error
	from, cands := 0, combos
	for i, targetError := range early {
		s.setStage(id, stageText(i+1, len(early)+1, len(cands), targetError))
		to := from + smartShares[i]
		complaint, runErr = s.stage(ctx, id, input, output, head, cands, sim.Options, targetError, from, to)
		if runErr != nil {
			break
		}
		var sets []simSetJSON
		if sets, runErr = readSets(output); runErr != nil {
			break
		}
		stages = append(stages, models.SimStage{Combos: len(cands), Error: targetError})
		cands, from = survivors(cands, sets), to
	}
	var result *models.SimResult
	var parseErr error
	if runErr == nil {
		if len(early) > 0 {
			s.setStage(id, stageText(len(early)+1, len(early)+1, len(cands), final))
			stages = append(stages, models.SimStage{Combos: len(cands), Error: final})
		}
		if complaint, runErr = s.stage(ctx, id, input, output, head, cands, sim.Options, final, from, 100); runErr == nil {
			result, parseErr = readSimResult(output, cands)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sim = s.runs[id].sim
	sim.TookMS, sim.Stage = s.Now().UnixMilli()-sim.StartedMS, ""
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		sim.State = models.SimCancelled
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		sim.State, sim.Error = models.SimFailed, "the sim ran over "+simTimeout.String()+" and was stopped"
	case runErr != nil || parseErr != nil:
		sim.State, sim.Error = models.SimFailed, cmp.Or(complaint, fmt.Sprint(cmp.Or(runErr, parseErr)))
	default:
		for i := range equipped {
			result.Equipped = append(result.Equipped, *slimItem(&equipped[i]))
		}
		result.Stages = stages // none for a sim run once
		if len(result.Weights) > 0 {
			result.Pawn = pawnString(sim.Character+"-"+sim.Spec, result.Weights)
		}
		sim.State, sim.Progress, sim.Result = models.SimDone, 100, result
		if err := cachefile.WriteJSON(filepath.Join(s.Dir, sim.ID+".json.gz"), sim); err != nil {
			log.Printf("saving sim %s: %v", sim.ID, err)
			sim.State, sim.Error, sim.Result = models.SimFailed, "the result couldn't be saved", nil
		} else {
			s.prune()
		}
	}
	s.runs[id].cancel()
	delete(s.runs, id)
	if sim.State != models.SimDone {
		s.keepEnded(sim)
	}
}

// stage writes the input for cands and runs simc on it, stopping each sim at targetError; its progress takes from to
// to of the running sim's bar. It returns the last error line simc printed.
func (s *Sims) stage(ctx context.Context, id, input, output, head string, cands []gearCombo, o models.SimOptions,
	targetError float64, from, to int) (string, error) {
	if err := os.WriteFile(input, []byte(head+gearProfilesets(cands)), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", input, err)
	}
	pr, pw := io.Pipe()
	said := make(chan string)
	go func() { said <- s.follow(pr, id, from, to) }()
	err := s.Engine.Run(ctx, simArgs(input, output, o, targetError, len(cands) > 0), pw)
	pw.Close()
	return <-said, err
}

// setStage says which stage of a raced sim the running sim with this ID is in.
func (s *Sims) setStage(id, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run := s.runs[id]; run != nil {
		run.sim.Stage = text
	}
}

// keepEnded remembers a sim that ended without a result to save, so the page can read why, and forgets the oldest ones
// past keepEnded. The caller holds s.mu.
func (s *Sims) keepEnded(sim models.Sim) {
	if s.ended == nil {
		s.ended = map[string]models.Sim{}
	}
	s.ended[sim.ID] = sim
	for len(s.ended) > keepEnded {
		oldest := ""
		for id, e := range s.ended {
			if oldest == "" || e.StartedMS < s.ended[oldest].StartedMS {
				oldest = id
			}
		}
		delete(s.ended, oldest)
	}
}

// follow reads simc's output, updating the progress of the running sim with this ID (simc's 0 to 100 spread over from
// to to of the bar), and returns the last error line it printed.
func (s *Sims) follow(r io.Reader, id string, from, to int) string {
	var complaint string
	scan := bufio.NewScanner(r)
	scan.Split(splitProgress)
	for scan.Scan() {
		line := scan.Text()
		if p, ok := parseSimProgress(line); ok {
			s.mu.Lock()
			if run := s.runs[id]; run != nil {
				run.sim.Progress = from + p*(to-from)/100
			}
			s.mu.Unlock()
		} else if strings.HasPrefix(line, "Error") {
			complaint = line[:min(len(line), 300)]
		}
	}
	_, _ = io.Copy(io.Discard, r) // a line over the scanner's limit mustn't leave simc blocked on a full pipe
	return complaint
}

// splitProgress splits at a carriage return as well as a newline, since simc redraws its progress line with one.
func splitProgress(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// parseSimProgress is how far along line says the sim is, 0 to 99, false for any other line.
func parseSimProgress(line string) (int, bool) {
	m := simProgress.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	stage, stages, done, total := atoi(m[1]), atoi(m[2]), atoi(m[3]), atoi(m[4])
	if stages == 0 || total == 0 {
		return 0, false
	}
	p := (float64(stage-1) + min(1, float64(done)/float64(total))) / float64(stages) * 100
	return min(99, max(0, int(p))), true
}

// simJSON is the part of simc's json2 file a result reads: the first player's DPS, abilities and buffs, and the
// profilesets' DPS.
type simJSON struct {
	Sim struct {
		Profilesets struct {
			Results []simSetJSON `json:"results"`
		} `json:"profilesets"`
		Players []struct {
			Collected struct {
				DPS struct {
					Mean       float64 `json:"mean"`
					MeanStdDev float64 `json:"mean_std_dev"`
				} `json:"dps"`
			} `json:"collected_data"`
			ScaleFactors map[string]float64 `json:"scale_factors"`
			Stats        []struct {
				ID        int    `json:"id"`
				SpellName string `json:"spell_name"`
				Name      string `json:"name"`
				School    string `json:"school"`
				Executes  struct {
					Mean float64 `json:"mean"`
				} `json:"num_executes"`
				Actual struct {
					Mean float64 `json:"mean"`
				} `json:"actual_amount"`
				PortionDPS struct {
					Mean float64 `json:"mean"`
				} `json:"portion_aps"`
				Portion float64 `json:"portion_amount"`
			} `json:"stats"`
			Buffs []struct {
				SpellName string  `json:"spell_name"`
				Name      string  `json:"name"`
				Spell     int     `json:"spell"`
				Uptime    float64 `json:"uptime"`
			} `json:"buffs"`
		} `json:"players"`
	} `json:"sim"`
}

// readSimResult is the result in simc's json2 file at path, with the DPS of each of a Top Gear sim's combos.
func readSimResult(path string, combos []gearCombo) (*models.SimResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("simc wrote no result: %w", err)
	}
	var out simJSON
	if err := json.Unmarshal(data, &out); err != nil || len(out.Sim.Players) == 0 {
		return nil, fmt.Errorf("reading simc's result: %w", cmp.Or(err, errors.New("no player in it")))
	}
	player := out.Sim.Players[0]
	result := &models.SimResult{Weights: statWeights(player.ScaleFactors), DPS: player.Collected.DPS.Mean,
		DPSError:  player.Collected.DPS.MeanStdDev,
		Abilities: []models.SimAbility{}, Buffs: []models.SimBuff{}}
	for _, st := range player.Stats {
		if st.PortionDPS.Mean > 0 {
			ability := models.SimAbility{Name: cmp.Or(st.SpellName, st.Name), ID: st.ID, School: st.School,
				DPS: st.PortionDPS.Mean, Share: st.Portion, Casts: st.Executes.Mean}
			if st.Executes.Mean > 0 {
				ability.PerCast = st.Actual.Mean / st.Executes.Mean // actual_amount is a fight's total
			}
			result.Abilities = append(result.Abilities, ability)
		}
	}
	for _, b := range player.Buffs {
		if b.Uptime >= 1 {
			result.Buffs = append(result.Buffs, models.SimBuff{Name: cmp.Or(b.SpellName, b.Name), ID: b.Spell, Uptime: b.Uptime})
		}
	}
	slices.SortStableFunc(result.Abilities, func(a, b models.SimAbility) int { return cmp.Compare(b.DPS, a.DPS) })
	slices.SortStableFunc(result.Buffs, func(a, b models.SimBuff) int { return cmp.Compare(b.Uptime, a.Uptime) })
	result.Abilities = result.Abilities[:min(len(result.Abilities), maxSimRows)]
	result.Buffs = result.Buffs[:min(len(result.Buffs), maxSimRows)]
	switch {
	case len(combos) == 0:
	case combos[0].option != nil:
		result.Options = optionResults(out.Sim.Profilesets.Results, result.DPS, combos)
	default:
		result.Combos = gearResults(out.Sim.Profilesets.Results, result.DPS, combos)
	}
	return result, nil
}

// Get is the sim with this ID: a running one with its progress, one of the last that failed or were cancelled, or a
// saved one; fs.ErrNotExist for any other.
func (s *Sims) Get(id string) (models.Sim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run := s.runs[id]; run != nil {
		return run.sim, nil
	}
	if sim, ok := s.ended[id]; ok {
		return sim, nil
	}
	return s.read(id)
}

// read is the saved sim with this ID.
func (s *Sims) read(id string) (models.Sim, error) {
	if !simIDPattern.MatchString(id) {
		return models.Sim{}, fs.ErrNotExist
	}
	for _, path := range s.paths(id) {
		sim, err := cachefile.ReadJSON[models.Sim](path)
		if !errors.Is(err, fs.ErrNotExist) {
			return sim, err
		}
	}
	return models.Sim{}, fmt.Errorf("reading sim %s: %w", id, fs.ErrNotExist)
}

// paths are the files a saved sim can be in: gzipped, or plain as an earlier version saved it.
func (s *Sims) paths(id string) [2]string {
	return [2]string{filepath.Join(s.Dir, id+".json.gz"), filepath.Join(s.Dir, id+".json")}
}

// removeSaved deletes a saved sim's file, whether it was found.
func (s *Sims) removeSaved(id string) (bool, error) {
	found := false
	for _, path := range s.paths(id) {
		err := os.Remove(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return found, err
		}
		found = found || err == nil
	}
	return found, nil
}

// List is the running sims, then the saved ones, newest first, without their results.
func (s *Sims) List() ([]models.Sim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, err := s.saved()
	if err != nil {
		return nil, err
	}
	list := []models.Sim{}
	for _, run := range s.runs {
		list = append(list, run.sim)
	}
	slices.SortFunc(list, func(a, b models.Sim) int { return cmp.Compare(b.StartedMS, a.StartedMS) })
	for _, sim := range saved {
		if sim.Result != nil {
			sim.DPS, sim.Result = sim.Result.DPS, nil
		}
		list = append(list, sim)
	}
	return list, nil
}

// saved is the sims in Dir, newest first.
func (s *Sims) saved() ([]models.Sim, error) {
	sims, err := cachefile.ReadAll[models.Sim](s.Dir)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(sims, func(a, b models.Sim) int { return cmp.Compare(b.StartedMS, a.StartedMS) })
	return sims, nil
}

// Prune deletes the saved sims older than KeepDays.
func (s *Sims) Prune() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
}

// prune is Prune for a caller that holds s.mu.
func (s *Sims) prune() {
	if s.KeepDays == nil || s.KeepDays() <= 0 {
		return
	}
	sims, err := s.saved()
	if err != nil {
		log.Printf("pruning sims: %v", err)
		return
	}
	cutoff := s.Now().Add(-time.Duration(s.KeepDays()) * 24 * time.Hour).UnixMilli()
	for _, sim := range sims {
		if sim.StartedMS < cutoff {
			if _, err := s.removeSaved(sim.ID); err != nil {
				log.Printf("deleting the old sim %s: %v", sim.ID, err)
			}
		}
	}
}

// Remove stops the running sim with this ID, forgets the failed or cancelled one, or deletes the saved one;
// fs.ErrNotExist for any other.
func (s *Sims) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run := s.runs[id]; run != nil {
		run.cancel()
		return nil
	}
	if _, ok := s.ended[id]; ok {
		delete(s.ended, id)
		return nil
	}
	if !simIDPattern.MatchString(id) {
		return fs.ErrNotExist
	}
	found, err := s.removeSaved(id)
	if err == nil && !found {
		err = fs.ErrNotExist
	}
	if err != nil {
		return fmt.Errorf("deleting sim %s: %w", id, err)
	}
	return nil
}

// newSimID is 16 random hex digits.
func newSimID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("making a sim ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}
