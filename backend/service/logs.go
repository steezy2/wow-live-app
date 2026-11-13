package service

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"wow-live/backend/models"
)

const (
	// dayMS is a day in ms, for the ages of logs.
	dayMS = 24 * 60 * 60_000
	// archiveAfterMS: a log last written this long ago (and read, and not active) is ticked for archiving at first.
	archiveAfterMS = 14 * dayMS
)

// ErrBusy is a read, archive or delete of the log folder asked for while another one runs.
var ErrBusy = errors.New("already working on the log folder")

// ErrBadLog is a log name the listing doesn't offer for that action (archive or delete).
var ErrBadLog = errors.New("no such log to do that to")

// LogFiles marks up the log folder's logs (Read set for those read), the archived ones and the Warcraft Logs imports
// for Settings: which the game may still write (the newest, and any written in the last quietMS), that an archived one
// is read (it was, before it was zipped), how many saved keys and pulls started in each (an import: are its fight, and
// it's read once one is saved), and which to tick at first. The folder's logs come first, then the archived ones, then
// the imports, each newest first.
func LogFiles(files []models.LogFile, keys []models.KeySummary, pulls []models.RaidPullSummary,
	nowMS int64) []models.LogFile {
	files = slices.Clone(files)
	slices.SortFunc(files, func(a, b models.LogFile) int {
		return cmp.Or(cmp.Compare(logRank(a), logRank(b)), cmp.Compare(b.WrittenMS, a.WrittenMS))
	})
	newest := true
	for i := range files {
		f := &files[i]
		if logRank(*f) == 0 {
			f.Active = newest || nowMS-f.WrittenMS < quietMS
			newest = false
		}
		in := func(start int64, report *models.ReportLink) bool {
			if f.Source == models.LogSourceWCL {
				return report != nil && *report == *f.Report
			}
			return f.FirstMS != 0 && start >= f.FirstMS && start <= f.WrittenMS
		}
		f.Keys = countFunc(keys, func(k models.KeySummary) bool { return in(k.Start, k.Report) })
		f.Pulls = countFunc(pulls, func(p models.RaidPullSummary) bool { return in(p.Start, p.Report) })
		f.Read = f.Read || f.Archived || (f.Source == models.LogSourceWCL && f.Keys+f.Pulls > 0)
		f.Suggested = canArchive(*f) && f.Read && nowMS-f.WrittenMS >= archiveAfterMS
	}
	return files
}

// logRank orders the list: the log folder's logs, the archived ones, the imports.
func logRank(f models.LogFile) int {
	switch {
	case f.Source == models.LogSourceWCL:
		return 2
	case f.Archived:
		return 1
	}
	return 0
}

// canArchive is whether f is a log in the folder the game no longer writes.
func canArchive(f models.LogFile) bool {
	return f.Source == models.LogSourceGame && !f.Archived && !f.Active
}

// canDelete is whether f is an archived log or an import: the game's logs are archived before they're deleted.
func canDelete(f models.LogFile) bool {
	return f.Archived || f.Source == models.LogSourceWCL
}

// PickLogs is the listed logs of source (models.LogSourceGame or LogSourceWCL) that names asks for: for archiving only
// logs in the folder the game no longer writes, for deleting only archived ones and imports. Any other name is
// ErrBadLog, so a request never names a file of its own choosing.
func PickLogs(files []models.LogFile, source string, names []string, archive bool) ([]models.LogFile, error) {
	may := canDelete
	if archive {
		may = canArchive
	}
	picked := make([]models.LogFile, 0, len(names))
	for _, name := range names {
		i := slices.IndexFunc(files, func(f models.LogFile) bool { return f.Name == name && f.Source == source && may(f) })
		if i < 0 {
			return nil, fmt.Errorf("%q: %w", name, ErrBadLog)
		}
		picked = append(picked, files[i])
	}
	return picked, nil
}

// AutoPicks is what the auto rule does to the listed logs now: the game's logs to archive (read, no longer written,
// last written at least ArchiveGameLogsDays ago) and the imports to delete (whose key or pull is saved, of a fight at
// least DeleteImportsDays old). A rule of 0 days picks nothing.
func AutoPicks(files []models.LogFile, auto models.AutoSettings, nowMS int64) (archive, remove []models.LogFile) {
	old := func(f models.LogFile, days int) bool { return days > 0 && nowMS-f.WrittenMS >= int64(days)*dayMS }
	for _, f := range files {
		switch {
		case !f.Read:
		case canArchive(f) && old(f, auto.ArchiveGameLogsDays):
			archive = append(archive, f)
		case f.Source == models.LogSourceWCL && old(f, auto.DeleteImportsDays):
			remove = append(remove, f)
		}
	}
	return archive, remove
}
