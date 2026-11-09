package service

import (
	"testing"
	"time"
)

func TestChangesSendAtMostOncePerEveryWithTheNewestVersion(t *testing.T) {
	c := &Changes{Every: 50 * time.Millisecond}
	versions, stop := c.Subscribe()
	defer stop()
	for _, v := range []int64{1, 1, 2, 3} {
		c.Publish(v)
	}
	if v := <-versions; v != 1 {
		t.Fatalf("first notice %d, want 1 right away", v)
	}
	select {
	case v := <-versions:
		if v != 3 {
			t.Fatalf("second notice %d, want the newest, 3", v)
		}
	case <-time.After(time.Second):
		t.Fatal("no notice for the changes after the first one")
	}
	select {
	case v := <-versions:
		t.Fatalf("an extra notice %d", v)
	case <-time.After(100 * time.Millisecond):
	}
}
