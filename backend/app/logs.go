package app

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	"wow-live/backend/clients/combatlog"
	"wow-live/backend/models"
	"wow-live/backend/service"
)

// LogFiles lists the game's combat logs and the archived ones for Settings (service.LogFiles marks them up).
func (s *Site) LogFiles() ([]models.LogFile, error) {
	dir := s.opts.LogsDir()
	logs, err := combatlog.WrittenSince(dir, time.Time{})
	if err != nil {
		return nil, err
	}
	zips, err := combatlog.Archived(dir)
	if err != nil {
		return nil, err
	}
	learnt := readLearned(filepath.Join(s.opts.Cache, "learned.json"))
	var files []models.LogFile
	for _, path := range logs {
		entry, err := logEntry(path)
		if err != nil {
			return nil, err
		}
		files = append(files, models.LogFile{Name: entry.File, Path: path, Bytes: entry.Size, WrittenMS: entry.ModTime,
			FirstMS: combatlog.FirstMS(path), Read: learnt.has(entry)})
	}
	for _, z := range zips {
		files = append(files, models.LogFile{Name: z.Name, Path: z.Path, Bytes: z.Bytes,
			WrittenMS: z.Written.UnixMilli(), FirstMS: combatlog.FirstMS(z.Path), Archived: true})
	}
	var keyStarts, pullStarts []int64
	for _, k := range s.Keys.Saved("") {
		keyStarts = append(keyStarts, k.Start)
	}
	for _, night := range s.Keys.RaidNights("") {
		for _, p := range night.Pulls {
			pullStarts = append(pullStarts, p.Start)
		}
	}
	return service.LogFiles(files, keyStarts, pullStarts, s.opts.Now().UnixMilli()), nil
}

// ArchiveLogs zips the named logs into the log folder's archive (reading the folder first when one isn't read yet,
// so no key or pull is lost) on a pool of goroutines, and lists the logs again; a log that couldn't be archived
// carries its error. service.ErrBadLog for a name the list doesn't offer, service.ErrBusy while another log job runs.
func (s *Site) ArchiveLogs(names []string) ([]models.LogFile, error) {
	return s.logJobOn(names, false, combatlog.Archive)
}

// DeleteArchivedLogs removes the named archived logs for good and lists the logs again; errors as ArchiveLogs.
func (s *Site) DeleteArchivedLogs(names []string) ([]models.LogFile, error) {
	return s.logJobOn(names, true, os.Remove)
}

// logJobOn does do to each named log (archived ones when archived), holding the log job's lock; a log in the folder
// not read yet is read first.
func (s *Site) logJobOn(names []string, archived bool, do func(path string) error) ([]models.LogFile, error) {
	if !s.logJob.TryLock() {
		return nil, service.ErrBusy
	}
	defer s.logJob.Unlock()
	files, err := s.LogFiles()
	if err != nil {
		return nil, err
	}
	picked, err := service.PickLogs(files, names, archived)
	if err != nil {
		return nil, err
	}
	if slices.ContainsFunc(picked, func(f models.LogFile) bool { return !f.Read }) {
		if _, _, err := s.learn(false); err != nil {
			return nil, err
		}
	}
	failed := make([]error, len(picked))
	var wg sync.WaitGroup
	slots := make(chan struct{}, max(runtime.NumCPU()-1, 1))
	for i, f := range picked {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			failed[i] = do(f.Path)
		})
	}
	wg.Wait()
	if files, err = s.LogFiles(); err != nil {
		return nil, err
	}
	for i, f := range picked {
		if failed[i] == nil {
			continue
		}
		log.Print(failed[i])
		for j := range files {
			if files[j].Name == f.Name && files[j].Archived == archived {
				files[j].Error = failed[i].Error()
			}
		}
	}
	return files, nil
}
