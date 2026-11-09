package service

import (
	"context"
	"errors"
	"slices"

	"wow-live/backend/models"
)

// tilesPerSide is how many tiles a floor is wide and tall at the zoom the site uses (Keystone.guru's zoom 2).
const tilesPerSide = 4

// MapTiles serves the map tiles of the season's dungeon floors.
type MapTiles struct {
	Facts  map[int]models.DungeonFacts
	Source models.TileSource
}

// Tile is one tile of a dungeon's floor (by the combat log's map IDs); ErrNoSuchKey when the floor has no map or the
// tile is off it.
func (m MapTiles) Tile(ctx context.Context, mapID, uiMapID, x, y int) ([]byte, error) {
	floors := m.Facts[mapID].Floors
	i := slices.IndexFunc(floors, func(f models.DungeonFloor) bool { return f.UIMapID == uiMapID })
	if i < 0 || x < 0 || y < 0 || x >= tilesPerSide || y >= tilesPerSide {
		return nil, ErrNoSuchKey
	}
	return m.Source.Tile(ctx, floors[i].Tiles, x, y)
}

// ErrNoSuchMap is a uiMap no raid boss in the reference file is fought on.
var ErrNoSuchMap = errors.New("no raid boss is fought on that map")

// RaidMaps serves the raid rooms' map images: only the reference file's rooms, so a request can't ask for any file.
type RaidMaps struct {
	Bosses map[string]models.RaidBoss
	Source models.MapImageSource
}

// Image is a raid room's map as PNG; ErrNoSuchMap when no boss is fought on it.
func (m RaidMaps) Image(ctx context.Context, uiMapID int) ([]byte, error) {
	for _, b := range m.Bosses {
		if slices.ContainsFunc(b.Maps, func(r models.RaidMap) bool { return r.UIMapID == uiMapID }) {
			return m.Source.Image(ctx, uiMapID)
		}
	}
	return nil, ErrNoSuchMap
}
