package mdt

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"

	"wow-live/backend/models"
)

// Mythic Dungeon Tools keeps its routes in the game's SavedVariables: a Lua file of `Name = table` assignments the
// game writes on /reload or logout. It is read as data by a table-literal reader; nothing in it is run.

// SavedRoutes reads the routes Mythic Dungeon Tools saved in every WoW account next to logsDir (<WoW>\_retail_\Logs).
// A route saved in several accounts comes from the newest file. "<New Route>" placeholders and routes without an
// enemy are left out. No file is no routes.
func SavedRoutes(logsDir string) ([]models.MDTPreset, error) {
	paths, err := filepath.Glob(filepath.Join(logsDir, "..", "WTF", "Account", "*", "SavedVariables",
		"MythicDungeonTools.lua"))
	if err != nil {
		return nil, fmt.Errorf("finding MDT's saved routes: %w", err)
	}
	modified := map[string]time.Time{}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("finding MDT's saved routes: %w", err)
		}
		modified[path] = info.ModTime()
	}
	slices.SortStableFunc(paths, func(a, b string) int { return modified[b].Compare(modified[a]) })
	seen := map[string]bool{}
	var out []models.MDTPreset
	for _, path := range paths {
		presets, err := savedPresets(path)
		if err != nil {
			log.Printf("skipping MDT's saved routes: %v", err)
			continue
		}
		for _, p := range presets {
			if p.UID != "" && seen[p.UID] {
				continue
			}
			seen[p.UID] = true
			out = append(out, p)
		}
	}
	return out, nil
}

// savedPresets reads MythicDungeonToolsDB.global.presets[dungeon][n] from one SavedVariables file.
func savedPresets(path string) ([]models.MDTPreset, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading MDT's saved routes: %w", err)
	}
	vars, err := readLua(src)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	db := table(vars["MythicDungeonToolsDB"])
	var out []models.MDTPreset
	for _, dungeon := range list(get(table(get(db, "global")), "presets")) {
		for _, tree := range list(dungeon) {
			if _, _, err := preset(tree); err != nil {
				continue // a "<New Route>" placeholder: its value is 0
			}
			p, err := presetOf(tree)
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", path, err)
			}
			if slices.ContainsFunc(p.Pulls, func(pull models.MDTPull) bool { return len(pull.Enemies) > 0 }) {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

// luaReader reads Lua literals into the shapes the CBOR decoder gives a route string: strings as []byte (as
// cbor.ByteString keys), whole numbers as uint64 (int64 when negative), other numbers as float64, and a table as []any
// when it has only positional items, else as a map with those items keyed from 1.
type luaReader struct {
	src []byte
	pos int
}

// luaEscapes are Lua 5.1's one-letter string escapes (WoW's Lua has no \x, \z or \u).
var luaEscapes = map[byte]byte{'n': '\n', 't': '\t', 'r': '\r', 'a': '\a', 'b': '\b', 'f': '\f', 'v': '\v',
	'\\': '\\', '"': '"', '\'': '\'', '\n': '\n'}

// readLua reads a SavedVariables file's `Name = value` assignments, by name.
func readLua(src []byte) (map[string]any, error) {
	r := &luaReader{src: src}
	vars := map[string]any{}
	for r.space(); r.pos < len(r.src); r.space() {
		name := r.name()
		if name == "" {
			return nil, r.errorf("want Name = value")
		}
		if err := r.expect('='); err != nil {
			return nil, err
		}
		v, err := r.value()
		if err != nil {
			return nil, err
		}
		vars[name] = v
	}
	return vars, nil
}

func (r *luaReader) value() (any, error) {
	r.space()
	if r.pos == len(r.src) {
		return nil, r.errorf("unexpected end")
	}
	switch c := r.src[r.pos]; {
	case c == '{':
		return r.table()
	case c == '"' || c == '\'':
		return r.str()
	case c == '-' || c == '.' || isDigit(c):
		return r.number()
	}
	switch r.name() {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "nil":
		return nil, nil
	}
	return nil, r.errorf("want a value")
}

func (r *luaReader) table() (any, error) {
	r.pos++
	items, keyed := []any{}, map[any]any{}
	for r.space(); !r.eat('}'); r.space() {
		key, err := r.key()
		if err != nil {
			return nil, err
		}
		v, err := r.value()
		if err != nil {
			return nil, err
		}
		if key == nil {
			items = append(items, v)
		} else if v != nil {
			keyed[key] = v
		}
		r.space()
		if !r.eat(',') && !r.eat(';') && !r.at('}') {
			return nil, r.errorf("want , or }")
		}
	}
	if len(keyed) == 0 {
		return items, nil
	}
	for i, v := range items {
		keyed[uint64(i+1)] = v
	}
	return keyed, nil
}

// key reads a table entry's `[key] =` or `name =`, or nothing (a nil key) for a positional item.
func (r *luaReader) key() (any, error) {
	start := r.pos
	if r.eat('[') {
		k, err := r.value()
		if err != nil {
			return nil, err
		}
		if err := r.expect(']'); err != nil {
			return nil, err
		}
		if err := r.expect('='); err != nil {
			return nil, err
		}
		switch k := k.(type) {
		case []byte:
			return cbor.ByteString(k), nil
		case uint64, int64, float64, bool:
			return k, nil
		}
		return nil, r.errorf("a table key must be a string, number or boolean")
	}
	if name := r.name(); name != "" {
		r.space()
		if r.eat('=') {
			return cbor.ByteString(name), nil
		}
	}
	r.pos = start
	return nil, nil
}

func (r *luaReader) str() (any, error) {
	quote := r.src[r.pos]
	out := []byte{}
	for r.pos++; r.pos < len(r.src); r.pos++ {
		c := r.src[r.pos]
		switch {
		case c == quote:
			r.pos++
			return out, nil
		case c == '\n':
			return nil, r.errorf("unfinished string")
		case c != '\\':
			out = append(out, c)
		case r.pos+1 == len(r.src):
			return nil, r.errorf("unfinished string")
		default:
			r.pos++
			e := r.src[r.pos]
			if b, ok := luaEscapes[e]; ok {
				out = append(out, b)
				continue
			}
			if !isDigit(e) {
				return nil, r.errorf(`unknown escape \%c`, e)
			}
			n, end := 0, r.pos
			for ; end < len(r.src) && end < r.pos+3 && isDigit(r.src[end]); end++ {
				n = n*10 + int(r.src[end]-'0')
			}
			if n > 255 {
				return nil, r.errorf(`escape \%d is past 255`, n)
			}
			out = append(out, byte(n))
			r.pos = end - 1
		}
	}
	return nil, r.errorf("unfinished string")
}

func (r *luaReader) number() (any, error) {
	start := r.pos
	for r.pos++; r.pos < len(r.src); r.pos++ {
		c, prev := r.src[r.pos], r.src[r.pos-1]
		if !isDigit(c) && !isLetter(c) && c != '.' && !((c == '+' || c == '-') && (prev == 'e' || prev == 'E')) {
			break
		}
	}
	text := string(r.src[start:r.pos])
	if n, err := strconv.ParseInt(text, 10, 64); err == nil {
		if n < 0 {
			return n, nil
		}
		return uint64(n), nil
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, r.errorf("bad number %q", text)
	}
	return f, nil
}

// name reads an identifier, or nothing.
func (r *luaReader) name() string {
	start := r.pos
	for r.pos < len(r.src) && (isLetter(r.src[r.pos]) || r.pos > start && isDigit(r.src[r.pos])) {
		r.pos++
	}
	return string(r.src[start:r.pos])
}

// space skips white space and comments.
func (r *luaReader) space() {
	for r.pos < len(r.src) {
		rest := r.src[r.pos:]
		switch {
		case bytes.HasPrefix(rest, []byte("--[[")):
			r.pos += past(rest, "]]")
		case bytes.HasPrefix(rest, []byte("--")):
			r.pos += past(rest, "\n")
		case strings.IndexByte(" \t\r\n", rest[0]) >= 0:
			r.pos++
		default:
			return
		}
	}
}

// past is how far b runs through the first end, or all of b.
func past(b []byte, end string) int {
	if i := bytes.Index(b, []byte(end)); i >= 0 {
		return i + len(end)
	}
	return len(b)
}

func (r *luaReader) at(c byte) bool { return r.pos < len(r.src) && r.src[r.pos] == c }

func (r *luaReader) eat(c byte) bool {
	if !r.at(c) {
		return false
	}
	r.pos++
	return true
}

func (r *luaReader) expect(c byte) error {
	r.space()
	if !r.eat(c) {
		return r.errorf("want %c", c)
	}
	return nil
}

func (r *luaReader) errorf(format string, args ...any) error {
	return fmt.Errorf("line %d: %s", bytes.Count(r.src[:r.pos], []byte("\n"))+1, fmt.Sprintf(format, args...))
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

func isLetter(c byte) bool { return c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }
