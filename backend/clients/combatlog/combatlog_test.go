package combatlog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"wow-live/backend/models"
)

func TestParseLineReadsTimeEventAndFields(t *testing.T) {
	line, err := ParseLine(`11/3/2026 19:15:04.493-7  UNIT_DIED,0000000000000000,nil,0x80000000,0x80000000,` +
		`Creature-0-3019-3004-1081-217228-00006A951E,"Blood Beast",0x2114,0x80000000,0` + "\r\n")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 4, 2, 15, 4, 493e6, time.UTC).UnixMilli()
	if line.Time != want || line.Event != "UNIT_DIED" || line.Fields[5] != "Blood Beast" || len(line.Fields) != 9 {
		t.Fatalf("got %+v, want time %d, UNIT_DIED, Blood Beast, 9 fields", line, want)
	}
}

func TestParseLineKeepsQuotedCommasAndBracketsInOneField(t *testing.T) {
	line, err := ParseLine(`11/3/2026 19:14:54.254+1  COMBATANT_INFO,Player-76-0BFCA737,1,[(62085,80141,1),(62086,80142,1)],` +
		`"Name, with comma",(1,2),63`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Player-76-0BFCA737", "1", "[(62085,80141,1),(62086,80142,1)]", "Name, with comma", "(1,2)", "63"}
	if !reflect.DeepEqual(line.Fields, want) {
		t.Fatalf("fields = %q, want %q", line.Fields, want)
	}
	if utc := time.Date(2026, 11, 3, 18, 14, 54, 254e6, time.UTC).UnixMilli(); line.Time != utc {
		t.Fatalf("time = %d, want %d (UTC+1)", line.Time, utc)
	}
}

func TestParseLineRejectsALineWithoutAnEvent(t *testing.T) {
	if _, err := ParseLine("11/3/2026 19:14:54.254-7"); err == nil {
		t.Fatal("want an error for a line with no event")
	}
}

func TestTailKeepsAHalfWrittenLineAndFollowsANewFile(t *testing.T) {
	dir := t.TempDir()
	if lines, path, err := (&Tail{Dir: dir}).Read(); err != nil || lines != nil || path != "" {
		t.Fatalf("no log yet: got %q, %q, %v; want nothing", lines, path, err)
	}
	first := filepath.Join(dir, "WoWCombatLog-110326_190027.txt")
	write(t, first, "a\r\nb\nhalf")
	tail := &Tail{Dir: dir}
	if lines, _, err := tail.Read(); err != nil || !reflect.DeepEqual(lines, []string{"a", "b"}) {
		t.Fatalf("first read = %q, %v; want a, b", lines, err)
	}
	appendTo(t, first, "-line\n")
	if lines, _, _ := tail.Read(); !reflect.DeepEqual(lines, []string{"half-line"}) {
		t.Fatalf("second read = %q, want the completed line", lines)
	}
	appendTo(t, first, "no newline at the end")
	if lines, _, _ := tail.Read(); lines != nil || tail.Rest() != "no newline at the end" {
		t.Fatalf("an unterminated last line should wait for Rest, got %q", lines)
	}
	second := filepath.Join(dir, "WoWCombatLog-110426_161556.txt")
	write(t, second, "new\n")
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(second, later, later); err != nil {
		t.Fatal(err)
	}
	if lines, path, _ := tail.Read(); path != second || !reflect.DeepEqual(lines, []string{"new"}) {
		t.Fatalf("after a new file: %q from %s, want new from %s", lines, path, second)
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func TestEachLineKeepsOrder(t *testing.T) {
	const n = 10_000
	var text strings.Builder
	for i := range n {
		fmt.Fprintf(&text, "11/3/2026 19:15:04.493-7  SPELL_CAST_SUCCESS,%d\n", i)
	}
	path := filepath.Join(t.TempDir(), "WoWCombatLog-110326_190027.txt")
	write(t, path, strings.TrimSuffix(text.String(), "\n"))
	var got []string
	if err := EachLine(path, func(line models.LogLine) { got = append(got, line.Fields[0]) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("got %d lines, want %d", len(got), n)
	}
	for i, field := range got {
		if field != strconv.Itoa(i) {
			t.Fatalf("line %d = %s, want lines in file order", i, field)
		}
	}
}

// TestParseARealLog parses a whole log when COMBATLOG_FILE names one (skipped otherwise): every line must parse,
// and it reports the speed.
func TestParseARealLog(t *testing.T) {
	path := os.Getenv("COMBATLOG_FILE")
	if path == "" {
		t.Skip("set COMBATLOG_FILE to a WoWCombatLog-*.txt to run")
	}
	start := time.Now()
	tail := &Tail{File: path}
	lines, events := 0, map[string]int{}
	for {
		batch, _, err := tail.Read()
		if err != nil {
			t.Fatal(err)
		}
		if len(batch) == 0 {
			break
		}
		for _, text := range batch {
			line, err := ParseLine(text)
			if err != nil {
				t.Fatalf("line %d: %v", lines+1, err)
			}
			events[line.Event]++
			lines++
		}
	}
	t.Logf("%d lines, %d event types in %s", lines, len(events), time.Since(start).Round(time.Millisecond))
}

func TestWatcherWakesOnAWriteOrAfterEvery(t *testing.T) {
	dir := t.TempDir()
	w, err := Watch(dir, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	go os.WriteFile(filepath.Join(dir, "WoWCombatLog-110826_103639.txt"), []byte("x\n"), 0o644)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if start := time.Now(); !w.Wait(ctx) || time.Since(start) > time.Second {
		t.Fatalf("a write didn't wake the watcher (waited %s)", time.Since(start))
	}
	unwatched := &Watcher{Every: 10 * time.Millisecond}
	if !unwatched.Wait(ctx) {
		t.Fatal("a watcher without events didn't wake after Every")
	}
	cancel()
	if unwatched.Wait(ctx) {
		t.Fatal("Wait should be false once the context is done")
	}
}

func TestDelaysLogTheMedianAnd95thPercentileOfLiveLinesOnce(t *testing.T) {
	d := &Delays{File: filepath.Join(t.TempDir(), "live-delay.log")}
	read := time.Date(2026, 11, 9, 2, 15, 5, 0, time.UTC)        // 19:15:05 at -7
	d.Add([]string{"11/8/2026 18:00:00.000-7  UNIT_DIED"}, read) // the backlog
	d.Add(nil, read)
	d.Add([]string{"11/8/2026 19:15:04.900-7  UNIT_DIED", "11/8/2026 19:15:04.500-7  UNIT_DIED",
		"11/8/2026 19:15:03.000-7  UNIT_DIED", "not a line"}, read)
	if err := d.Log("key started 19:00"); err != nil {
		t.Fatal(err)
	}
	if err := d.Log("key started 19:30"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(d.File)
	if err != nil {
		t.Fatal(err)
	}
	want := "key started 19:00: read 0.50 s behind the game (median), 2.00 s (95th percentile), 3 lines\n"
	if !strings.HasSuffix(string(data), want) || strings.Count(string(data), "\n") != 1 {
		t.Fatalf("live-delay.log = %q, want one line ending %q", data, want)
	}
}
