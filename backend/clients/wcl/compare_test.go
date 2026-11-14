package wcl

import (
	"encoding/json"
	"reflect"
	"testing"

	"wow-live/backend/models"
)

func TestRankedPlayerReadsTheTables(t *testing.T) {
	var tables map[string]table
	raw := `{
		"damage": {"data": {"entries": [{"guid": 1, "name": "Blast", "total": 900, "hitCount": 9}]}},
		"healing": {"data": {"entries": [{"guid": 2, "name": "Barrier", "total": 50, "hitCount": 1}]}},
		"casts": {"data": {"entries": [{"guid": 1, "name": "Blast", "hitCount": 12}, {"guid": 3, "name": "Surge", "hitCount": 2}]}},
		"buffs": {"data": {"auras": [{"guid": 7, "name": "Tempo", "totalUptime": 4000}]}},
		"taken": {"data": {"entries": [{"total": 300, "activeTime": 8000}]}},
		"deaths": {"data": {"entries": [{}, {}]}}}`
	if err := json.Unmarshal([]byte(raw), &tables); err != nil {
		t.Fatal(err)
	}
	want := models.RankedPlayer{DurationMS: 10_000, Damage: 900, Healing: 50, Taken: 300, ActiveMS: 8000, Deaths: 2,
		Abilities: []models.RankedAbility{{ID: 2, Name: "Barrier", Healing: 50, Hits: 1},
			{ID: 1, Name: "Blast", Damage: 900, Casts: 12, Hits: 9}, {ID: 3, Name: "Surge", Casts: 2}},
		Buffs: []models.RankedBuff{{ID: 7, Name: "Tempo", UptimeMS: 4000}}}
	if got := rankedPlayer(tables, 10_000); !reflect.DeepEqual(got, want) {
		t.Errorf("rankedPlayer = %+v, want %+v", got, want)
	}
}

func TestFindActor(t *testing.T) {
	actors := []playerActor{{1, "Mage", "Sargeras"}, {2, "Mage", "Area-52"}, {3, "Solo", "Sargeras"}}
	tests := []struct {
		name, server string
		want         int
		ok           bool
	}{
		{"Mage", "Area-52", 2, true},              // the server tells two of a name apart
		{"Solo", "Spelled Another Way", 3, true},  // the only one of its name
		{"Mage", "Spelled Another Way", 0, false}, // two of the name and neither on that server
		{"Nobody", "Sargeras", 0, false},
	}
	for _, tc := range tests {
		if got, ok := findActor(actors, tc.name, tc.server); got != tc.want || ok != tc.ok {
			t.Errorf("findActor(%s, %s) = %d, %v; want %d, %v", tc.name, tc.server, got, ok, tc.want, tc.ok)
		}
	}
}
