// Package mdt reads Mythic Dungeon Tools' dungeon data from GitHub (Nnoggie/MythicDungeonTools, GPL-2.0): each
// dungeon's forces, mobs and spell flags. Only those facts are kept, never its code. Downloads are cached in CacheDir.
package mdt

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

const (
	// Repo is Mythic Dungeon Tools on GitHub.
	Repo = "Nnoggie/MythicDungeonTools"
	// Folder holds the current expansion's dungeon files.
	Folder = "Midnight"
	// maxAge is how long downloads are reused.
	maxAge = 24 * time.Hour
)

// Client downloads Mythic Dungeon Tools' data. API and Raw default to GitHub's; HTTP may be nil.
type Client struct {
	HTTP     *http.Client
	CacheDir string
	API      string
	Raw      string
}

// Dungeons is every dungeon in Folder at the repository's newest commit, and that commit.
func (c Client) Dungeons(ctx context.Context) ([]models.MDTDungeon, string, error) {
	api, raw := cmp.Or(c.API, "https://api.github.com"), cmp.Or(c.Raw, "https://raw.githubusercontent.com")
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := c.getJSON(ctx, api+"/repos/"+Repo+"/commits/master", "commit.json", &commit); err != nil {
		return nil, "", err
	}
	var files []struct {
		Name string `json:"name"`
	}
	if err := c.getJSON(ctx, api+"/repos/"+Repo+"/contents/"+Folder+"?ref="+commit.SHA, "files-"+commit.SHA+".json",
		&files); err != nil {
		return nil, "", err
	}
	var dungeons []models.MDTDungeon
	for _, f := range files {
		if !strings.HasSuffix(f.Name, ".lua") {
			continue
		}
		data, err := c.get(ctx, raw+"/"+Repo+"/"+commit.SHA+"/"+Folder+"/"+f.Name, commit.SHA+"-"+f.Name)
		if err != nil {
			return nil, "", err
		}
		if d := Parse(string(data)); d.MapID > 0 {
			dungeons = append(dungeons, d)
		}
	}
	return dungeons, commit.SHA, nil
}

var (
	englishName = regexp.MustCompile(`englishName = "([^"]+)"`)
	mapID       = regexp.MustCompile(`\bmapID = (\d+)`)
	totalCount  = regexp.MustCompile(`dungeonTotalCount\[dungeonIndex\] = \{ normal = (\d+)`)
	field       = regexp.MustCompile(`^\s*\["(\w+)"\] = "?([^",]*)"?,?$`)
	spellStart  = regexp.MustCompile(`^\s*\[(\d+)\] = \{`)
)

// Parse reads one dungeon file: its name, map ID and forces total, then each mob (name, id, count, isBoss) and the
// flags of the spells in its "spells" block. A file that isn't a dungeon has MapID 0.
func Parse(lua string) models.MDTDungeon {
	d := models.MDTDungeon{Mobs: map[int]models.MDTMob{}, Spells: map[int]models.MDTSpell{}}
	if m := englishName.FindStringSubmatch(lua); m != nil {
		d.Name = m[1]
	}
	if m := mapID.FindStringSubmatch(lua); m != nil {
		d.MapID, _ = strconv.Atoi(m[1])
	}
	if m := totalCount.FindStringSubmatch(lua); m != nil {
		d.Total, _ = strconv.Atoi(m[1])
	}
	var mob models.MDTMob
	mobID, spell, inSpells := 0, 0, false
	save := func() {
		if mobID > 0 {
			d.Mobs[mobID] = mob
		}
	}
	for _, line := range strings.Split(lua, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == `["spells"] = {`:
			inSpells = true
		case trimmed == `["clones"] = {`:
			inSpells = false
		case inSpells && spellStart.MatchString(line):
			spell, _ = strconv.Atoi(spellStart.FindStringSubmatch(line)[1])
			d.Spells[spell] = d.Spells[spell]
		}
		m := field.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key, value := m[1], m[2]
		switch {
		case inSpells && key == "interruptible" && value == "true":
			s := d.Spells[spell]
			s.Interruptible = true
			d.Spells[spell] = s
		case inSpells && key == "magic" && value == "true":
			s := d.Spells[spell]
			s.Magic = true
			d.Spells[spell] = s
		case key == "name" && !inSpells:
			save()
			mob, mobID = models.MDTMob{Name: value}, 0
		case key == "id" && mobID == 0 && mob.Name != "":
			mobID, _ = strconv.Atoi(value)
		case key == "count":
			mob.Count, _ = strconv.Atoi(value)
		case key == "isBoss":
			mob.Boss = value == "true"
		}
	}
	save()
	return d
}

func (c Client) getJSON(ctx context.Context, url, file string, out any) error {
	data, err := c.get(ctx, url, file)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("reading %s: %w", url, err)
	}
	return nil
}

func (c Client) get(ctx context.Context, url, file string) ([]byte, error) {
	path := filepath.Join(c.CacheDir, file)
	if err := cachefile.Fresh(ctx, c.HTTP, url, path, maxAge); err != nil {
		return nil, fmt.Errorf("Mythic Dungeon Tools: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}
