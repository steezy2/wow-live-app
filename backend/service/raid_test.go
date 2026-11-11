package service

import (
	"fmt"
	"io/fs"
	"slices"
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
func (m *memPulls) ListPulls() ([]models.RaidPull, error) { return nil, nil }
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
		"cooldowns/Raid cooldowns 5-0 Testshaman · Tranquility false spike/Damage spike 3-9 22k/s false avoidable/Caustic Waves 9-0 Testshaman true " +
		"soak/Spectral Coils 30-0 1 soaked false stacks/Eternal Venom 40-0 Testshaman 6 true]"
	if fmt.Sprint(got) != want {
		t.Fatalf("lanes' last marks =\n%v\nwant\n%s", got, want)
	}
}

func TestSpikeIsTheWorstDamageTakenAndWhoCoveredIt(t *testing.T) {
	track := models.KeyTrack{Meters: map[string][]models.MeterPoint{
		"A": {{ClockMS: 2000, Taken: 100}, {ClockMS: 4000, Taken: 200}, {ClockMS: 6000, Taken: 2200}},
		"B": {{ClockMS: 2000}, {ClockMS: 4000}, {ClockMS: 6000, Taken: 1800}},
	}}
	cooldowns := []models.KeyCast{{Player: "C", Ability: "Tranquility", Kind: "raid cooldown", ClockMS: 3000},
		{Player: "D", Kind: "combat potion", ClockMS: 5000}}
	s := spike(track, cooldowns, 2000)
	if s == nil || s.ClockMS != 6000 || s.PerSecond != 1900 || fmt.Sprint(s.Covered) != "[C · Tranquility]" {
		t.Fatalf("spike = %+v; want 3800 taken from 4 to 6 s, covered by the Tranquility, not the potion", s)
	}
	if spike(models.KeyTrack{}, nil, 2000) != nil {
		t.Fatal("a pull with no damage taken has no spike")
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
		Spike:     &models.RaidSpike{ClockMS: 90_000, PerSecond: 1_200_000, Covered: []string{}},
		Stacks:    []models.RaidStacks{{Ability: "Eternal Venom", Limit: 5, Players: []models.PlayerCount{{Player: "F", Count: 6}, {Player: "G", Count: 5}}}},
		WentOff:   []models.KeyStop{{Spell: "Anguished Cry"}, {Spell: "Anguished Cry"}},
		Adds:      []models.RaidAdd{{Name: "Blightscale Clutch", LongestMS: 30_000}},
		Priority:  []models.RaidPriority{{Target: "Ula'tek's Heart", Damage: 1_000_000}},
	}
	best := &models.RaidPull{Number: 4, Adds: []models.RaidAdd{{Name: "Blightscale Clutch", LongestMS: 12_000}},
		Priority: []models.RaidPriority{{Target: "Ula'tek's Heart", Damage: 3_000_000}}}
	want := []string{
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
	rules := models.SuggestionRules{Most: 20, AddsLonger: 1.25, PriorityBelow: 0.8}
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
		Damage: 5000, Hits: 1}) || fmt.Sprint(p.Targets) != "[{Testmage Testboss 5000}]" {
		t.Fatalf("abilities = %+v, targets = %+v; want the mage's Arcane Barrage for 5000 into Testboss", p.Abilities,
			p.Targets)
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

func TestARaidPullCountsACombatPotionDrunkBeforeIt(t *testing.T) {
	keys, store := newRaidTracker(t)
	keys.kinds["Light's Potential"] = "combat potion"
	removed := func(s float64, unit string) string {
		return raidAt(3, s, "SPELL_AURA_REMOVED,"+unit+","+unit+`,1236616,"Light's Potential",0x1,BUFF`)
	}
	feed(t, keys, pullLines(3, 0, 120, 40, false, removed(25, testPlayer),
		raidAt(3, 10, "SPELL_CAST_SUCCESS,"+shamanUnit+","+noUnit+`,1236616,"Light's Potential",0x1,`+
			advancedOf("Player-76-0000BBBB", "0000000000000000")),
		removed(40, shamanUnit),
	)...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range store.pulls[0].Cooldowns {
		got = append(got, fmt.Sprintf("%s %d", c.Player, c.ClockMS/1000))
	}
	if fmt.Sprint(got) != "[Testmage 0 Testshaman 10]" {
		t.Fatalf("combat potions = %v; want the mage's from before the pull at 0 and the shaman's at 10 s", got)
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
