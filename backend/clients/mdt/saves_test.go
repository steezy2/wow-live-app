package mdt

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

func TestSavedRoutesReadsLuaTable(t *testing.T) {
	wow := t.TempDir()
	logs := filepath.Join(wow, "Logs")
	if routes, err := SavedRoutes(logs); routes != nil || err != nil {
		t.Fatalf("with no file: %v, %v; want nil, nil", routes, err)
	}
	fixture, err := os.ReadFile("testdata/MythicDungeonTools.lua")
	if err != nil {
		t.Fatal(err)
	}
	// Account 1 saved the same route earlier under another name; the newer file (account 2) wins.
	older := `MythicDungeonToolsDB = { global = { presets = { [153] = { { uid = "AbC(1)xyz", text = "Old",
		value = { currentDungeonIdx = 153, pulls = { { color = "ffffff", [1] = { 1 } } } } } } } } }`
	for account, data := range map[string]string{"1#1": older, "2#1": string(fixture)} {
		path := filepath.Join(wow, "WTF", "Account", account, "SavedVariables", "MythicDungeonTools.lua")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		if account == "1#1" {
			if err := os.Chtimes(path, time.Time{}, time.Now().Add(-time.Hour)); err != nil {
				t.Fatal(err)
			}
		}
	}
	routes, err := SavedRoutes(logs)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 {
		t.Fatalf("got %d routes; want only the real one (no placeholder, no route without enemies)", len(routes))
	}
	p := routes[0]
	want := []map[int][]int{{1: {2, 3}, 16: {1}}, {18: {4, 3}}}
	if p.Name != "Easy route" || p.UID != "AbC(1)xyz" || p.DungeonIdx != 153 || len(p.Pulls) != 2 ||
		p.Pulls[0].Color != "ff3eff" || !reflect.DeepEqual(p.Pulls[0].Enemies, want[0]) ||
		!reflect.DeepEqual(p.Pulls[1].Enemies, want[1]) {
		t.Errorf("route = %+v; want Easy route, dungeon 153, pulls %v", p, want)
	}
	if _, err := EncodeRoute(p); err != nil {
		t.Errorf("EncodeRoute on a saved route: %v", err)
	}
}

func TestReadLua(t *testing.T) {
	type bs = cbor.ByteString
	for _, tc := range []struct {
		name, in string
		want     any
		err      string
	}{
		{"escapes", `"a\"b\\c\n\65\0661'"`, []byte("a\"b\\c\nAB1'"), ""},
		{"single quotes", `'say "hi"'`, []byte(`say "hi"`), ""},
		{"empty string", `""`, []byte{}, ""},
		{"whole", "12", uint64(12), ""},
		{"negative", "-8", int64(-8), ""},
		{"float", "-182.5", -182.5, ""},
		{"exponent", "1e-2", 0.01, ""},
		{"keys", `{ x = 1, ["y"] = true, [3] = -1, [-2.5] = "z", skip = nil, }`,
			map[any]any{bs("x"): uint64(1), bs("y"): true, uint64(3): int64(-1), -2.5: []byte("z")}, ""},
		{"list", "{ 1; false, nil }", []any{uint64(1), false, nil}, ""},
		{"empty table", "{}", []any{}, ""},
		{"mixed", `{ { 2 }, color = "c" }`, map[any]any{uint64(1): []any{uint64(2)}, bs("color"): []byte("c")}, ""},
		{"comments", "--[[ a\nblock ]] { -- a line\n true --[[ x ]] }", []any{true}, ""},
		{"unfinished string", `"abc`, nil, "unfinished string"},
		{"unclosed table", "{ 1, 2,", nil, "unexpected end"},
		{"missing comma", "{ 1 2 }", nil, "want , or }"},
		{"table key", "{ [{}] = 1 }", nil, "table key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars, err := readLua([]byte("x = " + tc.in))
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("readLua(%s) error = %v; want %q", tc.in, err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(vars["x"], tc.want) {
				t.Errorf("readLua(%s) = %#v; want %#v", tc.in, vars["x"], tc.want)
			}
		})
	}
}
