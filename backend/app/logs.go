package app

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	"wow-live/backend/clients/combatlog"
	"wow-live/backend/clients/wcl"
	"wow-live/backend/models"
	"wow-live/backend/service"
)

// LogFiles lists the game's combat logs, the archived ones and the Warcraft Logs imports for Settings
// (service.LogFiles marks them up).
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
	imports, err := s.imports()
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
		files = append(files, models.LogFile{Name: entry.File, Source: models.LogSourceGame, Path: path,
			Bytes: entry.Size, WrittenMS: entry.ModTime, FirstMS: combatlog.FirstMS(path), Read: learnt.has(entry)})
	}
	for _, z := range zips {
		files = append(files, models.LogFile{Name: z.Name, Source: models.LogSourceGame, Path: z.Path, Bytes: z.Bytes,
			WrittenMS: z.Written.UnixMilli(), FirstMS: combatlog.FirstMS(z.Path), Archived: true})
	}
	files = append(files, imports...)
	var pulls []models.RaidPullSummary
	for _, night := range s.Keys.RaidNights("") {
		pulls = append(pulls, night.Pulls...)
	}
	return service.LogFiles(files, s.Keys.Saved(""), pulls, s.opts.Now().UnixMilli()), nil
}

// imports lists the Warcraft Logs fights imported as zips (wcl.ImportFight), dated at the fight's end; a zip that
// isn't one readable fight is left out (and logged).
func (s *Site) imports() ([]models.LogFile, error) {
	paths, err := filepath.Glob(filepath.Join(s.opts.Cache, wcl.ImportDir, "*.zip"))
	if err != nil {
		return nil, err
	}
	var files []models.LogFile
	for _, path := range paths {
		link, ok := wcl.ImportedFight(path)
		if !ok {
			continue
		}
		z, err := combatlog.ReadZip(path)
		if err != nil {
			log.Printf("leaving out an imported fight: %v", err)
			continue
		}
		files = append(files, models.LogFile{Name: z.Name, Source: models.LogSourceWCL, Report: &link, Path: path,
			Bytes: z.Bytes, WrittenMS: z.Written.UnixMilli()})
	}
	return files, nil
}

// Storage measures the saved keys and raid pulls, the Warcraft Logs imports, the archived logs and the game's logs.
func (s *Site) Storage() (models.Storage, error) {
	dir := s.opts.LogsDir()
	storage := models.Storage{}
	for _, f := range []struct{ name, dir string }{
		{"localKeys", filepath.Join(s.opts.Cache, localKeysDir)},
		{"raidPulls", filepath.Join(s.opts.Cache, raidPullsDir)},
		{"wclImport", filepath.Join(s.opts.Cache, wcl.ImportDir)},
		{"archive", filepath.Join(dir, combatlog.ArchiveDir)},
	} {
		bytes, err := service.FolderBytes(f.dir)
		if err != nil {
			return models.Storage{}, err
		}
		storage.Folders = append(storage.Folders, models.StorageFolder{Name: f.name, Bytes: bytes})
	}
	logs, err := combatlog.WrittenSince(dir, time.Time{})
	if err != nil {
		return models.Storage{}, err
	}
	bytes, err := service.FilesBytes(logs)
	if err != nil {
		return models.Storage{}, err
	}
	storage.Folders = append(storage.Folders, models.StorageFolder{Name: "gameLogs", Bytes: bytes})
	return storage, nil
}

// ArchiveLogs zips the named game logs into the log folder's archive (reading the folder first when one isn't read
// yet, so no key or pull is lost) on a pool of goroutines, and lists the logs again; a log that couldn't be archived
// carries its error. service.ErrBadLog for a name the list doesn't offer (imports are never archived),
// service.ErrBusy while another log job runs.
func (s *Site) ArchiveLogs(source string, names []string) ([]models.LogFile, error) {
	return s.logJobOn(source, names, true)
}

// DeleteLogs removes the named archived logs or imports for good (their saved keys and pulls stay) and lists the logs
// again; errors as ArchiveLogs.
func (s *Site) DeleteLogs(source string, names []string) ([]models.LogFile, error) {
	return s.logJobOn(source, names, false)
}

// logJobOn archives (or deletes) each named log of source, holding the log job's lock; a log in the folder not read
// yet is read first.
func (s *Site) logJobOn(source string, names []string, archive bool) ([]models.LogFile, error) {
	if !s.logJob.TryLock() {
		return nil, service.ErrBusy
	}
	defer s.logJob.Unlock()
	files, err := s.LogFiles()
	if err != nil {
		return nil, err
	}
	picked, err := service.PickLogs(files, source, names, archive)
	if err != nil {
		return nil, err
	}
	if slices.ContainsFunc(picked, func(f models.LogFile) bool { return !f.Read && f.Source == models.LogSourceGame }) {
		if _, _, err := s.learn(false); err != nil {
			return nil, err
		}
	}
	do := os.Remove
	if archive {
		do = combatlog.Archive
	}
	failed := onEach(picked, do)
	if files, err = s.LogFiles(); err != nil {
		return nil, err
	}
	for i, f := range picked {
		if failed[i] == nil {
			continue
		}
		log.Print(failed[i])
		for j := range files {
			if files[j].Name == f.Name && files[j].Source == f.Source && files[j].Archived == f.Archived {
				files[j].Error = failed[i].Error()
			}
		}
	}
	return files, nil
}

// onEach does do to each file's path on a pool of goroutines, and says how each went.
func onEach(files []models.LogFile, do func(path string) error) []error {
	failed := make([]error, len(files))
	var wg sync.WaitGroup
	slots := make(chan struct{}, max(runtime.NumCPU()-1, 1))
	for i, f := range files {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			failed[i] = do(f.Path)
		})
	}
	wg.Wait()
	return failed
}

// Tidy runs the settings' auto rule (service.AutoPicks): archives the old game logs and deletes the old imports it
// picks. It does nothing without settings, with the rule off, or while another log job runs (the next key or pull
// that ends tries again).
func (s *Site) Tidy() error {
	if s.opts.Settings == nil {
		return nil
	}
	auto := s.opts.Settings.Settings.Get().Auto
	if auto == (models.AutoSettings{}) || !s.logJob.TryLock() {
		return nil
	}
	defer s.logJob.Unlock()
	files, err := s.LogFiles()
	if err != nil {
		return err
	}
	archive, remove := service.AutoPicks(files, auto, s.opts.Now().UnixMilli())
	if len(archive)+len(remove) == 0 {
		return nil
	}
	log.Printf("auto rule: archiving %d logs, deleting %d imports", len(archive), len(remove))
	return errors.Join(append(onEach(archive, combatlog.Archive), onEach(remove, os.Remove)...)...)
}
