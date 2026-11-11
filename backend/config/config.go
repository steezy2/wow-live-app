// Package config finds the repo and loads its secrets (.env), its M+ and raid rules (mplus.toml, raid-rules.toml),
// the dungeon facts and the raid's boss mechanics.
package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"wow-live/backend/models"
)

// Root is the folder at or above dir that holds mplus.toml.
func Root(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, RulesFile)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("run this inside the repo (no mplus.toml found above here)")
		}
		dir = parent
	}
}

// RootFromWorkingDir is Root of the working directory.
func RootFromWorkingDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("finding the working directory: %w", err)
	}
	return Root(dir)
}

// CachePath is logs/cache/ under the repo: data fetched once and reused.
func CachePath(root string) string {
	return filepath.Join(root, "logs", "cache")
}

// DelayLogPath is logs/live-delay.log: how far behind the game the live readers read each key or pull.
func DelayLogPath(root string) string {
	return filepath.Join(root, "logs", "live-delay.log")
}

// LoadEnv sets KEY=VALUE lines from a .env file in the environment, without overriding what's already set. A
// missing file is fine (everything can come from the environment).
func LoadEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, ok := strings.Cut(line, "=")
		if line == "" || strings.HasPrefix(line, "#") || !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if _, set := os.LookupEnv(key); !set {
			if err := os.Setenv(key, strings.Trim(strings.TrimSpace(value), `"`)); err != nil {
				return fmt.Errorf("setting %s: %w", key, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	return nil
}

// RulesFile holds the M+ rules (the spell and buff names checked) at the repo's root; the app builds it in.
const RulesFile = "mplus.toml"

// RaidRulesFile holds the raid rules at the repo's root; the app builds it in.
const RaidRulesFile = "raid-rules.toml"

// LoadRules reads RulesFile under root.
func LoadRules(root string) (models.MPlusRules, error) {
	return loadTOML[models.MPlusRules](filepath.Join(root, RulesFile))
}

// LoadRaidRules reads RaidRulesFile under root.
func LoadRaidRules(root string) (models.RaidRules, error) {
	return loadTOML[models.RaidRules](filepath.Join(root, RaidRulesFile))
}

func loadTOML[T any](path string) (T, error) {
	var v T
	md, err := toml.DecodeFile(path, &v)
	if err != nil {
		return v, fmt.Errorf("reading %s: %w", filepath.Base(path), err)
	}
	if unknown := md.Undecoded(); len(unknown) > 0 {
		return v, fmt.Errorf("reading %s: unknown keys %v (a typo would drop a rule without a word)", filepath.Base(path), unknown)
	}
	return v, nil
}

// defaultLogsDir is where the game writes its combat log on a default Windows install.
const defaultLogsDir = `C:\Program Files (x86)\World of Warcraft\_retail_\Logs`

// LogsDir is the game's combat log folder: WOW_LOGS_DIR (from .env or the environment), or the default install's.
func LogsDir() string {
	if dir := os.Getenv("WOW_LOGS_DIR"); dir != "" {
		return dir
	}
	return defaultLogsDir
}

// ReferencePath is the season's dungeon facts for the M+ page, written by cmd/mplusref.
func ReferencePath(root string) string {
	return filepath.Join(root, "reference", "mplus-dungeons.json")
}

// RaidReferencePath is the raid's boss mechanics for the raid overview, written by cmd/raidref.
func RaidReferencePath(root string) string {
	return filepath.Join(root, "reference", "raid-bosses.json")
}

// LoadReference reads the season's dungeon facts (ReferencePath).
func LoadReference(path string) (models.MPlusReference, error) {
	return loadJSON[models.MPlusReference](path, "mplusref")
}

// RaidMechanicsPath is the hand-kept boss mechanics cmd/raidref merges into the raid reference.
func RaidMechanicsPath(root string) string {
	return filepath.Join(root, "reference", "raid-mechanics.toml")
}

// LoadRaidMechanics reads the hand-kept boss mechanics (RaidMechanicsPath).
func LoadRaidMechanics(path string) (models.RaidMechanics, error) {
	return loadTOML[models.RaidMechanics](path)
}

// LoadRaidReference reads the raid's boss mechanics (RaidReferencePath).
func LoadRaidReference(path string) (models.RaidReference, error) {
	return loadJSON[models.RaidReference](path, "raidref")
}

// loadJSON reads a generated reference file, saying which command writes it when it's missing.
func loadJSON[T any](path, command string) (T, error) {
	var v T
	data, err := os.ReadFile(path)
	if err != nil {
		return v, fmt.Errorf("reading %s (run go run ./cmd/%s): %w", path, command, err)
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return v, fmt.Errorf("reading %s: %w", path, err)
	}
	return v, nil
}
