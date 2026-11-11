package combatlog

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"time"

	"wow-live/backend/models"
)

// maxRead caps one Read, so starting on a big log works through it in steps instead of loading it all at once.
const maxRead = 16 << 20

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

// parseChunk is how many lines one parse job holds, so handing chunks between goroutines costs little next to
// parsing them.
const parseChunk = 4096

// readBuffer is EachLine's read buffer; a longer line still reads whole.
const readBuffer = 1 << 20

// EachLine passes every line of the finished log r reads to add, parsed (lines that don't parse are skipped), the
// last one even without its newline. It streams: lines are parsed on a pool of goroutines, a few chunks at a time, but
// reach add in file order, one at a time, on the caller's goroutine.
func EachLine(r io.Reader, add func(models.LogLine)) error {
	var err error
	eachParsed(func(send func([]string)) { err = readLines(r, send) }, add)
	return err
}

// EachLineIn is EachLine on the finished log at path.
func EachLineIn(path string, add func(models.LogLine)) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer file.Close()
	if err := EachLine(file, add); err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	return nil
}

// readLines hands the lines r reads to send, parseChunk at a time and in order. Each line is a string of its own, not
// part of a shared one, as the tracker keeps names and GUIDs cut from it and would otherwise keep the whole chunk.
func readLines(r io.Reader, send func([]string)) error {
	reader := bufio.NewReaderSize(r, readBuffer)
	texts := make([]string, 0, parseChunk)
	var long []byte
	for {
		data, err := reader.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			long = append(long, data...)
			continue
		}
		if len(long) > 0 {
			data, long = append(long, data...), long[:0]
		}
		if text := bytes.TrimRight(data, "\r\n"); len(text) > 0 {
			texts = append(texts, string(text))
		}
		if len(texts) == parseChunk || err != nil && len(texts) > 0 {
			send(texts)
			texts = make([]string, 0, parseChunk)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// Parse is the lines of texts that parse, in order, on a pool of goroutines when there are more than one chunk's
// worth (Follow's backlog on start).
func Parse(texts []string) []models.LogLine {
	if len(texts) <= parseChunk {
		return parseAll(texts)
	}
	lines := make([]models.LogLine, 0, len(texts))
	eachParsed(func(send func([]string)) {
		for chunk := range slices.Chunk(texts, parseChunk) {
			send(chunk)
		}
	}, func(line models.LogLine) { lines = append(lines, line) })
	return lines
}

// eachParsed parses the chunks feed sends on a pool of goroutines and passes their lines to add in the order sent,
// on the caller's goroutine. At most 2 chunks per worker wait to be added, so memory stays flat however long the log.
func eachParsed(feed func(send func([]string)), add func(models.LogLine)) {
	workers := max(runtime.NumCPU()-1, 1)
	type job struct {
		texts []string
		done  chan []models.LogLine
	}
	jobs := make(chan job)
	ordered := make(chan chan []models.LogLine, 2*workers)
	for range workers {
		go func() {
			for j := range jobs {
				j.done <- parseAll(j.texts)
			}
		}()
	}
	go func() {
		defer close(ordered)
		defer close(jobs)
		feed(func(texts []string) {
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
