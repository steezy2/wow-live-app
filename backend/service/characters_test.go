package service

import (
	"reflect"
	"testing"

	"wow-live/backend/models"
)

func TestLoggedBy(t *testing.T) {
	realms := map[string]string{"Testmage": "MoonGuard"}
	for _, c := range []struct {
		name      string
		logger    string
		realms    map[string]string
		character string
		want      bool
	}{
		{"every character", "Testmage", realms, "", true},
		{"name and realm", "Testmage", realms, "Testmage-MoonGuard", true},
		{"Settings' spelling of the realm", "Testmage", realms, "Testmage-Moon Guard", true},
		{"Settings' case of the name", "Testmage", realms, "testmage-MoonGuard", true},
		{"another realm", "Testmage", realms, "Testmage-Sargeras", false},
		{"another name", "Testmage", realms, "Testrogue-MoonGuard", false},
		{"a run saved before realms", "Testmage", nil, "Testmage-Sargeras", true},
		{"no logger", "", nil, "Testmage", false},
	} {
		if got := loggedBy(c.logger, c.realms, c.character); got != c.want {
			t.Errorf("%s: loggedBy(%q, %v, %q) = %v; want %v", c.name, c.logger, c.realms, c.character, got, c.want)
		}
	}
}

func TestPlayedCharacters(t *testing.T) {
	mage := func(realm string) map[string]string { return map[string]string{"Testmage": realm} }
	rows := func(class string) models.KeyMeter {
		return models.KeyMeter{Rows: []models.KeyMeterRow{{Player: "Testmage", Class: class}}}
	}
	keys := []models.Key{ // newest first
		{Logger: "Testmage", Realms: mage("MoonGuard"), Overall: rows("Mage")},
		{Logger: "Testmage", Realms: mage("MoonGuard"), Overall: rows("Priest")},
		{Logger: "Testmage"}, // saved before realms: counts for the Moon Guard mage
		{Logger: "Testrogue", Realms: map[string]string{"Testrogue": "Sargeras"}},
		{},
	}
	pulls := []models.RaidPull{{Logger: "Testrogue", Realms: map[string]string{"Testrogue": "Sargeras"},
		Meter: models.KeyMeter{Rows: []models.KeyMeterRow{{Player: "Testrogue", Class: "Rogue"}}}}}
	got := playedCharacters(keys, pulls, "Testrogue-Sargeras")
	want := []models.Character{
		{ID: "Testmage-MoonGuard", Name: "Testmage", Realm: "MoonGuard", Class: "Mage", Keys: 3},
		{ID: "Testrogue-Sargeras", Name: "Testrogue", Realm: "Sargeras", Class: "Rogue", Keys: 1, Pulls: 1,
			Preferred: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("played = %+v\nwant %+v", got, want)
	}
}
