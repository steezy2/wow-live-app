package combatlog

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"wow-live/backend/models"
)

func TestArchiveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WoWCombatLog-111026_190211.txt")
	var text strings.Builder
	for i := range 5000 {
		text.WriteString("11/10/2026 19:02:11.000-7  ZONE_CHANGE,2805,\"Ula'tek\"," + strings.Repeat("1", i%7+1) + "\r\n")
	}
	if err := os.WriteFile(path, []byte(text.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	var want []models.LogLine
	if err := EachLineIn(path, func(l models.LogLine) { want = append(want, l) }); err != nil {
		t.Fatal(err)
	}
	if err := Archive(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the log is still there: %v", err)
	}
	zips, err := Archived(dir)
	if err != nil || len(zips) != 1 || zips[0].Name != filepath.Base(path) || zips[0].Bytes >= int64(text.Len())/10 {
		t.Fatalf("archived = %+v, %v; want one small zip of the log", zips, err)
	}
	var got []models.LogLine
	if err := EachZippedLine(zips[0].Path, func(l models.LogLine) { got = append(got, l) }); err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(got, want, func(a, b models.LogLine) bool {
		return a.Time == b.Time && a.Event == b.Event && slices.Equal(a.Fields, b.Fields)
	}) {
		t.Fatalf("zipped log gives %d lines, want the %d of the log", len(got), len(want))
	}
	if FirstMS(zips[0].Path) != want[0].Time || len(want) != 5000 {
		t.Fatalf("first line at %d, want %d", FirstMS(zips[0].Path), want[0].Time)
	}
	if logs, _ := WrittenSince(dir, zips[0].Written.AddDate(-1, 0, 0)); len(logs) != 0 {
		t.Fatalf("the folder still lists %v", logs)
	}
}
