//go:build windows

package main

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"wow-live/backend/app"
)

const (
	// minOnScreenW and minOnScreenH: this much of a saved window (in Windows' scaled pixels) has to be on a screen for
	// it to go back there, else it goes to its default place (a monitor unplugged, or the resolution changed).
	minOnScreenW, minOnScreenH = 100, 60
	// saveAfter: a window's place is saved once it has stood still this long, so a drag writes the file once.
	saveAfter = 500 * time.Millisecond
)

// fit is where a window saved at p goes: p, no bigger than the first screen enough of it is on; false when too little
// of it is on any screen.
func fit(p app.Place, screens []application.Rect) (application.Rect, bool) {
	r := application.Rect{X: p.X, Y: p.Y, Width: p.W, Height: p.H}
	for _, s := range screens {
		if in := r.Intersect(s); in.Width >= minOnScreenW && in.Height >= minOnScreenH {
			r.Width, r.Height = min(r.Width, s.Width), min(r.Height, s.Height)
			return r, true
		}
	}
	return application.Rect{}, false
}

// keepPlace puts window back where the settings last had it once its page is up (Wails drops a move before the
// window exists), or calls fallback when that's no longer on a screen (nil leaves it where Wails put it). From then on
// it saves the window's place each time it's moved or resized, once it stands still, except while it's minimised
// (Windows parks it off screen) or maximised (its size then is the screen's, not the one to come back to).
func keepPlace(wails *application.App, window *application.WebviewWindow, name string, settings *app.Settings,
	fallback func()) {
	save := time.AfterFunc(time.Hour, func() {
		if window.IsMinimised() || window.IsMaximised() {
			return
		}
		b := window.Bounds()
		if err := settings.SavePlace(name, app.Place{X: b.X, Y: b.Y, W: b.Width, H: b.Height}); err != nil {
			log.Printf("saving where the %s window is: %v", name, err)
		}
	})
	save.Stop()
	var once sync.Once
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		go once.Do(func() {
			restore(window, name, settings, wails.Screen.GetAll(), fallback)
			for _, e := range []events.WindowEventType{events.Common.WindowDidMove, events.Common.WindowDidResize} {
				window.OnWindowEvent(e, func(*application.WindowEvent) { save.Reset(saveAfter) })
			}
		})
	})
}

// restore puts window back where the settings last had it, if that's still on a screen, else calls fallback.
func restore(window *application.WebviewWindow, name string, settings *app.Settings, screens []*application.Screen,
	fallback func()) {
	bounds := make([]application.Rect, len(screens))
	for i, s := range screens {
		bounds[i] = s.Bounds
	}
	if p, ok := settings.Place(name); ok {
		if r, ok := fit(p, bounds); ok {
			window.SetBounds(r)
			return
		}
	}
	if fallback != nil {
		fallback()
	}
}

// reviewPrefix starts the path of a full review page (/review/mplus/{start}, /review/raid/{start}).
const reviewPrefix = "/review/"

// isReviewURL is whether the page asked for a path of a full review page, so the 'open-review' event can't open
// anything else (or another site) in a window.
func isReviewURL(url string) bool {
	return strings.HasPrefix(url, reviewPrefix) && !strings.Contains(url, "//") && !strings.Contains(url, "..")
}

// popOut shows the full review page at url in a window of its own, one per url: the one already open is brought up.
func popOut(wails *application.App, title, url string) {
	name := "review " + url
	if open, ok := wails.Window.GetByName(name); ok {
		open.Show()
		open.Focus()
		return
	}
	wails.Window.NewWithOptions(application.WebviewWindowOptions{Name: name, Title: title + " review", Width: 1300,
		Height: 900, URL: url})
}
