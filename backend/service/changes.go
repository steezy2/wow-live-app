package service

import (
	"sync"
	"time"
)

// Changes tells the pages that the keys changed, at most once per Every so a burst of lines is one notice; a change
// within Every of the last notice goes out when Every has passed. Each subscriber holds only the newest version, so
// a slow page never holds up the log reader.
type Changes struct {
	Every   time.Duration
	mu      sync.Mutex
	version int64
	sentAt  time.Time
	pending bool
	subs    map[chan int64]struct{}
}

// Subscribe returns a channel of versions and the function that ends the subscription.
func (c *Changes) Subscribe() (<-chan int64, func()) {
	ch := make(chan int64, 1)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subs == nil {
		c.subs = map[chan int64]struct{}{}
	}
	c.subs[ch] = struct{}{}
	return ch, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.subs, ch)
	}
}

// Publish notes the keys' version (LogTracker.Version) and tells the subscribers if it's new.
func (c *Changes) Publish(version int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if version == c.version {
		return
	}
	c.version = version
	if c.pending {
		return
	}
	if wait := c.Every - time.Since(c.sentAt); wait > 0 {
		c.pending = true
		time.AfterFunc(wait, func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.pending = false
			c.send()
		})
		return
	}
	c.send()
}

// send gives every subscriber the current version in place of one it hasn't read yet. Called with mu held.
func (c *Changes) send() {
	c.sentAt = time.Now()
	for ch := range c.subs {
		select {
		case <-ch:
		default:
		}
		ch <- c.version
	}
}
