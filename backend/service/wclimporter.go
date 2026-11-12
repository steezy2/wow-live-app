package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"wow-live/backend/models"
)

const (
	// importReservePoints are the hour's points an import leaves for key reviews: under them it waits for the hour.
	importReservePoints = 200
	// importRetryWait is how long an import waits when Warcraft Logs says the points ran out but not when they come
	// back.
	importRetryWait = 5 * time.Minute
	// importTries is how many times a fight is tried before the import marks it failed and goes on.
	importTries = 2
	// importStateFile is the import's state in its folder, which a restart resumes from.
	importStateFile = "state.json"
)

// The states an import is in (models.ImportStatus.State).
const (
	importIdle    = "idle"
	importListing = "listing"
	importRunning = "running"
	importWaiting = "waiting"
	importDone    = "done"
	importFailed  = "failed"
)

// ReportSource is Warcraft Logs as an import reads it (wcl.Client).
type ReportSource interface {
	CurrentZones(ctx context.Context, raidBosses []string) (models.WCLZones, error)
	RecentReports(ctx context.Context, region, slug, name string, page int) ([]models.CharacterReport, bool, error)
	RateLimit(ctx context.Context) (models.RateLimit, error)
}

// Importer fetches a character's Warcraft Logs fights of this season one at a time in the background, writes each
// as a combat log (Fetch) and saves its key or raid pull (Read), waiting for the hour's points when they run low.
// Its state is kept in Dir, so an app restart resumes it (Resume).
type Importer struct {
	Source     ReportSource
	Characters Characters
	// Fetch writes a fight as a zipped combat log and returns its path; its error wraps models.ErrRateLimited when
	// the points ran out (fetched pages are kept, so the next try goes on from there).
	Fetch func(ctx context.Context, ref models.FightRef, character string) (string, error)
	// Read saves the key or pull in a fetched fight's log, tagged with its report.
	Read func(path string, link models.ReportLink) error
	// Saved are the saved keys and raid pulls, so fights already saved or read from a local log are skipped.
	Saved func() ([]models.Key, []models.RaidPull, error)
	// Prepare runs before the reports are listed: it reads new local logs, so their keys win over imported copies.
	Prepare func() error
	// Count is how many saved keys and pulls are imported, and RemoveImported deletes them.
	Count          func() int
	RemoveImported func() error
	Bosses         []string // the raid's boss names, to find its Warcraft Logs zone
	Dir            string   // the import's folder (<cache>\wcl-import)
	Now            func() time.Time
	// Wait sleeps for d or until ctx ends (replaceable in tests).
	Wait func(ctx context.Context, d time.Duration) error

	mu     sync.Mutex
	status models.ImportStatus
	cancel context.CancelFunc
	done   chan struct{}
}

// importState is the import's state.json: the character, the fights to import and how far it got.
type importState struct {
	Region      string              `json:"region"`
	Character   string              `json:"character"`
	Fights      []models.FightRef   `json:"fights"`
	Done        []models.ReportLink `json:"done"`
	Failed      []models.ReportLink `json:"failed"`
	PointsSpent float64             `json:"pointsSpent"`
	Keys        int                 `json:"keys"`
	Pulls       int                 `json:"pulls"`
}

// Start checks character and imports its fights of this season in the background, answering at once. It is
// ErrBusy while an import runs; a bad character wraps ErrBadCharacter, missing keys models.ErrNoCredentials.
func (im *Importer) Start(ctx context.Context, region, character string) error {
	name, slug, err := im.Characters.Resolve(ctx, region, character)
	if err != nil {
		return err
	}
	old, _ := im.readState()
	state := importState{Region: region, Character: character}
	if old.Character == character { // a fight that saved nothing isn't fetched again
		state.Done = old.Done
	}
	return im.run(state, func(ctx context.Context, st *importState) error { return im.list(ctx, st, name, slug) })
}

// Resume goes on with an import a restart stopped, if one has fights left.
func (im *Importer) Resume() error {
	state, err := im.readState()
	if err != nil || state.Character == "" || len(state.left()) == 0 {
		return err
	}
	return im.run(state, nil)
}

// Stop ends the running import, if any, and waits for it; fetched fights and pages stay for the next Start.
func (im *Importer) Stop() {
	im.mu.Lock()
	cancel, done := im.cancel, im.done
	im.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

// Remove stops the import and deletes what it fetched and every imported key and raid pull.
func (im *Importer) Remove() error {
	im.Stop()
	if err := im.RemoveImported(); err != nil {
		return err
	}
	if err := os.RemoveAll(im.Dir); err != nil {
		return fmt.Errorf("removing the imported fights: %w", err)
	}
	im.mu.Lock()
	defer im.mu.Unlock()
	im.status = models.ImportStatus{State: importIdle}
	return nil
}

// Status is how the import stands.
func (im *Importer) Status() models.ImportStatus {
	im.mu.Lock()
	status := im.status
	im.mu.Unlock()
	if status.State == "" {
		status.State = importIdle
	}
	if done := status.FightsDone + status.FightsFailed; done > 0 {
		status.PointsPerFight = status.PointsSpent / float64(done)
	}
	status.Imported = im.Count()
	return status
}

// run starts the import of state in the background: first (when set) fills in its fights, then each is fetched.
func (im *Importer) run(state importState, first func(context.Context, *importState) error) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.cancel != nil {
		return ErrBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	im.cancel, im.done = cancel, make(chan struct{})
	im.status = models.ImportStatus{State: importRunning, Character: state.Character}
	im.show(&state)
	go func() {
		defer func() {
			im.mu.Lock()
			defer im.mu.Unlock()
			cancel()
			close(im.done)
			im.cancel = nil
		}()
		err := error(nil)
		if first != nil {
			err = first(ctx, &state)
		}
		if err == nil {
			err = im.fetchAll(ctx, &state)
		}
		im.mu.Lock()
		defer im.mu.Unlock()
		switch {
		case ctx.Err() != nil:
			im.status.State, im.status.ResumeAtMS = importIdle, 0
		case err != nil:
			log.Printf("importing from Warcraft Logs: %v", err)
			im.status.State, im.status.Problem = importFailed, err.Error()
		default:
			im.status.State = importDone
		}
	}()
	return nil
}

// list fills st's fights from the character's reports, back to the first page with none of this season's.
func (im *Importer) list(ctx context.Context, st *importState, name, slug string) error {
	im.setState(importListing, 0)
	if err := im.Prepare(); err != nil && !errors.Is(err, ErrBusy) {
		return err
	}
	zones, err := im.Source.CurrentZones(ctx, im.Bosses)
	if err != nil {
		return err
	}
	var reports []models.CharacterReport
	for page := 1; ; page++ {
		got, more, err := im.Source.RecentReports(ctx, st.Region, slug, name, page)
		if err != nil {
			return err
		}
		reports = append(reports, got...)
		if !more || !slices.ContainsFunc(got, func(r models.CharacterReport) bool {
			return r.ZoneID == zones.MPlusID || r.ZoneID == zones.RaidID
		}) {
			break
		}
	}
	keys, pulls, err := im.Saved()
	if err != nil {
		return err
	}
	st.Fights = PickFights(reports, zones, st.Character, keys, pulls)
	im.mu.Lock()
	im.show(st)
	im.mu.Unlock()
	return im.writeState(*st)
}

// fetchAll fetches st's fights left, oldest first, saving the state after each.
func (im *Importer) fetchAll(ctx context.Context, st *importState) error {
	var budget models.RateLimit // unknown at first, then the one read after each fetch
	var err error
	for _, ref := range st.left() {
		var fetchErr error
		for try := 1; try <= importTries; {
			if budget.LimitPerHour-budget.PointsSpentThisHour < importReservePoints {
				if budget, err = im.points(ctx); err != nil {
					return err
				}
			}
			path, err := im.Fetch(ctx, ref, st.Character)
			if err == nil {
				err = im.Read(path, ref.ReportLink)
			}
			if after, rateErr := im.Source.RateLimit(ctx); rateErr == nil {
				st.PointsSpent += spent(budget, after)
				budget = after
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, models.ErrRateLimited) { // the fetched pages are kept for the next try
				im.setState(importWaiting, im.now().Add(importRetryWait).UnixMilli())
				if err := im.Wait(ctx, importRetryWait); err != nil {
					return err
				}
				continue
			}
			if fetchErr = err; err == nil {
				break
			}
			try++
		}
		if fetchErr != nil {
			log.Printf("importing fight %d of report %s: %v", ref.FightID, ref.Code, fetchErr)
			st.Failed = append(st.Failed, ref.ReportLink)
		} else {
			st.Done = append(st.Done, ref.ReportLink)
			if ref.Kind == models.FightMPlus {
				st.Keys++
			} else {
				st.Pulls++
			}
		}
		im.mu.Lock()
		im.show(st)
		if fetchErr != nil {
			im.status.Problem = fetchErr.Error()
		}
		im.mu.Unlock()
		if err := im.writeState(*st); err != nil {
			return err
		}
	}
	return nil
}

// points is the hour's budget once at least importReservePoints are left, waiting for the hour to reset until then,
// and a while when Warcraft Logs can't say (down, or the points ran out); only keys it turns down stop the import.
func (im *Importer) points(ctx context.Context) (models.RateLimit, error) {
	for {
		budget, err := im.Source.RateLimit(ctx)
		wait := importRetryWait
		switch {
		case err == nil && budget.LimitPerHour-budget.PointsSpentThisHour >= importReservePoints:
			im.setState(importRunning, 0)
			return budget, nil
		case err == nil:
			wait = time.Duration(budget.PointsResetIn)*time.Second + time.Second
		case errors.Is(err, models.ErrNoCredentials) || errors.Is(err, models.ErrBadCredentials):
			return budget, err
		default:
			log.Printf("importing from Warcraft Logs: waiting %s: %v", wait, err)
		}
		im.setState(importWaiting, im.now().Add(wait).UnixMilli())
		if err := im.Wait(ctx, wait); err != nil {
			return budget, err
		}
	}
}

// spent is the points used between two budgets of the hour; after a reset, all the new hour's.
func spent(before, after models.RateLimit) float64 {
	if after.PointsSpentThisHour >= before.PointsSpentThisHour {
		return after.PointsSpentThisHour - before.PointsSpentThisHour
	}
	return after.PointsSpentThisHour
}

// left are st's fights neither done nor failed.
func (st importState) left() []models.FightRef {
	return slices.DeleteFunc(slices.Clone(st.Fights), func(f models.FightRef) bool {
		return slices.Contains(st.Done, f.ReportLink) || slices.Contains(st.Failed, f.ReportLink)
	})
}

// show copies st's counts into the status; im.mu is held.
func (im *Importer) show(st *importState) {
	im.status.Character, im.status.Fights = st.Character, len(st.Fights)
	im.status.FightsDone, im.status.FightsFailed = len(st.Done), len(st.Failed)
	im.status.KeysSaved, im.status.PullsSaved, im.status.PointsSpent = st.Keys, st.Pulls, st.PointsSpent
}

func (im *Importer) setState(state string, resumeAt int64) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.status.State, im.status.ResumeAtMS = state, resumeAt
}

func (im *Importer) now() time.Time {
	if im.Now != nil {
		return im.Now()
	}
	return time.Now()
}

func (im *Importer) readState() (importState, error) {
	var st importState
	data, err := os.ReadFile(filepath.Join(im.Dir, importStateFile))
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err == nil {
		err = json.Unmarshal(data, &st)
	}
	if err != nil {
		return st, fmt.Errorf("reading the import's state: %w", err)
	}
	return st, nil
}

// writeState saves st beside as .tmp, then over the old state, so a crash never leaves half a file.
func (im *Importer) writeState(st importState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("saving the import's state: %w", err)
	}
	path := filepath.Join(im.Dir, importStateFile)
	if err = os.MkdirAll(im.Dir, 0o755); err == nil {
		err = os.WriteFile(path+".tmp", data, 0o644)
	}
	if err == nil {
		err = os.Rename(path+".tmp", path)
	}
	if err != nil {
		return fmt.Errorf("saving the import's state: %w", err)
	}
	return nil
}

// WaitFor sleeps for d or until ctx ends, whose error it returns then (Importer.Wait outside tests).
func WaitFor(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
