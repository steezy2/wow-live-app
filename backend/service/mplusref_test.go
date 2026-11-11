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
