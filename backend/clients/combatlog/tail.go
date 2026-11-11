package combatlog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"wow-live/backend/models"
)

// maxRead caps one Read, so starting on a big log works through it in steps instead of loading it all at once.
const maxRead = 64 << 20

// Tail follows the newest combat log in Dir as the game writes it. WoW starts a new file each time logging is turned
// on, so Read moves to a newer file when one appears. With File set, it reads only that file (a past night).
type Tail struct {
	Dir     string
	File    string
	path    string
	offset  int64
	partial []byte
}

// Read returns the complete lines written since the last call, oldest first, and the file they came from. A
// half-written last line is kept for the next call. It returns no lines when nothing new was written, or there's
// no log yet.
func (t *Tail) Read() ([]string, string, error) {
	newest := t.File
	if newest == "" {
		found, err := Newest(t.Dir)
		if err != nil || found == "" {
			return nil, "", err
		}
		newest = found
	}
	if newest != t.path {
		t.path, t.offset, t.partial = newest, 0, nil
	}
	file, err := os.Open(t.path)
	if err != nil {
		return nil, "", fmt.Errorf("opening %s: %w", t.path, err)
	}
	defer file.Close()
	if _, err := file.Seek(t.offset, io.SeekStart); err != nil {
		return nil, "", fmt.Errorf("reading %s: %w", t.path, err)
	}
	chunk, err := io.ReadAll(io.LimitReader(file, maxRead))
	if err != nil {
		return nil, "", fmt.Errorf("reading %s: %w", t.path, err)
	}
	t.offset += int64(len(chunk))
	data := append(t.partial, chunk...)
	end := bytes.LastIndexByte(data, '\n')
	if end < 0 {
		t.partial = data
		return nil, t.path, nil
	}
	t.partial = append([]byte(nil), data[end+1:]...)
	var lines []string
	for _, line := range bytes.Split(data[:end], []byte{'\n'}) {
		if line = bytes.TrimRight(line, "\r"); len(line) > 0 {
			lines = append(lines, string(line))
		}
	}
	return lines, t.path, nil
}

// Rest returns and clears a last line that never got its newline: at the end of a finished log it's complete.
func (t *Tail) Rest() string {
	rest := strings.TrimRight(string(t.partial), "\r")
	t.partial = nil
	return rest
}

// parseChunk is how many lines one parse job holds, so handing chunks between goroutines costs little next to
// parsing them.
const parseChunk = 4096

// EachLine passes every line of the finished log at path to add, parsed (lines that don't parse are skipped), the last
// one even without its newline. Lines are parsed on a pool of goroutines but reach add in file order, one at a time,
// on the caller's goroutine.
func EachLine(path string, add func(models.LogLine)) error {
	workers := max(runtime.NumCPU()-1, 1)
	type job struct {
		texts []string
		done  chan []models.LogLine
	}
	jobs := make(chan job)
	ordered := make(chan chan []models.LogLine, 4*workers)
	for range workers {
		go func() {
			for j := range jobs {
				j.done <- parseAll(j.texts)
			}
		}()
	}
	var readErr error
	go func() {
		defer close(ordered)
		defer close(jobs)
		readErr = eachChunk(path, func(texts []string) {
			done := make(chan []models.LogLine, 1)
			ordered <- done
			jobs <- job{texts, done}
		})
	}()
	for done := range ordered {
		for _, line := range <-done {
			add(line)
		}
	}
	return readErr
}

// eachChunk reads the finished log at path and hands its lines to send, parseChunk at a time and in order.
func eachChunk(path string, send func([]string)) error {
	tail := &Tail{File: path}
	for {
		texts, _, err := tail.Read()
		if err != nil {
			return err
		}
		if len(texts) == 0 {
			break
		}
		for chunk := range slices.Chunk(texts, parseChunk) {
			send(chunk)
		}
	}
	send([]string{tail.Rest()})
	return nil
}

// parseAll is the lines of texts that parse.
func parseAll(texts []string) []models.LogLine {
	lines := make([]models.LogLine, 0, len(texts))
	for _, text := range texts {
		if line, err := ParseLine(text); err == nil {
			lines = append(lines, line)
		}
	}
	return lines
}

// Newest is the most recently written WoWCombatLog*.txt in dir, or "" when there is none.
func Newest(dir string) (string, error) {
	logs, err := WrittenSince(dir, time.Time{})
	if err != nil || len(logs) == 0 {
		return "", err
	}
	return logs[len(logs)-1], nil
}

// WrittenSince is every WoWCombatLog*.txt in dir last written after since, oldest first.
func WrittenSince(dir string, since time.Time) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "WoWCombatLog*.txt"))
	if err != nil {
		return nil, fmt.Errorf("listing combat logs in %s: %w", dir, err)
	}
	type written struct {
		path string
		at   time.Time
	}
	var found []written
	for _, path := range paths {
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		if info.ModTime().After(since) {
			found = append(found, written{path, info.ModTime()})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].at.Before(found[j].at) })
	logs := make([]string, len(found))
	for i, w := range found {
		logs[i] = w.path
	}
	return logs, nil
}
