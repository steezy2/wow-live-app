//go:build windows

package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"wow-live/backend/app"
)

func TestASavedWindowGoesBackOnlyWhileEnoughOfItIsOnAScreen(t *testing.T) {
	screens := []application.Rect{{Width: 1920, Height: 1080}, {X: 1920, Width: 1280, Height: 720}}
	for _, tc := range []struct {
		name  string
		place app.Place
		want  application.Rect
		ok    bool
	}{
		{"on the first screen", app.Place{X: 100, Y: 100, W: 420, H: 340},
			application.Rect{X: 100, Y: 100, Width: 420, Height: 340}, true},
		{"on the second screen, bigger than it", app.Place{X: 2000, Y: 0, W: 1400, H: 900},
			application.Rect{X: 2000, Width: 1280, Height: 720}, true},
		{"mostly off the edge", app.Place{X: 1880, Y: 900, W: 420, H: 340}, application.Rect{}, false},
		{"on a monitor that's gone", app.Place{X: -1500, Y: 0, W: 420, H: 340}, application.Rect{}, false},
	} {
		if got, ok := fit(tc.place, screens); got != tc.want || ok != tc.ok {
			t.Errorf("%s: %+v %v, want %+v %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestIsReviewURL(t *testing.T) {
	for url, want := range map[string]bool{
		"/review/mplus/1762800000000?tab=events": true,
		"/review/raid/1762800000000":             true,
		"/overlay":                               false,
		"https://example.com/review/x":           false,
		"/review//evil.example":                  false,
		"/review/../settings":                    false,
	} {
		if got := isReviewURL(url); got != want {
			t.Errorf("isReviewURL(%q) = %v, want %v", url, got, want)
		}
	}
}
