// Package simc installs SimulationCraft's simc.exe, the engine the app runs sims with. It comes from this
// repository's own release (built weekly by .github/workflows/simc.yml, which takes the newest official Windows nightly
// from simulationcraft/simc-publish and checks it against GitHub's digest), over HTTPS and checked against the
// sha256 in the release's engine.json. simulationcraft.org's own list is plain HTTP with no checksum, so the app never
// uses it.
// The owner can also point the app at a simc.exe of their own (UseFile).
package simc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

const (
	// maxEngineBytes bounds the download and any simc.exe taken in (about 116 MB in November 2026).
	maxEngineBytes = 256 << 20
	// exeName is the program kept in Client.Dir.
	exeName = "simc.exe"
	// downloadTimeout bounds the whole download, a few minutes on a slow line.
	downloadTimeout = 15 * time.Minute
	// listTimeout bounds reading the manifest.
	listTimeout = 30 * time.Second
	// bannerTimeout bounds running the new program once: the first run of a new file can wait on a virus scan.
	bannerTimeout = 2 * time.Minute
	// releaseBase is where the weekly job publishes simc.exe and engine.json (a release with a fixed tag).
	releaseBase = "https://github.com/steezy2/wow-live-app/releases/download/simc-engine/"
)

// banner is the line simc prints when run without an input.
var banner = regexp.MustCompile(`SimulationCraft (\S+) for World of Warcraft (\S+)`)

// Client installs simc.exe into Dir, from Base (the release's address; this repository's when empty).
type Client struct {
	Dir  string
	Base string
}

// manifest is the release's engine.json: what the weekly job found out about the simc.exe it published.
type manifest struct {
	File    string `json:"file"` // the nightly's file name, which names its commit
	BuiltMS int64  `json:"builtMS"`
	SHA256  string `json:"sha256"`  // of simc.exe
	Version string `json:"version"` // what simc printed when the job ran it ("1210-01"), so the app need not
	Patch   string `json:"patch"`   // run the new file to know (a first run can wait on a virus scan)
}

func (c Client) base() string {
	if c.Base != "" {
		return c.Base
	}
	return releaseBase
}

// Path is where simc.exe is kept once installed.
func (c Client) Path() string { return filepath.Join(c.Dir, exeName) }

func (c Client) infoPath() string { return filepath.Join(c.Dir, "engine.json") }

// Installed is the engine in Dir; its Installed is false until Install or UseFile has run.
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

// Install downloads the release's simc.exe, checks its sha256 against the manifest's and that it is a program that
// names its version, and replaces the installed one with it.
func (c Client) Install(ctx context.Context) (models.SimEngine, error) {
	m, err := c.manifest(ctx)
	if err != nil {
		return models.SimEngine{}, err
	}
	return c.replace(ctx, func(dest string) error { return download(ctx, c.base()+exeName, dest, m.SHA256) },
		models.SimEngine{File: m.File, BuiltMS: m.BuiltMS, Version: m.Version, Patch: m.Patch})
}

// UseFile makes the simc.exe at src (a path the owner chose) the engine: it is copied into Dir and must be a program
// that names its version. Its build time is the file's.
func (c Client) UseFile(ctx context.Context, src string) (models.SimEngine, error) {
	info, err := os.Stat(src)
	if err != nil || !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(src), ".exe") {
		return models.SimEngine{}, fmt.Errorf("%s is not a program (.exe) file", src)
	}
	return c.replace(ctx, func(dest string) error { return copyFile(src, dest) },
		models.SimEngine{File: filepath.Base(src), BuiltMS: info.ModTime().UnixMilli()})
}

// replace puts the program fetch writes in place of the installed one, once it checks out, and saves what is known of
// it (engine's File and BuiltMS, plus its size and, unless engine already has them, the version and patch it prints).
func (c Client) replace(ctx context.Context, fetch func(dest string) error,
	engine models.SimEngine) (models.SimEngine, error) {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return models.SimEngine{}, fmt.Errorf("making %s: %w", c.Dir, err)
	}
	next := filepath.Join(c.Dir, "simc-new.exe")
	defer os.Remove(next)
	if err := fetch(next); err != nil {
		return models.SimEngine{}, err
	}
	if err := checkProgram(next); err != nil {
		return models.SimEngine{}, err
	}
	if engine.Version == "" {
		var err error
		if engine.Version, engine.Patch, err = runBanner(ctx, next); err != nil {
			return models.SimEngine{}, fmt.Errorf("checking %s: %w", engine.File, err)
		}
	}
	if err := os.Rename(next, c.Path()); err != nil {
		return models.SimEngine{}, fmt.Errorf("replacing %s (is a sim running?): %w", c.Path(), err)
	}
	info, err := os.Stat(c.Path())
	if err != nil {
		return models.SimEngine{}, fmt.Errorf("measuring %s: %w", c.Path(), err)
	}
	engine.Installed, engine.Size = true, info.Size()
	if err := cachefile.WriteJSON(c.infoPath(), engine); err != nil {
		return models.SimEngine{}, err
	}
	return engine, nil
}

// manifest reads the release's engine.json.
func (c Client) manifest(ctx context.Context) (manifest, error) {
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	body, err := get(ctx, c.base()+"engine.json")
	if err != nil {
		return manifest{}, err
	}
	defer body.Close()
	var m manifest
	if err := json.NewDecoder(io.LimitReader(body, 1<<16)).Decode(&m); err != nil {
		return manifest{}, fmt.Errorf("reading the engine's manifest: %w", err)
	}
	if len(m.SHA256) != sha256.Size*2 || m.File == "" {
		return manifest{}, errors.New("the engine's manifest names no file or checksum")
	}
	return m, nil
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

// download streams url into dest, refusing a file over maxEngineBytes or whose sha256 is not wantSHA.
func download(ctx context.Context, url, dest, wantSHA string) error {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	body, err := get(ctx, url)
	if err != nil {
		return err
	}
	defer body.Close()
	sum, err := writeLimited(body, dest)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	if !strings.EqualFold(sum, wantSHA) {
		return fmt.Errorf("downloading %s: its sha256 is %s, not the manifest's %s", url, sum, wantSHA)
	}
	return nil
}

// copyFile copies the program at src to dest, refusing one over maxEngineBytes.
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer in.Close()
	if _, err := writeLimited(in, dest); err != nil {
		return fmt.Errorf("copying %s: %w", src, err)
	}
	return nil
}

// writeLimited writes r to dest (at most maxEngineBytes) and returns the sha256 of what it wrote, in hex.
func writeLimited(r io.Reader, dest string) (string, error) {
	f, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("creating %s: %w", dest, err)
	}
	sum := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, sum), io.LimitReader(r, maxEngineBytes+1))
	if err := errors.Join(copyErr, f.Close()); err != nil {
		return "", err
	}
	if n > maxEngineBytes {
		return "", fmt.Errorf("over %d MB", maxEngineBytes>>20)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
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

// bannerWaits is how long runBanner waits before each new try: the first run of a new file can be held up by a virus
// scan.
var bannerWaits = []time.Duration{3 * time.Second, 10 * time.Second, 30 * time.Second}

// runBanner runs exe without an input, which makes simc print its version and game patch, and returns them. It tries
// again after each of bannerWaits.
func runBanner(ctx context.Context, exe string) (version, patch string, err error) {
	for attempt := 1; ; attempt++ {
		started := time.Now()
		out, runErr, timedOut := bannerOutput(ctx, exe)
		version, patch, ok := parseBanner(out)
		if ok {
			return version, patch, nil
		}
		err = fmt.Errorf("it printed no SimulationCraft version (attempt %d: %v after %s, timed out %v, it said %q)", attempt,
			runErr, time.Since(started).Round(time.Millisecond), timedOut, strings.TrimSpace(out[:min(len(out), 200)]))
		if attempt > len(bannerWaits) || ctx.Err() != nil {
			return "", "", err
		}
		time.Sleep(bannerWaits[attempt-1])
	}
}

// bannerOutput is what exe prints on its own (stdout and stderr), how it ended (nil when it exited cleanly) and
// whether bannerTimeout stopped it.
func bannerOutput(ctx context.Context, exe string) (out string, runErr error, timedOut bool) {
	ctx, cancel := context.WithTimeout(ctx, bannerTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe)
	cmd.Dir = filepath.Dir(exe)
	HideWindow(cmd)
	raw, runErr := cmd.CombinedOutput() // it may exit with an error: there is nothing to sim
	return string(raw), runErr, ctx.Err() != nil
}

// parseBanner is the version ("1210-01") and patch ("12.1.5.70529") of simc's first line.
func parseBanner(out string) (version, patch string, ok bool) {
	m := banner.FindStringSubmatch(out)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}
