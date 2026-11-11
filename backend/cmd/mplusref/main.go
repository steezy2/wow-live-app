// Command mplusref writes reference/mplus-dungeons.json, the season's dungeon facts the M+ page reads: forces from
// Mythic Dungeon Tools checked against Keystone.guru, and the must-stop, avoidable, soak, dispel, purge and priority
// lists from Wowhead's overviews. Run it each season or patch and review the diff.
//
// Usage, from backend/:
//
//	go run ./cmd/mplusref           regenerate the file
//	go run ./cmd/mplusref --check   list abilities that hit some of the party and not others in the logged keys,
//	                                that the avoidable and soak lists don't have, and mobs killed
//	                                that Mythic Dungeon Tools doesn't list (one shows as forces short of 100%)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"time"

	"wow-live/backend/clients/combatlog"
	"wow-live/backend/clients/keystoneguru"
	"wow-live/backend/clients/mdt"
	"wow-live/backend/clients/raiderio"
	"wow-live/backend/clients/wowhead"
	"wow-live/backend/config"
	"wow-live/backend/models"
	"wow-live/backend/service"
)

func main() {
	check := flag.Bool("check", false, "list uneven hits in the logged keys instead of regenerating")
	flag.Parse()
	root, err := config.RootFromWorkingDir()
	if err != nil {
		log.Fatal(err)
	}
	path := config.ReferencePath(root)
	if *check {
		err = checkLogs(path)
	} else {
		err = generate(context.Background(), filepath.Join(config.CachePath(root), "mplusref"), path)
	}
	if err != nil {
		log.Fatal(err)
	}
}

// generate gathers each season dungeon's sources and writes the reference file.
func generate(ctx context.Context, cache, path string) error {
	season, dungeons, err := raiderio.Client{CacheDir: filepath.Join(cache, "raiderio")}.Season(ctx)
	if err != nil {
		return err
	}
	mdtDungeons, commit, err := mdt.Client{CacheDir: filepath.Join(cache, "mdt")}.Dungeons(ctx)
	if err != nil {
		return err
	}
	guides := wowhead.Client{CacheDir: filepath.Join(cache, "wowhead")}
	kg, err := keystoneguru.Client{CacheDir: filepath.Join(cache, "keystone-guru")}.Dungeons(ctx)
	if err != nil {
		return err
	}
	ref := models.MPlusReference{Season: season, Dungeons: map[int]models.DungeonFacts{}, Sources: map[string]string{
		"forces": fmt.Sprintf("github.com/%s@%s (GPL-2.0), totals checked against Keystone.guru's", mdt.Repo,
			commit),
		"spells": "Wowhead dungeon overviews (each dungeon's guide), interrupts checked against Mythic Dungeon Tools",
		"maps": fmt.Sprintf("github.com/%s dungeon data: floor bounds and forces totals only; the site fetches the tiles",
			keystoneguru.Repo),
		"written": time.Now().Format("2006-01-02"),
	}}
	for _, d := range dungeons {
		src := service.DungeonSources{Name: d.Name, KeystoneGuruTotal: kg[d.MapID].ForcesTotal}
		if i := slices.IndexFunc(mdtDungeons, func(m models.MDTDungeon) bool { return m.MapID == d.MapID }); i >= 0 {
			src.MDT = mdtDungeons[i]
		}
		if src.Guide, err = guides.Guide(ctx, d.Slug); err != nil {
			return err
		}
		facts, err := service.DungeonFacts(src)
		if err != nil {
			return err
		}
		facts.Floors = append([]models.DungeonFloor{}, kg[d.MapID].Floors...)
		ref.Dungeons[d.MapID] = facts
		log.Printf("%s: %d NPCs (forces total %d, Keystone.guru %d), %d must-stop, %d avoidable, %d dispel, %d to review",
			d.Name, len(facts.Forces.NPCs), facts.Forces.Total, src.KeystoneGuruTotal, len(facts.MustStop),
			len(facts.Avoidable), len(facts.Dispel), len(facts.Review))
	}
	data, err := json.MarshalIndent(ref, "", "  ")
	if err != nil {
		return fmt.Errorf("writing the reference: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	log.Printf("wrote %s", path)
	return nil
}

// checkLogs reads every log in the game's folder and prints the uneven abilities the reference doesn't list and the
// mobs killed that Mythic Dungeon Tools doesn't list.
func checkLogs(path string) error {
	ref, err := config.LoadReference(path)
	if err != nil {
		return err
	}
	logs, err := combatlog.WrittenSince(config.LogsDir(), time.Time{})
	if err != nil {
		return err
	}
	hits := service.NewHitCounter()
	for _, file := range logs {
		tail := &combatlog.Tail{File: file}
		for {
			lines, _, err := tail.Read()
			if err != nil {
				return err
			}
			if len(lines) == 0 {
				break
			}
			for _, text := range lines {
				if line, err := combatlog.ParseLine(text); err == nil {
					hits.Add(line)
				}
			}
		}
	}
	for _, u := range hits.Uneven() {
		d := ref.Dungeons[u.MapID]
		if !slices.Contains(d.Avoidable, u.SpellID) && !slices.Contains(d.Soak, u.SpellID) {
			fmt.Printf("%s\t%d\t%s\tuneven in %d key(s)\n", d.Name, u.SpellID, u.Name, u.Keys)
		}
	}
	for k, n := range hits.Kills {
		d, ok := ref.Dungeons[k[0]]
		if _, listed := d.Forces.NPCs[k[1]]; ok && !listed {
			fmt.Printf("%s\tNPC %d\tkilled %d times, not in Mythic Dungeon Tools\n", d.Name, k[1], n)
		}
	}
	return nil
}
