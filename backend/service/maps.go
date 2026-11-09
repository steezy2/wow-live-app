package service

import (
	"context"
	"slices"

	"wow-live/backend/models"
)

// tilesPerSide is how many tiles a floor is wide and tall at the zoom the site uses (Keystone.guru's zoom 2).
const tilesPerSide = 4

// MapTiles serves the map tiles of the season's dungeon floors (docs/specs/mplus-v2.md section 5).
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
