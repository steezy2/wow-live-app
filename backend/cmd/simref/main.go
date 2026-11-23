// Command simref writes reference/sim-drops.json, the gear the Droptimizer sims: each item the current season's raid
// bosses and Mythic+ dungeons drop (ID, name, slot, armor type and the specs that can loot it), read from wago.tools'
// game tables of the live build, with the item levels of reference/sim-drops.toml (kept by hand from the wiki; see the
// toml for what was checked). The raid is the journal instance whose loot carries the toml's display_season; the
// dungeons are reference/mplus-dungeons.json's. The tables (ItemSparse is 52 MB) are kept a week in the user's cache
// folder, out of the repo. Run it each season or patch and review the diff.
//
// Usage, from backend/:
//
//	go run ./cmd/simref
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"wow-live/backend/clients/wago"
	"wow-live/backend/config"
)

// fetchTimeout bounds the whole run's downloads; each download has its own shorter one (cachefile.Fresh).
const fetchTimeout = 10 * time.Minute

func main() {
	root, err := config.RootFromWorkingDir()
	if err != nil {
		log.Fatal(err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		log.Fatal(fmt.Errorf("finding the cache folder: %w", err))
	}
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	tables := wago.Client{CacheDir: filepath.Join(cache, "wow-live", "simref"), Branch: "wow"}
	if err := generate(ctx, tables, root); err != nil {
		log.Fatal(err)
	}
}

// generate reads the hand-kept levels, the season's dungeons and the game tables, and writes the reference file.
func generate(ctx context.Context, tables wago.Client, root string) error {
	levels, err := config.LoadSimDropLevels(config.SimDropLevelsPath(root))
	if err != nil {
		return err
	}
	mplus, err := config.LoadReference(config.ReferencePath(root))
	if err != nil {
		return err
	}
	if levels.Season != mplus.Season {
		return fmt.Errorf("sim-drops.toml is for %s, mplus-dungeons.json for %s: update the item levels first",
			levels.Season, mplus.Season)
	}
	var dungeonMaps []int
	for _, d := range mplus.Dungeons {
		dungeonMaps = append(dungeonMaps, d.InstanceID)
	}
	j, err := readJournal(ctx, tables)
	if err != nil {
		return err
	}
	drops, err := buildDrops(j, levels, dungeonMaps)
	if err != nil {
		return err
	}
	for _, s := range append(drops.Raids, drops.Dungeons...) {
		n := 0
		for _, b := range s.Bosses {
			n += len(b.Items)
		}
		log.Printf("%s: %d bosses, %d items", s.Name, len(s.Bosses), n)
	}
	data, err := json.MarshalIndent(drops, "", "  ")
	if err != nil {
		return fmt.Errorf("writing the drops: %w", err)
	}
	path := config.SimDropsPath(root)
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	log.Printf("wrote %s", path)
	return nil
}
