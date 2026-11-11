package service

import (
	"fmt"
	"reflect"
	"testing"

	"wow-live/backend/models"
)

const shamanUnit = `Player-76-0000BBBB,"Testshaman-Sargeras-US",0x511,0x80000000`

// mobUnit is a hostile mob by its number.
func mobUnit(n int) string {
	return fmt.Sprintf(`Creature-0-3882-2859-27547-24533%d-00006BE4EA,"Mob %d",0x10a48,0x80000000`, n, n)
}

func at(seconds float64, line string) string {
	return fmt.Sprintf("11/4/2026 20:00:%06.3f-7  %s", seconds, line)
}

func partyOfTwo(start string) []string {
	info := func(guid, spec string) string {
		return at(0.5, "COMBATANT_INFO,"+guid+",1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,"+spec+",[],[],[],[],[]")
	}
	return []string{start, info("Player-76-0000AAAA", "62"), info("Player-76-0000BBBB", "262")}
}

func TestKickLedgerAndDispelSpeed(t *testing.T) {
	keys := newTracker(t, nil)
	none := "0000000000000000"
	castStart := func(s float64, mob int) string {
		return at(s, "SPELL_CAST_START,"+mobUnit(mob)+","+none+",nil,0x80000000,0x80000000,1238063,\"Light Bolt\",0x2")
	}
	lines := partyOfTwo(at(0, `CHALLENGE_MODE_START,"Voidscar Arena",2923,585,10,[9]`))
	lines = append(lines,
		// Interrupted half a second in.
		castStart(1, 1),
		at(1.5, "SPELL_INTERRUPT,"+shamanUnit+","+mobUnit(1)+`,57994,"Wind Shear",0x8,1238063,"Light Bolt",2`),
		// Went off.
		castStart(2, 2),
		at(4, "SPELL_CAST_SUCCESS,"+mobUnit(2)+","+testPlayer+`,1238063,"Light Bolt",0x2,`+
			advancedOf("Creature-0-3882-2859-27547-245332-00006BE4EA", none)),
		// A kick that interrupted nothing.
		at(5, "SPELL_CAST_SUCCESS,"+shamanUnit+","+mobUnit(2)+`,57994,"Wind Shear",0x8,`+
			advancedOf("Player-76-0000BBBB", none)),
		// Stopped: Dragon's Breath landed during the cast, then the mob died.
		castStart(6, 1),
		at(6.5, "SPELL_AURA_APPLIED,"+testPlayer+","+mobUnit(1)+`,31661,"Dragon's Breath",0x4,DEBUFF`),
		at(7, "UNIT_DIED,"+none+",nil,0x80000000,0x80000000,"+mobUnit(1)+",0"),
		// The mob died first.
		castStart(8, 3),
		at(8.5, "UNIT_DIED,"+none+",nil,0x80000000,0x80000000,"+mobUnit(3)+",0"),
		// A dispellable debuff, cleansed 1.2 s after it landed (the game logs the removal first).
		at(9, "SPELL_AURA_APPLIED,"+mobUnit(2)+","+testPlayer+`,1234846,"Gorge",0x1,DEBUFF`),
		at(10.2, "SPELL_AURA_REMOVED,"+mobUnit(2)+","+testPlayer+`,1234846,"Gorge",0x1,DEBUFF`),
		at(10.2, "SPELL_DISPEL,"+shamanUnit+","+testPlayer+`,51886,"Cleanse Spirit",0x8,1234846,"Gorge",0x1,DEBUFF`),
		at(59, "CHALLENGE_MODE_END,2923,1,10,59000,100.0,3000.0"),
	)
	feed(t, keys, lines...)
	got, _ := keys.Latest(0)
	var outcomes [][2]string
	for _, s := range got.Stops {
		outcomes = append(outcomes, [2]string{s.Outcome, s.By})
	}
	want := [][2]string{{"interrupted", "Testshaman"}, {"went off", ""}, {"stopped", "Dragon's Breath"}, {"died", ""}}
	if !reflect.DeepEqual(outcomes, want) || got.Stops[0].IntoMS != 500 {
		t.Fatalf("stops = %+v; want %v, the interrupt 500 ms in", got.Stops, want)
	}
	if w := got.WastedKicks; len(w) != 1 || w[0].Player != "Testshaman" || w[0].Ability != "Wind Shear" {
		t.Fatalf("wasted kicks = %+v; want the shaman's second Wind Shear", w)
	}
	if d := got.Debuffs; len(d) != 1 || d[0] != (models.KeyDispel{Player: "Testmage", Spell: "Gorge",
		By: "Testshaman", AfterMS: 1200, ClockMS: 0}) {
		t.Fatalf("debuffs = %+v; want Gorge cleansed by the shaman after 1.2 s", d)
	}
}

func TestSpikesFindTheWorstBurst(t *testing.T) {
	keys := newTracker(t, nil)
	keys.rules.Defensives.Externals = []string{"Pain Suppression"}
	keys.kinds["Pain Suppression"] = "defensive"
	lines := partyOfTwo(at(0, `CHALLENGE_MODE_START,"Voidscar Arena",2923,585,10,[9]`))
	// A hit of 1,000 every second for 70 s, and 50,000 a second from 40 to 42 s.
	for s := 1; s <= 70; s++ {
		amount := 1000
		if s >= 40 && s <= 42 {
			amount = 50_000
		}
		if s == 5 { // far before the burst: no cover
			lines = append(lines, at(5, "SPELL_CAST_SUCCESS,"+shamanUnit+","+testPlayer+
				`,33206,"Pain Suppression",0x2,`+advancedOf("Player-76-0000BBBB", "0000000000000000")))
		}
		lines = append(lines, fmt.Sprintf("11/4/2026 20:%02d:%02d.000-7  SPELL_DAMAGE,%s,%s,1238063,\"Light Bolt\",0x2,%s,%d,%d,-1,2,0,0,0,nil,nil,nil,ST",
			s/60, s%60, mobUnit(1), testPlayer, advancedOf("Player-76-0000AAAA", "0000000000000000"), amount, amount))
	}
	fed := feed(t, keys, lines...)
	got, _ := keys.Latest(fed[len(fed)-1].Time + 1000)
	if len(got.Spikes) != 1 || got.Spikes[0].Damage != 150_000 || got.Spikes[0].Members[0].Player != "Testmage" ||
		got.Spikes[0].Members[0].Taken != 150_000 {
		t.Fatalf("spikes = %+v; want one, the 150,000 from 40 to 42 s, all on the mage", got.Spikes)
	}
	if cds := got.Spikes[0].PartyCDs; len(cds) != 0 {
		t.Fatalf("party cooldowns = %v; want none, the Pain Suppression came 35 s before", cds)
	}
}

func TestStopsAndDebuffsEndingTogetherKeepOneOrder(t *testing.T) {
	read := func() string {
		keys := newTracker(t, nil)
		lines := partyOfTwo(at(0, `CHALLENGE_MODE_START,"Voidscar Arena",2923,585,10,[9]`))
		for mob := range 4 { // four casts no one stops, all timed out by the same line
			lines = append(lines, at(1, "SPELL_CAST_START,"+mobUnit(mob)+
				`,0000000000000000,nil,0x80000000,0x80000000,1238063,"Light Bolt",0x2`))
		}
		for _, unit := range []string{testPlayer, shamanUnit} { // Gorge on both, running out together
			aura := `,1234846,"Gorge",0x1,DEBUFF`
			lines = append(lines, at(2, "SPELL_AURA_APPLIED,"+mobUnit(1)+","+unit+aura),
				at(3, "SPELL_AURA_REMOVED,"+mobUnit(1)+","+unit+aura))
		}
		lines = append(lines, at(20, "SPELL_CAST_SUCCESS,"+shamanUnit+","+mobUnit(1)+`,57994,"Wind Shear",0x8,`+
			advancedOf("Player-76-0000BBBB", "0000000000000000")))
		feed(t, keys, lines...)
		got, _ := keys.Latest(0)
		if len(got.Stops) != 4 || len(got.Debuffs) != 2 {
			t.Fatalf("stops %+v, debuffs %+v; want four stops and two debuffs", got.Stops, got.Debuffs)
		}
		return fmt.Sprint(got.Stops, got.Debuffs)
	}
	first := read()
	for range 20 { // map order changes from run to run: 20 reads catch a sweep that follows it
		if again := read(); again != first {
			t.Fatalf("the same lines gave another order:\n%s\n%s", first, again)
		}
	}
}
