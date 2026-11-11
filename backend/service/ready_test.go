package service

import (
	"fmt"
	"hash/crc32"
	"slices"
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
