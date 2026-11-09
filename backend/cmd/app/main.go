//go:build windows

// Command app is WoW Live, the desktop app: the live M+ page in its own window, reading the game's combat log on this
// PC, with nothing listening on the network. build.sh builds the page and copies what the
// app carries into embed/ before building it.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
	"github.com/zalando/go-keyring"
	"golang.org/x/sys/windows/registry"

	"wow-live/backend/app"
	"wow-live/backend/clients/wcl"
	"wow-live/backend/config"
	"wow-live/backend/controller"
	"wow-live/backend/models"
	"wow-live/backend/service"
)

const (
	// name is the app's name: its window, tray and data folder.
	name = "WoW Live"
	// moveOverlay switches the overlay between letting clicks through to the game and taking them (to move and resize
	// it), like Move overlay on the Settings page and in the tray.
	moveOverlay = "CmdOrCtrl+Alt+O" // not Ctrl+Shift+O, which Radeon Software takes for its own overlay
	// overlayLinger is how long the overlay stays up after a key ends.
	overlayLinger = time.Minute
	// overlayW and overlayH are the overlay's first size, overlayMargin how far it sits from the primary screen's top
	// right corner until it's moved.
	overlayW, overlayH, overlayMargin = 420, 340, 24
)

// files are what build.sh puts in: the built page (dist/), mplus.toml (the M+ rules), mplus-dungeons.json and the
// tray's icon.ico (from cmd/appicon).
//
//go:embed embed
var files embed.FS

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cache, err := os.UserCacheDir() // %LOCALAPPDATA%
	if err != nil {
		return err
	}
	data := filepath.Join(cache, name)
	if err := os.MkdirAll(data, 0o755); err != nil {
		return fmt.Errorf("making the data folder: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(data, "wow-live.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening the app's log: %w", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	opts, err := carried()
	if err != nil {
		return err
	}
	settings, err := app.LoadSettings(filepath.Join(data, "settings.json"),
		models.Settings{LogsDir: logsDir(), Region: "us", Overlay: models.OverlaySettings{Show: "keys", Opacity: 100}})
	if err != nil {
		return err
	}
	logs := &wcl.Client{CacheDir: filepath.Join(data, "cache")}
	keys := &service.WCLKeys{Store: credentialManager{}, Client: logs, Now: time.Now}
	if err := keys.Load(); err != nil {
		log.Printf("no key reviews until the Warcraft Logs keys are saved again: %v", err)
	}
	opts.Cache, opts.DelayLog, opts.Baselines, opts.Now = logs.CacheDir, filepath.Join(data, "live-delay.log"), logs,
		time.Now
	var site *app.Site
	opts.Settings = &controller.SettingsAPI{Settings: settings, Keys: keys,
		ReadOldKeys: func() (int, error) { return site.Learn() }}
	opts.LogsDir, opts.Region, opts.Hosts = settings.Current().LogsDir, settings.Current().Region,
		[]string{"wails.localhost"}
	ctx := context.Background()
	site, err = app.Start(ctx, opts)
	if err != nil {
		return err
	}
	if len(site.Keys.Saved()) == 0 { // a first run: the estimate needs earlier runs, and the log folder has them
		go func() {
			if _, err := site.Learn(); err != nil {
				log.Printf("reading old keys: %v", err)
			}
		}()
	}
	wails := application.New(application.Options{Name: name, Assets: application.AssetOptions{Handler: site.Handler},
		Windows: application.WindowsOptions{WebviewUserDataPath: filepath.Join(data, "webview")}})
	window := wails.Window.NewWithOptions(application.WebviewWindowOptions{Name: "main", Title: name, Width: 1400,
		Height: 900, URL: "/"})
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})
	show := func() {
		window.Show()
		window.Focus()
	}
	// See-through takes WebView2's composition hosting without Wails' frameless decorations, and click-through
	// switched on once the page is up: a window made click-through at creation is layered, which paints the page's
	// transparent parts white (tried Nov 8).
	overlay := wails.Window.NewWithOptions(application.WebviewWindowOptions{Name: "overlay", Title: name + " overlay",
		Width: overlayW, Height: overlayH, URL: "/overlay", Frameless: true, AlwaysOnTop: true, Hidden: true,
		BackgroundType: application.BackgroundTypeTransparent, BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		Windows: application.WindowsWindow{HiddenOnTaskbar: true, WebView2CompositionHosting: true,
			DisableFramelessWindowDecorations: true}})
	running := func() bool {
		key, ok := site.Keys.Latest(time.Now().UnixMilli())
		return ok && key.Status == "running" && !key.Quiet
	}
	// Moving: the overlay takes clicks so it can be dragged and resized, and shows whatever the settings say so it
	// can be placed.
	var moving atomic.Bool
	// During keys counts the time in the key's dungeon before it starts too, for the ready check.
	readyCheck := func() bool {
		_, ok := site.Keys.Ready()
		return ok
	}
	up := func(show string) bool {
		return moving.Load() || show == "always" || show == "keys" && (running() || readyCheck())
	}
	showOrHide := func(show string) {
		if up(show) {
			overlay.Show()
		} else {
			overlay.Hide()
		}
	}
	menu := wails.NewMenu()
	menu.Add("Open " + name).OnClick(func(*application.Context) { show() })
	moveItem := menu.AddCheckbox("Move or resize the overlay", false)
	setMoving := func(on bool) {
		moving.Store(on)
		moveItem.SetChecked(on)
		overlay.SetIgnoreMouseEvents(!on)
		wails.Event.Emit("overlay-moving", on)
		showOrHide(settings.Current().Overlay.Show)
	}
	moveItem.OnClick(func(*application.Context) { setMoving(moveItem.Checked()) })
	menu.Add("Quit").OnClick(func(*application.Context) { wails.Quit() })
	icon, err := files.ReadFile("embed/icon.ico")
	if err != nil {
		icon = icons.DefaultWindowsIcon
	}
	tray := wails.SystemTray.New().SetIcon(icon).SetMenu(menu).OnClick(show)
	tray.SetTooltip(name)

	// The Settings page's overlay buttons.
	wails.Event.On("overlay-move", func(e *application.CustomEvent) {
		if on, ok := e.Data.(bool); ok {
			setMoving(on)
		} else { // a page asking whether it's moving
			wails.Event.Emit("overlay-moving", moving.Load())
		}
	})
	defaultPlace := func() {
		area := wails.Screen.GetPrimary().WorkArea
		overlay.SetBounds(application.Rect{X: area.X + area.Width - overlayW - overlayMargin, Y: area.Y + overlayMargin,
			Width: overlayW, Height: overlayH})
	}
	wails.Event.On("overlay-reset", func(*application.CustomEvent) { defaultPlace() })
	following, stopFollowing := opts.LogsDir, follow(ctx, site, opts.LogsDir)
	settings.OnSave = func(next models.Settings) {
		wails.Event.Emit("settings", next)
		showOrHide(next.Overlay.Show)
		if next.LogsDir == following {
			return // following the same folder again would read its log into the keys twice
		}
		stopFollowing()
		following, stopFollowing = next.LogsDir, follow(ctx, site, next.LogsDir)
	}
	keepPlace(wails, window, "main", settings, nil)
	keepPlace(wails, overlay, "overlay", settings, defaultPlace)
	overlay.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		go overlay.SetIgnoreMouseEvents(!moving.Load())
	})
	// Once the app runs: a hotkey that clashes with another app's reaches this log (before that Wails only queues it).
	wails.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		showOrHide(settings.Current().Overlay.Show)
		if err := wails.GlobalShortcut.Register(moveOverlay, func() { setMoving(!moving.Load()) }); err != nil {
			log.Printf("no %s to move the overlay (use Settings or the tray): %v", moveOverlay, err)
		}
	})
	versions, _ := site.Changes.Subscribe()
	go func() {
		var hide *time.Timer
		for v := range versions {
			wails.Event.Emit("change", v)
			switch {
			case up(settings.Current().Overlay.Show):
				if hide != nil {
					hide.Stop()
					hide = nil
				}
				overlay.Show()
			case hide == nil && overlay.IsVisible():
				hide = time.AfterFunc(overlayLinger, func() { showOrHide(settings.Current().Overlay.Show) })
			}
		}
	}()
	return wails.Run()
}

// carried reads the page, the M+ rules and the dungeon facts built into the app.
func carried() (app.Options, error) {
	pages, err := fs.Sub(files, "embed/dist")
	if err != nil {
		return app.Options{}, err
	}
	opts := app.Options{Pages: pages}
	if _, err := toml.DecodeFS(files, "embed/"+config.RulesFile, &opts.Rules); err != nil {
		return opts, fmt.Errorf("reading the built-in M+ rules: %w", err)
	}
	reference, err := files.ReadFile("embed/mplus-dungeons.json")
	if err != nil {
		return opts, err
	}
	var ref models.MPlusReference
	if err := json.Unmarshal(reference, &ref); err != nil {
		return opts, fmt.Errorf("reading the built-in dungeon facts: %w", err)
	}
	opts.Dungeons = ref.Dungeons
	return opts, nil
}

// follow reads the log folder in the background until the returned stop is called.
func follow(ctx context.Context, site *app.Site, dir string) (stop func()) {
	ctx, stop = context.WithCancel(ctx)
	go site.Follow(ctx, dir)
	return stop
}

// logsDir is the game's combat log folder: under the install Battle.net registers, else the default install's.
func logsDir() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\World of Warcraft`, registry.QUERY_VALUE)
	if err != nil {
		return config.LogsDir()
	}
	defer key.Close()
	if dir, _, err := key.GetStringValue("InstallLocation"); err == nil && dir != "" {
		return filepath.Join(dir, "_retail_", "Logs")
	}
	return config.LogsDir()
}

// credentialManager keeps secrets in Windows Credential Manager, one entry per name under the app's name.
type credentialManager struct{}

func (credentialManager) Get(key string) (string, error) {
	secret, err := keyring.Get(name, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return secret, err
}

func (credentialManager) Set(key, secret string) error {
	return keyring.Set(name, key, secret)
}

func (credentialManager) Delete(key string) error {
	if err := keyring.Delete(name, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}
