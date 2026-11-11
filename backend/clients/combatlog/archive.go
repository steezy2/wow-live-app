package combatlog

import (
	"archive/zip"
	"bufio"
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"time"

	"wow-live/backend/models"
)

// ArchiveDir is the folder, inside the game's log folder, that Archive zips logs into. Follow and WrittenSince look
// only at the log folder itself, so an archived log stops loading.
const ArchiveDir = "WoW Live archive"

// Zipped is an archived log: the zip's path, and the log's name and last write as it was zipped.
type Zipped struct {
	Path    string
	Name    string
	Bytes   int64 // the zip's size, what deleting it frees
	Written time.Time
}

// Archived is every log zipped into dir's ArchiveDir, oldest first; a missing folder is none. A zip that isn't one
// readable log is left out (and logged), so one bad file doesn't hide the rest.
func Archived(dir string) ([]Zipped, error) {
	paths, err := filepath.Glob(filepath.Join(dir, ArchiveDir, "WoWCombatLog*.txt.zip"))
	if err != nil {
		return nil, fmt.Errorf("listing archived logs in %s: %w", dir, err)
	}
	var found []Zipped
	for _, path := range paths {
		z, err := zipped(path)
		if err != nil {
			log.Printf("leaving out an archived log: %v", err)
			continue
		}
		found = append(found, z)
	}
	slices.SortFunc(found, func(a, b Zipped) int { return a.Written.Compare(b.Written) })
	return found, nil
}

// zipped reads an archived log's name and last write from its zip's directory, without unzipping it.
func zipped(path string) (Zipped, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return Zipped{}, fmt.Errorf("reading %s: %w", path, err)
	}
	defer r.Close()
	if len(r.File) != 1 {
		return Zipped{}, fmt.Errorf("reading %s: want one log in it, found %d files", path, len(r.File))
	}
	info, err := os.Stat(path)
	if err != nil {
		return Zipped{}, fmt.Errorf("reading %s: %w", path, err)
	}
	return Zipped{Path: path, Name: r.File[0].Name, Bytes: info.Size(), Written: r.File[0].Modified}, nil
}

// EachZippedLine is EachLine on the log archived in the zip at path.
func EachZippedLine(path string, add func(models.LogLine)) error {
	return withZipped(path, func(r io.Reader) error { return EachLine(r, add) })
}

// withZipped opens the one log in the zip at path for use.
func withZipped(path string, use func(io.Reader) error) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	defer r.Close()
	if len(r.File) != 1 {
		return fmt.Errorf("reading %s: want one log in it, found %d files", path, len(r.File))
	}
	entry, err := r.File[0].Open()
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	defer entry.Close()
	if err := use(entry); err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	return nil
}

// FirstMS is the time of the first line that parses in the log at path (a zip from Archived, or a log), 0 when none
// does or it can't be read: it only places saved keys in their log.
func FirstMS(path string) int64 {
	var ms int64
	first := func(r io.Reader) error {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64<<10), readBuffer)
		for scanner.Scan() {
			if line, err := ParseLine(scanner.Text()); err == nil {
				ms = line.Time
				return nil
			}
		}
		return scanner.Err()
	}
	if filepath.Ext(path) == ".zip" {
		_ = withZipped(path, first)
		return ms
	}
	if file, err := os.Open(path); err == nil {
		defer file.Close()
		_ = first(file)
	}
	return ms
}

// Archive zips the log at path into its folder's ArchiveDir as <name>.zip, then removes the log. The zip is written
// beside as .tmp and read back whole (archive/zip checks its CRC) before it replaces any earlier zip of the same log,
// so a failure leaves the log as it was. A log the game still has open fails to be removed, and its zip goes too.
func Archive(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("archiving %s: %w", path, err)
	}
	dir := filepath.Join(filepath.Dir(path), ArchiveDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("archiving %s: %w", path, err)
	}
	target := filepath.Join(dir, info.Name()+".zip")
	tmp := target + ".tmp"
	err = writeZip(path, tmp, info)
	if err == nil {
		err = checkZip(tmp, info.Size())
	}
	if err == nil {
		err = os.Rename(tmp, target)
	}
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("archiving %s: %w", path, err)
	}
	if err := os.Remove(path); err != nil {
		os.Remove(target)
		return fmt.Errorf("archiving %s: the game may still be writing it: %w", path, err)
	}
	return nil
}

// writeZip writes the log at path, as one entry named and dated like it, into a new zip at out. BestSpeed: about three
// times faster than the default for a few percent more size on a text log.
func writeZip(path, out string, info os.FileInfo) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	file, err := os.Create(out)
	if err != nil {
		return err
	}
	w := zip.NewWriter(file)
	w.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.BestSpeed)
	})
	header, err := zip.FileInfoHeader(info)
	if err == nil {
		header.Method = zip.Deflate
		var entry io.Writer
		if entry, err = w.CreateHeader(header); err == nil {
			_, err = io.Copy(entry, src)
		}
	}
	if err = errors.Join(err, w.Close()); err == nil {
		err = file.Sync() // on disk before the log is removed, not only in the OS's cache
	}
	return errors.Join(err, file.Close())
}

// checkZip reads the zip at path back whole: archive/zip fails on a bad CRC, and the size must be the log's.
func checkZip(path string, size int64) error {
	return withZipped(path, func(r io.Reader) error {
		n, err := io.Copy(io.Discard, r)
		if err == nil && n != size {
			err = fmt.Errorf("the zip holds %d bytes of a %d byte log", n, size)
		}
		return err
	})
}
