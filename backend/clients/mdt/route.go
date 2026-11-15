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
	"strconv"
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

// EncodeRoute writes p as a route string Mythic Dungeon Tools imports: p.Raw with its name and pulls replaced and
// p.Drawings put before the drawings Raw still has. A p with no Raw (a route made in the app) gets the smallest preset
// MDT takes, from p.UID and p.DungeonIdx.
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
	objects := make([]any, 0, len(p.Drawings))
	for _, d := range p.Drawings {
		o, err := drawingTree(d)
		if err != nil {
			return "", err
		}
		objects = append(objects, o)
	}
	set(top, "text", []byte(p.Name))
	set(top, "objects", append(objects, list(get(top, "objects"))...))
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

// presetOf reads a preset's name, uid, dungeon, pulls and drawings (drawingOf's) from its decoded tree (a route
// string's or a saved route's), leaving the drawings it read out of Raw. An enemy listed with no clones is left out.
func presetOf(tree any) (models.MDTPreset, error) {
	top, value, err := preset(tree)
	if err != nil {
		return models.MDTPreset{}, err
	}
	dungeon, _ := toInt(get(value, "currentDungeonIdx"))
	p := models.MDTPreset{Name: toString(get(top, "text")), UID: toString(get(top, "uid")), DungeonIdx: dungeon}
	if objects := get(top, "objects"); objects != nil {
		others := []any{}
		for _, o := range list(objects) {
			d, ok := drawingOf(table(o))
			if !ok {
				others = append(others, o)
				continue
			}
			if d.Raw, err = encodeTree(o); err != nil {
				return models.MDTPreset{}, err
			}
			p.Drawings = append(p.Drawings, d)
		}
		set(top, "objects", others)
	}
	if p.Raw, err = encodeTree(tree); err != nil {
		return models.MDTPreset{}, err
	}
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

// A preset's "objects" are its drawings (MDT's Modules/PresetObjects.lua and Toolbar.lua), each with
// d = {size, lineFactor, sublevel, shown, color, drawLayer, smooth} and l = {x1, y1, x2, y2, ...}, its segments, for
// a line (an arrow adds t = {rotation}); d = {x, y, sublevel, shown, text} and n = true for a note pin; and a text
// label is a note with tx = true. Coordinates are on the sublevel's 840 by 555 map, written as strings to a tenth.
const (
	lineSize       = 3   // MDT's default brush size
	lineFactor     = 1.1 // what MDT's pencil and line tools write
	shownField     = 4
	sublevelField  = 3
	colorTextField = 5 // a line's color, a note's text
)

// drawingOf reads a shown line or note pin; ok is false for anything else (an undone drawing, a text label, a line the
// eraser broke into pieces), which stays MDT's own.
func drawingOf(o map[any]any) (d models.MDTDrawing, ok bool) {
	field := table(get(o, "d"))
	at := func(i int) any { return field[uint64(i)] }
	sublevel, ok := toInt(at(sublevelField))
	if !ok || at(shownField) != true || get(o, "tx") != nil {
		return d, false
	}
	d.Sublevel = sublevel
	if get(o, "n") == true {
		x, okX := toFloat(at(1))
		y, okY := toFloat(at(2))
		d.Note, d.Points, d.Text = true, [][2]float64{{x, y}}, toString(at(colorTextField))
		return d, okX && okY
	}
	l := list(get(o, "l"))
	if len(l) == 0 || len(l)%4 != 0 {
		return d, false
	}
	coords := make([]float64, len(l))
	for i, c := range l {
		if coords[i], ok = toFloat(c); !ok {
			return d, false
		}
	}
	d.Color, d.Points = toString(at(colorTextField)), [][2]float64{{coords[0], coords[1]}}
	for i := 0; i < len(coords); i += 4 {
		if [2]float64{coords[i], coords[i+1]} != d.Points[len(d.Points)-1] {
			return d, false
		}
		d.Points = append(d.Points, [2]float64{coords[i+2], coords[i+3]})
	}
	return d, true
}

// drawingTree is a drawing as a preset object: its Raw as it was, or a line of MDT's default size or a note pin.
func drawingTree(d models.MDTDrawing) (any, error) {
	if len(d.Raw) > 0 {
		return decodeTree(d.Raw)
	}
	coord := func(v float64) []byte { return strconv.AppendFloat(nil, v, 'f', 1, 64) }
	if d.Note {
		p := d.Points[0]
		return map[any]any{cbor.ByteString("n"): true, cbor.ByteString("d"): []any{coord(p[0]), coord(p[1]), d.Sublevel,
			true, []byte(d.Text)}}, nil
	}
	l := make([]any, 0, 4*len(d.Points))
	for i := 1; i < len(d.Points); i++ {
		a, b := d.Points[i-1], d.Points[i]
		l = append(l, coord(a[0]), coord(a[1]), coord(b[0]), coord(b[1]))
	}
	return map[any]any{cbor.ByteString("l"): l, cbor.ByteString("d"): []any{lineSize, lineFactor, d.Sublevel, true,
		[]byte(d.Color), 0, true}}, nil
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

// toFloat is a number, or a string of one (MDT writes its drawings' coordinates as strings), never NaN or infinite.
func toFloat(v any) (float64, bool) {
	var f float64
	switch n := v.(type) {
	case uint64:
		f = float64(n)
	case int64:
		f = float64(n)
	case float64:
		f = n
	case []byte, string:
		var err error
		if f, err = strconv.ParseFloat(toString(n), 64); err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return f, !math.IsNaN(f) && !math.IsInf(f, 0)
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
