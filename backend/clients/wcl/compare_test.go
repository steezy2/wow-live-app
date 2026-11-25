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
		"casts": {"data": {"entries": [{"guid": 1, "name": "Blast", "hitCount": 12},
			{"guid": 3, "name": "Surge", "hitCount": 2}]}},
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

func TestRankedPlayerListsABuffWithSeveralSpellIDsOnce(t *testing.T) {
	var tables map[string]table
	raw := `{"buffs": {"data": {"auras": [{"guid": 1244329, "name": "Overpowered Missiles", "totalUptime": 10000},
		{"guid": 1277009, "name": "Overpowered Missiles", "totalUptime": 2400}]}}}`
	if err := json.Unmarshal([]byte(raw), &tables); err != nil {
		t.Fatal(err)
	}
	want := []models.RankedBuff{{ID: 1244329, Name: "Overpowered Missiles", UptimeMS: 10000}}
	if got := rankedPlayer(tables, 10_000).Buffs; !reflect.DeepEqual(got, want) {
		t.Errorf("buffs = %+v, want %+v (not 124%%)", got, want)
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

func TestBuildOfReadsTheActorsTalentsGearAndStats(t *testing.T) {
	var events []event
	raw := `[{"type":"combatantinfo","sourceID":2,"talentTree":[{"id":9,"rank":1,"nodeID":8}]},
		{"type":"combatantinfo","sourceID":3,"critSpell":1500.4,"hasteSpell":900,"mastery":2000,"versatilityDamageDone":300,
		 "talentTree":[{"id":20,"rank":2,"nodeID":10}],
		 "gear":[{"id":50,"itemLevel":272,"permanentEnchant":7964,"gems":[{"id":240883,"itemLevel":0}]},{"id":0},{"id":0},
		 {"id":0},{"id":51,"itemLevel":270}]},{"type":"cast","sourceID":3}]`
	if err := json.Unmarshal([]byte(raw), &events); err != nil {
		t.Fatal(err)
	}
	want := build{
		talents: []models.TalentPick{{NodeID: 10, EntryID: 20, Rank: 2}},
		gear: []models.RankedGear{{Slot: "head", ID: 50, Level: 272, Enchant: 7964, Gems: []int{240883}},
			{Slot: "chest", ID: 51, Level: 270}},
		stats: &models.RankedStats{Crit: 1500, Haste: 900, Mastery: 2000, Vers: 300},
	}
	if got := buildOf(events, 3); !reflect.DeepEqual(got, want) {
		t.Errorf("buildOf = %+v, want %+v", got, want)
	}
	if got := buildOf(events, 4); !reflect.DeepEqual(got, build{}) {
		t.Errorf("buildOf for an actor with none = %+v, want empty", got)
	}
}
