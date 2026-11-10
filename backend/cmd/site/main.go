// Command site serves the site on this PC: the live Mythic+ page and its API. It follows
// the game's combat log, so the page shows the running key as the game writes it: in batches, as fights start and end.
//
// Usage, from backend/ (build the page first: cd frontend && npm run build):
//
//	go run ./cmd/site                              follow the newest log, page at http://localhost:8080
//	go run ./cmd/site --replay PATH --key 2 --speed 20   replay a past log's 2nd key, 20x faster
//	go run ./cmd/site --learn                      save every finished key in the game's log folder, then exit
//
// Finished keys are saved in logs/cache/local-keys/: the page lists them, and their boss times feed the estimate.
//
//	@title		WoW Live API
//	@version	1
//	@description	The site's API: the live Mythic+ key read from the combat log.
//	@BasePath	/
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"wow-live/backend/app"
	"wow-live/backend/clients/combatlog"
	"wow-live/backend/clients/wcl"
	"wow-live/backend/config"
	"wow-live/backend/controller"
	_ "wow-live/backend/docs"
)

func main() {
	port := flag.Int("port", 8080, "port on this PC")
	replay := flag.String("replay", "", "replay this log instead of following the newest one")
	key := flag.Int("key", 1, "with --replay: which key of the log to replay (1 is the first)")
	speed := flag.Float64("speed", 1, "with --replay: how many times faster than it happened")
	learn := flag.Bool("learn", false, "save every finished key in the game's log folder, then exit")
	region := flag.String("region", "us", "the region for affix names (us, eu, kr, tw)")
	flag.Parse()
	if err := run(*port, *replay, *key, *speed, *learn, *region); err != nil {
		log.Fatal(err)
	}
}

func run(port int, replay string, key int, speed float64, learn bool, region string) error {
	root, err := config.RootFromWorkingDir()
	if err != nil {
		return err
	}
	if err := config.LoadEnv(filepath.Join(root, ".env")); err != nil {
		return err
	}
	rules, err := config.LoadRules(root)
	if err != nil {
		return err
	}
	ref, err := config.LoadReference(config.ReferencePath(root))
	if err != nil {
		log.Printf("no dungeon facts, so no avoidable damage or forces: %v", err)
	}
	raidRules, err := config.LoadRaidRules(root)
	if err != nil {
		return err
	}
	raid, err := config.LoadRaidReference(config.RaidReferencePath(root))
	if err != nil {
		log.Printf("no raid boss mechanics, so raid pulls show deaths and meters only: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	clock := &replayClock{speed: speed}
	pages := filepath.Join(root, "frontend", "dist")
	if _, err := os.Stat(filepath.Join(pages, "index.html")); err != nil {
		log.Printf("no built page in %s: run npm run build in frontend/ (the API still works)", pages)
	}
	opts := app.Options{Cache: config.CachePath(root), DelayLog: config.DelayLogPath(root), LogsDir: config.LogsDir(),
		Region: region, Rules: rules, Dungeons: ref.Dungeons, RaidRules: raidRules, Bosses: raid.Bosses,
		Pages: os.DirFS(pages),
		Hosts: controller.LocalHosts(port), Now: clock.Now}
	if logs, err := wcl.New(opts.Cache); err != nil {
		log.Printf("no key reviews: %v", err)
	} else {
		opts.Baselines = logs
	}
	site, err := app.Start(ctx, opts)
	if err != nil {
		return err
	}
	if learn {
		_, err := site.Learn()
		return err
	}
	if replay == "" {
		go site.Follow(ctx, opts.LogsDir)
	} else {
		go func() {
			if err := replayKey(ctx, site, clock, replay, key); err != nil {
				log.Print(err)
			}
		}()
	}
	server := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", port), ReadHeaderTimeout: 5 * time.Second,
		Handler: site.Handler}
	go func() {
		<-ctx.Done()
		server.Close()
	}()
	log.Printf("site at http://localhost:%d (API spec at /swagger/index.html)", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serving the site: %w", err)
	}
	return nil
}

// replayKey feeds a past log to the keys: everything before the chosen key at once, then the key as it happened,
// speed times faster, with the clock following the replay.
func replayKey(ctx context.Context, site *app.Site, clock *replayClock, path string, want int) error {
	tail := &combatlog.Tail{File: path}
	seen, last := 0, int64(0)
	for {
		lines, _, err := tail.Read()
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			log.Printf("replay of %s done", path)
			return nil
		}
		for _, text := range lines {
			line, err := combatlog.ParseLine(text)
			if err != nil {
				continue
			}
			if line.Event == "CHALLENGE_MODE_START" {
				seen++
			}
			if seen >= want && last > 0 && line.Time > last {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(time.Duration(float64(line.Time-last)/clock.speed) * time.Millisecond):
				}
			}
			if seen >= want {
				last = line.Time
				clock.set(line.Time)
			}
			site.Keys.Add(line)
			site.Changes.Publish(site.Keys.Version())
		}
	}
}

// replayClock is the wall clock live, or during a replay the log time of the last line played plus the time since,
// sped up.
type replayClock struct {
	mu    sync.Mutex
	speed float64
	logMS int64
	at    time.Time
}

// Now is the clock's current time.
func (r *replayClock) Now() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.logMS == 0 {
		return time.Now()
	}
	return time.UnixMilli(r.logMS + int64(float64(time.Since(r.at).Milliseconds())*r.speed))
}

func (r *replayClock) set(ms int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logMS, r.at = ms, time.Now()
}
