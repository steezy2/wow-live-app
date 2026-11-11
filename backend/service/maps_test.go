package service

import (
	"context"
	"errors"
	"testing"

	"wow-live/backend/models"
)

type fakeTiles []string

func (f *fakeTiles) Tile(_ context.Context, tiles string, x, y int) ([]byte, error) {
	*f = append(*f, tiles)
	return []byte("webp"), nil
}

func TestMapTilesOnlyServeTheGrid(t *testing.T) {
	source := &fakeTiles{}
	m := MapTiles{Source: source, Facts: map[int]models.DungeonFacts{
		585: {Floors: []models.DungeonFloor{{UIMapID: 2574, Tiles: "midnight/voidscar_arena/1"}}}}}
	if tile, err := m.Tile(context.Background(), 585, 2574, 3, 3); err != nil || string(tile) != "webp" ||
		(*source)[0] != "midnight/voidscar_arena/1" {
		t.Fatalf("tile = %q, %v from %v; want the floor's tile", tile, err, *source)
	}
	for _, c := range [][4]int{{585, 2574, 4, 0}, {585, 9999, 0, 0}, {1, 2574, 0, 0}} {
		if _, err := m.Tile(context.Background(), c[0], c[1], c[2], c[3]); !errors.Is(err, ErrNoSuchKey) {
			t.Errorf("tile %v: %v, want ErrNoSuchKey", c, err)
		}
	}
}

type fakeImages struct{}

func (fakeImages) Image(context.Context, int) ([]byte, error) { return []byte("png"), nil }

func TestRaidMapsOnlyServeTheBossesRooms(t *testing.T) {
	m := RaidMaps{Source: fakeImages{}, Bosses: map[string]models.RaidBoss{"Ula'tek": {Maps: []models.RaidMap{{UIMapID: 2610}}}}}
	if img, err := m.Image(context.Background(), 2610); err != nil || string(img) != "png" {
		t.Fatalf("a boss's room: %q, %v; want its image", img, err)
	}
	if _, err := m.Image(context.Background(), 2611); !errors.Is(err, ErrNoSuchMap) {
		t.Fatalf("another map: %v; want ErrNoSuchMap", err)
	}
}
