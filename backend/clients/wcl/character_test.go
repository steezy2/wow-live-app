package wcl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"wow-live/backend/models"
)

func TestRealmsMapNormalizedNamesToSlugsAcrossPages(t *testing.T) {
	f := newFake(t,
		data(`{"worldData": {"regions": [{"id": 1, "compactName": "US"}, {"id": 2, "compactName": "EU"}]}}`),
		data(`{"worldData": {"region": {"servers": {"data": [{"name": "Area 52", "slug": "area-52"}], "has_more_pages": true}}}}`),
		data(`{"worldData": {"region": {"servers": {"data": [{"name": "Somerealm", "slug": "somerealm"}], "has_more_pages": false}}}}`))
	realms, err := f.client(t).Realms(context.Background(), "us")
	if want := map[string]string{"area52": "area-52", "somerealm": "somerealm"}; err != nil ||
		!reflect.DeepEqual(realms, want) {
		t.Fatalf("Realms = %v, %v; want %v", realms, err, want)
	}
}

func TestRecentReportsNamePlayersAndMarkAnotherPage(t *testing.T) {
	f := newFake(t, data(`{"characterData": {"character": {"recentReports": {"has_more_pages": true, "data": [
		{"code": "abc", "startTime": 1000, "endTime": 5000, "zone": {"id": 55}},
		{"code": "def", "startTime": 7000, "endTime": 8000, "zone": null}]}}}}`),
		data(`{"reportData": {
		"r0": {"masterData": {"actors": [{"id": 1, "name": "Testmage"}, {"id": 2, "name": "Testrogue"}]},
		       "fights": [{"id": 4, "encounterID": 12923, "keystoneLevel": 12, "difficulty": 10, "kill": true,
		                   "startTime": 10, "endTime": 900, "friendlyPlayers": [2, 1, 9]}]},
		"r1": {"masterData": {"actors": []}, "fights": []}}}`))
	reports, more, err := f.client(t).RecentReports(context.Background(), "us", "somerealm", "Testmage", 1)
	want := []models.CharacterReport{
		{Code: "abc", StartTime: 1000, EndTime: 5000, ZoneID: 55, Fights: []models.ReportFight{{ID: 4,
			EncounterID: 12923, KeystoneLevel: 12, Difficulty: 10, Kill: true, StartTime: 10, EndTime: 900,
			Players: []string{"Testrogue", "Testmage"}}}},
		{Code: "def", StartTime: 7000, EndTime: 8000}}
	if err != nil || !more || !reflect.DeepEqual(reports, want) {
		t.Fatalf("RecentReports = %+v, %v, %v; want %+v, true", reports, more, err, want)
	}
}

func TestRecentReportsAskOnlyForTheFightsOfReportsNewOrGrown(t *testing.T) {
	list := func(reports string) func(w http.ResponseWriter, body, auth string) {
		return data(`{"characterData": {"character": {"recentReports": {"data": [` + reports + `]}}}}`)
	}
	old := `{"code": "old", "startTime": 1000, "endTime": 5000}`
	f := newFake(t, list(old), data(`{"reportData": {"r0": {"fights": [{"id": 1}]}}}`),
		list(`{"code": "new", "startTime": 9000, "endTime": 9500}, `+old),
		data(`{"reportData": {"r0": {"fights": [{"id": 2}]}}}`),
		list(`{"code": "new", "startTime": 9000, "endTime": 9900}, `+old),
		data(`{"reportData": {"r0": {"fights": [{"id": 2}, {"id": 3}]}}}`))
	c := f.client(t)
	listFile := filepath.Join(c.CacheDir, "wcl-mplus", "reportlist-us-somerealm-Testmage-1.json")
	for range 3 {
		if _, _, err := c.RecentReports(context.Background(), "us", "somerealm", "Testmage", 1); err != nil {
			t.Fatal(err)
		}
		os.Remove(listFile) // as if the list's 10 minutes were up
	}
	if q := f.queries[3]; !strings.Contains(q, `"c0":"new"`) || strings.Contains(q, "old") {
		t.Errorf("second fights query %s, want only the new report's", q)
	}
	if q := f.queries[5]; !strings.Contains(q, `"c0":"new"`) || strings.Contains(q, "old") {
		t.Errorf("third fights query %s, want only the report that grew", q)
	}
	if files, _ := os.ReadDir(filepath.Join(c.CacheDir, "wcl-mplus", "reports")); len(files) != 2 {
		t.Errorf("%d cached reports, want one file a report", len(files))
	}
}

func TestRecentReportsStopAtTheLastPageWarcraftLogsServes(t *testing.T) {
	f := newFake(t, data(`{"characterData": {"character": {"recentReports": {"has_more_pages": true, "data": []}}}}`))
	if _, more, err := f.client(t).RecentReports(context.Background(), "us", "somerealm", "Testmage",
		models.MaxReportPages); err != nil || more {
		t.Fatalf("page %d: more %v, %v; want no page after it", models.MaxReportPages, more, err)
	}
}

func TestCharacterParsesKeepRankedEncountersByKind(t *testing.T) {
	f := newFake(t, data(`{"characterData": {"character": {
		"mplus": {"rankings": [{"encounter": {"name": "Ara-Kara"}, "rankPercent": 91.5, "medianPercent": 80, "totalKills": 4, "spec": "Fire"},
		                       {"encounter": {"name": "Skyreach"}, "rankPercent": null, "medianPercent": null, "totalKills": 0}]},
		"raid": {"rankings": [{"encounter": {"name": "Test Boss"}, "rankPercent": 40, "medianPercent": 35, "totalKills": 2, "spec": "Fire"}]}}}}`),
		data(`{"characterData": {"character": null}}`))
	c := f.client(t)
	got, err := c.CharacterParses(context.Background(), "us", "some-realm", "Testmage", models.WCLZones{MPlusID: 1, RaidID: 2})
	want := models.CharacterParses{URL: "https://www.warcraftlogs.com/character/us/some-realm/testmage", Found: true,
		Parses: []models.CharacterParse{
			{Kind: models.FightMPlus, Encounter: "Ara-Kara", Best: 91.5, Median: 80, Runs: 4, Spec: "Fire"},
			{Kind: models.FightRaid, Encounter: "Test Boss", Best: 40, Median: 35, Runs: 2, Spec: "Fire"}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("CharacterParses = %+v, %v; want %+v", got, err, want)
	}
	got, err = c.CharacterParses(context.Background(), "us", "some-realm", "Nobody", models.WCLZones{MPlusID: 1})
	if err != nil || got.Found || len(got.Parses) != 0 {
		t.Fatalf("CharacterParses of an unknown character = %+v, %v; want not found", got, err)
	}
}

func TestRecentReportsAskForManyReportsFightsInSeveralQueries(t *testing.T) {
	var codes, reportData []string
	for i := range 10 {
		codes = append(codes, fmt.Sprintf(`{"code": "r%d", "startTime": 0, "endTime": 5000}`, i))
	}
	answer := func(w http.ResponseWriter, body, _ string) { // each fights query is answered for the reports it names
		var q struct {
			Variables map[string]string `json:"variables"`
		}
		json.Unmarshal([]byte(body), &q)
		reportData = nil
		for n := range q.Variables {
			code := q.Variables[n]
			reportData = append(reportData, fmt.Sprintf(`"r%s": {"fights": [{"id": %s}]}`, strings.TrimPrefix(n, "c"),
				strings.TrimPrefix(code, "r")))
		}
		io.WriteString(w, `{"data": {"reportData": {`+strings.Join(reportData, ",")+`}}}`)
	}
	f := newFake(t, data(`{"characterData": {"character": {"recentReports": {"data": [`+strings.Join(codes, ",")+`]}}}}`),
		answer, answer)
	reports, _, err := f.client(t).RecentReports(context.Background(), "us", "somerealm", "Testmage", 1)
	if err != nil || len(reports) != 10 {
		t.Fatalf("RecentReports = %d reports, %v; want 10", len(reports), err)
	}
	for i, r := range reports {
		if len(r.Fights) != 1 || r.Fights[0].ID != i {
			t.Errorf("report r%d has fights %+v, want its own fight %d", i, r.Fights, i)
		}
	}
	if len(f.queries) != 3 {
		t.Errorf("%d queries, want the list and two fights queries of 8 and 2 reports", len(f.queries))
	}
}
