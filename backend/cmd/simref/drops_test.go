package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/clients/wago"
	"wow-live/backend/models"
)

const stats = ",StatModifier_bonusStat_0,StatModifier_bonusStat_1,StatModifier_bonusStat_2,StatModifier_bonusStat_3," +
	"StatModifier_bonusStat_4,StatModifier_bonusStat_5,StatModifier_bonusStat_6,StatModifier_bonusStat_7," +
	"StatModifier_bonusStat_8,StatModifier_bonusStat_9\n"

// tables is a raid (season 37), a dungeon on map 1762 and an older raid, with a plate helm, a strength axe, a ring,
// a trophy and a helm the journal shows in season 34 only.
var tables = map[string]string{
	"JournalInstance":  "Name_lang,ID,MapID\nAbyss,1,3004\nRest,2,1762\nOld,3,99\n",
	"JournalEncounter": "Name_lang,ID,JournalInstanceID,OrderIndex\nUla'tek,10,1,2\nNek'zali,11,1,1\nSerpent,20,2,1\nOld,30,3,1\n",
	"JournalEncounterItem": "JournalEncounterID,ItemID,DisplaySeasonID\n" +
		"10,100,0\n10,100,0\n10,101,37\n11,102,0\n10,103,34\n20,104,0\n30,105,0\n",
	"Item": "ID,ClassID,SubclassID,InventoryType\n100,4,4,1\n101,20,0,0\n102,2,0,13\n103,4,4,1\n104,4,0,11\n105,4,1,1\n",
	"ItemSparse": "Display_lang,ID,AllowableClass" + stats + "Helm,100,-1,74,7,0,0,0,0,0,0,0,0\n" +
		"Trophy,101,-1,0,0,0,0,0,0,0,0,0,0\nAxe,102,-1,4,7,0,0,0,0,0,0,0,0\nOld Helm,103,-1,74,0,0,0,0,0,0,0,0,0\n" +
		"Ring,104,-1,7,32,0,0,0,0,0,0,0,0\nHood,105,-1,5,0,0,0,0,0,0,0,0,0\n",
	"ItemSpecOverride":  "ItemID,SpecID\n",
	"ItemSpec":          "ItemType,PrimaryStat,SecondaryStat,SpecializationID\n4,2,40,71\n4,0,40,65\n5,2,7,71\n5,40,7,259\n",
	"ChrClasses":        "Name_lang,ID\nWarrior,1\nPaladin,2\nRogue,4\n",
	"ChrSpecialization": "Name_lang,ID,ClassID,OrderIndex\nArms,71,1,0\nHoly,65,2,0\nAssassination,259,4,0\nInitial,1446,1,4\n",
}

var levels = models.SimDropLevels{Season: "s2", DisplaySeason: 37, Raid: []models.RaidDropGroup{
	{Bosses: []string{"Nek'zali", "Ula'tek"}, Levels: models.DifficultyLevels{LFR: 1, Normal: 2, Heroic: 3, Mythic: 4}}}}

func readTestJournal(t *testing.T) journal {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		text, ok := tables[strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/db2/"), "/csv")]
		if !ok || r.URL.Query().Get("branch") != "wow" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, text)
	}))
	defer server.Close()
	j, err := readJournal(context.Background(), wago.Client{CacheDir: t.TempDir(), Base: server.URL, Branch: "wow"})
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestDropsAreTheSeasonsGearWithTheSpecsThatLootIt(t *testing.T) {
	drops, err := buildDrops(readTestJournal(t), levels, []int{1762})
	if err != nil {
		t.Fatal(err)
	}
	levels := `"levels":{"lfr":1,"normal":2,"heroic":3,"mythic":4}`
	want := `[{"instanceID":1,"mapID":3004,"name":"Abyss","bosses":[` +
		`{"name":"Nek'zali",` + levels + `,"items":[{"id":102,"name":"Axe","slot":"one_hand","specs":[71]}]},` +
		`{"name":"Ula'tek",` + levels + `,"items":[{"id":100,"name":"Helm","slot":"head","armor":"plate","specs":[65,71]}]}]}]` +
		`[{"instanceID":2,"mapID":1762,"name":"Rest","bosses":[` +
		`{"name":"Serpent","items":[{"id":104,"name":"Ring","slot":"finger"}]}]}]`
	raids, _ := cachefile.Marshal(drops.Raids)
	dungeons, _ := cachefile.Marshal(drops.Dungeons)
	if got := string(raids) + string(dungeons); got != want {
		t.Errorf("drops =\n%s\nwant\n%s", got, want)
	}
	if fmt.Sprint(drops.Specs) != "map[65:{paladin holy} 71:{warrior arms} 259:{rogue assassination}]" {
		t.Errorf("specs = %v; want the three playable ones, not the Initial one", drops.Specs)
	}
}

func TestDropsRefuseLevelsThatDontMatchTheJournal(t *testing.T) {
	j := readTestJournal(t)
	for name, change := range map[string]func(*models.SimDropLevels){
		"another season":        func(l *models.SimDropLevels) { l.DisplaySeason = 38 },
		"a boss with no levels": func(l *models.SimDropLevels) { l.Raid[0].Bosses = []string{"Ula'tek"} },
		"a boss not the raid's": func(l *models.SimDropLevels) { l.Raid[0].Bosses = append(l.Raid[0].Bosses, "Old") },
	} {
		l := levels
		l.Raid = []models.RaidDropGroup{{Bosses: append([]string{}, levels.Raid[0].Bosses...)}}
		change(&l)
		if _, err := buildDrops(j, l, []int{1762}); err == nil {
			t.Errorf("%s: built; want an error", name)
		}
	}
	if _, err := buildDrops(j, levels, []int{1762, 5555}); err == nil {
		t.Error("a dungeon the journal doesn't have: built; want an error")
	}
}

func TestDropsComeOutTheSameEveryRun(t *testing.T) {
	j := readTestJournal(t)
	first, _ := buildDrops(j, levels, []int{1762})
	want, _ := cachefile.Marshal(first)
	for range 20 {
		again, _ := buildDrops(j, levels, []int{1762})
		if got, _ := cachefile.Marshal(again); string(got) != string(want) {
			t.Fatalf("a rebuild differs:\n%s\nwant\n%s", got, want)
		}
	}
}
