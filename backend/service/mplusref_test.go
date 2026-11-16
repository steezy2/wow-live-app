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
			Mobs: map[int]models.MDTMob{241816: {Name: "Keen-Eyed Striker", Count: 7, Spells: []int{1239352, 1240000}},
				245752: {Name: "Keen-Eyed Striker", Count: 7},
				245855: {Name: "Spirit of Hunger", Count: 25},
				241812: {Name: "The Hoardmonger", Boss: true}},
			Spells: map[int]models.MDTSpell{1239352: {Interruptible: true}}},
		NPCs: map[int]models.DungeonMob{241816: {NPC: 241816, Level: 90, Type: "Humanoid", Class: "Elite", Health: 3243255,
			Flags: []string{"bolstering"}}, 241812: {NPC: 241812, Class: "Elite"}},
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
		Dispel: []int{1235548, 1238801}, Purge: []int{1238053},
		// Keystone.guru's NPC under Mythic Dungeon Tools' name and spells; a boss when either says so.
		Mobs: map[int]models.DungeonMob{
			241816: {NPC: 241816, Name: "Keen-Eyed Striker", Level: 90, Type: "Humanoid", Class: "Elite", Health: 3243255,
				Flags: []string{"bolstering"}, Spells: []models.MobSpell{{ID: 1239352, Interruptible: true}, {ID: 1240000}}},
			245752: {NPC: 245752, Name: "Keen-Eyed Striker"}, 245855: {NPC: 245855, Name: "Spirit of Hunger"},
			241812: {NPC: 241812, Name: "The Hoardmonger", Class: "Elite", Boss: true}}}
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

func TestMDTPlanes(t *testing.T) {
	// Floor 2514's enemies stand at lng = 0.5x + 10, lat = 0.5y - 4 but for the last, which Keystone.guru moved; floor
	// 2564 has one, too few to fit.
	type enemy struct{ x, y, off float64 }
	var clones []models.MDTClone
	var ksg []models.KeystoneGuruEnemy
	add := func(floor int, enemies ...enemy) {
		for _, e := range enemies {
			n := len(clones) + 1
			clones = append(clones, models.MDTClone{Index: 1, Clone: n, NPC: 100, X: e.x, Y: e.y, Sublevel: 1})
			ksg = append(ksg, models.KeystoneGuruEnemy{NPC: 100, MDTID: n, Lng: 0.5*e.x + 10 + e.off, Lat: 0.5*e.y - 4,
				Map: floor})
		}
	}
	add(2514, enemy{100, -100, 0}, enemy{300, -120, 0}, enemy{200, -400, 0}, enemy{250, -250, 0}, enemy{150, -300, 0},
		enemy{280, -350, 0}, enemy{120, -200, 0}, enemy{220, -150, 0}, enemy{120, -50, 40})
	add(2564, enemy{500, -300, 0})
	other := models.DungeonFloor{RaidMap: models.RaidMap{UIMapID: 2564}}
	got := MDTPlanes(models.MDTDungeon{Clones: clones}, []models.DungeonFloor{floor2514, other}, ksg)
	want := models.MDTPlane{Sublevel: 1, Scale: 0.5, X: 10, Y: -4, Box: [4]float64{100, -400, 300, -100}}
	if got[0].MDT == nil || *got[0].MDT != want || got[1].MDT != nil {
		t.Fatalf("planes = %+v, %+v; want %+v without the moved enemy, and none for 2564", got[0].MDT, got[1].MDT, want)
	}
}

func TestPatrolYards(t *testing.T) {
	// Two mobs walk patrol 9, a loop whose last point is a hair off its first; each stands on one of its points, so
	// the patrol's points there are the mobs' yards. Patrol 10 is a line with no one on it, 11 is on another floor.
	ksg := []models.KeystoneGuruEnemy{
		{NPC: 100, MDTID: 1, Patrol: 9, Lat: -100, Lng: 200, Map: 2514},
		{NPC: 100, MDTID: 2, Patrol: 9, Lat: -120, Lng: 210, Map: 2514},
	}
	m := models.MDTDungeon{Mobs: map[int]models.MDTMob{100: {Count: 1}},
		Clones: []models.MDTClone{{Index: 1, Clone: 2, NPC: 100}, {Index: 1, Clone: 1, NPC: 100}}}
	floors := []models.DungeonFloor{floor2514}
	enemies, _ := RouteEnemies(m, floors, ksg)
	got := RoutePatrols(floors, []models.KeystoneGuruPatrol{
		{ID: 9, Color: "#003280", Map: 2514, Points: [][2]float64{{-100, 200}, {-120, 210}, {-110, 230}, {-100.01, 200.02}}},
		{ID: 10, Color: "#ff0000", Map: 2514, Points: [][2]float64{{0, 0}, {-1, 1}}},
		{ID: 11, Map: 2564, Points: [][2]float64{{0, 0}, {-1, 1}}},
	}, ksg, enemies)
	if len(got) != 2 {
		t.Fatalf("RoutePatrols = %+v; want patrols 9 and 10, not the one on a floor the map lacks", got)
	}
	walk := got[0]
	for i, e := range enemies {
		if p := walk.Points[i]; p != [2]float64{e.X, e.Y} {
			t.Errorf("point %d = %v; want %v, %v, where Enemies puts the mob standing there", i, p, e.X, e.Y)
		}
	}
	want := []models.EnemyRef{{Index: 1, Clone: 1}, {Index: 1, Clone: 2}}
	if !walk.Loop || walk.Color != "003280" || !reflect.DeepEqual(walk.Enemies, want) {
		t.Errorf("patrol 9 = loop %v, color %q, enemies %+v; want a loop, 003280, %+v", walk.Loop, walk.Color,
			walk.Enemies, want)
	}
	if line := got[1]; line.Loop || line.Enemies == nil || len(line.Enemies) != 0 {
		t.Errorf("patrol 10 = loop %v, enemies %#v; want a line with an empty list", line.Loop, line.Enemies)
	}
}

func TestIsLoop(t *testing.T) {
	for _, tc := range []struct {
		name   string
		points [][2]float64
		want   bool
	}{
		{"round, last point a hair off the first", [][2]float64{{0, 0}, {10, 0}, {10, 10}, {0.1, 0}}, true},
		{"a line", [][2]float64{{0, 0}, {10, 0}, {20, 0}}, false},
		{"out and back (Temple of Sethraliss)", [][2]float64{{0, 0}, {10, 0}, {20, 0}, {10, 0}, {0, 0}}, false},
		{"two points on one spot", [][2]float64{{0, 0}, {0, 0}}, false},
	} {
		if got := isLoop(tc.points); got != tc.want {
			t.Errorf("%s: isLoop = %v; want %v", tc.name, got, tc.want)
		}
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
