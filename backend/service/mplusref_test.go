package service

import (
	"reflect"
	"strings"
	"testing"

	"wow-live/backend/models"
)

func TestDungeonFactsCombinesTheSources(t *testing.T) {
	src := DungeonSources{Name: "Den of Nalorakk", KeystoneGuruTotal: 729,
		MDT: models.MDTDungeon{MapID: 586, Total: 729,
			Mobs: map[int]models.MDTMob{241816: {Name: "Keen-Eyed Striker", Count: 7}, 245752: {Name: "Keen-Eyed Striker", Count: 7},
				245855: {Name: "Spirit of Hunger", Count: 25},
				241812: {Name: "The Hoardmonger", Boss: true}},
			Spells: map[int]models.MDTSpell{1239352: {Interruptible: true}}},
		Guide: models.DungeonGuide{URL: "https://www.wowhead.com/guide/x",
			Interrupts: map[int]string{1239352: "Keen-Eyed Striker", 1309919: "Frigid Mauler"},
			Dispels:    map[int]string{1238801: "Spirit of Hunger"}, Purges: map[int]string{1238053: "Territorial Matriarch"},
			Tips: map[int]string{1234021: "Dodge", 1234233: "Soak", 1235548: "Dispel", 1234681: "Watch Knock"}}}
	got, err := DungeonFacts(src)
	if err != nil {
		t.Fatal(err)
	}
	want := models.DungeonFacts{Name: "Den of Nalorakk", Guide: "https://www.wowhead.com/guide/x",
		Forces: models.Forces{Total: 729, NPCs: map[int]int{241812: 0, 241816: 7, 245752: 7, 245855: 25}},
		// Worth 25 against a median of 7: a miniboss.
		Bosses: []int{241812}, Minibosses: []int{245855},
		MustStop: []int{1239352}, Review: []int{1309919}, Avoidable: []int{1234021}, Soak: []int{1234233},
		Dispel: []int{1235548, 1238801}, Purge: []int{1238053}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DungeonFacts = %+v\nwant %+v", got, want)
	}
	src.KeystoneGuruTotal = 730
	if _, err := DungeonFacts(src); err == nil || !strings.Contains(err.Error(), "729") {
		t.Errorf("totals 729 and 730 gave %v; want an error naming both", err)
	}
}

func TestDungeonFactsCountsAnNPCTwinAsTheMobItCopies(t *testing.T) {
	got, err := DungeonFacts(DungeonSources{Name: "Kings' Rest", MDT: models.MDTDungeon{MapID: 249, Total: 608,
		Mobs: map[int]models.MDTMob{270502: {Name: "Half-Finished Mummy", Count: 7}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Forces.NPCs[273050] != 7 {
		t.Errorf("NPC 273050 counts %d; want 7, as the Half-Finished Mummy Mythic Dungeon Tools lists (270502)",
			got.Forces.NPCs[273050])
	}
}

// floor2514 is Den of Nalorakk's lower floor.
var floor2514 = models.DungeonFloor{RaidMap: models.RaidMap{UIMapID: 2514, MinX: 5856.25, MaxX: 4731.25, MinY: 4014.58,
	MaxY: 4764.58}}

func TestKSGToLog(t *testing.T) {
	// Keystone.guru's top left corner is the floor's maxY and -maxX, its bottom right its minY and -minX (where
	// KeyMap.tsx's onTiles draws them).
	for _, tc := range []struct{ lat, lng, x, y float64 }{{0, 0, 4764.6, -4731.3}, {-256, 384, 4014.6, -5856.3}} {
		if x, y := ksgToLog(floor2514, tc.lat, tc.lng); x != tc.x || y != tc.y {
			t.Errorf("ksgToLog(%v, %v) = %v, %v; want %v, %v", tc.lat, tc.lng, x, y, tc.x, tc.y)
		}
	}
}

func TestJoinEnemies(t *testing.T) {
	mobs := map[int]models.MDTMob{100: {Count: 5}, 200: {Count: 0, Boss: true}}
	for _, tc := range []struct {
		name         string
		clone        models.MDTClone
		ksg          models.KeystoneGuruEnemy
		want         []models.RouteEnemy
		wantUnplaced bool
	}{
		{"by mdt_id, packed by MDT's group", models.MDTClone{Index: 1, Clone: 3, NPC: 100, Group: 7},
			models.KeystoneGuruEnemy{NPC: 100, MDTID: 3, Lat: 0, Lng: 0, Map: 2514},
			[]models.RouteEnemy{{Index: 1, Clone: 3, NPC: 100, Count: 5, Pack: 7, X: 4764.6, Y: -4731.3,
				Map: 2514}}, false},
		{"by mdt_npc_id, packed by Keystone.guru", models.MDTClone{Index: 2, Clone: 1, NPC: 200, Group: 7},
			models.KeystoneGuruEnemy{NPC: 999, MDTNPC: 200, MDTID: 1, Pack: 42, Map: 2514},
			[]models.RouteEnemy{{Index: 2, Clone: 1, NPC: 200, Boss: true, Pack: 42, X: 4764.6, Y: -4731.3,
				Map: 2514}}, false},
		{"another clone unplaced", models.MDTClone{Index: 1, Clone: 4, NPC: 100},
			models.KeystoneGuruEnemy{NPC: 100, MDTID: 3, Map: 2514}, []models.RouteEnemy{}, true},
		{"on a floor the map lacks", models.MDTClone{Index: 1, Clone: 3, NPC: 100},
			models.KeystoneGuruEnemy{NPC: 100, MDTID: 3, Map: 2564}, []models.RouteEnemy{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, unplaced := RouteEnemies(models.MDTDungeon{Mobs: mobs, Clones: []models.MDTClone{tc.clone}},
				[]models.DungeonFloor{floor2514}, []models.KeystoneGuruEnemy{tc.ksg})
			if !reflect.DeepEqual(got, tc.want) || (len(unplaced) == 1) != tc.wantUnplaced {
				t.Errorf("RouteEnemies = %+v, unplaced %+v; want %+v, unplaced %v", got, unplaced, tc.want,
					tc.wantUnplaced)
			}
		})
	}
}

func TestHitCounterFindsAbilitiesThatMissedSomeone(t *testing.T) {
	h := NewHitCounter()
	line := func(event string, f ...string) { h.Add(models.LogLine{Event: event, Fields: f}) }
	hit := func(player string, spell string) {
		line("SPELL_DAMAGE", "Creature-1", "Mob", "0xa48", "0x0", player, "P", "0x514", "0x0", spell, "Swirl")
	}
	line("CHALLENGE_MODE_START", "Den of Nalorakk", "2825", "586", "16")
	for _, p := range []string{"Player-1", "Player-2", "Player-3"} {
		line("COMBATANT_INFO", p)
	}
	for range 3 {
		hit("Player-1", "100")
		hit("Player-1", "200")
		hit("Player-2", "200")
		hit("Player-3", "200")
	}
	line("CHALLENGE_MODE_END", "2825", "1", "16", "1456514")
	if got := h.Uneven(); len(got) != 1 || got[0].SpellID != 100 || got[0].MapID != 586 || got[0].Keys != 1 {
		t.Fatalf("Uneven = %+v; want spell 100 (hit one player 3 times, the others never), not 200 (hit everyone)", got)
	}
}
