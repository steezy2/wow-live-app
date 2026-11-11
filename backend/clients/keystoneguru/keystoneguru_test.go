package keystoneguru

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDungeonsKeepsMappedFloorsWithSafeTileFoldersAndTheNewestForces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/mapping_versions.json") {
			_, _ = w.Write([]byte(`[{"dungeon_id": 7, "version": 2, "enemy_forces_required": 687},
			  {"dungeon_id": 7, "version": 1, "enemy_forces_required": 650}]`))
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
	dungeons, err := Client{CacheDir: t.TempDir(), Raw: server.URL}.Dungeons(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	temple := dungeons[250]
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
