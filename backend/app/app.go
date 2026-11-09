// Package app starts the live M+ and raid site the same way for cmd/site (run from the repo) and the desktop app
// (installed): the key tracker with its saved keys and raid pulls, key reviews, map tiles, change notices, the page's router, and the
// follower that reads the game's log into them. It's wiring, not rules.
package app

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"wow-live/backend/clients/combatlog"
	"wow-live/backend/clients/keystoneguru"
	"wow-live/backend/clients/keystore"
	"wow-live/backend/clients/raiderio"
	"wow-live/backend/controller"
	"wow-live/backend/models"
	"wow-live/backend/service"
)

// pushEvery is the most often the page is told the keys changed, so a burst of lines is one notice.
const pushEvery = 250 * time.Millisecond

// Options say where the site reads and writes.
type Options struct {
	Cache     string                      // the cache: logs/cache/ in the repo
	DelayLog  string                      // logs/live-delay.log
	LogsDir   string                      // the game's combat log folder
	Region    string                      // for this week's affix names
	Rules     models.MPlusRules           // from mplus.toml
	Dungeons  map[int]models.DungeonFacts // from reference/mplus-dungeons.json
	RaidRules models.RaidRules            // from raid-rules.toml
	Bosses    map[string]models.RaidBoss  // from reference/raid-bosses.json
	Baselines models.MPlusBaselines       // Warcraft Logs; nil means no key reviews
	Pages     fs.FS                       // the built page
	Hosts     []string                    // the names the page reaches the site by, for the router's host check
	Settings  *controller.SettingsAPI     // the desktop app's settings and keys; nil in cmd/site
	Now       func() time.Time            // the clock keys are read at
}

// Site is a started site: its keys and change notices, and the page and API.
type Site struct {
	Keys    *service.KeyTracker
	Changes *service.Changes
	Handler http.Handler
	opts    Options
	reviews controller.Reviews
	topRuns func(ctx context.Context, key models.Key) ([]int64, error) // nil without Warcraft Logs
	newKeys func() (*service.KeyTracker, error)                        // another tracker on the same saved keys, for Learn
}

// Start builds the site from opts. Follow, Learn or a replay then feed it the game's log.
func Start(ctx context.Context, opts Options) (*Site, error) {
	timers, affixes := dungeonData(ctx, opts.Cache, opts.Region)
	store := keystore.Store{Dir: filepath.Join(opts.Cache, "local-keys")}
	pulls := keystore.Pulls{Dir: filepath.Join(opts.Cache, "raid-pulls")}
	newKeys := func() (*service.KeyTracker, error) {
		keys, err := service.NewKeyTracker(timers, affixes, opts.Rules, opts.Dungeons, store)
		if err != nil {
			return nil, err
		}
		return keys, keys.EnableRaid(opts.RaidRules, opts.Bosses, pulls)
	}
	keys, err := newKeys()
	if err != nil {
		return nil, err
	}
	s := &Site{Keys: keys, Changes: &service.Changes{Every: pushEvery}, opts: opts, newKeys: newKeys}
	if opts.Baselines != nil {
		reviewer := service.Reviewer{Keys: keys, Baselines: opts.Baselines, Store: store}
		s.reviews, s.topRuns = reviewer, reviewer.TopRuns
	}
	tiles := service.MapTiles{Facts: opts.Dungeons,
		Source: keystoneguru.Client{CacheDir: filepath.Join(opts.Cache, "keystone-guru")}}
	s.Handler = controller.Router(controller.MPlus{Keys: keys, Reviews: s.reviews, Changes: s.Changes,
		Now: opts.Now, Tiles: tiles}, &controller.Raid{Pulls: keys, Now: opts.Now}, opts.Settings, opts.Pages,
		opts.Hosts...)
	return s, nil
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

// Add feeds parsed lines to the keys and tells the pages if the keys changed.
func (s *Site) Add(lines ...string) {
	add(s.Keys, lines)
	s.Changes.Publish(s.Keys.Version())
}

// add feeds parsed lines to keys, skipping a line that doesn't parse.
func add(keys *service.KeyTracker, lines []string) {
	for _, text := range lines {
		if line, err := combatlog.ParseLine(text); err == nil {
			keys.Add(line)
		}
	}
}

// Learn reads every log in the game's folder, oldest first, into a tracker of its own (so it's safe while Follow
// runs), saves the keys and raid pulls that ended in them, and has the site's keys read the saved ones again, for the list and the
// estimate. It returns how many keys are saved.
func (s *Site) Learn() (int, error) {
	logs, err := combatlog.WrittenSince(s.opts.LogsDir, time.Time{})
	if err != nil {
		return 0, err
	}
	keys, err := s.newKeys()
	if err != nil {
		return 0, err
	}
	for _, path := range logs {
		if err := combatlog.EachLine(path, keys.Add); err != nil {
			return 0, err
		}
		if _, err := keys.SaveFinished(); err != nil {
			return 0, err
		}
		log.Printf("read %s", filepath.Base(path))
	}
	if err := s.Keys.Reload(); err != nil {
		return 0, err
	}
	s.Changes.Publish(s.Keys.Version())
	saved := len(s.Keys.Saved())
	log.Printf("%d keys saved", saved)
	return saved, nil
}

// Follow reads the newest log in dir into the keys as the game writes it until ctx ends, saving each key once it ends,
// logging how far behind the game it was read, and reviewing it (when there are Warcraft Logs keys and points to
// spare); a dungeon's first run gets its top Warcraft Logs runs for the estimate. A failed read is logged and tried
// again.
func (s *Site) Follow(ctx context.Context, dir string) {
	watcher, err := combatlog.Watch(dir, combatlog.FallbackEvery)
	if err != nil {
		log.Printf("reading every %s instead: %v", combatlog.FallbackEvery, err)
	}
	defer watcher.Close()
	tail := &combatlog.Tail{Dir: dir}
	delays := &combatlog.Delays{File: s.opts.DelayLog}
	caughtUp := false // keys that ended before the site started (read again on start) aren't reviewed on their own
	for {
		lines, _, err := tail.Read()
		if err != nil {
			log.Print(err)
		}
		delays.Add(lines, time.Now())
		s.Add(lines...)
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
