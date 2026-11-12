package service

import (
	"fmt"
	"strings"
	"testing"
)

// raidBuff is a buff landing on a raider s seconds into day 3's raid.
func raidBuff(s float64, src, dst string, id int, name string) string {
	return raidAt(3, s, fmt.Sprintf(`SPELL_AURA_APPLIED,%s,%s,%d,"%s",0x1,BUFF`, src, dst, id, name))
}

func TestApplicableBuffs(t *testing.T) {
	// A warrior shouts on the mage and leaves; the next pull is the mage and the shaman (pullLines), so its start
	// doesn't ask for Battle Shout.
	const warrior = `Player-76-0000DDDD,"Testwarrior-Sargeras-US",0x514,0x80000000`
	keys, store := newRaidTracker(t)
	keys.rules.Buffs["Battle Shout"] = "Warrior"
	keys.specs["Player-76-0000DDDD"] = 71 // Arms, from an earlier pull
	feed(t, keys, raidAt(3, -10, `ZONE_CHANGE,2939,"The Venomous Abyss",16`),
		raidBuff(-9, warrior, testPlayer, 6673, "Battle Shout"))
	feed(t, keys, pullLines(3, 0, 60, 40, false)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(store.pulls[0].BuffGaps); strings.Contains(got, "Battle Shout") ||
		!strings.Contains(got, "Arcane Intellect") {
		t.Errorf("pull start = %s; want Arcane Intellect asked, Battle Shout not", got)
	}
}
