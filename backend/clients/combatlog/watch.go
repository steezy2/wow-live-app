package combatlog

import (
	"context"
	"fmt"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FallbackEvery is how often a Watcher wakes without a change event. Events from WoW's open log arrived on time on
// Nov 8, so this is only a safety net.
const FallbackEvery = 250 * time.Millisecond

// Watcher wakes a reader of the log folder as soon as the game writes to it, and also every Every in case an event
// is held back or lost.
type Watcher struct {
	Every time.Duration
	fs    *fsnotify.Watcher // nil when the folder couldn't be watched: Every alone wakes it
}

// Watch watches dir, the folder rather than the file, so a new log from /combatlog is seen too. If the folder can't
// be watched it returns the error with a Watcher that wakes every Every only.
func Watch(dir string, every time.Duration) (*Watcher, error) {
	w := &Watcher{Every: every}
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return w, fmt.Errorf("watching %s: %w", dir, err)
	}
	if err := fs.Add(dir); err != nil {
		fs.Close()
		return w, fmt.Errorf("watching %s: %w", dir, err)
	}
	w.fs = fs
	return w, nil
}

// Wait blocks until the folder changes or Every passes; false once ctx is done. Changes that queued up meanwhile
// count as one, since one read takes everything written so far.
func (w *Watcher) Wait(ctx context.Context) bool {
	var events <-chan fsnotify.Event
	var errs <-chan error
	if w.fs != nil {
		events, errs = w.fs.Events, w.fs.Errors
	}
	select {
	case <-ctx.Done():
		return false
	case <-events:
		for {
			select {
			case <-events:
			default:
				return true
			}
		}
	case <-errs:
	case <-time.After(w.Every):
	}
	return true
}

// Close stops watching.
func (w *Watcher) Close() error {
	if w.fs == nil {
		return nil
	}
	return w.fs.Close()
}
