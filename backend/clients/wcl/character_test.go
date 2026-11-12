package wcl

import (
	"context"
	"reflect"
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
		{"code": "abc", "startTime": 1000, "endTime": 5000, "zone": {"id": 55},
		 "masterData": {"actors": [{"id": 1, "name": "Testmage"}, {"id": 2, "name": "Testrogue"}]},
		 "fights": [{"id": 4, "encounterID": 12923, "keystoneLevel": 12, "difficulty": 10, "kill": true,
		             "startTime": 10, "endTime": 900, "friendlyPlayers": [2, 1, 9]}]},
		{"code": "def", "startTime": 7000, "endTime": 8000, "zone": null, "masterData": {"actors": []}, "fights": []}]}}}}`))
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
