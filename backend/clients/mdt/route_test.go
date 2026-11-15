package mdt

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"wow-live/backend/models"
)

// ownersRoute is the owner's Murder Row route (MDT dungeon 160), exported from the game.
func ownersRoute(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/route-export.txt")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDecodeRouteReadsOwnersExport(t *testing.T) {
	p, err := DecodeRoute(ownersRoute(t))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "1" || p.UID != "3EXVCZYHtId" || p.DungeonIdx != 160 || len(p.Pulls) != 17 {
		t.Fatalf("route = %q %q dungeon %d, %d pulls; want \"1\" \"3EXVCZYHtId\" dungeon 160, 17 pulls", p.Name, p.UID,
			p.DungeonIdx, len(p.Pulls))
	}
	want := map[int][]int{1: {4, 5, 6}, 3: {3, 4, 5}, 5: {1}, 8: {3, 6, 5, 4}} // enemies 2, 12, 15, 17 list no clones
	if got := p.Pulls[0]; got.Color != "ff3eff" || !reflect.DeepEqual(got.Enemies, want) {
		t.Errorf("pull 1 = %+v; want color ff3eff and enemies %v", got, want)
	}
}

func TestEncodeRouteRoundTrips(t *testing.T) {
	p, err := DecodeRoute(ownersRoute(t))
	if err != nil {
		t.Fatal(err)
	}
	p.Name = "Edited"
	p.Pulls[1].Enemies[5] = p.Pulls[0].Enemies[5]
	delete(p.Pulls[0].Enemies, 5)
	code, err := EncodeRoute(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeRoute(code)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Edited" || !reflect.DeepEqual(got.Pulls, p.Pulls) {
		t.Fatalf("decoded %q %+v; want the edited name and pulls %+v", got.Name, got.Pulls, p.Pulls)
	}
	// Everything but the name and pulls is as the game wrote it.
	before, after := otherFields(t, p.Raw), otherFields(t, got.Raw)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("other fields = %v\nwant %v", after, before)
	}
}

// otherFields is a preset's tree without its name and pulls.
func otherFields(t *testing.T, raw []byte) any {
	t.Helper()
	tree, err := decodeTree(raw)
	if err != nil {
		t.Fatal(err)
	}
	top, value, err := preset(tree)
	if err != nil {
		t.Fatal(err)
	}
	set(top, "text", nil)
	set(value, "pulls", nil)
	return top
}

func TestDecodeRouteRefuses(t *testing.T) {
	var bomb bytes.Buffer
	w, _ := flate.NewWriter(&bomb, flate.BestCompression)
	_, _ = w.Write(make([]byte, maxInflated+1))
	_ = w.Close()
	for _, tc := range []struct{ name, code, want string }{
		{"wrong prefix", "!WA:2!abc", "not an MDT route"},
		{"bad Base64", routePrefix + "@@@@", "Base64"},
		{"too long", routePrefix + strings.Repeat("A", MaxRouteCode), "longer than"},
		{"zip bomb", routePrefix + base64.StdEncoding.EncodeToString(bomb.Bytes()), "inflates past"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeRoute(tc.code); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("DecodeRoute = %v; want an error with %q", err, tc.want)
			}
		})
	}
}

// TestRouteDrawings reads the preset objects as the owner's saved routes have them (coordinates as strings): the shown
// lines (an arrow too) and note pins, leaving the rest in Raw, and writes them back before Raw's.
func TestRouteDrawings(t *testing.T) {
	s := func(v string) []byte { return []byte(v) }
	k := func(name string) cbor.ByteString { return cbor.ByteString(name) }
	line := map[any]any{k("d"): []any{uint64(10), 1.1, uint64(1), true, s("fcfbff"), int64(-8), true},
		k("l"): []any{s("182.0"), s("-326.3"), s("185.1"), s("-328.0"), s("185.1"), s("-328.0"), s("187.9"), s("-330.6")}}
	arrow := map[any]any{k("d"): []any{uint64(11), uint64(1), uint64(2), true, s("ff0000")},
		k("l"): []any{10.5, -20.0, 30.0, -40.5}, k("t"): []any{1.2}}
	note := map[any]any{k("n"): true, k("d"): []any{s("145.1"), s("-182.1"), uint64(1), true, s("Bloodlust")}}
	label := map[any]any{k("n"): true, k("tx"): true, k("d"): []any{s("1.0"), s("-2.0"), uint64(1), true, s("Go")}}
	undone := map[any]any{k("d"): []any{uint64(2), 1.1, uint64(1), false, s("fcfbff")},
		k("l"): []any{1.0, -1.0, 2.0, -2.0}}
	broken := map[any]any{k("d"): []any{uint64(2), 1.1, uint64(1), true, s("fcfbff")},
		k("l"): []any{1.0, -1.0, 2.0, -2.0, 5.0, -5.0, 6.0, -6.0}}
	raw, err := encodeTree(map[any]any{k("uid"): s("abc"), k("text"): s("Drawn"),
		k("objects"): []any{line, label, arrow, undone, note, broken},
		k("value"):   map[any]any{k("currentDungeonIdx"): uint64(160), k("pulls"): []any{}}})
	if err != nil {
		t.Fatal(err)
	}
	code, err := EncodeRoute(models.MDTPreset{Raw: raw})
	if err != nil {
		t.Fatal(err)
	}
	p, err := DecodeRoute(code)
	if err != nil {
		t.Fatal(err)
	}
	want := []models.MDTDrawing{
		{Sublevel: 1, Color: "fcfbff", Points: [][2]float64{{182, -326.3}, {185.1, -328}, {187.9, -330.6}}},
		{Sublevel: 2, Color: "ff0000", Points: [][2]float64{{10.5, -20}, {30, -40.5}}},
		{Sublevel: 1, Note: true, Points: [][2]float64{{145.1, -182.1}}, Text: "Bloodlust"},
	}
	if len(p.Drawings) != len(want) {
		t.Fatalf("drawings = %+v; want %+v", p.Drawings, want)
	}
	for i := range want {
		if got := p.Drawings[i]; got.Raw == nil || !reflect.DeepEqual(withoutRaw(got), want[i]) {
			t.Errorf("drawing %d = %+v; want %+v with its Raw", i, got, want[i])
		}
	}
	if left := list(get(mustTop(t, p.Raw), "objects")); len(left) != 3 {
		t.Fatalf("Raw keeps %d objects; want the label, the undone line and the broken one", len(left))
	}

	made := models.MDTDrawing{Sublevel: 1, Note: true, Points: [][2]float64{{1, -1}}, Text: "Kick"}
	marker := models.MDTDrawing{Sublevel: 1, Note: true, Points: [][2]float64{{3.25, -4}}}
	drawn := models.MDTDrawing{Sublevel: 1, Color: "3eff3e", Points: [][2]float64{{1, -1}, {2.5, -3}, {4, -5}}}
	p.Drawings = append([]models.MDTDrawing{made, marker, drawn}, p.Drawings...)
	if code, err = EncodeRoute(p); err != nil {
		t.Fatal(err)
	}
	again, err := DecodeRoute(code)
	if err != nil {
		t.Fatal(err)
	}
	marker.Points = [][2]float64{{3.2, -4}} // written to a tenth, like MDT's own
	wantAgain := append([]models.MDTDrawing{made, marker, drawn}, want...)
	got := make([]models.MDTDrawing, len(again.Drawings))
	for i, d := range again.Drawings {
		got[i] = withoutRaw(d)
	}
	if !reflect.DeepEqual(got, wantAgain) {
		t.Fatalf("written back = %+v\nwant %+v", got, wantAgain)
	}
	if !reflect.DeepEqual(again.Drawings[3:], p.Drawings[3:]) || !reflect.DeepEqual(again.Raw, p.Raw) {
		t.Error("MDT's own drawings and the rest of the preset changed on the way back")
	}
}

func withoutRaw(d models.MDTDrawing) models.MDTDrawing {
	d.Raw = nil
	return d
}

func mustTop(t *testing.T, raw []byte) map[any]any {
	t.Helper()
	tree, err := decodeTree(raw)
	if err != nil {
		t.Fatal(err)
	}
	top, _, err := preset(tree)
	if err != nil {
		t.Fatal(err)
	}
	return top
}
