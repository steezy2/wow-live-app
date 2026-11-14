package mdt

import (
	"bytes"
	"cmp"
	"compress/flate"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/fxamacker/cbor/v2"

	"wow-live/backend/models"
)

// A route string is what Mythic Dungeon Tools' Export button writes (MDT:TableToString in its
// Modules/Transmission.lua): routePrefix, then Base64 of raw Deflate of the preset as CBOR. Lua strings come out as
// CBOR byte strings and Lua tables as arrays or as maps with integer keys.
const (
	routePrefix = "!~MDT2~"
	// MaxRouteCode is the longest route string read, in bytes.
	MaxRouteCode = 64 << 10
	// maxInflated is the most a route string may inflate to, against zip bombs.
	maxInflated = 1 << 20
)

// DecodeRoute reads a route string from Mythic Dungeon Tools' Export button.
func DecodeRoute(code string) (models.MDTPreset, error) {
	code = strings.TrimSpace(code)
	if len(code) > MaxRouteCode {
		return models.MDTPreset{}, fmt.Errorf("not an MDT route: longer than %d bytes", MaxRouteCode)
	}
	b64, ok := strings.CutPrefix(code, routePrefix)
	if !ok {
		return models.MDTPreset{}, errors.New("not an MDT route: those start with " + routePrefix)
	}
	deflated, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return models.MDTPreset{}, fmt.Errorf("can't read it: Base64: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(deflated)), maxInflated+1))
	if err != nil {
		return models.MDTPreset{}, fmt.Errorf("can't read it: inflating: %w", err)
	}
	if len(data) > maxInflated {
		return models.MDTPreset{}, fmt.Errorf("can't read it: inflates past %d bytes", maxInflated)
	}
	tree, err := decodeTree(data)
	if err != nil {
		return models.MDTPreset{}, err
	}
	return presetOf(tree)
}

// EncodeRoute writes p as a route string Mythic Dungeon Tools imports: p.Raw with its name and pulls replaced. A p
// with no Raw (a route made in the app) gets the smallest preset MDT takes, from p.UID and p.DungeonIdx.
func EncodeRoute(p models.MDTPreset) (string, error) {
	tree := any(map[any]any{cbor.ByteString("uid"): []byte(p.UID), cbor.ByteString("value"): map[any]any{
		cbor.ByteString("currentDungeonIdx"): p.DungeonIdx, cbor.ByteString("currentPull"): 1,
		cbor.ByteString("currentSublevel"): 1}})
	if len(p.Raw) > 0 {
		var err error
		if tree, err = decodeTree(p.Raw); err != nil {
			return "", err
		}
	}
	top, value, err := preset(tree)
	if err != nil {
		return "", err
	}
	pulls := make([]any, len(p.Pulls))
	for i, pull := range p.Pulls {
		m := map[any]any{cbor.ByteString("color"): []byte(pull.Color)}
		for index, clones := range pull.Enemies {
			m[index] = clones
		}
		pulls[i] = m
	}
	set(top, "text", []byte(p.Name))
	set(value, "pulls", pulls)
	data, err := encodeTree(top)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return "", fmt.Errorf("deflating the MDT route: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return "", fmt.Errorf("deflating the MDT route: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("deflating the MDT route: %w", err)
	}
	return routePrefix + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// presetOf reads a preset's name, uid, dungeon and pulls from its decoded tree (a route string's or a saved route's).
// An enemy listed with no clones is left out.
func presetOf(tree any) (models.MDTPreset, error) {
	top, value, err := preset(tree)
	if err != nil {
		return models.MDTPreset{}, err
	}
	raw, err := encodeTree(tree)
	if err != nil {
		return models.MDTPreset{}, err
	}
	dungeon, _ := toInt(get(value, "currentDungeonIdx"))
	p := models.MDTPreset{Name: toString(get(top, "text")), UID: toString(get(top, "uid")), DungeonIdx: dungeon,
		Raw: raw}
	for _, pull := range list(get(value, "pulls")) {
		m := table(pull)
		out := models.MDTPull{Color: toString(get(m, "color")), Enemies: map[int][]int{}}
		for k, clones := range m {
			index, ok := toInt(k)
			if !ok {
				continue
			}
			for _, c := range list(clones) {
				if clone, ok := toInt(c); ok {
					out.Enemies[index] = append(out.Enemies[index], clone)
				}
			}
		}
		p.Pulls = append(p.Pulls, out)
	}
	return p, nil
}

// preset is a preset tree's top table and its "value" table.
func preset(tree any) (top, value map[any]any, err error) {
	top, _ = tree.(map[any]any)
	value, _ = get(top, "value").(map[any]any)
	if value == nil {
		return nil, nil, errors.New("can't read it: no route in it")
	}
	return top, value, nil
}

func decodeTree(data []byte) (any, error) {
	dm, err := cbor.DecOptions{MaxNestedLevels: 16}.DecMode()
	if err != nil {
		return nil, fmt.Errorf("reading the MDT route: %w", err)
	}
	var tree any
	if err := dm.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("can't read it: CBOR: %w", err)
	}
	return tree, nil
}

func encodeTree(tree any) ([]byte, error) {
	em, err := cbor.EncOptions{Sort: cbor.SortCoreDeterministic}.EncMode()
	if err != nil {
		return nil, fmt.Errorf("writing the MDT route: %w", err)
	}
	data, err := em.Marshal(tree)
	if err != nil {
		return nil, fmt.Errorf("writing the MDT route: %w", err)
	}
	return data, nil
}

// get is a table's field by name, as a byte string key (MDT's) or a text one.
func get(m map[any]any, name string) any {
	if v, ok := m[cbor.ByteString(name)]; ok {
		return v
	}
	return m[name]
}

// set replaces a table's field by name, under the key it already has (a byte string when new).
func set(m map[any]any, name string, v any) {
	if _, ok := m[name]; ok {
		m[name] = v
		return
	}
	m[cbor.ByteString(name)] = v
}

// table is a Lua table as a map: an array's items keyed from 1 (a pull of enemies 1 to n and no color is one).
func table(v any) map[any]any {
	l, ok := v.([]any)
	if !ok {
		m, _ := v.(map[any]any)
		return m
	}
	m := make(map[any]any, len(l))
	for i, x := range l {
		m[uint64(i+1)] = x
	}
	return m
}

// list is a Lua list's values: an array, or a map with integer keys in key order.
func list(v any) []any {
	m, ok := v.(map[any]any)
	if !ok {
		l, _ := v.([]any)
		return l
	}
	keys := slices.SortedFunc(maps.Keys(m), func(a, b any) int {
		x, _ := toInt(a)
		y, _ := toInt(b)
		return cmp.Compare(x, y)
	})
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		if _, ok := toInt(k); ok {
			out = append(out, m[k])
		}
	}
	return out
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case uint64:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), n == math.Trunc(n)
	}
	return 0, false
}

func toString(v any) string {
	switch s := v.(type) {
	case []byte:
		return string(s)
	case string:
		return s
	}
	return ""
}
