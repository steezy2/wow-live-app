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
	// clickOverlay switches the overlay between taking clicks (a click on a player shows their abilities) and letting
	// them through to the game, like Overlay takes clicks on the Settings page and in the tray.
	clickOverlay = "CmdOrCtrl+Alt+K"
	// overlayLinger is how long the overlay stays up after a key ends.
	overlayLinger = time.Minute
	// overlayW and overlayH are the overlay's first size, overlayMargin how far it sits from the primary screen's top
	// right corner until it's moved.
	overlayW, overlayH, overlayMargin = 420, 340, 24
)

// files are what build.sh puts in: the built page (dist/), mplus.toml and raid-rules.toml (the rules),
// mplus-dungeons.json and raid-bosses.json (the reference files) and the tray's icon.ico (from cmd/appicon).
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
	opts.Cache, opts.DelayLog, opts.Baselines, opts.Realms, opts.WCL, opts.Now = logs.CacheDir,
		filepath.Join(data, "live-delay.log"), logs, logs, logs, time.Now
	opts.Settings = &controller.SettingsAPI{Settings: settings, Keys: keys}
	opts.LogsDir = func() string { return settings.Current().LogsDir }
	opts.Region, opts.Hosts = settings.Current().Region, []string{"wails.localhost"}
	ctx := context.Background()
	site, err := app.Start(ctx, opts)
	if err != nil {
		return err
	}
	if err := site.Import.Resume(); err != nil {
		log.Printf("going on with the Warcraft Logs import: %v", err)
	}
	if len(site.Keys.Saved()) == 0 { // a first run: the estimate needs earlier runs, and the log folder has them
		go func() {
			if _, _, err := site.Learn(false); err != nil {
				log.Printf("reading old logs: %v", err)
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
		return ok && key.Status == "running"
	}
	// Moving: the overlay takes clicks so it can be dragged and resized, and shows whatever the settings say so it
	// can be placed.
	var moving atomic.Bool
	// During keys counts the time in the key's dungeon before it starts too, for the ready check.
	readyCheck := func() bool {
		_, ok := site.Keys.Ready()
		return ok
	}
	// ...and between raid pulls, the last pull for service.RaidLinger.
	betweenPulls := func() bool { return site.Keys.BetweenPulls(time.Now().UnixMilli()) }
	up := func(show string) bool {
		return moving.Load() || show == "always" || show == "keys" && (running() || readyCheck() || betweenPulls())
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
	clickItem := menu.AddCheckbox("Overlay takes clicks", !settings.Current().Overlay.ClickThrough)
	// passClicks lets clicks through to the game unless the overlay is being moved or takes them.
	passClicks := func(o models.OverlaySettings) { overlay.SetIgnoreMouseEvents(o.ClickThrough && !moving.Load()) }
	setMoving := func(on bool) {
		moving.Store(on)
		moveItem.SetChecked(on)
		passClicks(settings.Current().Overlay)
		wails.Event.Emit("overlay-moving", on)
		showOrHide(settings.Current().Overlay.Show)
	}
	moveItem.OnClick(func(*application.Context) { setMoving(moveItem.Checked()) })
	setClicks := func(on bool) { // saved, so OnSave applies it and tells the pages
		next := settings.Current()
		next.Overlay.ClickThrough = !on
		if err := settings.Save(next); err != nil {
			log.Printf("saving whether the overlay takes clicks: %v", err)
		}
	}
	clickItem.OnClick(func(*application.Context) { setClicks(clickItem.Checked()) })
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
	following := filepath.Clean(opts.LogsDir()) // Settings saves the folder cleaned: compare like with like
	stopFollowing := follow(ctx, site, following)
	settings.OnSave = func(next models.Settings) {
		wails.Event.Emit("settings", next)
		showOrHide(next.Overlay.Show)
		clickItem.SetChecked(!next.Overlay.ClickThrough)
		passClicks(next.Overlay)
		if next.LogsDir == following {
			return // following the same folder again would read its log into the keys twice
		}
		stopFollowing()
		following, stopFollowing = next.LogsDir, follow(ctx, site, next.LogsDir)
	}
	keepPlace(wails, window, "main", settings, nil)
	keepPlace(wails, overlay, "overlay", settings, defaultPlace)
	overlay.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		go passClicks(settings.Current().Overlay)
	})
	// Once the app runs: a hotkey that clashes with another app's reaches this log (before that Wails only queues it).
	wails.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		showOrHide(settings.Current().Overlay.Show)
		if err := wails.GlobalShortcut.Register(moveOverlay, func() { setMoving(!moving.Load()) }); err != nil {
			log.Printf("no %s to move the overlay (use Settings or the tray): %v", moveOverlay, err)
		}
		if err := wails.GlobalShortcut.Register(clickOverlay, func() {
			setClicks(settings.Current().Overlay.ClickThrough)
		}); err != nil {
			log.Printf("no %s to switch the overlay's clicks (use Settings or the tray): %v", clickOverlay, err)
		}
	})
	versions, _ := site.Changes.Subscribe()
	go func() {
		var hide *time.Timer
		forRaid := false // the overlay is up for the last raid pull, until the next pull or key
		for v := range versions {
			wails.Event.Emit("change", v)
			switch {
			case up(settings.Current().Overlay.Show):
				if hide != nil {
					hide.Stop()
					hide = nil
				}
				overlay.Show()
				if forRaid = betweenPulls(); forRaid { // no change comes between pulls to take it down
					hide = time.AfterFunc(service.RaidLinger, func() { showOrHide(settings.Current().Overlay.Show) })
				}
			case forRaid: // a pull or a key started
				hide.Stop()
				hide, forRaid = nil, false
				showOrHide(settings.Current().Overlay.Show)
			case hide == nil && overlay.IsVisible():
				hide = time.AfterFunc(overlayLinger, func() { showOrHide(settings.Current().Overlay.Show) })
			}
		}
	}()
	return wails.Run()
}

// carried reads the page, the M+ and raid rules, the dungeon facts and the raid's boss mechanics built into the app.
func carried() (app.Options, error) {
	pages, err := fs.Sub(files, "embed/dist")
	if err != nil {
		return app.Options{}, err
	}
	opts := app.Options{Pages: pages}
	if err := config.DecodeTOML(files, "embed/"+config.RulesFile, &opts.Rules); err != nil {
		return opts, fmt.Errorf("the built-in M+ rules: %w", err)
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
	if err := config.DecodeTOML(files, "embed/"+config.RaidRulesFile, &opts.RaidRules); err != nil {
		return opts, fmt.Errorf("the built-in raid rules: %w", err)
	}
	bosses, err := files.ReadFile("embed/raid-bosses.json")
	if err != nil {
		return opts, err
	}
	var raid models.RaidReference
	if err := json.Unmarshal(bosses, &raid); err != nil {
		return opts, fmt.Errorf("reading the built-in raid bosses: %w", err)
	}
	opts.Bosses, opts.Tier = raid.Bosses, raid.Tier
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
