package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"wow-live/backend/models"
)

const (
	hitsMobGUID = "Creature-0-3882-2859-27547-245336-00006BE4EA"
	hitsMob     = hitsMobGUID + `,"Radiant Spellsower",0x10a48,0x80000000`
	hitsShaman  = `Player-76-0000BBBB,"Testshaman-Sargeras-US",0x512,0x80000000`
	hitsNone    = "0000000000000000"
)

// hitsTracker is a tracker with mplus.toml's [big_hits], Ice Block and Pain Suppression, in a key with Testmage in
// spec and Testshaman.
func hitsTracker(t *testing.T, spec int) (*LogTracker, []string) {
	t.Helper()
	rules := models.MPlusRules{Defensives: models.Defensives{WindowSeconds: 10, Names: []string{"Ice Block"},
		Externals: []string{"Pain Suppression"}},
		BigHits: models.BigHitRules{Seconds: 3, TankPct: 50, OtherPct: 40, RecapSeconds: 10}}
	keys, err := NewLogTracker(nil, nil, rules, nil, &memStore{})
	if err != nil {
		t.Fatal(err)
	}
	info := func(guid string, spec int) string {
		return at(0.5, fmt.Sprintf("COMBATANT_INFO,%s,1,%s%d,[],[],[],[],[]", guid, strings.Repeat("0,", 22), spec))
	}
	return keys, []string{at(0, `CHALLENGE_MODE_START,"Voidscar Arena",2923,585,10,[9]`),
		info("Player-76-0000AAAA", spec), info("Player-76-0000BBBB", 262)}
}

// hpBlock is an advanced-logging block for guid at hp of 1000.
func hpBlock(guid string, hp int) string {
	return fmt.Sprintf("%s,%s,%d,1000,0,0,1470,0,0,0,3,0,100,0,1599.81,-1.23,2610,3.1461,93", guid, hitsNone, hp)
}

// spellHit is a mob's spell hitting Testmage for amount (absorbed more on top), leaving them at hp.
func spellHit(s float64, amount, absorbed, hp int) string {
	return at(s, fmt.Sprintf(`SPELL_DAMAGE,%s,%s,1238063,"Light Bolt",0x2,%s,%d,%d,-1,2,0,0,%d,nil,nil,nil,ST`,
		hitsMob, testPlayer, hpBlock("Player-76-0000AAAA", hp), amount, amount, absorbed))
}

func diedAt(s float64) string {
	return at(s, `UNIT_DIED,`+hitsNone+`,nil,0x80000000,0x80000000,`+testPlayer+`,0`)
}

// TestRecapKeepsTheLastTenSeconds: a death's recap has the player's hits (absorbed part included, a swing without
// health), heals (overheal left out), shields, own defensives and externals on them with their health after each,
// none older than 10 s, and at most the newest recapEvents.
func TestRecapKeepsTheLastTenSeconds(t *testing.T) {
	keys, lines := hitsTracker(t, 62)
	lines = append(lines,
		spellHit(1, 100, 50, 850), // over 10 s before the death
		at(6, `SWING_DAMAGE,`+hitsMob+`,`+testPlayer+`,`+hpBlock(hitsMobGUID, 1000)+`,200,200,-1,1,0,0,0,nil,nil,nil`),
		at(7, `SPELL_HEAL,`+hitsShaman+`,`+testPlayer+`,2061,"Flash Heal",0x2,`+hpBlock("Player-76-0000AAAA", 700)+
			`,500,500,100,0,nil`),
		at(8, `SPELL_ABSORBED,`+hitsMob+`,`+testPlayer+`,1238725,"Feast of Misery",0x20,`+hitsShaman+
			`,17,"Power Word: Shield",0x2,250,250,nil`),
		at(9, `SPELL_CAST_SUCCESS,`+testPlayer+`,`+testPlayer+`,45438,"Ice Block",0x10,`+
			hpBlock("Player-76-0000AAAA", 300)),
		at(9.5, `SPELL_CAST_SUCCESS,`+hitsShaman+`,`+testPlayer+`,33206,"Pain Suppression",0x2,`+
			hpBlock("Player-76-0000BBBB", 1000)),
		diedAt(15))
	for i := range recapEvents + 10 { // after a battle res, more than a recap keeps
		lines = append(lines, at(20+float64(i)/25, `SWING_DAMAGE,`+hitsMob+`,`+testPlayer+`,`+
			hpBlock(hitsMobGUID, 1000)+`,1,1,-1,1,0,0,0,nil,nil,nil`))
	}
	feed(t, keys, append(lines, diedAt(25), at(30, `CHALLENGE_MODE_END,2923,1,10,30000,100.0,3000.0`))...)
	key, _ := keys.Latest(0)
	track, err := keys.Track(key.Start)
	if err != nil {
		t.Fatal(err)
	}
	if len(track.Recaps) != 2 || len(key.Deaths) != 2 {
		t.Fatalf("recaps %+v, deaths %+v; want one recap per death", track.Recaps, key.Deaths)
	}
	first := track.Recaps[0]
	if first.Player != "Testmage" || first.ClockMS != key.Deaths[0].ClockMS || first.MaxHP != 1000 {
		t.Errorf("recap %s at %d, max HP %d; want Testmage at the death's %d, 1000", first.Player, first.ClockMS,
			first.MaxHP, key.Deaths[0].ClockMS)
	}
	before := func(ms int64) int64 { return first.ClockMS - ms }
	want := []models.RecapEvent{
		{ClockMS: before(9000), Kind: "damage", Ability: "Melee", Source: "Radiant Spellsower", Amount: 200},
		{ClockMS: before(8000), Kind: "heal", Ability: "Flash Heal", Source: "Testshaman", Amount: 400,
			HPPct: 70},
		{ClockMS: before(7000), Kind: "absorb", Ability: "Power Word: Shield", Source: "Testshaman",
			Amount: 250},
		{ClockMS: before(6000), Kind: "cast", Ability: "Ice Block", Source: "Testmage", HPPct: 30},
		{ClockMS: before(5500), Kind: "cast", Ability: "Pain Suppression", Source: "Testshaman"},
	}
	if !reflect.DeepEqual(first.Events, want) {
		t.Errorf("first recap's events\n%+v\nwant\n%+v", first.Events, want)
	}
	last := track.Recaps[1].Events
	if len(last) != recapEvents || last[len(last)-1].ClockMS != track.Recaps[1].ClockMS-1440 {
		t.Errorf("second recap: %d events, newest at %d; want the newest %d, 1.44 s before %d", len(last),
			last[len(last)-1].ClockMS, recapEvents, track.Recaps[1].ClockMS)
	}
}

// TestBigHitsByRole: a big hit is a player taking 50% of their max HP within 3 s as a tank, 40% otherwise; a burst
// that stays over the bar is one hit; a death within 10 s marks it Died, whose recap is the death's. A big hit
// survived keeps its own recap, ending with the hit.
func TestBigHitsByRole(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spec  int
		hits  []int // per mille of max HP, one hit a second from 20 s
		death bool
		want  int64 // the big hit's damage, 0 for none
	}{
		{"a tank taking 45%", 73, []int{225, 225}, false, 0},
		{"a tank taking 55%", 73, []int{275, 275}, false, 550},
		{"a mage taking 41%", 62, []int{205, 205}, false, 410},
		{"a burst staying over the bar", 62, []int{250, 250, 250, 250}, false, 500},
		{"a mage dying", 62, []int{450}, true, 450},
	} {
		keys, lines := hitsTracker(t, tc.spec)
		lines = append(lines, at(15, `SPELL_HEAL,`+hitsShaman+`,`+testPlayer+`,2061,"Flash Heal",0x2,`+
			hpBlock("Player-76-0000AAAA", 1000)+`,10,10,0,0,nil`)) // max HP known before the first hit
		for i, n := range tc.hits {
			lines = append(lines, spellHit(20+float64(i), n, 0, 500))
		}
		if tc.death {
			lines = append(lines, diedAt(26))
		}
		feed(t, keys, append(lines, at(40, `CHALLENGE_MODE_END,2923,1,10,40000,100.0,3000.0`))...)
		key, _ := keys.Latest(0)
		track, err := keys.Track(key.Start)
		if err != nil {
			t.Fatal(err)
		}
		if tc.want == 0 {
			if len(key.BigHits) != 0 || len(track.Recaps) != 0 {
				t.Errorf("%s: big hits %+v, recaps %+v; want none", tc.name, key.BigHits, track.Recaps)
			}
			continue
		}
		if len(key.BigHits) != 1 || len(track.Recaps) != 1 {
			t.Fatalf("%s: big hits %+v, recaps %+v; want one of each", tc.name, key.BigHits, track.Recaps)
		}
		hit, recap := key.BigHits[0], track.Recaps[0]
		causes := []models.SpikeCause{{Ability: "Light Bolt", Source: "Radiant Spellsower", Damage: hit.Taken,
			Hits: min(len(tc.hits), 2)}}
		if hit.Player != "Testmage" || hit.Taken != tc.want || hit.Pct != float64(tc.want)/10 || hit.Died != tc.death ||
			!reflect.DeepEqual(hit.Causes, causes) {
			t.Errorf("%s: big hit %+v; want Testmage taking %d, causes %+v, died %v", tc.name, hit, tc.want, causes,
				tc.death)
		}
		if tc.death && recap.ClockMS != key.Deaths[0].ClockMS || !tc.death && recap.ClockMS != hit.ClockMS+3000 {
			t.Errorf("%s: recap at %d; want the death's or the hit's end", tc.name, recap.ClockMS)
		}
	}
}
