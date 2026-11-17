// Package cachefile writes the cache files atomically, as JSON without HTML escaping, and downloads files into the
// cache. Every Go writer of the cache goes through it.
package cachefile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ErrNotFound is returned (wrapped) by Fresh when the server has no such file.
var ErrNotFound = errors.New("not found")

// MaxDownload bounds one response body read by ReadBody: the largest file the app fetches (a raid map, a Warcraft
// Logs answer) is a few MB, so a bigger one is a broken or hostile server, not data to keep.
const MaxDownload = 64 << 20

// ErrTooBig is returned (wrapped) by ReadBody for a body over MaxDownload.
var ErrTooBig = errors.New("response over 64 MB")

// ReadBody reads a response body of at most MaxDownload bytes.
func ReadBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, MaxDownload+1))
	if err == nil && len(data) > MaxDownload {
		err = ErrTooBig
	}
	return data, err
}

// renameTries and renameWait: renameRetrying's tries when another process has the file open (Windows).
const (
	renameTries = 5
	renameWait  = 50 * time.Millisecond
)

// WriteJSON saves value as JSON at path atomically (a temp file renamed over it), so the other processes sharing
// the cache never read it half-written. On Windows the rename fails while a reader has the file open, so it's
// retried a few times; the temp file is removed if anything fails.
func WriteJSON(path string, value any) error {
	data, err := Marshal(value)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	return writeBytes(path, data)
}

// writeBytes saves data at path atomically, making the folder.
func writeBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("making %s: %w", filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating a temp file for %s: %w", path, err)
	}
	_, writeErr := tmp.Write(data)
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("writing %s: %w", tmp.Name(), err)
	}
	if err := renameRetrying(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

// ReadAll reads every JSON file in dir as a T; a missing folder is none.
func ReadAll[T any](dir string) ([]T, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", dir, err)
	}
	all := []T{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		var v T
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		all = append(all, v)
	}
	return all, nil
}

// AppendLine adds one line to a running log (live-delay.log), making the folder.
func AppendLine(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("making %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	_, writeErr := fmt.Fprintln(f, line)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// renameRetrying renames from over to, retrying while another process has the target open.
func renameRetrying(from, to string) error {
	for attempt := 1; ; attempt++ {
		err := os.Rename(from, to)
		if err == nil || attempt == renameTries {
			return err
		}
		time.Sleep(renameWait)
	}
}

// Marshal is JSON without HTML escaping, so names with & or < stay readable in the files.
func Marshal(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// UserAgent is how the app names itself to the sites it calls.
const UserAgent = "wow-live (https://github.com/steezy2/wow-live-app)"

// downloadTimeout bounds one download when the caller gives no HTTP client.
const downloadTimeout = 2 * time.Minute

// Forever is the maxAge of a file that never changes (a map image, a tile).
const Forever = 100 * 365 * 24 * time.Hour

// ReadJSON reads the JSON file at path as a T; the error wraps os.ErrNotExist when the file is missing.
func ReadJSON[T any](path string) (T, error) {
	var v T
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &v)
	}
	if err != nil {
		return v, fmt.Errorf("reading %s: %w", path, err)
	}
	return v, nil
}

// Fresh returns url's content, kept at path: read from there when it's no older than maxAge, else downloaded
// (atomically) and kept. A stale copy stands in, with the failure logged, when the download fails but the site hasn't
// said the file is gone. client may be nil.
func Fresh(ctx context.Context, client *http.Client, url, path string, maxAge time.Duration) ([]byte, error) {
	info, statErr := os.Stat(path)
	if statErr == nil && time.Since(info.ModTime()) < maxAge {
		return readKept(path)
	}
	data, err := download(ctx, client, url, path)
	if err != nil && statErr == nil && !errors.Is(err, ErrNotFound) {
		log.Printf("using the stale %s: %v", filepath.Base(path), err)
		return readKept(path)
	}
	return data, err
}

// readKept reads the file Fresh keeps at path.
func readKept(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}

// download returns url's content and writes it to path, atomically.
func download(ctx context.Context, client *http.Client, url, path string) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: downloadTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("downloading %s: %w", url, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", url, resp.Status)
	}
	data, err := ReadBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	return data, writeBytes(path, data)
}
