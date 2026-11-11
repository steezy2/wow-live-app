package service

import (
	"errors"
	"testing"

	"wow-live/backend/models"
)

func TestLogFilesSuggested(t *testing.T) {
	const day = 24 * 60 * 60_000
	now := int64(100 * day)
	files := []models.LogFile{
		{Name: "old-read", WrittenMS: now - 20*day, FirstMS: now - 21*day},
		{Name: "old-unread", WrittenMS: now - 30*day, FirstMS: now - 31*day},
		{Name: "recent", WrittenMS: now - 3*day, FirstMS: now - 4*day},
		{Name: "newest", WrittenMS: now - day/2, FirstMS: now - day}, // Follow's
		{Name: "zipped", WrittenMS: now - 50*day, Archived: true},
	}
	files[3].WrittenMS = now - day/2
	read := func(name string) bool { return name != "old-unread" }
	got := LogFiles(files, read, []int64{now - 21*day + 5, now - 20*day + 1, now - 4*day}, []int64{now - 31*day + 1}, now)
	want := []struct {
		name                      string
		active, suggested, isRead bool
		keys, pulls               int
	}{
		{"newest", true, false, true, 0, 0},
		{"recent", false, false, true, 1, 0},
		{"old-read", false, true, true, 1, 0},
		{"old-unread", false, false, false, 0, 1},
		{"zipped", false, false, true, 0, 0},
	}
	for i, w := range want {
		f := got[i]
		if f.Name != w.name || f.Active != w.active || f.Suggested != w.suggested || f.Read != w.isRead ||
			f.Keys != w.keys || f.Pulls != w.pulls {
			t.Errorf("row %d = %+v, want %+v", i, f, w)
		}
	}
}

func TestPickLogs(t *testing.T) {
	files := []models.LogFile{{Name: "a.txt"}, {Name: "live.txt", Active: true}, {Name: "z.txt", Archived: true}}
	for _, tc := range []struct {
		names    []string
		archived bool
		ok       bool
	}{
		{[]string{"a.txt"}, false, true},
		{[]string{"z.txt"}, true, true},
		{[]string{"live.txt"}, false, false},
		{[]string{"z.txt"}, false, false},
		{[]string{"a.txt"}, true, false},
		{[]string{`..\a.txt`}, false, false},
		{[]string{`C:\Windows\a.txt`}, false, false},
		{[]string{"a.txt", "nope.txt"}, false, false},
	} {
		_, err := PickLogs(files, tc.names, tc.archived)
		if (err == nil) != tc.ok || (err != nil && !errors.Is(err, ErrBadLog)) {
			t.Errorf("PickLogs(%v, archived %v) = %v, want ok %v", tc.names, tc.archived, err, tc.ok)
		}
	}
}
