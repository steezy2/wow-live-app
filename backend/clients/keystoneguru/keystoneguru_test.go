package keystoneguru

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"wow-live/backend/models"
)

func TestDungeonsKeepsMappedFloorsWithSafeTileFoldersAndTheNewestMappingsForcesAndEnemies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/mapping_versions.json"):
			_, _ = w.Write([]byte(`[{"id": 31, "dungeon_id": 7, "version": 2, "enemy_forces_required": 687},
			  {"id": 30, "dungeon_id": 7, "version": 1, "enemy_forces_required": 650}]`))
			return
		case strings.HasSuffix(r.URL.Path, "/bfa/templeofsethraliss/1/enemies.json"):
			_, _ = w.Write([]byte(`[{"mapping_version_id": 30, "npc_id": 1, "mdt_id": 1, "lat": -1, "lng": 1},
			  {"mapping_version_id": 31, "npc_id": 2, "mdt_npc_id": 3, "mdt_id": 4, "enemy_pack_id": 5, "lat": -6, "lng": 7}]`))
			return
		}
		_, _ = w.Write([]byte(`[
		 {"id": 7, "challenge_mode_id": 250, "key": "templeofsethraliss", "name": "dungeons.bfa.temple.name", "floors": [
		   {"index": 1, "ui_map_id": 1038, "ingame_min_x": -2789.58, "ingame_max_x": -3916.67, "ingame_min_y": 3187.5, "ingame_max_y": 3937.5},
		   {"index": 3, "ui_map_id": 0, "ingame_min_x": 0, "ingame_max_x": 0, "ingame_min_y": 0, "ingame_max_y": 0}]},
		 {"id": 8, "challenge_mode_id": 1, "key": "../../evil", "name": "dungeons.bfa.x.name", "floors": [
		   {"index": 1, "ui_map_id": 5, "ingame_min_x": 1, "ingame_max_x": 2, "ingame_min_y": 1, "ingame_max_y": 2}]}]`))
	}))
	defer server.Close()
	c := Client{CacheDir: t.TempDir(), Raw: server.URL, Assets: server.URL}
	dungeons, err := c.Dungeons(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	temple := dungeons[250]
	enemies, err := c.Enemies(context.Background(), temple)
	if err != nil {
		t.Fatal(err)
	}
	if want := []models.KeystoneGuruEnemy{{NPC: 2, MDTNPC: 3, MDTID: 4, Pack: 5, Lat: -6, Lng: 7, Map: 1038}}; !reflect.DeepEqual(enemies, want) {
		t.Errorf("Temple enemies = %+v; want %+v, the newest mapping's", enemies, want)
	}
	if f := temple.Floors; len(f) != 1 || f[0].Tiles != "bfa/templeofsethraliss/1" || f[0].UIMapID != 1038 {
		t.Errorf("Temple floors = %+v; want floor 1 only (the overview has no bounds)", temple.Floors)
	}
	if temple.ForcesTotal != 687 {
		t.Errorf("Temple forces = %d; want 687, the newest mapping's", temple.ForcesTotal)
	}
	if len(dungeons[1].Floors) != 0 {
		t.Errorf("a key with ../ gave floors %+v; want none", dungeons[1].Floors)
	}
}

func TestAFloorWithoutTilesTakesThoseOfItsTwinAndAFloorWithNoneGoes(t *testing.T) {
	// Like Den of Nalorakk: floor 1 (2513) has no tiles, floor 3 has its bounds and tiles under another ID, floor 2 is
	// fine, and floor 4 has no tiles and no twin.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/mapping_versions.json"):
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(r.URL.Path, "/den/1/") || strings.Contains(r.URL.Path, "/den/4/"):
			http.NotFound(w, r)
		case strings.Contains(r.URL.Path, "/tiles_webp/"):
			_, _ = w.Write([]byte("webp"))
		default:
			_, _ = w.Write([]byte(`[{"id": 9, "challenge_mode_id": 586, "key": "den", "name": "dungeons.midnight.den.name", "floors": [
			  {"index": 1, "ui_map_id": 2513, "ingame_min_x": 5350, "ingame_max_x": 4406, "ingame_min_y": 4500, "ingame_max_y": 5129},
			  {"index": 2, "ui_map_id": 2514, "ingame_min_x": 5856, "ingame_max_x": 4731, "ingame_min_y": 4014, "ingame_max_y": 4764},
			  {"index": 3, "ui_map_id": 2564, "ingame_min_x": 5350, "ingame_max_x": 4406, "ingame_min_y": 4500, "ingame_max_y": 5129},
			  {"index": 4, "ui_map_id": 2600, "ingame_min_x": 1, "ingame_max_x": 2, "ingame_min_y": 1, "ingame_max_y": 2}]}]`))
		}
	}))
	defer server.Close()
	dungeons, err := Client{CacheDir: t.TempDir(), Raw: server.URL, Assets: server.URL}.Dungeons(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range dungeons[586].Floors {
		got = append(got, fmt.Sprint(f.UIMapID, " ", f.Tiles))
	}
	if want := []string{"2513 midnight/den/3", "2514 midnight/den/2"}; !slices.Equal(got, want) {
		t.Errorf("floors = %q, want %q", got, want)
	}
}
