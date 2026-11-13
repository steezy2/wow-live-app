package service

import (
	"errors"
	"slices"
	"testing"

	"wow-live/backend/models"
)

func TestLogFilesBothSources(t *testing.T) {
	const day = 24 * 60 * 60_000
	now := int64(100 * day)
	fight, other := models.ReportLink{Code: "abc", FightID: 3}, models.ReportLink{Code: "abc", FightID: 4}
	game := models.LogSourceGame
	files := []models.LogFile{
		{Name: "import", Source: models.LogSourceWCL, Report: &fight, WrittenMS: now - 40*day},
		{Name: "import-unsaved", Source: models.LogSourceWCL, Report: &other, WrittenMS: now - 2*day},
		{Name: "old-read", Source: game, WrittenMS: now - 20*day, FirstMS: now - 21*day, Read: true},
		{Name: "old-unread", Source: game, WrittenMS: now - 30*day, FirstMS: now - 31*day},
		{Name: "recent", Source: game, WrittenMS: now - 3*day, FirstMS: now - 4*day, Read: true},
		{Name: "newest", Source: game, WrittenMS: now - day/2, FirstMS: now - day, Read: true}, // Follow's
		{Name: "zipped", Source: game, WrittenMS: now - 50*day, Archived: true},
	}
	keys := []models.KeySummary{{Start: now - 21*day + 5}, {Start: now - 20*day + 1}, {Start: now - 4*day},
		{Start: now - 40*day - 1000, Report: &fight, Imported: true}}
	pulls := []models.RaidPullSummary{{Start: now - 31*day + 1}, {Start: now - 40*day - 500, Report: &fight}}
	got := LogFiles(files, keys, pulls, now)
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
		{"import-unsaved", false, false, false, 0, 0},
		{"import", false, false, true, 1, 1},
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
	game, wcl := models.LogSourceGame, models.LogSourceWCL
	files := []models.LogFile{{Name: "a.txt", Source: game}, {Name: "live.txt", Source: game, Active: true},
		{Name: "z.txt", Source: game, Archived: true}, {Name: "f-1.txt", Source: wcl}}
	for _, tc := range []struct {
		source  string
		names   []string
		archive bool
		ok      bool
	}{
		{game, []string{"a.txt"}, true, true},
		{game, []string{"z.txt"}, false, true},
		{wcl, []string{"f-1.txt"}, false, true},
		{wcl, []string{"f-1.txt"}, true, false}, // imports are zips already
		{game, []string{"f-1.txt"}, false, false},
		{wcl, []string{"z.txt"}, false, false},
		{game, []string{"live.txt"}, true, false},
		{game, []string{"z.txt"}, true, false},
		{game, []string{"a.txt"}, false, false},
		{game, []string{`..\a.txt`}, true, false},
		{game, []string{`C:\Windows\a.txt`}, true, false},
		{wcl, []string{`..\..\settings.json`}, false, false},
		{game, []string{"a.txt", "nope.txt"}, true, false},
	} {
		_, err := PickLogs(files, tc.source, tc.names, tc.archive)
		if (err == nil) != tc.ok || (err != nil && !errors.Is(err, ErrBadLog)) {
			t.Errorf("PickLogs(%s %v, archive %v) = %v, want ok %v", tc.source, tc.names, tc.archive, err, tc.ok)
		}
	}
}

func TestAutoPicks(t *testing.T) {
	now := int64(100 * dayMS)
	game, wcl := models.LogSourceGame, models.LogSourceWCL
	files := []models.LogFile{
		{Name: "active", Source: game, WrittenMS: now - 40*dayMS, Read: true, Active: true},
		{Name: "unread", Source: game, WrittenMS: now - 40*dayMS},
		{Name: "old", Source: game, WrittenMS: now - 40*dayMS, Read: true},
		{Name: "young", Source: game, WrittenMS: now - 10*dayMS, Read: true},
		{Name: "zipped", Source: game, WrittenMS: now - 40*dayMS, Read: true, Archived: true},
		{Name: "import-unsaved", Source: wcl, WrittenMS: now - 40*dayMS},
		{Name: "import-old", Source: wcl, WrittenMS: now, ImportedMS: now - 40*dayMS, Read: true},
		{Name: "import-young", Source: wcl, WrittenMS: now - 90*dayMS, ImportedMS: now - 10*dayMS, Read: true},
	}
	names := func(files []models.LogFile) []string {
		var got []string
		for _, f := range files {
			got = append(got, f.Name)
		}
		return got
	}
	for _, tc := range []struct {
		auto            models.AutoSettings
		archive, remove []string
	}{
		{models.AutoSettings{}, nil, nil},
		{models.AutoSettings{ArchiveGameLogsDays: 30}, []string{"old"}, nil},
		{models.AutoSettings{DeleteImportsDays: 30}, nil, []string{"import-old"}},
		{models.AutoSettings{ArchiveGameLogsDays: 5, DeleteImportsDays: 5}, []string{"old", "young"},
			[]string{"import-old", "import-young"}},
	} {
		archive, remove := AutoPicks(files, tc.auto, now)
		if got, got2 := names(archive), names(remove); !slices.Equal(got, tc.archive) || !slices.Equal(got2, tc.remove) {
			t.Errorf("AutoPicks(%+v) = %v, %v; want %v, %v", tc.auto, got, got2, tc.archive, tc.remove)
		}
	}
}
