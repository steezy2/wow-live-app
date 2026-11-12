package service

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"slices"
	"strings"
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
type memPulls struct {
	pulls  []models.RaidPull
	tracks map[int64]models.KeyTrack
}

func (m *memPulls) SavePull(p models.RaidPull) error      { m.pulls = append(m.pulls, p); return nil }
func (m *memPulls) ListPulls() ([]models.RaidPull, error) { return m.pulls, nil }
func (m *memPulls) DeletePull(start int64) error {
	m.pulls = slices.DeleteFunc(m.pulls, func(p models.RaidPull) bool { return p.Start == start })
	return nil
}
func (m *memPulls) SaveTrack(start int64, t models.KeyTrack) error {
	m.tracks[start] = t
	return nil
}
func (m *memPulls) Track(start int64) (models.KeyTrack, error) {
	if t, ok := m.tracks[start]; ok {
		return t, nil
	}
	return models.KeyTrack{}, fs.ErrNotExist
}

// raidAt is a log line s seconds after 20:00 on day (of November 2026).
func raidAt(day int, s float64, line string) string {
	at := time.Date(2026, 11, day, 20, 0, 0, 0, time.UTC).Add(time.Duration(s * float64(time.Second)))
	return at.Format("1/2/2006 15:04:05.000") + "-7  " + line
}

func newRaidTracker(t *testing.T) (*LogTracker, *memPulls) {
	t.Helper()
	keys := newTracker(t, nil)
	rules := models.RaidRules{Phases: map[string][]models.PhaseTrigger{"Testboss": {{ID: 2, Event: "SPELL_CAST_START",
		Spell: 999}}}}
	rules.Deaths.First, rules.Deaths.BeforeWipeSeconds = 1, 45
	rules.Deaths.MassKillPlayers, rules.Deaths.MassKillWindowSeconds = 2, 10
	rules.Soaks.WindowMS = 1500
	rules.Cooldowns.Names = []string{"Tranquility"}
	rules.Suggestions = models.SuggestionRules{Most: 5, SpikeSeconds: 6, AddsLonger: 1.25, PriorityBelow: 0.8}
	bosses := map[string]models.RaidBoss{"Testboss": {NPCs: []int{240001}, Avoidable: []int{1001}, Soak: []int{1002},
		Stacks: map[int]int{1003: 5}, Priority: []string{"Testboss"}}}
	store := &memPulls{tracks: map[int64]models.KeyTrack{}}
	if err := keys.EnableRaid(rules, bosses, nil, store); err != nil {
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
	p, _ := keys.RaidPull(store.pulls[0].Start) // numbered when read
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
	if len(p.Avoidable) != 1 || len(p.Avoidable[0].Players) != 3 || p.Avoidable[0].Players[0].Count != 1 ||
		len(p.AvoidableHits) != 3 || p.AvoidableHits[0].Damage == 0 {
		t.Fatalf("avoidable = %+v, hits %+v; want one direct hit each with its damage, the tick left out", p.Avoidable,
			p.AvoidableHits)
	}
	if len(p.Soaks) != 1 || fmt.Sprint(p.Soaks[0]) != "{Spectral Coils 1 1 1 0 [{30000 [Testpriest]}] [Testpriest]}" || len(p.Stacks) != 1 || p.Stacks[0].Limit != 5 || p.Stacks[0].Players[0].Count != 6 {
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
		nights[1].Pulls[0].Number != 2 || nights[0].Pulls[0].Number != 1 {
		t.Fatalf("nights = %+v; want Nov 4's kill first, then Nov 3's pulls 2 and 1, the newest first", nights)
	}
	first, err := keys.RaidPull(nights[1].Pulls[1].Start)
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

func TestRaidPullLanesPutEachMechanicOnTheTimeline(t *testing.T) {
	keys, store := newRaidTracker(t)
	feed(t, keys, pullLines(3, 0, 120, 40, false,
		raidAt(3, 5, "SPELL_CAST_SUCCESS,"+shamanUnit+","+noUnit+`,740,"Tranquility",0x8,`+
			advancedOf("Player-76-0000BBBB", "0000000000000000")),
		hitOn(3, 9, "SPELL_DAMAGE", shamanUnit, 1001, "Caustic Waves"),
		hitOn(3, 30, "SPELL_DAMAGE", shamanUnit, 1002, "Spectral Coils"),
		raidAt(3, 40, "SPELL_AURA_APPLIED_DOSE,"+bossUnit+","+shamanUnit+`,1003,"Eternal Venom",0x8,DEBUFF,6`),
		raidAt(3, 60, "SPELL_CAST_START,"+bossUnit+","+noUnit+`,999,"Phase Shift",0x8`),
		raidAt(3, 70, "UNIT_DIED,"+noUnit+","+shamanUnit+",0"),
	)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range store.pulls[0].Lanes {
		m := l.Marks[len(l.Marks)-1]
		got = append(got, fmt.Sprintf("%s/%s %d-%d %s %v", l.Kind, l.Name, m.ClockMS/1000, m.EndMS/1000, m.Text, m.Bad))
	}
	want := "[phases/Phases 60-120 P2 false deaths/Deaths 70-0 Testshaman · Spectral Coils true " +
		"cooldowns/Raid cooldowns 5-0 Testshaman · Tranquility false avoidable/Caustic Waves 9-0 Testshaman true " +
		"soak/Spectral Coils 30-0 1 soaked false stacks/Eternal Venom 40-0 Testshaman 6 true]"
	if fmt.Sprint(got) != want {
		t.Fatalf("lanes' last marks =\n%v\nwant\n%s", got, want)
	}
}

func TestSuggestionsPutWhatToFixFirst(t *testing.T) {
	death := func(player, ability string, early bool, ready ...string) models.RaidDeath {
		return models.RaidDeath{KeyDeath: models.KeyDeath{Player: player, Ability: ability, Ready: ready}, Early: early}
	}
	p := models.RaidPull{
		Deaths: []models.RaidDeath{death("A", "Caustic Waves", true), death("B", "Caustic Waves", true, "Ice Block"),
			death("C", "Melee", false)},
		Avoidable: []models.RaidHits{{Ability: "Caustic Waves", Players: []models.PlayerCount{{Player: "A", Count: 1}, {Player: "B", Count: 2}}},
			{Ability: "Toxic Spill", Players: []models.PlayerCount{{Player: "D", Count: 1}}}},
		Soaks:     []models.RaidSoak{{Ability: "Spectral Coils", Died: []string{"E"}}},
		MassKills: []models.MassKill{{Ability: "Caustic Waves", Killed: 2, ClockMS: 61_000}},
		Spikes:    []models.KeySpike{{ClockMS: 90_000, Damage: 7_200_000, PartyCDs: []string{}}},
		Stacks: []models.RaidStacks{{Ability: "Eternal Venom", Limit: 5, Players: []models.PlayerCount{{Player: "F", Count: 6}, {Player: "G", Count: 5}},
			Over: []string{"F"}}},
		WentOff: []models.KeyStop{{Spell: "Anguished Cry"}, {Spell: "Anguished Cry"}},
		Adds: []models.RaidAdd{{Name: "Blightscale Clutch", LongestMS: 30_000}, {Name: "Blightscale Viper",
			Spawns: []models.RaidSpawn{{ClockMS: 75_000, Cause: "Caustic Waves", Players: []string{"H"}, Spawner: true},
				{ClockMS: 80_000, Cause: "Not picked up", Players: []string{}}}}},
		Priority: []models.RaidPriority{{Target: "Ula'tek's Heart", Damage: 1_000_000}},
	}
	best := &models.RaidPull{Number: 4, Adds: []models.RaidAdd{{Name: "Blightscale Clutch", LongestMS: 12_000}},
		Priority: []models.RaidPriority{{Target: "Ula'tek's Heart", Damage: 3_000_000}}}
	want := []string{
		"2 Blightscale Vipers spawned: H (Caustic Waves at 1:15), 1 not picked up (first at 1:20)",
		"2 deaths to Caustic Waves, which can be dodged: A, B",
		"1 death soaking Spectral Coils: E",
		"1 early death with a defensive ready: B (Ice Block)",
		"Caustic Waves killed 2 raiders at 1:01",
		"No raid cooldown for the worst damage (1.2M/s at 1:30)",
		"1 raider went past 5 stacks of Eternal Venom: F",
		"Anguished Cry went off 2 times",
		"Blightscale Clutch lived up to 0:30 (0:12 in pull 4)",
		"Toxic Spill hit 1 raider 1 time",
		"Damage into Ula'tek's Heart: 1.0M (3.0M in pull 4)",
	}
	rules := models.SuggestionRules{Most: 20, SpikeSeconds: 6, AddsLonger: 1.25, PriorityBelow: 0.8}
	if got := suggestions(p, best, rules); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("suggestions =\n%q\nwant\n%q", got, want)
	}
	rules.Most = 3
	if got := suggestions(p, nil, rules); len(got) != 3 {
		t.Fatalf("suggestions = %q; want the first 3", got)
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

func TestARaidPullStaysOnThroughASilentLog(t *testing.T) {
	keys, _ := newRaidTracker(t)
	lines := pullLines(3, 0, 60, 50, false)
	last := feed(t, keys, lines[:len(lines)-1]...)[len(lines)-2].Time // the game writes nothing during a fight
	keys.CloseQuiet(last + 2*quietMS)
	if keys.Abandon() || !keys.RaidLive(0).Pulling {
		t.Fatal("a raid pull was closed for a silent log; want it still on, however long the fight")
	}
}

func TestAnAddWhoseHealthReachesZeroDiedWithoutADeathLine(t *testing.T) {
	keys, store := newRaidTracker(t)
	add := "Creature-0-3882-2939-27547-253032-00006BE4EB"
	hit := func(s float64, hp int) string {
		return raidAt(3, s, "SPELL_DAMAGE,"+testPlayer+","+add+`,"Testfin",0xa48,0x80000000,44425,"Arcane Barrage",0x40,`+
			fmt.Sprintf("%s,0000000000000000,%d,40,0,0,1470,0,0,0,3,0,100,0,1599.81,-1.23,2610,3.1461,93", add, hp)+
			",5000,5000,-1,64,0,0,0,nil,nil,nil,ST")
	}
	feed(t, keys, pullLines(3, 0, 60, 50, false, hit(10, 60), hit(20, 0))...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	if a := store.pulls[0].Adds; len(a) != 1 || a[0].Killed != 1 || a[0].AliveEnd != 0 || a[0].LongestMS != 10_000 {
		t.Fatalf("adds = %+v; want the add killed after 10 s, as its health reached 0 with no UNIT_DIED", a)
	}
}

func TestRaidPullTrackKeepsTheRaidersAndTheBoss(t *testing.T) {
	keys, store := newRaidTracker(t)
	feed(t, keys, pullLines(3, 0, 60, 50, false, hitOn(3, 9, "SPELL_DAMAGE", shamanUnit, 1001, "Caustic Waves"))...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	start := store.pulls[0].Start
	track, err := keys.RaidTrack(start)
	if err != nil || len(track.Players["Testshaman"]) != 1 || track.Players["Testshaman"][0].Map != 2610 ||
		len(track.Bosses[bossGUID]) != 1 || track.Bosses[bossGUID][0].Name != "Testboss" ||
		track.Bosses[bossGUID][0].ClockMS != 1000 {
		t.Fatalf("track = %+v (%v); want the shaman's place 9 s in and the boss's 1 s in, on map 2610", track, err)
	}
	if _, err := keys.RaidTrack(start + 1); err != ErrNoSuchPull {
		t.Fatalf("unknown pull's track: %v; want ErrNoSuchPull", err)
	}
}

func TestAnEncounterNobodyFoughtIsNotAPull(t *testing.T) {
	keys, store := newRaidTracker(t)
	feed(t, keys, raidAt(3, 0, `ENCOUNTER_START,3180,"Testboss",16,20,2939`),
		raidAt(3, 0.004, `ENCOUNTER_END,3180,"Testboss",16,20,0,4`))
	feed(t, keys, pullLines(3, 60, 60, 50, false)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	if pulls := keys.RaidNights()[0].Pulls; len(pulls) != 1 || pulls[0].Number != 1 || len(store.pulls) != 1 {
		t.Fatalf("pulls = %+v, saved %d; want only the fought pull, as pull 1", pulls, len(store.pulls))
	}
}

func TestMassKillsLeaveOutDeathsWithNoHitSeen(t *testing.T) {
	deaths := []models.KeyDeath{{ClockMS: 1000}, {ClockMS: 2000}, {Ability: "Malice", ClockMS: 3000},
		{Ability: "Malice", ClockMS: 4000}}
	if got := massKills(deaths, 2, 10_000); len(got) != 1 || got[0].Ability != "Malice" {
		t.Fatalf("mass kills = %+v; want only Malice's, not the two deaths with no known cause", got)
	}
}

func TestRaidPullKeepsAbilitiesTargetsAndHowLongBuffsLasted(t *testing.T) {
	keys, store := newRaidTracker(t)
	keys.rules.Lust.Names = []string{"Bloodlust"}
	aura := func(s float64, event, spell string) string {
		return raidAt(3, s, event+","+shamanUnit+","+testPlayer+`,740,"`+spell+`",0x8,BUFF`)
	}
	feed(t, keys, pullLines(3, 0, 120, 40, false,
		raidAt(3, 5, "SPELL_CAST_SUCCESS,"+shamanUnit+","+noUnit+`,740,"Tranquility",0x8,`+
			advancedOf("Player-76-0000BBBB", "0000000000000000")),
		aura(13, "SPELL_AURA_REMOVED", "Tranquility"),
		aura(20, "SPELL_AURA_APPLIED", "Bloodlust"), aura(60, "SPELL_AURA_REMOVED", "Bloodlust"),
	)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	p := store.pulls[0]
	if len(p.Abilities) != 2 || p.Abilities[0] != (models.KeyAbility{Player: "Testmage", Ability: "Arcane Barrage",
		Damage: 5000, Hits: 1, School: 0x40}) || fmt.Sprint(p.Targets) != "[{Testmage Testboss 5000}]" {
		t.Fatalf("abilities = %+v, targets = %+v; want the mage's arcane Arcane Barrage for 5000 into Testboss",
			p.Abilities, p.Targets)
	}
	var got []string
	for _, l := range p.Lanes {
		if l.Kind == "lust" || l.Kind == "cooldowns" {
			got = append(got, fmt.Sprintf("%s %d-%d", l.Kind, l.Marks[0].ClockMS/1000, l.Marks[0].EndMS/1000))
		}
	}
	if fmt.Sprint(got) != "[lust 20-60 cooldowns 5-13]" || p.Cooldowns[0].EndMS != 13_000 {
		t.Fatalf("stretches = %v, cooldowns = %+v; want Bloodlust from 20 to 60 s and Tranquility from 5 to 13",
			got, p.Cooldowns)
	}
}

func TestARaidDeathKnowsTheDefensivesUsedInEarlierPullsThatNight(t *testing.T) {
	keys, _ := newRaidTracker(t)
	keys.kinds["Light's Potential"] = "combat potion"
	cast := func(s float64, spell string) string {
		return raidAt(3, s, "SPELL_CAST_SUCCESS,"+testPlayer+","+noUnit+`,45438,"`+spell+`",0x10,`+
			advancedOf("Player-76-0000AAAA", "0000000000000000"))
	}
	died := func(day int, s float64) string { return raidAt(day, s, "UNIT_DIED,"+noUnit+","+testPlayer+",0") }
	lines := pullLines(3, 0, 60, 60, false, cast(5, "Ice Block"), cast(6, "Light's Potential"))
	feed(t, keys, append(lines, pullLines(3, 600, 60, 60, false, died(3, 610))...)...)
	if d := keys.RaidLive(0).Last.Deaths; len(d) != 1 || fmt.Sprint(d[0].Ready) != "[Ice Block]" {
		t.Fatalf("deaths = %+v; want the mage dead with the Ice Block of the pull before still ready", d)
	}
	feed(t, keys, pullLines(4, 0, 60, 60, false, died(4, 10))...)
	if d := keys.RaidLive(0).Last.Deaths; len(d) != 1 || len(d[0].Ready) > 0 {
		t.Fatalf("deaths = %+v; want nothing known ready the next night", d)
	}
}

func TestSuggestionsForAPullWithoutReferenceMechanics(t *testing.T) {
	death := func(player string) models.RaidDeath {
		return models.RaidDeath{KeyDeath: models.KeyDeath{Player: player, Ability: "Venom Bolt"}, Early: true}
	}
	rows := []models.KeyMeterRow{{Player: "A"}, {Player: "B"}, {Player: "C"}}
	p := models.RaidPull{DurationMS: 120_000, Deaths: []models.RaidDeath{death("A"), death("B")},
		Meter: models.KeyMeter{Rows: rows},
		Dispels: []models.KeyDispel{{Player: "D", Spell: "Exploding Infection", AfterMS: 8000},
			{Player: "F", Spell: "Exploding Infection", By: "E", AfterMS: 6300},
			{Player: "G", Spell: "Exploding Infection", By: "E", AfterMS: 2000}},
		Cooldowns: []models.KeyCast{{Player: "A", Kind: "combat potion", ClockMS: 1000},
			{Player: "B", Ability: "Tranquility", Kind: "raid cooldown", ClockMS: 130_000},
			{Player: "C", Ability: "Revival", Kind: "raid cooldown", ClockMS: 132_000},
			{Player: "C", Ability: "Rewind", Kind: "raid cooldown", ClockMS: 150_000}}}
	want := []string{
		"2 deaths to Venom Bolt: A, B",
		"Exploding Infection sat too long on 2 raiders: D (ran out), F (6.3 s)",
		"No Bloodlust this pull",
		"2 raiders used no combat potion: B, C",
		"Raid cooldowns stacked at 2:10: Tranquility, Revival",
	}
	rules := models.SuggestionRules{Most: 20, LustAfterSeconds: 60, DispelSlowSeconds: 4, CooldownOverlapSeconds: 3}
	if got := suggestions(p, nil, rules); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("suggestions =\n%q\nwant\n%q", got, want)
	}
	p.DurationMS, p.Lusts = 30_000, nil
	if got := suggestions(p, nil, rules); slices.Contains(got, "No Bloodlust this pull") || len(got) != 3 {
		t.Fatalf("suggestions = %q; a 30 s pull needs no Bloodlust or potions yet", got)
	}
}

func TestRaidBossPctIsTheBossesOwnHealth(t *testing.T) {
	const maxHP = 1_673_145_000
	// Pull 23 on Nov 10: Ula'tek, its Venomous Heart and its two Gore Rattle tails at the wipe (Warcraft Logs: 48%).
	ulatek := map[string]*raidUnit{
		"Vehicle-0-1-1-1-257758-01":  {name: "Ula'tek", hp: 795_742_315, maxHP: maxHP},
		"Creature-0-1-1-1-267460-01": {name: "Venomous Heart", hp: 807_840_368, maxHP: maxHP},
		"Vehicle-0-1-1-1-259555-01":  {name: "Gore Rattle", hp: 1_460_667_250, maxHP: maxHP},
		"Vehicle-0-1-1-1-259555-02":  {name: "Gore Rattle", hp: 1_366_868_049, maxHP: maxHP},
	}
	council := map[string]*raidUnit{
		"Creature-0-1-1-1-11-01": {name: "Trader Gebbo", hp: 30, maxHP: 100},
		"Creature-0-1-1-1-12-01": {name: "Mor'zahi", hp: 50, maxHP: 100},
		"Creature-0-1-1-1-99-01": {name: "Add", hp: 0, maxHP: 100},
	}
	for _, c := range []struct {
		name  string
		boss  models.RaidBoss
		units map[string]*raidUnit
		want  float64
	}{
		{"the unit named as the encounter", models.RaidBoss{Name: "Ula'tek", NPCs: []int{257758, 259555, 267460}},
			ulatek, 47.6},
		{"the reference's health NPCs", models.RaidBoss{Name: "Ula'tek", NPCs: []int{259555}, Health: []int{267460}},
			ulatek, 48.3},
		{"a council: all its NPCs", models.RaidBoss{Name: "The Lost Explorers", NPCs: []int{11, 12}}, council, 40},
	} {
		got := (&raidRun{boss: c.boss, units: c.units}).bossPct(false)
		if math.Abs(got-c.want) > 0.05 {
			t.Errorf("%s: boss at %.2f%%; want %.1f%%", c.name, got, c.want)
		}
	}
}

func TestRaidPullPickupsBreaksAndSpawns(t *testing.T) {
	keys, store := newRaidTracker(t)
	boss := keys.raid.bosses["Testboss"]
	boss.Pickups = []models.RaidPickupRule{{Aura: 2001, Name: "an egg"}}
	boss.Breaks = []models.RaidBreakRule{{Aura: 2002, Effect: 2003, WithinMS: 250}}
	boss.Spawns = []models.RaidSpawnRule{{Add: "Viper", Cause: 2004, WithinMS: 2000},
		{Add: "Warden", Cause: 2005, WithinMS: 3000}}
	keys.raid.bosses["Testboss"] = boss
	aura := func(s float64, event, unit string, spell int, name string) string {
		return raidAt(3, s, fmt.Sprintf(`%s,%s,%s,%d,"%s",0x8,DEBUFF`, event, bossUnit, unit, spell, name))
	}
	add := func(s float64, guid, name string) string { // a raider hitting an add: it's seen, and it was hit
		return raidAt(3, s, "SPELL_DAMAGE,"+testPlayer+","+guid+`,"`+name+`",0xa48,0x80000000,44425,"Arcane Barrage",0x40`)
	}
	feed(t, keys, pullLines(3, 0, 120, 40, false,
		raidAt(3, 0.6, "COMBATANT_INFO,Player-76-0000CCCC,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,257,[],[],[],[],[]"),
		aura(10, "SPELL_AURA_APPLIED", testPlayer, 2001, "Noxious Shell"),
		aura(10.5, "SPELL_AURA_APPLIED", shamanUnit, 2001, "Noxious Shell"),
		aura(30, "SPELL_AURA_APPLIED", priestUnit, 2001, "Noxious Shell"), // the next wave
		aura(40, "SPELL_AURA_REMOVED", testPlayer, 2002, "Grasping Fangs"),
		aura(40.01, "SPELL_AURA_APPLIED", testPlayer, 2003, "Blight Vein"),
		aura(40.02, "SPELL_AURA_APPLIED", shamanUnit, 2003, "Blight Vein"),
		aura(50, "SPELL_AURA_REMOVED", shamanUnit, 2002, "Grasping Fangs"), // nothing followed
		aura(51, "SPELL_AURA_REMOVED", testPlayer, 2002, "Grasping Fangs"), // right after it: the same set
		aura(70, "SPELL_AURA_REMOVED", priestUnit, 2002, "Grasping Fangs"), // went with the priest's death
		raidAt(3, 70.2, "UNIT_DIED,"+noUnit+","+priestUnit+",0"),
		aura(59, "SPELL_AURA_APPLIED", priestUnit, 2004, "Noxious Splash"),
		aura(59.2, "SPELL_AURA_APPLIED", shamanUnit, 2004, "Noxious Splash"),
		add(60, "Creature-0-1-2939-1-300-01", "Viper"),
		add(90, "Creature-0-1-2939-1-300-02", "Viper"), // no splash just before
		hitOn(3, 94, "SPELL_DAMAGE", testPlayer, 2004, "Noxious Splash"),
		add(95, "Creature-0-1-2939-1-300-03", "Viper"), // a splash that landed as a hit
		raidAt(3, 98, "SPELL_CAST_START,"+bossUnit+","+noUnit+`,2005,"Hatching Doom",0x8`),
		add(100, "Creature-0-1-2939-1-301-01", "Warden"),
	)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	p := store.pulls[0]
	if got := fmt.Sprint(p.Pickups); got != "[{10000 an egg [Testmage Testshaman] []} {30000 an egg [Testpriest] []}]" {
		t.Errorf("pickups = %s; want two waves", got)
	}
	if got := fmt.Sprint(p.Breaks); got != "[{40000 Testmage Grasping Fangs Blight Vein 2 []} "+
		"{50000 Testshaman Grasping Fangs  0 [Testmage]}]" {
		t.Errorf("breaks = %s; want the first setting off Blight Vein on 2, the second nothing with the mage's right after it", got)
	}
	spawns := map[string]string{}
	for _, a := range p.Adds {
		spawns[a.Name] = fmt.Sprint(a.Spawns)
	}
	if spawns["Viper"] != "[{60000 Noxious Splash [Testpriest Testshaman] false} {90000  [] false} "+
		"{95000 Noxious Splash [Testmage] false}]" ||
		spawns["Warden"] != "[{100000 Hatching Doom [] false}]" {
		t.Errorf("spawns = %v; want the first Viper from the splash, the second from nothing, the Warden from the cast",
			spawns)
	}
	// An enemy's cast lands on nobody, and the page reads the spawn's players as a list.
	warden := p.Adds[slices.IndexFunc(p.Adds, func(a models.RaidAdd) bool { return a.Name == "Warden" })]
	if got, _ := json.Marshal(warden.Spawns); string(got) != `[{"clockMS":100000,"cause":"Hatching Doom","players":[]}]` {
		t.Errorf("Warden spawns = %s; want its players an empty list, not null", got)
	}
}

func TestRaidBuffGapsAtThePull(t *testing.T) {
	keys, store := newRaidTracker(t)
	keys.raid.rules.Consumables.Vantus = []string{"Vantus Rune"}
	keys.raid.rules.Consumables.VantusDifficulties = []string{"Mythic"}
	named := func(id int, name string) string { // a line that tells the log's name of an aura
		return raidAt(3, 0, fmt.Sprintf(`SPELL_AURA_APPLIED,%s,%s,%d,"%s",0x8,BUFF`, testPlayer, testPlayer, id, name))
	}
	info := func(guid, spec, auras string) string {
		return raidAt(3, 10.5, "COMBATANT_INFO,"+guid+",1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,"+spec+
			",[],[],[],[],["+auras+"]")
	}
	const mage = "Player-76-0000AAAA"
	feed(t, keys,
		raidAt(3, 0, `ZONE_CHANGE,2939,"The Venomous Abyss",16`),
		named(3001, "Flask of Tests"), named(3002, "Well Fed"), named(3003, "Vantus Rune: Radiant"),
		named(1459, "Arcane Intellect"), named(3004, "Skyfury"),
		raidAt(3, 1, `SPELL_AURA_APPLIED,`+shamanUnit+`,`+shamanUnit+`,3004,"Skyfury",0x8,BUFF`),
		raidAt(3, 10, `ENCOUNTER_START,3180,"Testboss",16,20,2939`),
		info(mage, "62", mage+",3001,1,"+mage+",3002,1,"+mage+",3003,1,"+mage+",1459,1,"+mage+",3004,1"),
		info("Player-76-0000BBBB", "262", mage+",3001,1,"+mage+",1459,1"),
		raidAt(3, 11, "SPELL_DAMAGE,"+testPlayer+","+bossUnit+`,44425,"Arcane Barrage",0x40,`+
			bossGUID+",0000000000000000,40,100,0,0,1470,0,0,0,3,0,100,0,1599.81,-1.23,2610,3.1461,93"+
			",5000,5000,-1,64,0,0,0,nil,nil,nil,ST"),
		raidAt(3, 30, "UNIT_DIED,"+noUnit+","+testPlayer+",0"), // a death takes their raid buffs off
		raidAt(3, 60, `ENCOUNTER_END,3180,"Testboss",16,20,0,50000`),
	)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(store.pulls[0].BuffGaps); got != "[{Testshaman Skyfury 0 50000} "+
		"{Testshaman Vantus Rune 0 50000} {Testshaman food 0 50000}]" {
		t.Errorf("missing at the pull = %s; want the shaman's food, Vantus Rune and Skyfury the whole pull", got)
	}
}

func TestMissingWindows(t *testing.T) {
	keys, store := newRaidTracker(t)
	aura := func(s float64, event, unit string, id int, name string) string {
		return raidAt(3, s, fmt.Sprintf(`%s,%s,%s,%d,"%s",0x40,BUFF`, event, testPlayer, unit, id, name))
	}
	died := func(s float64, unit string) string { return raidAt(3, s, "UNIT_DIED,"+noUnit+","+unit+",0") }
	feed(t, keys, pullLines(3, 0, 120, 40, false,
		raidAt(3, 0.6, "COMBATANT_INFO,Player-76-0000CCCC,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,257,[],[],[],[],[]"),
		aura(20, "SPELL_AURA_REMOVED", shamanUnit, 1459, "Arcane Intellect"), // ran out: 20 s without
		aura(40, "SPELL_AURA_APPLIED", shamanUnit, 1459, "Arcane Intellect"),
		died(50, testPlayer), aura(50, "SPELL_AURA_REMOVED", testPlayer, 1459, "Arcane Intellect"),
		raidAt(3, 70, "SPELL_RESURRECT,"+priestUnit+","+testPlayer+`,20484,"Rebirth",0x8`), // battle-res'd
		aura(75, "SPELL_AURA_APPLIED", testPlayer, 1459, "Arcane Intellect"),
		died(80, priestUnit), aura(80, "SPELL_AURA_REMOVED", priestUnit, 1459, "Arcane Intellect"), // dead to the end
		aura(90, "SPELL_AURA_REMOVED", shamanUnit, 3001, "Flask of Tests"), // back 2 s later: too short
		aura(92, "SPELL_AURA_APPLIED", shamanUnit, 3001, "Flask of Tests"),
		aura(100, "SPELL_AURA_REMOVED", shamanUnit, 3002, "Well Fed"), // never back
	)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	var got []models.RaidMissing
	for _, g := range store.pulls[0].BuffGaps {
		if g.FromMS > 0 { // the start check's gaps are TestRaidBuffGapsAtThePull's
			got = append(got, g)
		}
	}
	if fmt.Sprint(got) != "[{Testshaman Arcane Intellect 20000 40000} {Testmage Arcane Intellect 50000 75000} "+
		"{Testshaman food 100000 120000}]" {
		t.Errorf("buff gaps = %v; want the shaman's 20 s, the mage's death to the buff after the battle res, and the "+
			"food to the end, not the priest's (dead throughout) or the 2 s flask", got)
	}
}

func TestPickupExpect(t *testing.T) {
	// Kith'ix: every living raider takes a light mote before Extinguish. The druid misses the first wave (the priest
	// died before it), and nobody picks one up for the second.
	const druid = `Player-76-0000DDDD,"Testdruid-Sargeras-US",0x512,0x80000000`
	keys, store := newRaidTracker(t)
	boss := keys.raid.bosses["Testboss"]
	boss.Pickups = []models.RaidPickupRule{{Aura: 2001, Name: "a light mote", Expect: 2010}}
	keys.raid.bosses["Testboss"] = boss
	mote := func(s float64, event, unit string) string {
		return raidAt(3, s, fmt.Sprintf(`%s,%s,%s,2001,"Light's Embrace",0x2,DEBUFF`, event, bossUnit, unit))
	}
	extinguish := func(s float64) string {
		return raidAt(3, s, "SPELL_CAST_SUCCESS,"+bossUnit+","+noUnit+`,2010,"Extinguish",0x20`)
	}
	feed(t, keys, pullLines(3, 0, 120, 40, false,
		raidAt(3, 0.6, "COMBATANT_INFO,Player-76-0000CCCC,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,257,[],[],[],[],[]"),
		raidAt(3, 0.7, "COMBATANT_INFO,Player-76-0000DDDD,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,105,[],[],[],[],[]"),
		hitOn(3, 5, "SPELL_DAMAGE", druid, 1004, "Bite"),
		hitOn(3, 6, "SPELL_DAMAGE", priestUnit, 1004, "Bite"),
		raidAt(3, 20, "UNIT_DIED,"+noUnit+","+priestUnit+",0"),
		mote(30, "SPELL_AURA_APPLIED", testPlayer),
		mote(33, "SPELL_AURA_APPLIED", shamanUnit), // a wave takes seconds: still the first
		extinguish(36),
		mote(36.2, "SPELL_AURA_REMOVED", testPlayer), mote(36.2, "SPELL_AURA_REMOVED", shamanUnit),
		extinguish(80),
	)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	p, _ := keys.RaidPull(store.pulls[0].Start)
	if got := fmt.Sprint(p.Pickups); got != "[{30000 a light mote [Testmage Testshaman] [Testdruid]} "+
		"{80000 a light mote [] [Testdruid Testmage Testshaman]}]" {
		t.Errorf("pickups = %s; want the druid missing the first wave and the three living raiders the second", got)
	}
	lane := slices.IndexFunc(p.Lanes, func(l models.RaidLane) bool { return l.Kind == "pickups" })
	if lane < 0 || len(p.Lanes[lane].Marks) != 2 || !p.Lanes[lane].Marks[0].Bad || !p.Lanes[lane].Marks[1].Bad {
		t.Errorf("lanes = %+v; want a pickups lane with both waves marked bad", p.Lanes)
	}
	if !slices.Contains(p.Suggestions, "Didn't pick up a light mote: Testdruid, Testmage, Testshaman") {
		t.Errorf("suggestions = %q; want the raiders who missed a light mote", p.Suggestions)
	}
}

func TestRaidTrackKeepsBossHealthCastsAndAbilities(t *testing.T) {
	keys, store := newRaidTracker(t)
	cast := func(s float64, event string, spell int, name string) string {
		return raidAt(3, s, fmt.Sprintf(`%s,%s,%s,%d,"%s",0x40,`, event, testPlayer, noUnit, spell, name)+
			advancedOf("Player-76-0000AAAA", "0000000000000000"))
	}
	feed(t, keys, pullLines(3, 0, 30, 40, false,
		cast(2, "SPELL_CAST_START", 116, "Frostbolt"),
		cast(4, "SPELL_CAST_SUCCESS", 116, "Frostbolt"),
		cast(5, "SPELL_CAST_SUCCESS", 44425, "Arcane Barrage"), // an instant
		cast(6, "SPELL_CAST_START", 116, "Frostbolt"),
		// a key pressed again mid-cast
		raidAt(3, 6.5, "SPELL_CAST_FAILED,"+testPlayer+","+noUnit+`,116,"Frostbolt",0x10,"Not yet recovered"`),
		raidAt(3, 7, "SPELL_CAST_FAILED,"+testPlayer+","+noUnit+`,116,"Frostbolt",0x10,"Interrupted"`),
		cast(8, "SPELL_CAST_START", 116, "Frostbolt"), // still casting when the pull ended
	)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	track := store.tracks[store.pulls[0].Start]
	var hp []float64
	for _, points := range track.Bosses {
		for _, p := range points {
			hp = append(hp, p.HP)
		}
	}
	if fmt.Sprint(hp) != "[40]" || fmt.Sprint(track.Health) != "["+bossGUID+"]" {
		t.Errorf("boss health in the track = %v, shown %v; want the boss at 40%% in the box", hp, track.Health)
	}
	if got := fmt.Sprint(track.Casts["Testmage"]); got != "[{2000 Frostbolt 4000 false} {5000 Arcane Barrage 0 false} "+
		"{6000 Frostbolt 7000 true} {8000 Frostbolt 0 true}]" {
		t.Errorf("casts = %s; want the Frostbolt from 2 to 4 s, the instant at 5 s, one interrupted at 7 s (not at the "+
			"key pressed again) and one the pull cut", got)
	}
	if got := fmt.Sprint(track.Abilities["Testmage"]); got != "[{Arcane Barrage [[2000 5000 0 0]]}]" ||
		fmt.Sprint(track.TargetSeries["Testmage"]) != "[{Testboss [[2000 5000 0 0]]}]" ||
		fmt.Sprint(track.AbilityTimes) != "[2000 4000 6000 8000 30000]" {
		t.Errorf("abilities = %v, targets = %v, closed at %v; want the hit in the stretch the first line past 2 s closed, "+
			"one stretch per 2 s boundary a line crossed (not per line), then the pull's end",
			track.Abilities["Testmage"], track.TargetSeries["Testmage"], track.AbilityTimes)
	}
}

func TestBuffCheckCountsAuraIDs(t *testing.T) {
	const mage = "Player-76-0000AAAA"
	for _, c := range []struct {
		name    string
		auras   []int
		missing bool
	}{
		{"by ID only, never named in a line", []int{1303187}, false},
		{"by name only", []int{3003}, false},
		{"missing", []int{}, true},
	} {
		keys, _ := newRaidTracker(t)
		keys.raid.rules.Consumables.Vantus = []string{"Vantus Rune"}
		keys.raid.rules.Consumables.VantusIDs = []int{1303187}
		keys.raid.rules.Consumables.VantusDifficulties = []string{"Heroic"}
		keys.spells[3003], keys.names[mage] = "Vantus Rune: Radiant", "Testmage"
		checks := keys.buffCheck(&keyRun{party: []string{mage}, auras: map[string][]int{mage: c.auras},
			raid: &raidRun{difficulty: "Heroic"}})
		if got := len(checks) > 0 && slices.Contains(checks[0].Missing, "Vantus Rune"); got != c.missing {
			t.Errorf("%s: missing Vantus Rune = %v (%+v); want %v", c.name, got, checks, c.missing)
		}
	}
}

func TestACombatPotionDrunkBeforeThePullCountsAtZero(t *testing.T) {
	const priest = "Player-76-0000CCCC"
	drink := func(s float64) string {
		return raidAt(3, s, "SPELL_CAST_SUCCESS,"+priestUnit+","+noUnit+`,1295132,"Liquid Luster",0x1,`+
			advancedOf(priest, "0000000000000000"))
	}
	removed := func(s float64, unit string) string {
		return raidAt(3, s, "SPELL_AURA_REMOVED,"+unit+","+unit+`,1236616,"Light's Potential",0x1,BUFF`)
	}
	startAura := raidAt(3, 0.6, "COMBATANT_INFO,"+priest+",1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,257,[],[],[],[],["+
		priest+",1295132,1]")
	for _, c := range []struct {
		name          string
		before, extra []string
		want          string
	}{
		{"the buff leaves in the pull", nil, []string{removed(25, testPlayer),
			raidAt(3, 10, "SPELL_CAST_SUCCESS,"+shamanUnit+","+noUnit+`,1236616,"Light's Potential",0x1,`+
				advancedOf("Player-76-0000BBBB", "0000000000000000")),
			removed(40, shamanUnit)}, "[Testmage 0 true Testshaman 10 false]"},
		{"the buff at the start, no drink in the pull", []string{drink(-14)}, []string{startAura,
			hitOn(3, 5, "SPELL_DAMAGE", priestUnit, 1004, "Bite")}, "[Testpriest 0 true]"},
		{"a drink in the pull too: the drink only", []string{drink(-14)}, []string{startAura,
			hitOn(3, 5, "SPELL_DAMAGE", priestUnit, 1004, "Bite"), drink(20)}, "[Testpriest 20 false]"},
	} {
		keys, store := newRaidTracker(t)
		keys.kinds["Light's Potential"], keys.kinds["Liquid Luster"] = "combat potion", "combat potion"
		feed(t, keys, c.before...)
		feed(t, keys, pullLines(3, 0, 120, 40, false, c.extra...)...)
		if _, err := keys.SaveFinished(); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, p := range store.pulls[0].Cooldowns {
			got = append(got, fmt.Sprintf("%s %d %v", p.Player, p.ClockMS/1000, p.Prepot))
		}
		if fmt.Sprint(got) != c.want {
			t.Errorf("%s: combat potions = %v; want %s", c.name, got, c.want)
		}
	}
}

func TestARaidPullBumpsTheVersionOnlyAtItsStartAndEnd(t *testing.T) {
	keys, _ := newRaidTracker(t)
	lines := pullLines(3, 0, 120, 40, false, hitOn(3, 5, "SPELL_DAMAGE", priestUnit, 1004, "Bite"),
		hitOn(3, 6, "SPELL_DAMAGE", priestUnit, 1004, "Bite"))
	var seen []int64
	for _, l := range lines {
		feed(t, keys, l)
		seen = append(seen, keys.Version())
	}
	changes := 0
	for i := 1; i < len(seen); i++ {
		if seen[i] != seen[i-1] {
			changes++
		}
	}
	if changes > 2 {
		t.Fatalf("the version changed %d times over a pull's lines (%v); want at its start and end", changes, seen)
	}
}

func TestHealthPotionLabel(t *testing.T) {
	keys, store := newRaidTracker(t)
	keys.kinds["Concentrated Silvermoon Health Potion"] = "health potion"
	feed(t, keys, pullLines(3, 0, 60, 40, false,
		raidAt(3, 5, "SPELL_CAST_SUCCESS,"+testPlayer+","+noUnit+`,1234768,"Concentrated Silvermoon Health Potion",0x1,`+
			advancedOf("Player-76-0000AAAA", "0000000000000000")))...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	if got := store.pulls[0].Casts; len(got) != 1 || got[0].Ability != "Health potion" || got[0].Kind != "health potion" {
		t.Fatalf("casts = %+v; want the health potion saved as Health potion", got)
	}
}

func TestHealerOnlyCooldown(t *testing.T) {
	const shaman = "Player-76-0000BBBB"
	for _, c := range []struct {
		spec int
		want bool
	}{{264, true}, {262, false}} { // Restoration, Elemental
		keys, _ := newRaidTracker(t)
		keys.raid.cooldowns["Ascendance"] = true
		keys.raid.rules.Cooldowns.HealerOnly = []string{"Ascendance"}
		keys.specs[shaman] = c.spec
		if got := keys.raidCooldown(shaman, "Ascendance"); got != c.want {
			t.Errorf("spec %d: Ascendance a raid cooldown = %v; want %v", c.spec, got, c.want)
		}
	}
}

func TestRaidSpikeCountsRaidCooldowns(t *testing.T) {
	keys, store := newRaidTracker(t)
	keys.rules.Defensives.Externals = []string{"Pain Suppression"}
	keys.kinds["Pain Suppression"] = "defensive"
	lines := []string{
		raidAt(3, 0.6, "COMBATANT_INFO,Player-76-0000CCCC,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,257,[],[],[],[],[]")}
	// A hit of 1,000 every second for 70 s and 50,000 a second from 40 to 42 s; Tranquility up from 39 to 45 s,
	// Pain Suppression at 40 s.
	for s := 1; s <= 70; s++ {
		amount := 1000
		if s >= 40 && s <= 42 {
			amount = 50_000
		}
		lines = append(lines, raidAt(3, float64(s), fmt.Sprintf(`SPELL_DAMAGE,%s,%s,1238063,"Light Bolt",0x2,%s,%d,%d,`+
			`-1,2,0,0,0,nil,nil,nil,ST`, mobUnit(1), testPlayer, advancedOf("Player-76-0000AAAA", "0000000000000000"),
			amount, amount)))
		switch s {
		case 39:
			lines = append(lines, raidAt(3, 39, "SPELL_CAST_SUCCESS,"+shamanUnit+","+noUnit+`,740,"Tranquility",0x8,`+
				advancedOf("Player-76-0000BBBB", "0000000000000000")))
		case 40:
			lines = append(lines, raidAt(3, 40, "SPELL_CAST_SUCCESS,"+priestUnit+","+testPlayer+
				`,33206,"Pain Suppression",0x2,`+advancedOf("Player-76-0000CCCC", "0000000000000000")))
		case 45:
			lines = append(lines, raidAt(3, 45, "SPELL_AURA_REMOVED,"+shamanUnit+","+testPlayer+
				`,740,"Tranquility",0x8,BUFF`))
		}
	}
	feed(t, keys, pullLines(3, 0, 75, 40, false, lines...)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	if s := store.pulls[0].Spikes; len(s) != 1 || fmt.Sprint(s[0].PartyCDs) != "[Tranquility (Testshaman)]" {
		t.Fatalf("spikes = %+v; want one at 40 s covered by the Tranquility up then, not by Pain Suppression", s)
	}
	lane := slices.IndexFunc(store.pulls[0].Lanes, func(l models.RaidLane) bool { return l.Kind == "spike" })
	if lane < 0 || store.pulls[0].Lanes[lane].Marks[0].Bad {
		t.Fatalf("lanes = %+v; want a covered Damage spike lane", store.pulls[0].Lanes)
	}
}

func TestBuffCheckTierPieces(t *testing.T) {
	priestTier := map[string][]int{"Priest": {271553, 271554, 271555, 271556, 271558}}
	for _, c := range []struct {
		name   string
		tier   map[string][]int
		pieces int
		want   string
	}{
		{"1 piece", priestTier, 1, "[2-set]"},
		{"3 pieces", priestTier, 3, "[4-set]"},
		{"4 pieces", priestTier, 4, "[]"},
		{"a class without a list", map[string][]int{"Warlock": {271544}}, 0, "[]"},
	} {
		keys, store := newRaidTracker(t)
		keys.raid.tier = c.tier
		gear := []string{"(268265,341,(),(6652,13668),(240983,295,240908,295))"} // a ring with gems: not tier
		for _, id := range priestTier["Priest"][:c.pieces] {
			gear = append(gear, fmt.Sprintf("(%d,334,(7961,0,0),(13334,6652),())", id))
		}
		feed(t, keys, pullLines(3, 0, 60, 40, false,
			raidAt(3, 0.6, "COMBATANT_INFO,Player-76-0000CCCC,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,257,"+
				"[(1,2,1)],(0,1,2,3),["+strings.Join(gear, ",")+"],[]"),
			hitOn(3, 5, "SPELL_DAMAGE", priestUnit, 1004, "Bite"))...)
		if _, err := keys.SaveFinished(); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, g := range store.pulls[0].BuffGaps {
			if strings.HasSuffix(g.Buff, "-set") {
				got = append(got, g.Buff)
			}
		}
		if fmt.Sprint(list(got)) != c.want {
			t.Errorf("%s: sets missing = %v; want %s", c.name, got, c.want)
		}
	}
}

// TestViperSpawnerIsEggCarrier replays pull 28 on Nov 10: two carriers' eggs breaking to Caustic Waves 2.6 s apart and
// a third 7 s later while the waves tick on two other raiders, then two carriers' Noxious Splash.
func TestViperSpawnerIsEggCarrier(t *testing.T) {
	keys, store := newRaidTracker(t)
	boss := keys.raid.bosses["Testboss"]
	boss.Spawns = []models.RaidSpawnRule{{Add: "Viper", Cause: 1307635, Lost: 1307612, WithinMS: 3000},
		{Add: "Viper", Cause: 1292403, Lost: 1307612, WithinMS: 3000}}
	keys.raid.bosses["Testboss"] = boss
	egg := func(n int) string {
		return fmt.Sprintf(`Player-76-0000E00%d,"Egg%d-Sargeras-US",0x512,0x80000000`, n, n)
	}
	aura := func(s float64, event, unit string, spell int, name string) string {
		return raidAt(3, s, fmt.Sprintf(`%s,%s,%s,%d,"%s",0x8,DEBUFF`, event, bossUnit, unit, spell, name))
	}
	broke := func(s float64, n int) string { return aura(s, "SPELL_AURA_REMOVED", egg(n), 1307612, "Noxious Shell") }
	waves := func(s float64, event, unit string) string { return hitOn(3, s, event, unit, 1292403, "Caustic Waves") }
	splash := func(s float64, n int) string { return aura(s, "SPELL_AURA_APPLIED", egg(n), 1307635, "Noxious Splash") }
	viper := func(s float64, n int) string {
		return raidAt(3, s, "SPELL_DAMAGE,"+testPlayer+fmt.Sprintf(`,Creature-0-1-2939-1-261915-0%d,"Viper",0xa48,`, n)+
			`0x80000000,44425,"Arcane Barrage",0x40`)
	}
	var lines []string
	for n := 1; n <= 5; n++ {
		lines = append(lines, raidAt(3, 0.6, fmt.Sprintf("COMBATANT_INFO,Player-76-0000E00%d,1,0,0,0,0,0,0,0,0,0,0,0,0,"+
			"0,0,0,0,0,0,0,0,0,0,258,[],[],[],[],[]", n)), aura(1, "SPELL_AURA_APPLIED", egg(n), 1307612, "Noxious Shell"))
	}
	lines = append(lines, waves(10, "SPELL_DAMAGE", testPlayer), waves(10, "SPELL_DAMAGE", shamanUnit))
	for s := 11.0; s <= 30; s++ { // the waves ticking on the two of them
		lines = append(lines, waves(s, "SPELL_PERIODIC_DAMAGE", testPlayer), waves(s+0.5, "SPELL_PERIODIC_DAMAGE", shamanUnit))
	}
	lines = append(lines,
		viper(11.5, 6), // no egg broke, and the waves just hit two other raiders: nobody to name
		broke(17.786, 1), waves(17.786, "SPELL_DAMAGE", egg(1)), viper(18.854, 1),
		waves(20.386, "SPELL_DAMAGE", egg(2)), broke(20.39, 2), viper(20.909, 2), // the hit logged first
		broke(27.627, 3), waves(27.627, "SPELL_DAMAGE", egg(3)), viper(29.897, 3),
		broke(31.294, 4), splash(31.294, 4), broke(31.36, 5), splash(31.36, 5), viper(31.394, 4), viper(31.4, 5),
	)
	slices.SortStableFunc(lines, func(a, b string) int { return strings.Compare(a[:23], b[:23]) })
	feed(t, keys, pullLines(3, 0, 60, 40, false, lines...)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range store.pulls[0].Adds {
		for _, s := range a.Spawns {
			got = append(got, fmt.Sprintf("%.1f %s %v %v", float64(s.ClockMS)/1000, s.Cause, s.Players, s.Spawner))
		}
	}
	want := []string{"11.5 Not picked up [] false", "18.9 Caustic Waves [Egg1] true", "20.9 Caustic Waves [Egg2] true",
		"29.9 Caustic Waves [Egg3] true", "31.4 Noxious Splash [Egg4] true", "31.4 Noxious Splash [Egg5] true"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("vipers =\n%q\nwant\n%q", got, want)
	}
}

func TestRaidPullSavesTheSameEveryRead(t *testing.T) {
	read := func() string {
		keys, store := newRaidTracker(t)
		boss := keys.raid.bosses["Testboss"]
		boss.Spawns = []models.RaidSpawnRule{{Add: "Viper", Cause: 2004, WithinMS: 2000}}
		keys.raid.bosses["Testboss"] = boss
		add := func(guid string) string { // three Vipers seen at once: one from the splash, one killed
			return raidAt(3, 60, "SPELL_DAMAGE,"+testPlayer+","+guid+`,"Viper",0xa48,0x80000000,44425,"Arcane Barrage",0x40`)
		}
		feed(t, keys, pullLines(3, 0, 120, 40, false,
			hitOn(3, 59.5, "SPELL_DAMAGE", testPlayer, 2004, "Noxious Splash"),
			add("Creature-0-1-2939-1-300-01"), add("Creature-0-1-2939-1-300-02"), add("Creature-0-1-2939-1-300-03"),
			raidAt(3, 70, "UNIT_DIED,"+noUnit+",Creature-0-1-2939-1-300-02,\"Viper\",0xa48,0x0,0"),
		)...)
		if _, err := keys.SaveFinished(); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(store.pulls[0])
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	first := read()
	for range 20 { // map order changes from run to run: 20 reads catch a list that follows it
		if again := read(); again != first {
			t.Fatalf("a second read of the same lines saved a different pull:\n%s\n%s", first, again)
		}
	}
}
