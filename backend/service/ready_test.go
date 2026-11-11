package service

import (
	"fmt"
	"hash/crc32"
	"slices"
	"strings"
	"testing"

	"wow-live/backend/models"
)

func TestReadyCheckBeforeAKeyFromBuffsSeenSinceZoningIn(t *testing.T) {
	keys := newTracker(t, nil)
	const shaman = `Player-76-0000BBBB,"Testshaman-Sargeras-US",0x512,0x80000000`
	buff := func(s float64, event, dst, name string) string {
		return at(s, fmt.Sprintf(`%s,%s,%s,%d,"%s",0x40,BUFF`, event, testPlayer, dst, crc32.ChecksumIEEE([]byte(name)), name))
	}
	feed(t, keys,
		at(0, `ZONE_CHANGE,2813,"Murder Row",23`),
		buff(1, "SPELL_AURA_APPLIED", testPlayer, "Arcane Intellect"),
		buff(1, "SPELL_AURA_APPLIED", shaman, "Arcane Intellect"),
		buff(2, "SPELL_AURA_APPLIED", testPlayer, "Flask of the Shattered Sun"),
		buff(3, "SPELL_AURA_APPLIED", shaman, "Well Fed"),
		buff(4, "SPELL_AURA_REMOVED", shaman, "Arcane Intellect"), // died
		at(4.5, `ZONE_CHANGE,2813,"Murder Row",23`),               // ran back in: the buffs seen so far still count
	)
	got, ok := keys.Ready()
	want := []models.ReadyPlayer{
		{Player: "Testmage", Missing: []string{}, NotSeen: []string{"food"}},
		{Player: "Testshaman", Missing: []string{"Arcane Intellect"}, NotSeen: []string{"flask"}},
	}
	if !ok || got.Dungeon != "Murder Row" || !slices.EqualFunc(got.Players, want, func(a, b models.ReadyPlayer) bool {
		return a.Player == b.Player && slices.Equal(a.Missing, b.Missing) && slices.Equal(a.NotSeen, b.NotSeen)
	}) {
		t.Fatalf("ready = %+v, %v; want %+v", got, ok, want)
	}
	feed(t, keys, at(5, `CHALLENGE_MODE_START,"Murder Row",2813,587,10,[9]`))
	if _, ok := keys.Ready(); ok {
		t.Error("a ready check once the key started; want none (COMBATANT_INFO has the buffs)")
	}
}

// raidBuff is a buff landing on a raider s seconds into day 3's raid.
func raidBuff(s float64, src, dst string, id int, name string) string {
	return raidAt(3, s, fmt.Sprintf(`SPELL_AURA_APPLIED,%s,%s,%d,"%s",0x1,BUFF`, src, dst, id, name))
}

func TestApplicableBuffs(t *testing.T) {
	// A warrior shouts on the mage and leaves; the next pull is the mage and the shaman (pullLines), so neither its
	// start nor the ready check after it asks for Battle Shout.
	const warrior = `Player-76-0000DDDD,"Testwarrior-Sargeras-US",0x514,0x80000000`
	keys, store := newRaidTracker(t)
	keys.rules.Buffs["Battle Shout"] = "Warrior"
	keys.specs["Player-76-0000DDDD"] = 71 // Arms, from an earlier pull
	feed(t, keys, raidAt(3, -10, `ZONE_CHANGE,2939,"The Venomous Abyss",16`),
		raidBuff(-9, warrior, testPlayer, 6673, "Battle Shout"))
	if ready := keys.RaidLive(0).Ready; ready == nil || !strings.Contains(fmt.Sprint(ready.Players), "Battle Shout") {
		t.Fatalf("ready with the warrior = %+v; want Battle Shout asked of the warrior, who lacks it", ready)
	}
	feed(t, keys, pullLines(3, 0, 60, 40, false)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	ready := keys.RaidLive(0).Ready
	if got := fmt.Sprint(store.pulls[0].BuffGaps, ready.Players); strings.Contains(got, "Battle Shout") ||
		!strings.Contains(got, "Arcane Intellect") {
		t.Errorf("pull start and ready = %s; want Arcane Intellect asked, Battle Shout not", got)
	}
}

func TestReadyVantusDifficulty(t *testing.T) {
	for _, c := range []struct {
		difficulty string
		want       bool
	}{{"17", false}, {"14", true}, {"16", true}} { // LFR, Normal, Mythic
		keys, _ := newRaidTracker(t)
		keys.raid.rules.Consumables.Vantus = []string{"Vantus Rune"}
		keys.raid.rules.Consumables.VantusDifficulties = []string{"Normal", "Heroic", "Mythic"}
		feed(t, keys, raidAt(3, 0, `ZONE_CHANGE,2939,"The Venomous Abyss",`+c.difficulty),
			raidBuff(1, testPlayer, testPlayer, 1459, "Arcane Intellect"))
		ready := keys.RaidLive(0).Ready
		if got := ready != nil && strings.Contains(fmt.Sprint(ready.Players), "Vantus Rune"); got != c.want {
			t.Errorf("difficulty %s: Vantus Rune asked = %v (%+v); want %v", c.difficulty, got, ready, c.want)
		}
	}
}
