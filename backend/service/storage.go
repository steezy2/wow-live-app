package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FolderBytes is the size of every file under dir, walked now (no caching); a missing dir is 0.
func FolderBytes(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err == nil {
			total += info.Size()
		}
		return err
	})
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("measuring %s: %w", dir, err)
	}
	return total, nil
}

// FilesBytes is the size of the files at paths, together.
func FilesBytes(paths []string) (int64, error) {
	var total int64
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return 0, fmt.Errorf("measuring %s: %w", path, err)
		}
		total += info.Size()
	}
	return total, nil
}
