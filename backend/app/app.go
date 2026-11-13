// Package app starts the live M+ and raid site the same way for cmd/site (run from the repo) and the desktop app
// (installed): the log tracker with its saved keys and raid pulls, key reviews, map tiles, change notices, the page's
// router, and the follower that reads the game's log into them. It's wiring, not rules.
package app

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/clients/combatlog"
	"wow-live/backend/clients/keystoneguru"
	"wow-live/backend/clients/keystore"
	"wow-live/backend/clients/raiderio"
	"wow-live/backend/clients/wcl"
	"wow-live/backend/clients/wowhead"
	"wow-live/backend/controller"
	"wow-live/backend/models"
	"wow-live/backend/service"
)

// pushEvery is the most often the page is told the keys changed, so a burst of lines is one notice.
const pushEvery = 250 * time.Millisecond

// The folders, in the cache, of the saved keys and the saved raid pulls.
const (
	localKeysDir = "local-keys"
	raidPullsDir = "raid-pulls"
)

// spellTimeout bounds one spell lookup on Wowhead, so a hover never waits on a site that is down.
const spellTimeout = 10 * time.Second

// Options say where the site reads and writes.
type Options struct {
	Cache     string                      // the cache: logs/cache/ in the repo
	DelayLog  string                      // logs/live-delay.log
	LogsDir   func() string               // the game's combat log folder now (the owner can change it)
	Region    string                      // for this week's affix names
	Rules     models.MPlusRules           // from mplus.toml
	Dungeons  map[int]models.DungeonFacts // from reference/mplus-dungeons.json
	RaidRules models.RaidRules            // from raid-rules.toml
	Bosses    map[string]models.RaidBoss  // from reference/raid-bosses.json
	Tier      map[string][]int            // each class's tier items, from reference/raid-bosses.json
	Baselines models.MPlusBaselines       // Warcraft Logs; nil means no key reviews
	Pages     fs.FS                       // the built page
	Hosts     []string                    // the names the page reaches the site by, for the router's host check
	Settings  *controller.SettingsAPI     // the desktop app's settings and keys; nil in cmd/site
	Realms    service.RealmSource         // Warcraft Logs' realms, to check the settings' character
	WCL       FightImporter               // Warcraft Logs, to import a character's fights; nil: no import
	Now       func() time.Time            // the clock keys are read at
}

// FightImporter is Warcraft Logs as an import reads it (wcl.Client).
type FightImporter interface {
	service.ReportSource
	service.ParseSource
	ImportFight(ctx context.Context, code string, fightID int, opts wcl.FightOptions) (string, error)
}

// Site is a started site: its keys and change notices, and the page and API.
type Site struct {
	Keys    *service.LogTracker
	Changes *service.Changes
	Handler http.Handler
	Import  *service.Importer // nil without Settings and Warcraft Logs
	opts    Options
	reviews controller.Reviews
	topRuns func(ctx context.Context, key models.Key) ([]int64, error) // nil without Warcraft Logs
	newKeys func() (*service.LogTracker, error)                        // another tracker on the same saved keys, for Learn
	logJob  sync.Mutex                                                 // one read, archive or delete of logs at a time
}

// Start builds the site from opts. Follow, Learn or a replay then feed it the game's log.
func Start(ctx context.Context, opts Options) (*Site, error) {
	timers, affixes := dungeonData(ctx, opts.Cache, opts.Region)
	store := keystore.Store{Dir: filepath.Join(opts.Cache, localKeysDir)}
	pulls := keystore.Pulls{Dir: filepath.Join(opts.Cache, raidPullsDir)}
	newKeys := func() (*service.LogTracker, error) {
		keys, err := service.NewLogTracker(timers, affixes, opts.Rules, opts.Dungeons, store)
		if err != nil {
			return nil, err
		}
		if err := keys.EnableRaid(opts.RaidRules, opts.Bosses, opts.Tier, pulls); err != nil {
			return nil, err
		}
		return keys, nil
	}
	keys, err := newKeys()
	if err != nil {
		return nil, err
	}
	s := &Site{Keys: keys, Changes: &service.Changes{Every: pushEvery}, opts: opts, newKeys: newKeys}
	if opts.Settings != nil {
		characters := service.Characters{Realms: opts.Realms, Keys: store, Logs: opts.WCL,
			Bosses: slices.Sorted(maps.Keys(opts.Bosses))}
		opts.Settings.Logs, opts.Settings.Characters = s, characters
		if opts.WCL != nil {
			s.Import = s.importer(characters, store, pulls)
			opts.Settings.Import = s.Import
		}
	}
	if opts.Baselines != nil {
		reviewer := service.Reviewer{Keys: keys, Baselines: opts.Baselines, Store: store}
		s.reviews, s.topRuns = reviewer, reviewer.TopRuns
	}
	tiles := service.MapTiles{Facts: opts.Dungeons,
		Source: keystoneguru.Client{CacheDir: filepath.Join(opts.Cache, "keystone-guru")}}
	raidMaps := service.RaidMaps{Bosses: opts.Bosses,
		Source: wcl.MapImages{CacheDir: filepath.Join(opts.Cache, "raid-maps")}}
	recaps := service.Recaps{Saved: keys.History, Region: opts.Region, Now: opts.Now, Logs: opts.Baselines,
		Season: raiderio.Client{CacheDir: filepath.Join(opts.Cache, "raiderio")}}
	spells := &service.Spells{Keys: keys, Source: wowhead.Client{CacheDir: filepath.Join(opts.Cache, "wowhead"),
		HTTP: &http.Client{Timeout: spellTimeout}}}
	s.Handler = controller.Router(controller.MPlus{Keys: keys, Reviews: s.reviews, Changes: s.Changes,
		Now: opts.Now, Tiles: tiles, Recaps: recaps,
		Spells: spells}, &controller.Raid{Pulls: keys, Maps: raidMaps, Now: opts.Now}, opts.Settings,
		opts.Pages, opts.Hosts...)
	return s, nil
}

// importer is the import of a character's Warcraft Logs fights into the saved keys and pulls.
func (s *Site) importer(characters service.Characters, store keystore.Store, pulls keystore.Pulls) *service.Importer {
	return &service.Importer{Source: s.opts.WCL, Characters: characters,
		Fetch: func(ctx context.Context, ref models.FightRef, character string) (string, error) {
			return s.opts.WCL.ImportFight(ctx, ref.Code, ref.FightID,
				wcl.FightOptions{Logger: character, Dungeons: s.opts.Dungeons})
		},
		Read: s.readImported,
		Saved: func() ([]models.Key, []models.RaidPull, error) {
			keys, err := store.List()
			if err != nil {
				return nil, nil, err
			}
			saved, err := pulls.ListPulls()
			return keys, saved, err
		},
		Link: func(keyLinks, pullLinks map[int64]models.ReportLink) (int, error) {
			s.logJob.Lock() // a log read now would save these keys again from its own copy, without the link
			defer s.logJob.Unlock()
			n, err := s.Keys.LinkReports(keyLinks, pullLinks)
			if n > 0 {
				s.Changes.Publish(s.Keys.Version())
			}
			return n, err
		},
		Prepare: func() error { _, _, err := s.Learn(false); return err },
		Count:   s.Keys.ImportedCount,
		RemoveImported: func() error {
			if err := s.Keys.RemoveImported(); err != nil {
				return err
			}
			s.Changes.Publish(s.Keys.Version())
			return nil
		},
		Bosses: characters.Bosses,
		Dir:    filepath.Join(s.opts.Cache, wcl.ImportDir),
		Now:    s.opts.Now,
		Wait:   service.WaitFor,
	}
}

// readImported saves the key or raid pull in an imported fight's log, tagged with its report, for the lists.
func (s *Site) readImported(path string, link models.ReportLink) error {
	s.logJob.Lock()
	defer s.logJob.Unlock()
	keys, err := s.newKeys()
	if err != nil {
		return err
	}
	if err := s.readZip(keys, path, link); err != nil {
		return err
	}
	if err := s.Keys.Reload(); err != nil {
		return err
	}
	s.Changes.Publish(s.Keys.Version())
	return nil
}

// readZip reads an imported fight's log into keys, a tracker apart from the local logs' so no local key is open
// around it; each zip holds a whole fight, so one tracker reads them all.
func (s *Site) readZip(keys *service.LogTracker, path string, link models.ReportLink) error {
	keys.SetSource(&link)
	return s.readInto(keys, path, combatlog.EachZippedLine)
}

// dungeonData is the season's dungeon timers and this week's affix names from Raider.IO; without them the page
// still works, with no timer or affix names.
func dungeonData(ctx context.Context, cache, region string) (map[int]int64, map[int]string) {
	rio := raiderio.Client{CacheDir: filepath.Join(cache, "raiderio")}
	timers, err := rio.Timers(ctx)
	if err != nil {
		log.Printf("no dungeon timers: %v", err)
	}
	affixes, err := rio.Affixes(ctx, region)
	if err != nil {
		log.Printf("no affix names: %v", err)
	}
	return timers, affixes
}

// add feeds parsed lines to keys, skipping a line that doesn't parse.
func add(keys *service.LogTracker, lines []string) {
	for _, line := range combatlog.Parse(lines) {
		keys.Add(line)
	}
}

// readerVersion is the version of what Learn saves from a log. Bump it with any change that saves new key or raid pull
// fields, so the next Learn reads every log again.
const readerVersion = 16

// learned is learned.json: the logs Learn has read and the reader that read them, so it reads only new or changed
// logs next time.
type learned struct {
	ReaderVersion int          `json:"readerVersion"`
	Logs          []learnedLog `json:"logs"`
}

// learnedLog is a log as Learn read it.
type learnedLog struct {
	File    string `json:"file"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"` // Unix ms
}

// Learn reads the logs in the game's folder, oldest first, into a tracker of its own (so it's safe while Follow
// runs), saves the keys and raid pulls that ended in them, and has the site's keys read the saved ones again, for the
// list and the estimate. A log already read by this readerVersion, unchanged since, is skipped unless all is set; so
// are archived logs, which are read only then. It returns how many logs it read and how many keys are saved, or
// service.ErrBusy while another read, archive or delete runs.
func (s *Site) Learn(all bool) (read, saved int, err error) {
	if !s.logJob.TryLock() {
		return 0, 0, service.ErrBusy
	}
	defer s.logJob.Unlock()
	return s.learn(all)
}

// learn is Learn, with the log job's lock held.
func (s *Site) learn(all bool) (read, saved int, err error) {
	dir := s.opts.LogsDir()
	logs, err := combatlog.WrittenSince(dir, time.Time{})
	if err != nil {
		return 0, 0, err
	}
	keys, err := s.newKeys()
	if err != nil {
		return 0, 0, err
	}
	path := filepath.Join(s.opts.Cache, "learned.json")
	var before learned
	if !all {
		before = readLearned(path)
	}
	type source struct { // a log in the folder (entry set) or an archived one, read in the order they were written
		path    string
		modTime int64
		each    func(string, func(models.LogLine)) error
		entry   *learnedLog
	}
	var sources []source
	for _, logPath := range logs {
		entry, err := logEntry(logPath)
		if err != nil {
			return 0, 0, err
		}
		sources = append(sources, source{logPath, entry.ModTime, combatlog.EachLineIn, &entry})
	}
	if before.ReaderVersion != readerVersion { // a new reader (or all): the archived logs too
		zips, err := combatlog.Archived(dir)
		if err != nil {
			return 0, 0, err
		}
		for _, z := range zips {
			sources = append(sources, source{z.Path, z.Written.UnixMilli(), combatlog.EachZippedLine, nil})
		}
		slices.SortStableFunc(sources, func(a, b source) int { return cmp.Compare(a.modTime, b.modTime) })
	}
	now := learned{ReaderVersion: readerVersion}
	for _, src := range sources {
		if src.entry == nil || !before.has(*src.entry) {
			if err := s.readInto(keys, src.path, src.each); err != nil {
				return read, 0, err
			}
			read++
		}
		if src.entry != nil {
			now.Logs = append(now.Logs, *src.entry)
			if err := cachefile.WriteJSON(path, now); err != nil { // after each, so a stopped read goes on from there
				return read, 0, err
			}
		}
	}
	if before.ReaderVersion != readerVersion { // a new reader: the imported fights again too
		n, err := s.readImportedZips()
		if err != nil {
			return read, 0, err
		}
		read += n
	}
	if err := cachefile.WriteJSON(path, now); err != nil {
		return read, 0, err
	}
	if err := s.Keys.Reload(); err != nil {
		return read, 0, err
	}
	s.Changes.Publish(s.Keys.Version())
	saved = len(s.Keys.Saved(""))
	log.Printf("%d logs read, %d keys saved", read, saved)
	return read, saved, nil
}

// readImportedZips reads every imported fight's log again, returning how many.
func (s *Site) readImportedZips() (int, error) {
	zips, err := filepath.Glob(filepath.Join(s.opts.Cache, wcl.ImportDir, "*.zip"))
	if err != nil {
		return 0, err
	}
	keys, err := s.newKeys()
	if err != nil {
		return 0, err
	}
	read := 0
	for _, path := range zips {
		if link, ok := wcl.ImportedFight(path); ok {
			if err := s.readZip(keys, path, link); err != nil {
				return read, err
			}
			read++
		}
	}
	return read, nil
}

// readInto reads one log (each reads it, a log or a zip) into keys and saves what ended in it.
func (s *Site) readInto(keys *service.LogTracker, path string, each func(string, func(models.LogLine)) error) error {
	if err := each(path, keys.Add); err != nil {
		return err
	}
	if _, err := keys.SaveFinished(); err != nil {
		return err
	}
	log.Printf("read %s", filepath.Base(path))
	return nil
}

// has is whether l's reader is this one and it read f, unchanged since.
func (l learned) has(f learnedLog) bool {
	return l.ReaderVersion == readerVersion && slices.Contains(l.Logs, f)
}

// logEntry is the log at path as learned.json keeps it.
func logEntry(path string) (learnedLog, error) {
	info, err := os.Stat(path)
	if err != nil {
		return learnedLog{}, fmt.Errorf("reading %s: %w", path, err)
	}
	return learnedLog{File: info.Name(), Size: info.Size(), ModTime: info.ModTime().UnixMilli()}, nil
}

// readLearned is the learned.json at path; a missing, unreadable or odd one is nothing learned, so every log is read.
func readLearned(path string) learned {
	var l learned
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &l)
	}
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("reading every log again: %s: %v", path, err)
		}
		return learned{}
	}
	return l
}

// Follow reads the newest log in dir into the keys as the game writes it until ctx ends, closing a key the log went
// silent on, saving each key once it ends, logging how far behind the game it was read, and reviewing it (when there
// are Warcraft Logs keys and points to spare); a dungeon's first run gets its top Warcraft Logs runs for the estimate.
// Once caught up with the log, and after each key or raid pull that ends, it runs the auto rule (Tidy) in the
// background. A failed read is logged and tried again.
func (s *Site) Follow(ctx context.Context, dir string) {
	watcher, err := combatlog.Watch(dir, combatlog.FallbackEvery)
	if err != nil {
		log.Printf("reading every %s instead: %v", combatlog.FallbackEvery, err)
	}
	defer watcher.Close()
	tail := &combatlog.Tail{Dir: dir}
	delays := &combatlog.Delays{File: s.opts.DelayLog}
	caughtUp := false     // keys that ended before the site started (read again on start) aren't reviewed on their own
	tidiedAt := int64(-1) // the LastEnd the auto rule last ran after
	for {
		lines, _, err := tail.Read()
		if err != nil {
			log.Print(err)
		}
		delays.Add(lines, time.Now())
		add(s.Keys, lines)
		if len(lines) == 0 { // not while working through a backlog, whose lines are old
			s.Keys.CloseQuiet(time.Now().UnixMilli())
		}
		s.Changes.Publish(s.Keys.Version())
		saved, err := s.Keys.SaveFinished()
		if err != nil {
			log.Print(err)
		}
		for _, start := range saved {
			if !caughtUp {
				continue
			}
			if err := delays.Log("key started " + time.UnixMilli(start).Format("15:04")); err != nil {
				log.Print(err)
			}
			if s.reviews != nil {
				go func() {
					if _, err := s.reviews.Review(ctx, start); err != nil {
						log.Printf("reviewing the key that just ended: %v", err)
					}
				}()
			}
		}
		caughtUp = caughtUp || len(lines) == 0
		if end := s.Keys.LastEnd(); caughtUp && end != tidiedAt {
			tidiedAt = end
			go func() {
				if err := s.Tidy(); err != nil {
					log.Printf("auto rule: %v", err)
				}
			}()
		}
		if caughtUp && s.topRuns != nil { // FirstRun marks the run asked, so not while reading the log from the start
			if key, ok := s.Keys.FirstRun(); ok {
				go s.fetchTopRuns(ctx, key)
			}
		}
		if !watcher.Wait(ctx) {
			return
		}
	}
}

// fetchTopRuns gives the keys a first run's top Warcraft Logs runs, for its estimate.
func (s *Site) fetchTopRuns(ctx context.Context, key models.Key) {
	ms, err := s.topRuns(ctx, key)
	if err != nil {
		log.Printf("no top runs for the first %s: %v", key.Dungeon, err)
		return
	}
	s.Keys.SetTopRuns(key.MapID, key.Level, ms)
	s.Changes.Publish(s.Keys.Version())
}
