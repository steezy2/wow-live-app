package service

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"wow-live/backend/models"
)

// archiveAfterMS: a log last written this long ago (and read, and not active) is ticked for archiving at first.
const archiveAfterMS = 14 * 24 * 60 * 60_000

// ErrBusy is a read, archive or delete of the log folder asked for while another one runs.
var ErrBusy = errors.New("already working on the log folder")

// ErrBadLog is a log name the listing doesn't offer for that action (archive or delete).
var ErrBadLog = errors.New("no such log to do that to")

// LogFiles marks up the log folder's logs and the archived ones for Settings: which the game may still write (the
// newest, and any written in the last quietMS), which are read (read says, for a log in the folder; an archived one
// was read before it was zipped), how many saved keys and pulls started in each, and which to tick at first. The
// folder's logs come first, then the archived ones, each newest first.
func LogFiles(files []models.LogFile, read func(name string) bool, keyStarts, pullStarts []int64,
	nowMS int64) []models.LogFile {
	files = slices.Clone(files)
	slices.SortFunc(files, func(a, b models.LogFile) int {
		switch {
		case a.Archived == b.Archived:
			return cmp.Compare(b.WrittenMS, a.WrittenMS)
		case a.Archived:
			return 1
		}
		return -1
	})
	newest := true
	for i := range files {
		f := &files[i]
		if !f.Archived {
			f.Active = newest || nowMS-f.WrittenMS < quietMS
			newest = false
		}
		f.Read = f.Archived || read(f.Name)
		f.Keys, f.Pulls = startedIn(keyStarts, *f), startedIn(pullStarts, *f)
		f.Suggested = !f.Archived && !f.Active && f.Read && nowMS-f.WrittenMS >= archiveAfterMS
	}
	return files
}

// startedIn counts the starts between the log's first line and its last write.
func startedIn(starts []int64, f models.LogFile) int {
	n := 0
	for _, s := range starts {
		if f.FirstMS > 0 && s >= f.FirstMS && s <= f.WrittenMS {
			n++
		}
	}
	return n
}

// PickLogs is the listed logs names asks for: for archiving (archived false) only logs in the folder the game no
// longer writes, for deleting only archived ones. Any other name is ErrBadLog, so a request never names a file of its
// own choosing.
func PickLogs(files []models.LogFile, names []string, archived bool) ([]models.LogFile, error) {
	picked := make([]models.LogFile, 0, len(names))
	for _, name := range names {
		i := slices.IndexFunc(files, func(f models.LogFile) bool {
			return f.Name == name && f.Archived == archived && !f.Active
		})
		if i < 0 {
			return nil, fmt.Errorf("%q: %w", name, ErrBadLog)
		}
		picked = append(picked, files[i])
	}
	return picked, nil
}
