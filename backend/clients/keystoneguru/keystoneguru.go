// Package keystoneguru reads Keystone.guru's dungeon maps: each floor's in-game bounds and enemies, each dungeon's
// forces total and each NPC's health and flags from its repository's dungeon data (github.com/Wotuu/keystone.guru,
// numbers only), and its map tiles from its asset
// server, each downloaded once and kept in CacheDir (logs/cache/, never committed). The page credits Keystone.guru on
// the map.
package keystoneguru

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
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
	// zoom is the tile zoom the site uses: 2^zoom by 2^zoom tiles of 384 by 256 pixels (a 1536 by 1024 map).
	zoom = 2
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

// Dungeon is what Keystone.guru has for a dungeon: its instance ID, its floors with in-game bounds (overview floors
// without bounds left out) and the enemy forces a key needs, from its newest mapping.
type Dungeon struct {
	InstanceID  int
	Floors      []models.DungeonFloor
	ForcesTotal int
	mapping     int // the newest mapping's ID, which Enemies keeps
}

// Dungeons is each dungeon Keystone.guru maps, by challenge mode ID (the combat log's map ID).
func (c Client) Dungeons(ctx context.Context) (map[int]Dungeon, error) {
	var dungeons []struct {
		ID              int    `json:"id"`
		ChallengeModeID *int   `json:"challenge_mode_id"`
		MapID           int    `json:"map_id"` // the game's instance ID
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
		ID        int `json:"id"`
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
	newest := map[int][3]int{} // dungeon ID -> version, forces, mapping ID
	for _, m := range mappings {
		if m.Version > newest[m.DungeonID][0] {
			newest[m.DungeonID] = [3]int{m.Version, m.Forces, m.ID}
		}
	}
	out := map[int]Dungeon{}
	for _, d := range dungeons {
		parts := strings.Split(d.Name, ".")
		if d.ChallengeModeID == nil || len(parts) < 2 {
			continue
		}
		dungeon := Dungeon{InstanceID: d.MapID, ForcesTotal: newest[d.ID][1], mapping: newest[d.ID][2]}
		for _, f := range d.Floors {
			tiles := fmt.Sprintf("%s/%s/%d", parts[1], d.Key, f.Index)
			if f.UIMapID == 0 || f.MinX == f.MaxX || f.MinY == f.MaxY || !tileFolder.MatchString(tiles) {
				continue
			}
			dungeon.Floors = append(dungeon.Floors, models.DungeonFloor{
				RaidMap: models.RaidMap{UIMapID: f.UIMapID, MinX: f.MinX, MaxX: f.MaxX, MinY: f.MinY, MaxY: f.MaxY}, Tiles: tiles})
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
				log.Printf("Keystone.guru: floor %d has no tiles, so it takes %s and floor %d goes", f.UIMapID, g.Tiles,
					g.UIMapID)
				floors[i].Tiles, drop[i], drop[j] = g.Tiles, false, true
				break
			}
		}
	}
	var kept []models.DungeonFloor
	for i, f := range floors {
		if !drop[i] {
			kept = append(kept, f)
		} else if !tiled[i] {
			log.Printf("Keystone.guru: floor %d (%s) has no tiles; the page draws where parties walked", f.UIMapID,
				f.Tiles)
		}
	}
	return kept, nil
}

// Enemies is every enemy of the dungeon's newest mapping on its floors, from each floor's enemies.json (in the
// dungeon data's folder named like the floor's tiles).
func (c Client) Enemies(ctx context.Context, d Dungeon) ([]models.KeystoneGuruEnemy, error) {
	var out []models.KeystoneGuruEnemy
	for _, f := range d.Floors {
		var enemies []struct {
			Mapping int     `json:"mapping_version_id"`
			NPC     int     `json:"npc_id"`
			MDTNPC  int     `json:"mdt_npc_id"`
			MDTID   int     `json:"mdt_id"`
			Pack    int     `json:"enemy_pack_id"`
			Patrol  int     `json:"enemy_patrol_id"`
			Lat     float64 `json:"lat"`
			Lng     float64 `json:"lng"`
		}
		if err := c.getJSON(ctx, f.Tiles+"/enemies.json", &enemies); err != nil {
			return nil, err
		}
		for _, e := range enemies {
			if e.Mapping == d.mapping {
				out = append(out, models.KeystoneGuruEnemy{NPC: e.NPC, MDTNPC: e.MDTNPC, MDTID: e.MDTID, Pack: e.Pack,
					Patrol: e.Patrol, Lat: e.Lat, Lng: e.Lng, Map: f.UIMapID})
			}
		}
	}
	return out, nil
}

// Patrols is every patrol path of the dungeon's newest mapping on its floors, from each floor's enemy_patrols.json
// (beside its enemies.json); a floor without the file has none.
func (c Client) Patrols(ctx context.Context, d Dungeon) ([]models.KeystoneGuruPatrol, error) {
	var out []models.KeystoneGuruPatrol
	for _, f := range d.Floors {
		var patrols []struct {
			ID       int `json:"id"`
			Mapping  int `json:"mapping_version_id"`
			Polyline struct {
				Color    string `json:"color"`
				Vertices string `json:"vertices_json"` // a JSON list of {lat, lng} in a string
			} `json:"polyline"`
		}
		err := c.getJSON(ctx, f.Tiles+"/enemy_patrols.json", &patrols)
		if errors.Is(err, cachefile.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, p := range patrols {
			if p.Mapping != d.mapping {
				continue
			}
			var vertices []struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			}
			if err := json.Unmarshal([]byte(p.Polyline.Vertices), &vertices); err != nil {
				return nil, fmt.Errorf("reading Keystone.guru patrol %d: %w", p.ID, err)
			}
			patrol := models.KeystoneGuruPatrol{ID: p.ID, Color: p.Polyline.Color, Map: f.UIMapID}
			for _, v := range vertices {
				patrol.Points = append(patrol.Points, [2]float64{v.Lat, v.Lng})
			}
			out = append(out, patrol)
		}
	}
	return out, nil
}

// retail is Keystone.guru's game version ID for the live game, whose health row NPCs uses.
const retail = 1

// npcTypes and npcClassifications name Keystone.guru's NPC type and classification IDs (app/Models/Npc).
var (
	npcTypes = map[int]string{1: "Aberration", 2: "Beast", 3: "Critter", 4: "Demon", 5: "Dragonkin", 6: "Elemental",
		7: "Giant", 8: "Humanoid", 9: "Mechanical", 10: "Undead", 11: "Uncategorized", 12: "Totem"}
	npcClassifications = map[int]string{1: "Normal", 2: "Elite", 3: "Boss", 4: "Final boss", 5: "Rare"}
)

// NPCs is every NPC in Keystone.guru's npcs.json by NPC ID, without a name (its file has translation keys only):
// level, type, classification (Boss set for a boss or final boss), the live game's base health (scaled by the row's
// percentage), aggressiveness and the flags set, in the order DungeonMob lists them.
func (c Client) NPCs(ctx context.Context) (map[int]models.DungeonMob, error) {
	var npcs []struct {
		ID             int    `json:"id"`
		Level          int    `json:"level"`
		Type           int    `json:"npc_type_id"`
		Classification int    `json:"classification_id"`
		Aggressiveness string `json:"aggressiveness"`
		Bolstering     int    `json:"bolstering"`
		Bursting       int    `json:"bursting"`
		Sanguine       int    `json:"sanguine"`
		Dangerous      int    `json:"dangerous"`
		Truesight      int    `json:"truesight"`
		RunsAway       int    `json:"runs_away_in_fear"`
		Healths        []struct {
			GameVersion int      `json:"game_version_id"`
			Health      float64  `json:"health"`
			Percentage  *float64 `json:"percentage"`
		} `json:"npc_healths"`
	}
	if err := c.getJSON(ctx, "npcs.json", &npcs); err != nil {
		return nil, err
	}
	out := make(map[int]models.DungeonMob, len(npcs))
	for _, n := range npcs {
		mob := models.DungeonMob{NPC: n.ID, Level: n.Level, Type: npcTypes[n.Type],
			Class: npcClassifications[n.Classification], Boss: n.Classification == 3 || n.Classification == 4,
			Aggressiveness: n.Aggressiveness}
		for _, h := range n.Healths {
			if h.GameVersion != retail {
				continue
			}
			pct := 100.0
			if h.Percentage != nil && *h.Percentage > 0 {
				pct = *h.Percentage
			}
			mob.Health = int(math.Round(h.Health * pct / 100))
		}
		for _, f := range []struct {
			name string
			on   int
		}{{"bolstering", n.Bolstering}, {"bursting", n.Bursting}, {"sanguine", n.Sanguine},
			{"dangerous", n.Dangerous}, {"truesight", n.Truesight}, {"runsAwayInFear", n.RunsAway}} {
			if f.on != 0 {
				mob.Flags = append(mob.Flags, f.name)
			}
		}
		out[n.ID] = mob
	}
	return out, nil
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

// Tile is one map tile of a floor (its Tiles path) at zoom, as WebP: downloaded the first time, then from the cache.
func (c Client) Tile(ctx context.Context, tiles string, x, y int) ([]byte, error) {
	name := fmt.Sprintf("%d_%d.webp", x, y)
	path := filepath.Join(c.CacheDir, "tiles", filepath.FromSlash(tiles), name)
	url := fmt.Sprintf("%s/tiles_webp/%s/%d/%s", cmp.Or(c.Assets, "https://assets.keystone.guru"), tiles, zoom, name)
	if err := cachefile.Fresh(ctx, c.HTTP, url, path, 100*365*24*time.Hour); err != nil {
		return nil, fmt.Errorf("Keystone.guru tile: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}
