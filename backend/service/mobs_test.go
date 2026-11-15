package service

import (
	"fmt"
	"testing"

	"wow-live/backend/models"
)

func TestScaleMob(t *testing.T) {
	// Keystone.guru's base health of a Temple of Sethraliss trash mob (NPC 134364) and boss (262530); the owner's +17
	// log gave them 13933023 and 43540697 max health (the game rounds the boss one lower).
	trash, boss := models.DungeonMob{Health: 3243255}, models.DungeonMob{Health: 9729765, Boss: true}
	for _, tc := range []struct {
		level             int
		mult              float64
		trash, bossHealth int
	}{
		{2, 1.07, 3296769, 10410849},   // trash 5% lower up to +5
		{10, 1.84, 7161107, 22378460},  // Fortified and Tyrannical from +10
		{17, 3.58, 13933023, 43540698}, // 10% a level from +11
		{30, 12.37, 48142877, 150446491},
	} {
		if got := keyMult(tc.level); got != tc.mult {
			t.Errorf("+%d multiplier %v; want %v", tc.level, got, tc.mult)
		}
		if got := ScaleMob(trash, tc.level).Health; got != tc.trash {
			t.Errorf("+%d trash health %d; want %d", tc.level, got, tc.trash)
		}
		if got := ScaleMob(boss, tc.level).Health; got != tc.bossHealth {
			t.Errorf("+%d boss health %d; want %d", tc.level, got, tc.bossHealth)
		}
	}
}

func TestMobsStartAtTheNewestSavedKeysLevelAndAnOldReferenceHasNone(t *testing.T) {
	facts := map[int]models.DungeonFacts{250: {Mobs: map[int]models.DungeonMob{
		2: {NPC: 2, Health: 1_000_000}, 1: {NPC: 1, Health: 2_000_000, Boss: true}}}, 585: {}}
	keys, err := NewLogTracker(nil, nil, models.MPlusRules{}, facts, &memStore{keys: []models.Key{
		{MapID: 585, Level: 20}, {MapID: 250, Level: 17}, {MapID: 250, Level: 12}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name               string
		mapID, level       int
		wantLevel, wantDef int
		wantHealth         []int
	}{
		{"the newest key's level", 250, 0, 17, 17, []int{8_950_000, 4_296_000}},
		{"the level asked for", 250, 2, 2, 17, []int{2_140_000, 1_016_500}},
		{"no saved key: +10", 399, 0, 10, 10, nil},
		{"a reference without mobs", 585, 0, 20, 20, nil},
	} {
		got := keys.Mobs(tc.mapID, tc.level)
		var health []int
		for _, m := range got.Mobs {
			health = append(health, m.Health)
		}
		if got.Level != tc.wantLevel || got.DefaultLevel != tc.wantDef || got.HealthMult != keyMult(tc.wantLevel) ||
			got.Mobs == nil || fmt.Sprint(health) != fmt.Sprint(tc.wantHealth) {
			t.Errorf("%s: %+v; want level %d (default %d), healths %v by NPC", tc.name, got, tc.wantLevel, tc.wantDef,
				tc.wantHealth)
		}
	}
}
