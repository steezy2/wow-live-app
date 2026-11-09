package service

import (
	"fmt"
	"testing"
	"time"

	"wow-live/backend/models"
)

const (
	bossGUID   = "Creature-0-3882-2939-27547-240001-00006BE4EA"
	bossUnit   = bossGUID + `,"Testboss",0x10a48,0x80000000`
	priestUnit = `Player-76-0000CCCC,"Testpriest-Sargeras-US",0x512,0x80000000`
	noUnit     = "0000000000000000,nil,0x80000000,0x80000000"
)

// memPulls is a PullStore in memory.
type memPulls struct{ pulls []models.RaidPull }

func (m *memPulls) SavePull(p models.RaidPull) error      { m.pulls = append(m.pulls, p); return nil }
func (m *memPulls) ListPulls() ([]models.RaidPull, error) { return nil, nil }

// raidAt is a log line s seconds after 20:00 on day (of November 2026).
func raidAt(day int, s float64, line string) string {
	at := time.Date(2026, 11, day, 20, 0, 0, 0, time.UTC).Add(time.Duration(s * float64(time.Second)))
	return at.Format("1/2/2006 15:04:05.000") + "-7  " + line
}

func newRaidTracker(t *testing.T) (*KeyTracker, *memPulls) {
	t.Helper()
	keys := newTracker(t, nil)
	rules := models.RaidRules{Phases: map[string][]models.PhaseTrigger{"Testboss": {{ID: 2, Event: "SPELL_CAST_START",
		Spell: 999}}}}
	rules.Deaths.First, rules.Deaths.BeforeWipeSeconds = 1, 45
	rules.Deaths.MassKillPlayers, rules.Deaths.MassKillWindowSeconds = 2, 10
	rules.Soaks.WindowMS = 1500
	rules.Cooldowns.Names = []string{"Tranquility"}
	bosses := map[string]models.RaidBoss{"Testboss": {Avoidable: []int{1001}, Soak: []int{1002},
		Stacks: map[int]int{1003: 5}, Priority: []string{"Testboss"}}}
	store := &memPulls{}
	if err := keys.EnableRaid(rules, bosses, store); err != nil {
		t.Fatal(err)
	}
	return keys, store
}

// pullLines is a pull of Testboss on Mythic on day from start s for length seconds, ending in a kill or a wipe
// with the boss at hp of 100 health, after a hit into it.
func pullLines(day int, s, length float64, hp int, kill bool, extra ...string) []string {
	end := "0"
	if kill {
		end = "1"
	}
	info := func(guid, spec string) string {
		return raidAt(day, s+0.5, "COMBATANT_INFO,"+guid+",1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,"+spec+
			",[],[],[],[],[]")
	}
	lines := []string{raidAt(day, s, `ENCOUNTER_START,3180,"Testboss",16,20,2939`),
		info("Player-76-0000AAAA", "62"), info("Player-76-0000BBBB", "262")}
	lines = append(lines, raidAt(day, s+1, "SPELL_DAMAGE,"+testPlayer+","+bossUnit+`,44425,"Arcane Barrage",0x40,`+
		fmt.Sprintf("%s,0000000000000000,%d,100,0,0,1470,0,0,0,3,0,100,0,1599.81,-1.23,2610,3.1461,93", bossGUID, hp)+
		",5000,5000,-1,64,0,0,0,nil,nil,nil,ST"))
	lines = append(lines, extra...)
	return append(lines, raidAt(day, s+length, `ENCOUNTER_END,3180,"Testboss",16,20,`+end+`,120000`))
}

// hitOn is a hit of a boss spell on a raider s seconds in.
func hitOn(day int, s float64, event, unit string, spell int, name string) string {
	return raidAt(day, s, event+","+bossUnit+","+unit+fmt.Sprintf(`,%d,"%s",0x8,`, spell, name)+
		"Player-76-0000BBBB,0000000000000000,9,9,0,0,1470,0,0,0,3,0,100,0,1599.81,-1.23,2610,3.1461,93,"+
		"90000,90000,-1,8,0,0,0,nil,nil,nil,ST")
}

func TestRaidPullDeathsMechanicsAndProgress(t *testing.T) {
	keys, store := newRaidTracker(t)
	died := func(s float64, unit string) string { return raidAt(3, s, "UNIT_DIED,"+noUnit+","+unit+",0") }
	feed(t, keys, pullLines(3, 0, 120, 40, false,
		raidAt(3, 0.6, "COMBATANT_INFO,Player-76-0000CCCC,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,257,[],[],[],[],[]"),
		raidAt(3, 5, "SPELL_CAST_SUCCESS,"+shamanUnit+","+noUnit+`,740,"Tranquility",0x8,`+
			advancedOf("Player-76-0000BBBB", "0000000000000000")),
		raidAt(3, 6, "SPELL_CAST_SUCCESS,"+shamanUnit+","+noUnit+`,740,"Tranquility",0x8,`+ // a channel's tick
			advancedOf("Player-76-0000BBBB", "0000000000000000")),
		hitOn(3, 9, "SPELL_DAMAGE", shamanUnit, 1001, "Caustic Waves"),
		hitOn(3, 9.5, "SPELL_PERIODIC_DAMAGE", shamanUnit, 1001, "Caustic Waves"), // a tick isn't a hit
		hitOn(3, 9.6, "SPELL_DAMAGE", testPlayer, 1001, "Caustic Waves"),
		hitOn(3, 9.7, "SPELL_DAMAGE", priestUnit, 1001, "Caustic Waves"),
		died(10, shamanUnit), died(15, testPlayer),
		hitOn(3, 30, "SPELL_DAMAGE", priestUnit, 1002, "Spectral Coils"),
		raidAt(3, 40, "SPELL_AURA_APPLIED_DOSE,"+bossUnit+","+priestUnit+`,1003,"Eternal Venom",0x8,DEBUFF,6`),
		raidAt(3, 60, "SPELL_CAST_START,"+bossUnit+","+noUnit+`,999,"Phase Shift",0x8`),
		died(100, priestUnit),
	)...)
	if len(store.pulls) != 0 {
		t.Fatal("saved before SaveFinished")
	}
	if _, err := keys.SaveFinished(); err != nil || len(store.pulls) != 1 {
		t.Fatalf("saved %d pulls (%v); want the one", len(store.pulls), err)
	}
	p := store.pulls[0]
	if p.Boss != "Testboss" || p.Difficulty != "Mythic" || p.Number != 1 || p.Kill || p.BossPct != 40 ||
		p.Phase != "P2" || p.DurationMS != 120_000 || p.Night != "2026-11-03" {
		t.Fatalf("pull = %+v; want Mythic Testboss pull 1, a wipe at 40%% in P2 after 120 s", p)
	}
	var early []bool
	for _, d := range p.Deaths {
		early = append(early, d.Early)
	}
	if fmt.Sprint(early) != "[true true false]" || p.Deaths[0].Ability != "Caustic Waves" ||
		len(p.Deaths[0].Recap) != 2 || p.Deaths[0].Recap[1].BeforeMS != 500 {
		t.Fatalf("deaths = %+v; want the first, one 105 s before the end, not one 20 s before, each with its last "+
			"hits (ticks too)", p.Deaths)
	}
	if len(p.MassKills) != 1 || p.MassKills[0] != (models.MassKill{Ability: "Caustic Waves", Killed: 2, ClockMS: 10_000}) {
		t.Fatalf("mass kills = %+v; want Caustic Waves killing 2 at 10 s", p.MassKills)
	}
	if len(p.Avoidable) != 1 || len(p.Avoidable[0].Players) != 3 || p.Avoidable[0].Players[0].Count != 1 {
		t.Fatalf("avoidable = %+v; want one direct hit each, the tick left out", p.Avoidable)
	}
	if len(p.Soaks) != 1 || fmt.Sprint(p.Soaks[0]) != "{Spectral Coils 1 1 1 [Testpriest]}" || len(p.Stacks) != 1 || p.Stacks[0].Limit != 5 || p.Stacks[0].Players[0].Count != 6 {
		t.Fatalf("soaks = %+v, stacks = %+v; want one soak by one raider who then died to it, and 6 stacks of 5", p.Soaks, p.Stacks)
	}
	if len(p.Cooldowns) != 1 || p.Cooldowns[0].Ability != "Tranquility" || p.Cooldowns[0].ClockMS != 5_000 ||
		len(p.Priority) != 1 || p.Priority[0].Damage != 5_000 {
		t.Fatalf("cooldowns = %+v, priority = %+v; want Tranquility once at 5 s and the hit into the boss", p.Cooldowns,
			p.Priority)
	}
}

func TestRaidPullsComparedListedAndOverlayTiming(t *testing.T) {
	keys, _ := newRaidTracker(t)
	var lines []string
	lines = append(lines, pullLines(3, 0, 60, 60, false)...)
	lines = append(lines, pullLines(3, 120, 60, 30, false)...)
	lines = append(lines, raidAt(3, 300, `ENCOUNTER_START,3180,"Testboss",8,5,2939`)) // an M+ boss outside a key
	lines = append(lines, pullLines(4, 0, 60, 0, true)...)
	feed(t, keys, lines...)
	nights := keys.RaidNights()
	if len(nights) != 2 || nights[0].Night != "2026-11-04" || len(nights[1].Pulls) != 2 ||
		nights[1].Pulls[1].Number != 2 || nights[0].Pulls[0].Number != 1 {
		t.Fatalf("nights = %+v; want Nov 4's kill first, then Nov 3's two pulls numbered 1 and 2", nights)
	}
	first, err := keys.RaidPull(nights[1].Pulls[0].Start)
	if err != nil || first.Comparisons == nil || first.Comparisons.Tonight.Number != 2 ||
		!first.Comparisons.Ever.Kill {
		t.Fatalf("first pull's comparisons = %+v (%v); want tonight's 30%% pull and the kill as the best ever",
			first.Comparisons, err)
	}
	if _, err := keys.RaidPull(1); err != ErrNoSuchPull {
		t.Fatalf("unknown pull: %v; want ErrNoSuchPull", err)
	}
	end := keys.RaidLive(0).Last.End
	if !keys.BetweenPulls(end+1000) || keys.BetweenPulls(end+RaidLinger.Milliseconds()+1) {
		t.Fatal("the overlay should show the last pull for RaidLinger after it ends, then not")
	}
	feed(t, keys, raidAt(4, 200, `ENCOUNTER_START,3180,"Testboss",16,20,2939`))
	if live := keys.RaidLive(0); !live.Pulling || live.Boss != "Testboss" || keys.BetweenPulls(end+1000) {
		t.Fatalf("live = %+v; want a pull on and the overlay's last-pull card hidden", live)
	}
	if _, ok := keys.Latest(0); ok {
		t.Fatal("a raid pull showed as an M+ key")
	}
}

func TestRaidBossFactsSortsLinesByTheirWords(t *testing.T) {
	g := models.RaidGuide{Name: "Testboss", Lines: []models.RaidGuideLine{
		{Spells: []int{1}, Text: "Caustic Waves - Watch the wings to dodge the waves."},
		{Spells: []int{2}, Text: "Spectral Coils - Soak the group soaks."},
		{Spells: []int{3}, Text: "Eternal Venom - Avoid getting to 11 stacks or you die."},
		{Spells: []int{4}, Text: "Blighted Blood - Dispel the debuff as soon as possible."},
		{Spells: []int{5}, Text: "Anguished Cry - Interrupt the Weakened Doomscale."},
		{Spells: []int{6, 7}, Text: "Mark of Acid / Mark of Blood - Stand 40 yards away to avoid stacks."},
		{Spells: []int{8}, Text: "Rage of the Shackled - Burst down Ula'tek's Heart as it takes double damage."},
	}}
	b := RaidBossFacts(g)
	if fmt.Sprint(b.Avoidable, b.Soak, b.Dispel, b.MustStop, b.Priority) != "[1] [2] [4] [5] [Ula'tek's Heart]" ||
		fmt.Sprint(b.Stacks) != "map[3:11 6:0 7:0]" {
		t.Fatalf("facts = %+v", b)
	}
}

func TestRaidPullWithoutAnEndEndsAtItsLastFighting(t *testing.T) {
	keys, _ := newRaidTracker(t)
	lines := pullLines(3, 0, 60, 50, false)
	lines = lines[:len(lines)-1] // no ENCOUNTER_END: the next pull starts
	lines = append(lines, pullLines(3, 120, 60, 40, false)...)
	lines = lines[:len(lines)-1] // nor here: nothing in the log for an hour (a crash), then a trash line
	lines = append(lines, raidAt(3, 3600+120, "SPELL_DAMAGE,"+testPlayer+","+bossUnit+`,44425,"Arcane Barrage",0x40,`+
		bossGUID+",0000000000000000,40,100,0,0,1470,0,0,0,3,0,100,0,1599.81,-1.23,2610,3.1461,93,"+
		"5000,5000,-1,64,0,0,0,nil,nil,nil,ST"))
	feed(t, keys, lines...)
	pulls := keys.RaidNights()[0].Pulls
	if len(pulls) != 2 || pulls[0].DurationMS != 1000 || pulls[1].DurationMS != 1000 || keys.RaidLive(0).Pulling {
		t.Fatalf("pulls = %+v; want two pulls, each ended at its last hit (1 s in), none still on", pulls)
	}
}
