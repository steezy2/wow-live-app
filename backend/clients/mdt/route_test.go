package mdt

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"os"
	"reflect"
	"strings"
	"testing"
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
