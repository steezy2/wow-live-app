// Package keystoneguru reads Keystone.guru's dungeon maps: each floor's in-game bounds from its repository's dungeon
// data (github.com/Wotuu/keystone.guru, numbers only), and its map tiles from its asset server, each downloaded once
// and kept in CacheDir (logs/cache/, never committed). The page credits Keystone.guru on the map.
package keystoneguru

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

const (
	// Repo is Keystone.guru on GitHub.
	Repo = "Wotuu/keystone.guru"
	// Zoom is the tile zoom the site uses: 2^Zoom by 2^Zoom tiles of 384 by 256 pixels (a 1536 by 1024 map).
	Zoom = 2
	// dataMaxAge is how long the dungeon data is reused; tiles are kept for good.
	dataMaxAge = 7 * 24 * time.Hour
)

// tileFolder is the only shape a tile folder may have ("bfa/templeofsethraliss/1"): it becomes part of a file path in
// the cache, so downloaded data can't point it anywhere else.
var tileFolder = regexp.MustCompile(`^[a-z0-9_]+/[a-z0-9_]+/[0-9]+$`)

// Client downloads Keystone.guru's data and tiles. Raw and Assets default to GitHub's and Keystone.guru's; HTTP may
// be nil.
type Client struct {
	HTTP     *http.Client
	CacheDir string
	Raw      string
	Assets   string
}

// Floors is each dungeon's floors with in-game bounds, by challenge mode ID (the combat log's map ID). Overview
// floors without bounds are left out.
func (c Client) Floors(ctx context.Context) (map[int][]models.DungeonFloor, error) {
	url := cmp.Or(c.Raw, "https://raw.githubusercontent.com") + "/" + Repo +
		"/master/database/seeders/dungeondata/dungeons.json"
	path := filepath.Join(c.CacheDir, "dungeons.json")
	if err := cachefile.Fresh(ctx, c.HTTP, url, path, dataMaxAge); err != nil {
		return nil, fmt.Errorf("Keystone.guru dungeons: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var dungeons []struct {
		ChallengeModeID *int   `json:"challenge_mode_id"`
		Key             string `json:"key"`
		Name            string `json:"name"` // "dungeons.midnight.altar_of_fangs.name": the expansion's tile folder
		Floors          []struct {
			Index   int     `json:"index"`
			UIMapID int     `json:"ui_map_id"`
			MinX    float64 `json:"ingame_min_x"`
			MaxX    float64 `json:"ingame_max_x"`
			MinY    float64 `json:"ingame_min_y"`
			MaxY    float64 `json:"ingame_max_y"`
		} `json:"floors"`
	}
	if err := json.Unmarshal(data, &dungeons); err != nil {
		return nil, fmt.Errorf("reading Keystone.guru's dungeons: %w", err)
	}
	floors := map[int][]models.DungeonFloor{}
	for _, d := range dungeons {
		parts := strings.Split(d.Name, ".")
		if d.ChallengeModeID == nil || len(parts) < 2 {
			continue
		}
		for _, f := range d.Floors {
			tiles := fmt.Sprintf("%s/%s/%d", parts[1], d.Key, f.Index)
			if f.UIMapID == 0 || f.MinX == f.MaxX || f.MinY == f.MaxY || !tileFolder.MatchString(tiles) {
				continue
			}
			floors[*d.ChallengeModeID] = append(floors[*d.ChallengeModeID], models.DungeonFloor{UIMapID: f.UIMapID,
				MinX: f.MinX, MaxX: f.MaxX, MinY: f.MinY, MaxY: f.MaxY, Tiles: tiles})
		}
	}
	return floors, nil
}

// Tile is one map tile of a floor (its Tiles path) at Zoom, as WebP: downloaded the first time, then from the cache.
func (c Client) Tile(ctx context.Context, tiles string, x, y int) ([]byte, error) {
	name := fmt.Sprintf("%d_%d.webp", x, y)
	path := filepath.Join(c.CacheDir, "tiles", filepath.FromSlash(tiles), name)
	url := fmt.Sprintf("%s/tiles_webp/%s/%d/%s", cmp.Or(c.Assets, "https://assets.keystone.guru"), tiles, Zoom, name)
	if err := cachefile.Fresh(ctx, c.HTTP, url, path, 100*365*24*time.Hour); err != nil {
		return nil, fmt.Errorf("Keystone.guru tile: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}
