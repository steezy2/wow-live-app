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
			  {"mapping_version_id": 31, "npc_id": 2, "mdt_npc_id": 3, "mdt_id": 4, "enemy_pack_id": 5,
			   "enemy_patrol_id": 8, "lat": -6, "lng": 7}]`))
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
	if want := []models.KeystoneGuruEnemy{{NPC: 2, MDTNPC: 3, MDTID: 4, Pack: 5, Patrol: 8, Lat: -6, Lng: 7,
		Map: 1038}}; !reflect.DeepEqual(enemies, want) {
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

func TestPatrolsReadTheNewestMappingsPathsAndSkipAFloorWithoutTheFile(t *testing.T) {
	// The shape of Murder Row's floor 3 file: the path's points are JSON in a string.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/midnight/murder_row/3/enemy_patrols.json") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[
		 {"id": 22280, "mapping_version_id": 906, "floor_id": 406, "teeming": null, "faction": "any",
		  "polyline": {"color": "#003280", "color_animated": null, "weight": 3,
		   "vertices_json": "[{\"lat\":-200.8082,\"lng\":216.1495},{\"lat\":-189.9236,\"lng\":215.0353}]"},
		  "mdt_polyline": null},
		 {"id": 1, "mapping_version_id": 812, "polyline": {"color": "#fff", "vertices_json": "[]"}}]`))
	}))
	defer server.Close()
	d := Dungeon{mapping: 906, Floors: []models.DungeonFloor{
		{RaidMap: models.RaidMap{UIMapID: 2433}, Tiles: "midnight/murder_row/1"},
		{RaidMap: models.RaidMap{UIMapID: 2435}, Tiles: "midnight/murder_row/3"}}}
	got, err := Client{CacheDir: t.TempDir(), Raw: server.URL}.Patrols(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	want := []models.KeystoneGuruPatrol{{ID: 22280, Color: "#003280", Map: 2435,
		Points: [][2]float64{{-200.8082, 216.1495}, {-189.9236, 215.0353}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Patrols = %+v\nwant %+v", got, want)
	}
}

func TestNPCsNameTypesAndClassesAndKeepTheLiveGamesHealthAndSetFlags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
		 {"id": 262530, "level": 92, "npc_type_id": 8, "classification_id": 4,
		  "bolstering": 0, "bursting": 0, "sanguine": 0, "dangerous": 1, "truesight": 1, "runs_away_in_fear": 0,
		  "npc_healths": [{"game_version_id": 4, "health": 1}, {"game_version_id": 1, "health": 9729765, "percentage": null}]},
		 {"id": 7, "level": null, "npc_type_id": 100, "classification_id": 2,
		  "bolstering": 1, "bursting": 1, "sanguine": 1, "dangerous": 0, "truesight": 0, "runs_away_in_fear": 1,
		  "npc_healths": [{"game_version_id": 1, "health": 1000, "percentage": 30}]}]`))
	}))
	defer server.Close()
	got, err := Client{CacheDir: t.TempDir(), Raw: server.URL}.NPCs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]models.DungeonMob{
		262530: {NPC: 262530, Level: 92, Type: "Humanoid", Class: "Final boss", Boss: true, Health: 9729765,
			Flags: []string{"dangerous", "truesight"}},
		7: {NPC: 7, Class: "Elite", Health: 300,
			Flags: []string{"bolstering", "bursting", "sanguine", "runsAwayInFear"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NPCs = %+v\nwant %+v", got, want)
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
