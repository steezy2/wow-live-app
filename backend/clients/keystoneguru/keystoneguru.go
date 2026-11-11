// Package keystoneguru reads Keystone.guru's dungeon maps: each floor's in-game bounds and each dungeon's forces total
// from its repository's dungeon data (github.com/Wotuu/keystone.guru, numbers only), and its map tiles from its asset
// server, each downloaded once and kept in CacheDir (logs/cache/, never committed). The page credits Keystone.guru on
// the map.
package keystoneguru

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
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

// Dungeon is what Keystone.guru has for a dungeon: its floors with in-game bounds (overview floors without bounds left
// out) and the enemy forces a key needs, from its newest mapping.
type Dungeon struct {
	Floors      []models.DungeonFloor
	ForcesTotal int
}

// Dungeons is each dungeon Keystone.guru maps, by challenge mode ID (the combat log's map ID).
func (c Client) Dungeons(ctx context.Context) (map[int]Dungeon, error) {
	var dungeons []struct {
		ID              int    `json:"id"`
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
	var mappings []struct {
		DungeonID int `json:"dungeon_id"`
		Version   int `json:"version"`
		Forces    int `json:"enemy_forces_required"`
	}
	if err := c.getJSON(ctx, "dungeons.json", &dungeons); err != nil {
		return nil, err
	}
	if err := c.getJSON(ctx, "mapping_versions.json", &mappings); err != nil {
		return nil, err
	}
	newest := map[int][2]int{} // dungeon ID -> version, forces
	for _, m := range mappings {
		if m.Version > newest[m.DungeonID][0] {
			newest[m.DungeonID] = [2]int{m.Version, m.Forces}
		}
	}
	out := map[int]Dungeon{}
	for _, d := range dungeons {
		parts := strings.Split(d.Name, ".")
		if d.ChallengeModeID == nil || len(parts) < 2 {
			continue
		}
		dungeon := Dungeon{ForcesTotal: newest[d.ID][1]}
		for _, f := range d.Floors {
			tiles := fmt.Sprintf("%s/%s/%d", parts[1], d.Key, f.Index)
			if f.UIMapID == 0 || f.MinX == f.MaxX || f.MinY == f.MaxY || !tileFolder.MatchString(tiles) {
				continue
			}
			dungeon.Floors = append(dungeon.Floors, models.DungeonFloor{UIMapID: f.UIMapID, MinX: f.MinX, MaxX: f.MaxX,
				MinY: f.MinY, MaxY: f.MaxY, Tiles: tiles})
		}
		floors, err := c.withTiles(ctx, dungeon.Floors)
		if err != nil {
			return nil, err
		}
		dungeon.Floors = floors
		out[*d.ChallengeModeID] = dungeon
	}
	return out, nil
}

// withTiles keeps the floors whose tiles exist. A floor without tiles takes those of a floor with the same bounds, and
// that floor goes, since its bounds are a copy (Den of Nalorakk's 2513 is drawn as index 3, under 2564's ID); a floor
// left without tiles goes too, so the page draws where parties walked instead. A center tile is probed for each.
func (c Client) withTiles(ctx context.Context, floors []models.DungeonFloor) ([]models.DungeonFloor, error) {
	tiled := make([]bool, len(floors))
	for i, f := range floors {
		_, err := c.Tile(ctx, f.Tiles, 1, 1)
		if err != nil && !errors.Is(err, cachefile.ErrNotFound) {
			return nil, err
		}
		tiled[i] = err == nil
	}
	drop := make([]bool, len(floors))
	for i, f := range floors {
		if tiled[i] {
			continue
		}
		drop[i] = true
		for j, g := range floors {
			if tiled[j] && !drop[j] && f.MinX == g.MinX && f.MaxX == g.MaxX && f.MinY == g.MinY && f.MaxY == g.MaxY {
				floors[i].Tiles, drop[i], drop[j] = g.Tiles, false, true
				break
			}
		}
	}
	var kept []models.DungeonFloor
	for i, f := range floors {
		if !drop[i] {
			kept = append(kept, f)
		}
	}
	return kept, nil
}

// getJSON decodes one of the repository's dungeon data files, downloaded at most every dataMaxAge.
func (c Client) getJSON(ctx context.Context, file string, out any) error {
	url := cmp.Or(c.Raw, "https://raw.githubusercontent.com") + "/" + Repo + "/master/database/seeders/dungeondata/" + file
	path := filepath.Join(c.CacheDir, file)
	if err := cachefile.Fresh(ctx, c.HTTP, url, path, dataMaxAge); err != nil {
		return fmt.Errorf("Keystone.guru %s: %w", file, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("reading Keystone.guru's %s: %w", file, err)
	}
	return nil
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
