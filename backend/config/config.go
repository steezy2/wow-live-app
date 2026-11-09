// Package config finds the repo and loads its secrets (.env), its M+ rules (mplus.toml) and the dungeon facts.
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

// LoadRules reads RulesFile under root.
func LoadRules(root string) (models.MPlusRules, error) {
	var rules models.MPlusRules
	if _, err := toml.DecodeFile(filepath.Join(root, RulesFile), &rules); err != nil {
		return rules, fmt.Errorf("reading %s: %w", RulesFile, err)
	}
	return rules, nil
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

// LoadReference reads the season's dungeon facts (ReferencePath).
func LoadReference(path string) (models.MPlusReference, error) {
	var ref models.MPlusReference
	data, err := os.ReadFile(path)
	if err != nil {
		return ref, fmt.Errorf("reading %s (run go run ./cmd/mplusref): %w", path, err)
	}
	if err := json.Unmarshal(data, &ref); err != nil {
		return ref, fmt.Errorf("reading %s: %w", path, err)
	}
	return ref, nil
}
