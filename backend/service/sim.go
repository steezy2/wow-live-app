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
	"os/exec"
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
	// keepSims is how many finished sims are kept; the oldest go.
	keepSims = 20
	// simTimeout ends a sim that runs this long (a 300-drop Droptimizer takes minutes on a slow PC).
	simTimeout = 30 * time.Minute
	// engineStaleDays is how old the installed simc may be before the page warns: each patch makes an older build wrong.
	engineStaleDays = 14
	// maxSimIterations caps a sim whatever its accuracy asks for.
	maxSimIterations = 100000
	minSimLength     = 30
	maxSimLength     = 900
	maxSimTargets    = 20
	// maxSimRows is how many abilities and buffs a result keeps.
	maxSimRows = 40
)

var (
	// ErrSimBusy is a sim or an engine download asked for while another runs.
	ErrSimBusy = errors.New("a sim or the engine download is already running")
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

	defaultDropDifficulty = "heroic"
	defaultDropKey        = 10

	simIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)
	// simProgress is simc's progress line: "Generating Profileset: t50 2/76 [====>...] 1122/4300 146.2 Mean=...", the
	// stage (a baseline is 1/1, each profileset a stage) and the iterations in it.
	simProgress = regexp.MustCompile(`(\d+)/(\d+) \[[=>. ]*\] (\d+)/(\d+)`)
)

// SimEngineSource is the program sims run: where it is, whether it's installed and downloading it (simc.Client).
type SimEngineSource interface {
	Path() string
	Installed() models.SimEngine
	Install(ctx context.Context) (models.SimEngine, error)
}

// Sims runs sims on this PC one at a time with the downloaded SimulationCraft and keeps the last keepSims in Dir.
type Sims struct {
	Engine SimEngineSource
	Drops  models.SimDrops // reference/sim-drops.json, for a Droptimizer
	Dir    string
	Now    func() time.Time
	Ctx    context.Context // the app's: canceled on Quit, which stops a running sim and download; nil never cancels

	mu         sync.Mutex
	run        *simRun     // the sim running now
	last       *models.Sim // the last sim that ended without a result to save (failed or cancelled), for the page's poll
	installing bool
	installErr string
}

// simRun is the sim running: what the page sees of it, how to stop it, and a Top Gear sim's combinations.
type simRun struct {
	sim    models.Sim
	cancel context.CancelFunc
	combos []gearCombo
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
	return models.SimChoices{FightStyles: simFightStyles, Accuracies: accuracies, MinLength: minSimLength,
		MaxLength: maxSimLength, MaxTargets: maxSimTargets, MaxCombos: maxCombos, MaxDrops: maxDrops,
		Difficulties: simDifficulties, KeyLevels: keyLevels, Defaults: simDefaults,
		DefaultDifficulty: defaultDropDifficulty, DefaultKeyLevel: defaultKey}
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
		return fmt.Errorf("%w: send a kind (%v), a fight style (%v), a length (%d to %d s), targets (1 to %d) and an accuracy",
			ErrBadSimOptions, models.SimKinds, simFightStyles, minSimLength, maxSimLength, maxSimTargets)
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

// InstallEngine downloads the engine in the background; poll EngineStatus. It is ErrSimBusy while a sim or another
// download runs.
func (s *Sims) InstallEngine() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != nil || s.installing {
		return ErrSimBusy
	}
	s.installing, s.installErr = true, ""
	go func() {
		_, err := s.Engine.Install(s.base())
		s.mu.Lock()
		defer s.mu.Unlock()
		s.installing = false
		if err != nil {
			log.Printf("downloading the sim engine: %v", err)
			s.installErr = err.Error()
		}
	}()
	return nil
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
	var combos []gearCombo
	switch req.Kind {
	case "topgear":
		combos, err = topGearCombos(profile, req.Equipped, req.Bags)
	case "drops":
		combos, err = dropCombos(profile, s.Drops, req.Difficulty, req.KeyLevel)
	}
	if err != nil {
		return models.Sim{}, err
	}
	if !s.Engine.Installed().Installed {
		return models.Sim{}, ErrNoEngine
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != nil || s.installing {
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
	if err := os.WriteFile(input, []byte(SimInput(profile)+gearProfilesets(combos)), 0o644); err != nil {
		return models.Sim{}, fmt.Errorf("writing %s: %w", input, err)
	}
	ctx, cancel := context.WithTimeout(s.base(), simTimeout)
	sim := models.Sim{ID: id, Kind: req.Kind, State: models.SimRunning, Character: profile.Name, Spec: profile.Spec,
		Options: req.Options, StartedMS: s.Now().UnixMilli()}
	s.run, s.last = &simRun{sim: sim, cancel: cancel, combos: combos}, nil
	go s.execute(ctx, input, output)
	return sim, nil
}

// simArgs are simc's arguments: the input file, the options and the JSON file to write. Nothing but those validated
// values goes in, and no shell reads them. A sim with profilesets runs no finer than setTargetError.
func simArgs(input, output string, o models.SimOptions, profilesets bool) []string {
	accuracy, _ := simAccuracyOf(o.Accuracy)
	targetError := accuracy.TargetError
	if profilesets {
		targetError = max(targetError, setTargetError)
	}
	return []string{input, "fight_style=" + o.FightStyle, "max_time=" + strconv.Itoa(o.Length),
		"desired_targets=" + strconv.Itoa(o.Targets), "target_error=" + strconv.FormatFloat(targetError, 'f', -1, 64),
		"iterations=" + strconv.Itoa(maxSimIterations), "threads=" + strconv.Itoa(max(1, runtime.NumCPU()-1)),
		"json2=" + output}
}

// execute runs simc for the running sim, following its progress, and saves what it found.
func (s *Sims) execute(ctx context.Context, input, output string) {
	defer func() {
		os.Remove(input)
		os.Remove(output)
	}()
	s.mu.Lock()
	sim, combos := s.run.sim, s.run.combos
	s.mu.Unlock()
	cmd := exec.CommandContext(ctx, s.Engine.Path(), simArgs(input, output, sim.Options, len(combos) > 0)...)
	cmd.Dir = filepath.Dir(input)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	said := make(chan string)
	go func() { said <- s.follow(pr) }()
	runErr := cmd.Run()
	pw.Close()
	complaint := <-said

	var result *models.SimResult
	var parseErr error
	if runErr == nil {
		result, parseErr = readSimResult(output, combos)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sim = s.run.sim
	sim.TookMS = s.Now().UnixMilli() - sim.StartedMS
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		sim.State = models.SimCancelled
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		sim.State, sim.Error = models.SimFailed, "the sim ran over "+simTimeout.String()+" and was stopped"
	case runErr != nil || parseErr != nil:
		sim.State, sim.Error = models.SimFailed, cmp.Or(complaint, fmt.Sprint(cmp.Or(runErr, parseErr)))
	default:
		sim.State, sim.Progress, sim.Result = models.SimDone, 100, result
		if err := cachefile.WriteJSON(filepath.Join(s.Dir, sim.ID+".json"), sim); err != nil {
			log.Printf("saving sim %s: %v", sim.ID, err)
			sim.State, sim.Error, sim.Result = models.SimFailed, "the result couldn't be saved", nil
		} else {
			s.prune()
		}
	}
	s.run.cancel()
	s.run, s.last = nil, nil
	if sim.State != models.SimDone {
		s.last = &sim
	}
}

// base is the context sims and downloads run under.
func (s *Sims) base() context.Context { return cmp.Or(s.Ctx, context.Background()) }

// follow reads simc's output, updating the running sim's progress, and returns the last error line it printed.
func (s *Sims) follow(r io.Reader) string {
	var complaint string
	scan := bufio.NewScanner(r)
	scan.Split(splitProgress)
	for scan.Scan() {
		line := scan.Text()
		if p, ok := parseSimProgress(line); ok {
			s.mu.Lock()
			if s.run != nil {
				s.run.sim.Progress = p
			}
			s.mu.Unlock()
		} else if strings.HasPrefix(line, "Error") {
			complaint = line[:min(len(line), 300)]
		}
	}
	io.Copy(io.Discard, r) // a line over the scanner's limit mustn't leave simc blocked on a full pipe
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
			Stats []struct {
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
	result := &models.SimResult{DPS: player.Collected.DPS.Mean, DPSError: player.Collected.DPS.MeanStdDev,
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
	if len(combos) > 0 {
		result.Combos = gearResults(out.Sim.Profilesets.Results, result.DPS, combos)
	}
	return result, nil
}

// Get is the sim with this ID: the running one with its progress, the last one that failed or was cancelled, or a
// saved one; fs.ErrNotExist for any other.
func (s *Sims) Get(id string) (models.Sim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.run != nil && s.run.sim.ID == id:
		return s.run.sim, nil
	case s.last != nil && s.last.ID == id:
		return *s.last, nil
	}
	return s.read(id)
}

// read is the saved sim with this ID.
func (s *Sims) read(id string) (models.Sim, error) {
	if !simIDPattern.MatchString(id) {
		return models.Sim{}, fs.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, id+".json"))
	if err != nil {
		return models.Sim{}, fmt.Errorf("reading sim %s: %w", id, err)
	}
	var sim models.Sim
	if err := json.Unmarshal(data, &sim); err != nil {
		return models.Sim{}, fmt.Errorf("reading sim %s: %w", id, err)
	}
	return sim, nil
}

// List is the running sim, then the saved ones, newest first, without their results.
func (s *Sims) List() ([]models.Sim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, err := s.saved()
	if err != nil {
		return nil, err
	}
	list := []models.Sim{}
	if s.run != nil {
		list = append(list, s.run.sim)
	}
	for _, sim := range saved {
		sim.Result = nil
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

// prune deletes the saved sims past keepSims, oldest first. The caller holds s.mu.
func (s *Sims) prune() {
	sims, err := s.saved()
	if err != nil {
		log.Printf("pruning sims: %v", err)
		return
	}
	for _, sim := range sims[min(len(sims), keepSims):] {
		os.Remove(filepath.Join(s.Dir, sim.ID+".json"))
	}
}

// Remove stops the running sim with this ID, or deletes the saved one; fs.ErrNotExist for any other.
func (s *Sims) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != nil && s.run.sim.ID == id {
		s.run.cancel()
		return nil
	}
	if s.last != nil && s.last.ID == id {
		s.last = nil
		return nil
	}
	if !simIDPattern.MatchString(id) {
		return fs.ErrNotExist
	}
	if err := os.Remove(filepath.Join(s.Dir, id+".json")); err != nil {
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
