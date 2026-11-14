// Package simc finds and installs SimulationCraft's simc.exe, the engine the app runs sims with: the newest Windows
// nightly from simulationcraft.org (a 7z of about 115 MB), of which only simc.exe is kept.
package simc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"time"

	"github.com/bodgit/sevenzip"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

const (
	// maxEngineBytes bounds the download and the extracted simc.exe (115 MB and 116 MB in November 2026). It's above
	// cachefile.MaxDownload, which reads a body into memory; this one is streamed to disk.
	maxEngineBytes = 256 << 20
	// exeName is the program kept in Client.Dir.
	exeName = "simc.exe"
	// downloadTimeout bounds the whole download, a few minutes on a slow line.
	downloadTimeout = 15 * time.Minute
	// listTimeout bounds reading the list of nightlies and running the new program once.
	listTimeout = 30 * time.Second
)

var (
	// nightly is one row of the nightly list: a Windows build's file name and its time (UTC).
	nightly = regexp.MustCompile(`<a href="(simc-[0-9a-z.]+-win64\.7z)">[^<]*</a></td><td align="right">(\d{4}-\d\d-\d\d \d\d:\d\d)`)
	// banner is the line simc prints when run without an input.
	banner = regexp.MustCompile(`SimulationCraft (\S+) for World of Warcraft (\S+)`)
)

// Client installs simc.exe into Dir, from Base (the nightly list; simulationcraft.org's when empty). The site serves
// plain HTTP only (its HTTPS certificate names another host) and lists no checksum, so Install checks what it can
// of the file itself.
type Client struct {
	Dir  string
	Base string
}

// Build is a nightly on offer: its file name, address and build time (ms since the epoch).
type Build struct {
	File    string
	URL     string
	BuiltMS int64
}

func (c Client) base() string {
	if c.Base != "" {
		return c.Base
	}
	return "http://downloads.simulationcraft.org/nightly/"
}

// Path is where simc.exe is kept once installed.
func (c Client) Path() string { return filepath.Join(c.Dir, exeName) }

func (c Client) infoPath() string { return filepath.Join(c.Dir, "engine.json") }

// Installed is the engine in Dir; its Installed is false until Install has run.
func (c Client) Installed() models.SimEngine {
	var e models.SimEngine
	if _, err := os.Stat(c.Path()); err != nil {
		return e
	}
	if data, err := os.ReadFile(c.infoPath()); err == nil {
		_ = json.Unmarshal(data, &e)
	}
	e.Installed = true
	return e
}

// Latest is the newest Windows nightly on the list (the list is sorted newest first).
func (c Client) Latest(ctx context.Context) (Build, error) {
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	body, err := get(ctx, c.base()+"?C=M;O=D")
	if err != nil {
		return Build{}, fmt.Errorf("reading the nightly list: %w", err)
	}
	defer body.Close()
	page, err := io.ReadAll(io.LimitReader(body, 1<<20))
	if err != nil {
		return Build{}, fmt.Errorf("reading the nightly list: %w", err)
	}
	return parseNightly(string(page), c.base())
}

// parseNightly is the first Windows build in the list's page.
func parseNightly(page, base string) (Build, error) {
	m := nightly.FindStringSubmatch(page)
	if m == nil {
		return Build{}, errors.New("no Windows build in the nightly list")
	}
	built, err := time.Parse("2006-01-02 15:04", m[2])
	if err != nil {
		return Build{}, fmt.Errorf("reading the time of %s: %w", m[1], err)
	}
	return Build{File: m[1], URL: base + m[1], BuiltMS: built.UnixMilli()}, nil
}

func get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", cachefile.UserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("downloading %s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

// Install downloads the newest nightly, takes simc.exe out of it, checks that it is a program that names its
// version and replaces the installed one with it. The 7z is deleted either way.
func (c Client) Install(ctx context.Context) (models.SimEngine, error) {
	build, err := c.Latest(ctx)
	if err != nil {
		return models.SimEngine{}, err
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return models.SimEngine{}, fmt.Errorf("making %s: %w", c.Dir, err)
	}
	archive := filepath.Join(c.Dir, "engine.7z.part")
	defer os.Remove(archive)
	if err := download(ctx, build.URL, archive); err != nil {
		return models.SimEngine{}, err
	}
	next := filepath.Join(c.Dir, "simc-new.exe")
	defer os.Remove(next)
	if err := extractExe(archive, next); err != nil {
		return models.SimEngine{}, fmt.Errorf("unpacking %s: %w", build.File, err)
	}
	version, patch, err := runBanner(ctx, next)
	if err != nil {
		return models.SimEngine{}, fmt.Errorf("checking %s: %w", build.File, err)
	}
	if err := os.Rename(next, c.Path()); err != nil {
		return models.SimEngine{}, fmt.Errorf("replacing %s (is a sim running?): %w", c.Path(), err)
	}
	info, err := os.Stat(c.Path())
	if err != nil {
		return models.SimEngine{}, fmt.Errorf("measuring %s: %w", c.Path(), err)
	}
	engine := models.SimEngine{Installed: true, File: build.File, Version: version, Patch: patch, BuiltMS: build.BuiltMS,
		Size: info.Size()}
	if err := cachefile.WriteJSON(c.infoPath(), engine); err != nil {
		return models.SimEngine{}, err
	}
	return engine, nil
}

// download streams url into dest, refusing a file over maxEngineBytes.
func download(ctx context.Context, url, dest string) error {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	body, err := get(ctx, url)
	if err != nil {
		return err
	}
	defer body.Close()
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dest, err)
	}
	n, copyErr := io.Copy(f, io.LimitReader(body, maxEngineBytes+1))
	if err := errors.Join(copyErr, f.Close()); err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	if n > maxEngineBytes {
		return fmt.Errorf("downloading %s: over %d MB", url, maxEngineBytes>>20)
	}
	return nil
}

// extractExe writes simc.exe from the 7z at archive to dest.
func extractExe(archive, dest string) error {
	r, err := sevenzip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("opening the 7z: %w", err)
	}
	defer r.Close()
	return copyExe(r, dest)
}

// copyExe copies the first simc.exe in fsys (at its top or one folder down) to dest, as a program (an "MZ" header)
// of at most maxEngineBytes. dest is the only file written, so no name inside the archive decides where anything goes.
func copyExe(fsys fs.FS, dest string) error {
	var found string
	_ = fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return fs.SkipAll
		}
		if !d.IsDir() && path.Base(name) == exeName && path.Dir(path.Dir(name)) == "." {
			found = name
		}
		return nil
	})
	if found == "" {
		return fmt.Errorf("no %s inside", exeName)
	}
	src, err := fsys.Open(found)
	if err != nil {
		return fmt.Errorf("opening %s: %w", found, err)
	}
	defer src.Close()
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dest, err)
	}
	n, copyErr := io.Copy(out, io.LimitReader(src, maxEngineBytes+1))
	if err := errors.Join(copyErr, out.Close()); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	if n > maxEngineBytes {
		return fmt.Errorf("%s is over %d MB", exeName, maxEngineBytes>>20)
	}
	return checkProgram(dest)
}

// checkProgram is nil when the file starts with "MZ", as every Windows program does.
func checkProgram(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("opening %s: %w", file, err)
	}
	defer f.Close()
	head := make([]byte, 2)
	if _, err := io.ReadFull(f, head); err != nil || string(head) != "MZ" {
		return fmt.Errorf("%s is not a Windows program", exeName)
	}
	return nil
}

// runBanner runs exe without an input, which makes simc print its version and game patch, and returns them.
func runBanner(ctx context.Context, exe string) (version, patch string, err error) {
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, exe).CombinedOutput() // it exits with an error: there is nothing to sim
	version, patch, ok := parseBanner(string(out))
	if !ok {
		return "", "", errors.New("it printed no SimulationCraft version")
	}
	return version, patch, nil
}

// parseBanner is the version ("1210-01") and patch ("12.1.5.70529") of simc's first line.
func parseBanner(out string) (version, patch string, ok bool) {
	m := banner.FindStringSubmatch(out)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}
