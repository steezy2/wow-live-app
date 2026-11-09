package keystoneguru

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFloorsKeepsOnlyMappedFloorsWithSafeTileFolders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
		 {"challenge_mode_id": 250, "key": "templeofsethraliss", "name": "dungeons.bfa.temple.name", "floors": [
		   {"index": 1, "ui_map_id": 1038, "ingame_min_x": -2789.58, "ingame_max_x": -3916.67, "ingame_min_y": 3187.5, "ingame_max_y": 3937.5},
		   {"index": 3, "ui_map_id": 0, "ingame_min_x": 0, "ingame_max_x": 0, "ingame_min_y": 0, "ingame_max_y": 0}]},
		 {"challenge_mode_id": 1, "key": "../../evil", "name": "dungeons.bfa.x.name", "floors": [
		   {"index": 1, "ui_map_id": 5, "ingame_min_x": 1, "ingame_max_x": 2, "ingame_min_y": 1, "ingame_max_y": 2}]}]`))
	}))
	defer server.Close()
	floors, err := Client{CacheDir: t.TempDir(), Raw: server.URL}.Floors(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := floors[250]; len(got) != 1 || got[0].Tiles != "bfa/templeofsethraliss/1" || got[0].UIMapID != 1038 {
		t.Errorf("Temple floors = %+v; want floor 1 only (the overview has no bounds)", got)
	}
	if len(floors[1]) != 0 {
		t.Errorf("a key with ../ gave floors %+v; want none", floors[1])
	}
}
