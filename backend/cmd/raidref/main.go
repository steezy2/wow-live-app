// Command raidref writes reference/raid-bosses.json, the raid's boss mechanics the raid overview reads: each boss's
// spells to dodge, soak, dispel and interrupt, the debuffs whose stacks matter and the units to burst, sorted from
// Wowhead's boss cheat sheets by their words (service.RaidBossFacts), and its NPC IDs from a top Warcraft Logs kill
// (WCL_CLIENT_ID and WCL_CLIENT_SECRET in .env; without them the overview tells the boss by its health). Run it each
// tier and review the diff.
//
// Usage, from backend/:
//
//	go run ./cmd/raidref                     regenerate the file for The Venomous Abyss
//	go run ./cmd/raidref --raid SLUG         another raid, by Wowhead's slug ("venomous-abyss")
//	go run ./cmd/raidref --check             list abilities that hit some raiders often and others never in the
//	                                         logged raid pulls, that a boss's avoidable and soak lists don't have
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
	"strings"
	"time"

	"wow-live/backend/clients/combatlog"
	"wow-live/backend/clients/wcl"
	"wow-live/backend/clients/wowhead"
	"wow-live/backend/config"
	"wow-live/backend/models"
	"wow-live/backend/service"
)

func main() {
	raid := flag.String("raid", "venomous-abyss", "the raid's Wowhead slug")
	check := flag.Bool("check", false, "list uneven hits in the logged raid pulls instead of regenerating")
	flag.Parse()
	root, err := config.RootFromWorkingDir()
	if err != nil {
		log.Fatal(err)
	}
	path := config.RaidReferencePath(root)
	if err := config.LoadEnv(filepath.Join(root, ".env")); err != nil {
		log.Fatal(err)
	}
	if *check {
		err = checkLogs(path)
	} else {
		err = generate(context.Background(), config.CachePath(root), *raid, path)
	}
	if err != nil {
		log.Fatal(err)
	}
}

// generate reads the raid's boss cheat sheets and its bosses' NPC IDs, and writes the reference file.
func generate(ctx context.Context, cache, raid, path string) error {
	guides, err := wowhead.Client{CacheDir: filepath.Join(cache, "raidref")}.RaidGuides(ctx, raid)
	if err != nil {
		return err
	}
	var names []string
	for i := range guides {
		guides[i].Name = strings.ReplaceAll(guides[i].Name, "’", "'") // the combat log's names use a plain apostrophe
		names = append(names, guides[i].Name)
	}
	npcs := map[string][]int{}
	if logs, err := wcl.New(cache); err != nil {
		log.Printf("no boss NPC IDs: %v", err)
	} else if npcs, err = logs.RaidBossNPCs(ctx, names); err != nil {
		return err
	}
	ref := models.RaidReference{Raid: raid, Bosses: map[string]models.RaidBoss{}, Sources: map[string]string{
		"mechanics": "Wowhead's boss cheat sheets (each boss's guide): spell IDs sorted by the words of their line",
		"npcs":      "Warcraft Logs: the boss actors of each encounter's top-ranked kill",
		"written":   time.Now().Format(time.DateOnly),
	}}
	for _, g := range guides {
		b := service.RaidBossFacts(g)
		b.NPCs = append([]int{}, npcs[b.Name]...)
		ref.Bosses[b.Name] = b
		log.Printf("%s: NPCs %v, %d avoidable, %d soak, %d dispel, %d must-stop, %d stacks, burst %v", b.Name, b.NPCs,
			len(b.Avoidable), len(b.Soak), len(b.Dispel), len(b.MustStop), len(b.Stacks), b.Priority)
	}
	data, err := json.MarshalIndent(ref, "", "  ")
	if err != nil {
		return fmt.Errorf("writing the raid reference: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	log.Printf("wrote %s", path)
	return nil
}

// checkLogs reads every log in the game's folder and prints the uneven abilities of each raid boss that its
// avoidable and soak lists don't have.
func checkLogs(path string) error {
	ref, err := config.LoadRaidReference(path)
	if err != nil {
		return err
	}
	logs, err := combatlog.WrittenSince(config.LogsDir(), time.Time{})
	if err != nil {
		return err
	}
	hits := service.NewHitCounter()
	for _, file := range logs {
		if err := combatlog.EachLine(file, hits.Add); err != nil {
			return err
		}
	}
	for _, u := range hits.Uneven() {
		boss, ok := hits.Encounters[u.MapID]
		if !ok {
			continue // a key's
		}
		b := ref.Bosses[boss]
		if !slices.Contains(b.Avoidable, u.SpellID) && !slices.Contains(b.Soak, u.SpellID) {
			fmt.Printf("%s\t%d\t%s\tuneven in %d pull(s)\n", boss, u.SpellID, u.Name, u.Keys)
		}
	}
	return nil
}
