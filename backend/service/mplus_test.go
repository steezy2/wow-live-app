package service

import (
	"cmp"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"

	"wow-live/backend/clients/combatlog"
	"wow-live/backend/models"
)

const keyStart = "11/3/2026 23:01:39.565-7  "

// memStore is a KeyStore in memory.
type memStore struct {
	keys   []models.Key
	tracks map[int64]models.KeyTrack
}

func (m *memStore) Save(k models.Key) error     { m.keys = append(m.keys, k); return nil }
func (m *memStore) List() ([]models.Key, error) { return m.keys, nil }
func (m *memStore) Delete(int64) error          { return nil }
func (m *memStore) Clear() error                { m.keys = nil; return nil }
func (m *memStore) SaveTrack(start int64, track models.KeyTrack) error {
	if m.tracks == nil {
		m.tracks = map[int64]models.KeyTrack{}
	}
	m.tracks[start] = track
	return nil
}
func (m *memStore) Track(start int64) (models.KeyTrack, error) {
	t, ok := m.tracks[start]
	if !ok {
		return t, fs.ErrNotExist
	}
	return t, nil
}

func newTracker(t *testing.T, timers map[int]int64) *KeyTracker {
	t.Helper()
	rules := models.MPlusRules{
		Defensives:  models.Defensives{WindowSeconds: 10, Names: []string{"Ice Block"}},
		Consumables: models.Consumables{Flask: []string{"Flask of"}, Food: []string{"Well Fed"}},
		Buffs:       map[string]string{"Arcane Intellect": "Mage", "Skyfury": "Shaman"},
	}
	rules.Kicks.Names = []string{"Wind Shear"}
	rules.CC.Names = []string{"Capacitor Totem"}
	// Voidscar Arena's avoidable list has Light Bolt, so TestKeyStep2's hit counts as avoidable.
	// The Radiant Spellsower (NPC 245336) is a quarter of the forces. Den of Nalorakk's (586) mob 3 is a boss.
	facts := map[int]models.DungeonFacts{586: {Minibosses: []int{245331, 245332}, Bosses: []int{245333}}, 585: {Avoidable: []int{1238063}, MustStop: []int{1238063}, Dispel: []int{1234846},
		Floors: []models.DungeonFloor{{UIMapID: 2610, Tiles: "midnight/voidscar_arena/1"}},
		Forces: models.Forces{Total: 40, NPCs: map[int]int{245336: 10}}}}
	keys, err := NewKeyTracker(timers, map[int]string{9: "Tyrannical"}, rules, facts, &memStore{})
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func feed(t *testing.T, keys *KeyTracker, raw ...string) []models.LogLine {
	t.Helper()
	var lines []models.LogLine
	for _, text := range raw {
		line, err := combatlog.ParseLine(text)
		if err != nil {
			t.Fatal(err)
		}
		keys.Add(line)
		lines = append(lines, line)
	}
	return lines
}

func death(at string) string {
	return "11/3/2026 " + at + "-7  UNIT_DIED,0000000000000000,nil,0x80000000,0x80000000," + testPlayer + ",0"
}

// TestKeyClockMatchesTheGame replays Nov 3's timed Voidscar Arena +17: 1,590.3 s from start to end with 7 deaths;
// the game's own time was 1,686.8 s.
func TestKeyClockMatchesTheGame(t *testing.T) {
	keys := newTracker(t, map[int]int64{585: 1_800_000})
	feed(t, keys,
		keyStart+`CHALLENGE_MODE_START,"Voidscar Arena",2923,585,17,[9,10,147]`,
		"11/3/2026 23:07:47.558-7  ENCOUNTER_START,3285,\"Taz'Rah\",8,5,2923",
		death("23:09:00.000"),
		"11/3/2026 23:10:48.564-7  ENCOUNTER_END,3285,\"Taz'Rah\",8,5,1,181004",
		death("23:12:00.000"), death("23:12:01.000"), death("23:20:00.000"), death("23:21:00.000"),
		death("23:22:00.000"),
		"11/3/2026 23:22:30.000-7  UNIT_DIED,0000000000000000,nil,0x80000000,0x80000000,"+testPlayer+",1",
		death("23:23:00.000"),
		"11/3/2026 23:28:09.901-7  CHALLENGE_MODE_END,2923,1,17,1686807,442.358185,3535.061279",
	)
	got, ok := keys.Latest(0)
	if !ok || got.Status != "timed" || got.Level != 17 || got.Affixes[0] != (models.Affix{ID: 9, Name: "Tyrannical"}) {
		t.Fatalf("key = %+v; want a timed +17 with Tyrannical", got.Key)
	}
	if len(got.Deaths) != 7 || got.Deaths[0].Player != "Testmage" {
		t.Fatalf("deaths = %+v; want 7 (a feign death doesn't count)", got.Deaths)
	}
	if computed := clock(got.Start, got.End, len(got.Deaths)); max(computed-got.FinalMS, got.FinalMS-computed) > 1000 {
		t.Fatalf("clock at the end = %d ms, game's = %d ms; want within 1 s", computed, got.FinalMS)
	}
	boss := got.Bosses[0]
	if boss.Name != "Taz'Rah" || !boss.Kill || boss.StartMS != 358_993 || boss.EndMS != 539_999+15_000 {
		t.Fatalf("boss = %+v; want Taz'Rah killed, on the key clock with one death's penalty", boss)
	}
	if got.ClockMS != got.FinalMS || got.PenaltyMS != 105_000 || got.Chests != 1 {
		t.Fatalf("view = clock %d, penalty %d, chests %d; want the game's time, 105 s, +1", got.ClockMS,
			got.PenaltyMS, got.Chests)
	}
}

// advancedOf is an advanced-logging block for an actor and its owner.
func advancedOf(guid, owner string) string {
	return guid + "," + owner + ",9,9,0,0,1470,0,0,0,3,0,100,0,1599.81,-1.23,2610,3.1461,93"
}

// TestKeyStep2 reads one short key: an interrupt, a pet's hit counted to its owner, a heal, a dispel, an avoidable
// hit taken, a defensive just before a death, the buffs each player lacked at the start, and the key saved when it
// ends.
func TestKeyStep2(t *testing.T) {
	keys := newTracker(t, nil)
	const shaman = `Player-76-0000BBBB,"Testshaman-Sargeras-US",0x512,0x80000000`
	const petGUID = "Creature-0-3019-3004-1081-95072-00006A951E"
	const mobGUID = "Creature-0-3882-2859-27547-245336-00006BE4EA"
	const mob = mobGUID + `,"Radiant Spellsower",0x10a48,0x80000000`
	none := "0000000000000000"
	info := func(guid, spec, auras string) string {
		return `11/4/2026 19:01:00.000-7  COMBATANT_INFO,` + guid + `,1,` + strings.Repeat("0,", 22) + spec +
			`,[(1,2,1)],(0,0,0),[],[],[` + auras + `]`
	}
	feed(t, keys,
		`11/4/2026 19:00:00.000-7  SPELL_AURA_APPLIED,`+testPlayer+`,`+testPlayer+`,1459,"Arcane Intellect",0x40,BUFF`,
		`11/4/2026 19:01:00.000-7  CHALLENGE_MODE_START,"Voidscar Arena",2923,585,10,[9]`,
		info("Player-76-0000AAAA", "62", "Player-76-0000AAAA,1459,1"),
		info("Player-76-0000BBBB", "262", "Player-76-0000BBBB,462854,1"),
		// The shaman's name shows up only after the key started: still in the party.
		`11/4/2026 19:01:10.000-7  SPELL_AURA_APPLIED,`+shaman+`,`+shaman+`,462854,"Skyfury",0x8,BUFF`,
		`11/4/2026 19:01:19.000-7  SPELL_CAST_SUCCESS,`+shaman+`,`+none+`,nil,0x80000000,0x80000000,192058,"Capacitor Totem",0x8,`+
			advancedOf("Player-76-0000BBBB", none),
		`11/4/2026 19:01:20.000-7  SPELL_INTERRUPT,`+shaman+`,`+mob+`,57994,"Wind Shear",0x8,1238063,"Light Bolt",2`,
		`11/4/2026 19:01:21.000-7  SWING_DAMAGE,`+petGUID+`,"Greater Earth Elemental",0x1114,0x80000000,`+mob+`,`+
			advancedOf(petGUID, "Player-76-0000BBBB")+`,1000,1000,-1,1,0,0,0,nil,nil,nil`,
		`11/4/2026 19:01:22.000-7  SPELL_DAMAGE,`+testPlayer+`,`+mob+`,44425,"Arcane Barrage",0x40,`+
			advancedOf(mobGUID, none)+`,5000,5000,-1,64,0,0,0,nil,nil,nil,ST`,
		`11/4/2026 19:01:23.000-7  SPELL_HEAL,`+shaman+`,`+testPlayer+`,77472,"Healing Wave",0x8,`+
			advancedOf("Player-76-0000AAAA", none)+`,3000,3000,1000,0,nil`,
		`11/4/2026 19:01:23.500-7  SPELL_DISPEL,`+shaman+`,`+testPlayer+`,51886,"Cleanse Spirit",0x8,1234846,"Gorge",0x1,DEBUFF`,
		`11/4/2026 19:01:24.000-7  SPELL_CAST_SUCCESS,`+testPlayer+`,`+none+`,nil,0x80000000,0x80000000,45438,"Ice Block",0x10,`+
			advancedOf("Player-76-0000AAAA", none),
		`11/4/2026 19:01:25.000-7  SPELL_DAMAGE,`+mob+`,`+testPlayer+`,1238063,"Light Bolt",0x2,`+
			advancedOf("Player-76-0000AAAA", none)+`,9000,9000,100,2,0,0,0,nil,nil,nil,ST`,
		`11/4/2026 19:01:25.100-7  UNIT_DIED,`+none+`,nil,0x80000000,0x80000000,`+testPlayer+`,0`,
		`11/4/2026 19:01:30.000-7  UNIT_DIED,`+none+`,nil,0x80000000,0x80000000,`+mob+`,0`,
		`11/4/2026 19:01:31.000-7  UNIT_DIED,`+none+`,nil,0x80000000,0x80000000,`+mob+`,0`,
		`11/4/2026 19:01:32.000-7  UNIT_DIED,`+none+`,nil,0x80000000,0x80000000,`+
			`Creature-0-3882-2859-27547-999999-00006BE4EB,"Odd Add",0x10a48,0x80000000,0`,
		`11/4/2026 19:03:00.000-7  CHALLENGE_MODE_END,2923,1,10,111000,100.0,3000.0`,
	)
	got, _ := keys.Latest(0)
	if i := got.Interrupts; len(i) != 1 || i[0] != (models.KeyInterrupt{Player: "Testshaman", Kick: "Wind Shear",
		Target: "Radiant Spellsower", Spell: "Light Bolt", ClockMS: 11_000}) {
		t.Fatalf("interrupts = %+v", i)
	}
	rows := map[string]models.KeyMeterRow{}
	for _, r := range got.Overall.Rows {
		rows[r.Player] = r
	}
	hobbit, sham := rows["Testmage"], rows["Testshaman"]
	if hobbit.Damage != 5000 || sham.Damage != 1000 || sham.Healing != 2000 || sham.Interrupts != 1 ||
		sham.Dispels != 1 || hobbit.DamageTaken != 9000 || hobbit.Avoidable != 9000 || hobbit.Class != "Mage" ||
		sham.Class != "Shaman" || !got.Reference {
		t.Fatalf("meter = %+v; want 5000, the pet's 1000 to the shaman, 2000 healing, an interrupt and a dispel, "+
			"9000 avoidable taken, and classes", got.Overall.Rows)
	}
	if d := got.Deaths[0]; d.Killer != "Radiant Spellsower" || d.Ability != "Light Bolt" || d.Defensive != "Ice Block" ||
		!slices.ContainsFunc(got.Casts, func(c models.KeyCast) bool { return c.Kind == "defensive" }) ||
		!slices.ContainsFunc(got.Casts, func(c models.KeyCast) bool { return c.Kind == "cc" && c.Player == "Testshaman" }) {
		t.Fatalf("death = %+v, casts %+v; want killed by Light Bolt with Ice Block just before", d, got.Casts)
	}
	missing := map[string][]string{}
	for _, b := range got.Buffs {
		missing[b.Player] = b.Missing
	}
	if !slices.Contains(missing["Testmage"], "Skyfury") || !slices.Contains(missing["Testmage"], "flask") ||
		!slices.Contains(missing["Testshaman"], "Arcane Intellect") {
		t.Fatalf("buffs = %+v; want each missing the other's class buff, and a flask", got.Buffs)
	}
	if f := got.Forces; f == nil || f.Count != 10 || f.At[0].Pct != 25 || f.Pct != 100 || len(f.At) != 2 ||
		!f.NeedsCheck || len(f.Uncounted) != 1 || f.Uncounted[0] != (models.UncountedNPC{NPC: 999999, Name: "Odd Add",
		Deaths: 1}) {
		t.Fatalf("forces = %+v; want 10 counted once (a mob dying twice counts once), 100%% at the end with the "+
			"table flagged, and the add the table lacks", f)
	}
	wantAbilities := []models.KeyAbility{
		{Player: "Testmage", Ability: "Arcane Barrage", Damage: 5000, Hits: 1, School: 0x40},
		{Player: "Testshaman", Ability: "Healing Wave", Healing: 2000, Hits: 1, School: 0x8},
		{Player: "Testshaman", Ability: "Melee", Damage: 1000, Hits: 1, School: 0x1}, // the pet's, under its owner
		{Player: "Testmage", Ability: "Ice Block", Casts: 1, School: 0x10},
		{Player: "Testshaman", Ability: "Capacitor Totem", Casts: 1, School: 0x8}}
	wantTaken := []models.KeyHit{{Player: "Testmage", Ability: "Light Bolt", Damage: 9000, School: 0x2}}
	if !reflect.DeepEqual(got.Abilities, wantAbilities) || !reflect.DeepEqual(got.Taken, wantTaken) {
		t.Fatalf("abilities = %+v\nwant %+v\ntaken = %+v, want %+v", got.Abilities, wantAbilities, got.Taken, wantTaken)
	}
	if got.Logger != "Testmage" {
		t.Fatalf("logger = %q; want Testmage, whose unit flags say the log is theirs", got.Logger)
	}
	if a := got.Activity["Testmage"]; a <= 0 || a > 100 || got.Lusts == nil || got.BetweenPulls == nil {
		t.Fatalf("activity = %v, lusts %v, between pulls %v; want the mage busy for part of the pull, and lists",
			got.Activity, got.Lusts, got.BetweenPulls)
	}
	if _, err := keys.SaveFinished(); err != nil || len(keys.Saved()) != 1 {
		t.Fatalf("saved %d keys, %v; want the key saved", len(keys.Saved()), err)
	}
	track, err := keys.Track(got.Start)
	if me := track.Players["Testmage"]; err != nil || len(me) == 0 || me[0].X != 1599.81 || me[0].Map != 2610 ||
		len(track.Deaths) != 1 || track.Deaths[0].Name != "Radiant Spellsower" || track.Classes["Testmage"] != "Mage" ||
		len(track.Meters["Testmage"]) == 0 {
		t.Fatalf("track = %+v, %v; want the mage's position and the mob's death", track, err)
	}
	if m, err := keys.DungeonMap(585); err != nil || len(m.Walked) == 0 || len(m.Floors) != 1 {
		t.Fatalf("dungeon map = %+v, %v; want the saved track's squares and the dungeon file's floor", m, err)
	}
}

// earlierRun is a timed +10 of 25:00 with forces: boss A 5:00-7:00 and boss B 23:00-25:00 (4 minutes of bosses,
// 21 of trash), 50% forces at 3:20 and 60% at 5:00.
var earlierRun = models.Key{MapID: 585, Level: 10, Status: "timed", FinalMS: 1_500_000,
	Bosses: []models.KeyBoss{{Name: "A", StartMS: 300_000, EndMS: 420_000, Kill: true},
		{Name: "B", StartMS: 1_380_000, EndMS: 1_500_000, Kill: true}},
	Forces: &models.KeyForces{At: []models.ForcesPoint{{ClockMS: 100_000, Pct: 25}, {ClockMS: 200_000, Pct: 50},
		{ClockMS: 300_000, Pct: 60}, {ClockMS: 1_200_000, Pct: 100}}}}

// TestKillLoggedOnlyAsPartyKillCounts: the game sometimes logs a party's kill as PARTY_KILL with no UNIT_DIED (Nov 9's
// Blinding Vale +13 missed 32 forces that way); either line counts the mob's death, once, though PARTY_KILL comes
// before the killing blow's hit and UNIT_DIED after it.
func TestKillLoggedOnlyAsPartyKillCounts(t *testing.T) {
	keys := newTracker(t, nil)
	guid := func(id string) string { return `Creature-0-3882-2859-27547-245336-` + id }
	mob := func(id string) string { return guid(id) + `,"Radiant Spellsower",0x10a48,0x80000000` }
	hit := func(at, id string) string {
		return `11/4/2026 ` + at + `-7  SPELL_DAMAGE,` + testPlayer + `,` + mob(id) + `,44425,"Arcane Barrage",0x40,` +
			advancedOf(guid(id), "0000000000000000") + `,5000,5000,-1,64,0,0,0,nil,nil,nil,ST`
	}
	feed(t, keys,
		`11/4/2026 19:01:00.000-7  CHALLENGE_MODE_START,"Voidscar Arena",2923,585,10,[9]`,
		hit("19:01:29.000", "00006BE4EA"), hit("19:01:29.000", "00006BE4EB"),
		`11/4/2026 19:01:30.000-7  PARTY_KILL,`+testPlayer+`,`+mob("00006BE4EA")+`,0`,
		`11/4/2026 19:01:31.000-7  PARTY_KILL,`+testPlayer+`,`+mob("00006BE4EB")+`,0`,
		hit("19:01:31.000", "00006BE4EB"),
		`11/4/2026 19:01:31.000-7  UNIT_DIED,0000000000000000,nil,0x80000000,0x80000000,`+mob("00006BE4EB")+`,0`,
	)
	got, _ := keys.Latest(0)
	if f := got.Forces; f == nil || f.Count != 20 {
		t.Fatalf("forces = %+v; want 20: the PARTY_KILL-only mob counted, the one with both lines once", f)
	}
	if track, err := keys.Track(got.Start); err != nil || len(track.Deaths) != 2 {
		t.Fatalf("map deaths = %+v (%v); want one per mob", track.Deaths, err)
	}
}

func TestProjectionFromForcesAndBosses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		clock  int64
		forces float64
		bosses []models.KeyBoss
		final  int64
		reach  int64 // when the key reached its forces; the clock when 0
	}{
		{"the same pace as the earlier run", 200_000, 50, nil, 1_500_000, 0},
		{"twice as slow on trash", 400_000, 50, nil, 400_000 + 2*1_060_000 + 240_000, 0},
		// 60 s after reaching 50% at the same pace: those 60 s of trash come off what's left.
		{"a minute into the next pack", 260_000, 50, nil, 1_500_000, 200_000},
		// Trash 260 s to the earlier run's 300 s, boss A in half its time: 960 s of trash left at 0.867, B at 0.5.
		{"fast on boss A", 320_000, 60, []models.KeyBoss{{Name: "A", StartMS: 250_000, EndMS: 310_000, Kill: true}},
			320_000 + 832_000 + 60_000, 0},
	} {
		key := models.Key{MapID: 585, Level: 11, TimerMS: 1_800_000, Bosses: tc.bosses,
			Forces: &models.KeyForces{Pct: tc.forces, At: []models.ForcesPoint{{ClockMS: cmp.Or(tc.reach, tc.clock),
				Pct: tc.forces}}}}
		if p := projection(key, tc.clock, []models.Key{earlierRun}); p == nil || p.FinalMS != tc.final {
			t.Errorf("%s: %+v, want %d ms", tc.name, p, tc.final)
		}
	}
}

func TestProjectionBeforeAnyKill(t *testing.T) {
	// A minute in at 0%: all of the earlier run's trash is still ahead, less the minute already spent on it.
	key := models.Key{MapID: 585, Level: 11, TimerMS: 1_800_000, Forces: &models.KeyForces{}}
	if p := projection(key, 60_000, []models.Key{earlierRun}); p == nil || p.FinalMS != 1_500_000 {
		t.Fatalf("projection = %+v, want 1,500,000 ms", p)
	}
}

func TestProjectionWeighsRunsAndShowsTheBestRun(t *testing.T) {
	// A +20 without a forces timeline counts half, from boss kills alone: its 50:00 before any kill.
	far := models.Key{MapID: 585, Level: 20, Status: "depleted", FinalMS: 3_000_000,
		Bosses: []models.KeyBoss{{Name: "A", EndMS: 900_000, Kill: true}}}
	key := models.Key{MapID: 585, Level: 10, TimerMS: 1_800_000,
		Forces: &models.KeyForces{Pct: 50, At: []models.ForcesPoint{{ClockMS: 200_000, Pct: 50}}}}
	p := projection(key, 200_000, []models.Key{earlierRun, far})
	want := &models.KeyProjection{FinalMS: 1_500_000, LowMS: 1_500_000, HighMS: 3_000_000, Chests: 1,
		Odds: []float64{1.0 / 3, 2.0 / 3, 0, 0}, Runs: 2, Source: "2 of your runs, forces and bosses",
		Best: []models.KeyGhost{{Name: "A", KillMS: 420_000, NeedByMS: 720_000},
			{Name: "B", KillMS: 1_500_000, NeedByMS: 1_800_000}}}
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("projection = %+v\nwant %+v", p, want)
	}
	if projection(models.Key{MapID: 1}, 0, []models.Key{earlierRun}) != nil {
		t.Error("another dungeon's runs gave a projection")
	}
}

func TestARunningKeyCountsTheClockToNowAndAnAbandonedKeyEnds(t *testing.T) {
	keys := newTracker(t, nil)
	if _, ok := keys.Latest(0); ok {
		t.Fatal("want no key before any CHALLENGE_MODE_START")
	}
	mob := `Creature-0-3882-2859-27547-245336-00006BE4EA,"Radiant Spellsower",0x10a48,0x80000000`
	lines := feed(t, keys, keyStart+`CHALLENGE_MODE_START,"Voidscar Arena",2923,585,17,[9]`,
		"11/3/2026 23:01:40.000-7  COMBATANT_INFO,Player-76-0000AAAA,1,"+strings.Repeat("0,", 22)+"62,[],[],[],[],[]",
		death("23:02:00.000"),
		"11/3/2026 23:02:30.000-7  SPELL_DAMAGE,"+testPlayer+","+mob+",44425,\"Arcane Barrage\",0x40,"+
			advancedOf("Creature-0-3882-2859-27547-245336-00006BE4EA", "0000000000000000")+
			",5000,5000,-1,64,0,0,0,nil,nil,nil,ST")
	if got, _ := keys.Latest(lines[3].Time + 1_000); !got.LastPull.Ongoing {
		t.Fatal("a second after a hit: want the pull ongoing")
	}
	now := lines[0].Time + 69_000
	got, _ := keys.Latest(now)
	if got.Status != "running" || got.ClockMS != 60_000+15_000 || got.Chests != -1 || got.Bosses == nil ||
		got.LastPull.Ongoing {
		t.Fatalf("running key = %+v; want 75 s on the clock and no timer", got)
	}
	feed(t, keys, "11/4/2026 18:00:00.000-7  CHALLENGE_MODE_END,2923,0,0,0,0.000000,0.000000")
	if got, _ := keys.Latest(now); got.Status != "abandoned" || got.ClockMS != 41_435+15_000 {
		t.Fatalf("after 0,0,0 the next day: %+v at %d ms; want the key abandoned at its last line, 0:56", got.Key,
			got.ClockMS)
	}
}

// silentKey is a tracker on a key whose log stopped mid-fight, and the time of its last line.
func silentKey(t *testing.T) (*KeyTracker, int64) {
	keys := newTracker(t, map[int]int64{585: 1_800_000})
	lines := feed(t, keys,
		keyStart+`CHALLENGE_MODE_START,"Voidscar Arena",2923,585,17,[9,10,147]`,
		"11/3/2026 23:07:47.558-7  ENCOUNTER_START,3285,\"Taz'Rah\",8,5,2923",
		"11/3/2026 23:10:48.564-7  ENCOUNTER_END,3285,\"Taz'Rah\",8,5,1,181004",
		"11/3/2026 23:12:00.000-7  ENCOUNTER_START,3286,\"Second\",8,5,2923",
	)
	return keys, lines[len(lines)-1].Time
}

func TestASilentKeyClosesAsAbandonedAndGoesOnIfTheLogDoes(t *testing.T) {
	keys, last := silentKey(t)
	keys.CloseQuiet(last + 60_000)
	got, _ := keys.Latest(last + 60_000)
	track, err := keys.Track(got.Start)
	if got.Status != "running" || got.ClockMS <= clock(got.Start, last, 0) || err != nil || len(track.Pulls) != 2 ||
		track.Pulls[0].EndMS < track.Pulls[0].StartMS || track.Pulls[0].EndMS == 0 || track.Pulls[1].EndMS != 0 {
		t.Fatalf("a minute after the last line: %s at %d ms, pulls %+v (%v); want it running, a closed first pull and "+
			"an open second one", got.Status, got.ClockMS, track.Pulls, err)
	}
	if got.SilentMS != 60_000 {
		t.Errorf("silent for %d ms a minute after the last line; want 60000", got.SilentMS)
	}
	keys.CloseQuiet(last + quietMS + 1)
	got, _ = keys.Latest(last + quietMS + 1)
	if saved, err := keys.SaveFinished(); got.Status != "abandoned" || got.End != last ||
		got.ClockMS != clock(got.Start, last, 0) || err != nil || len(saved) != 1 || keys.Abandon() {
		t.Fatalf("after the silence: %s, ended %d, %d ms, saved %v (%v); want it abandoned at the last line and saved, "+
			"with no key left to abandon", got.Status, got.End, got.ClockMS, saved, err)
	}
	// The game only wrote its log late: the fight went on, and then the party left.
	killed := feed(t, keys, "11/3/2026 23:13:00.000-7  ENCOUNTER_END,3286,\"Second\",8,5,1,60000")[0].Time
	got, _ = keys.Latest(last + 5*60_000)
	if track, _ := keys.Track(got.Start); got.Status != "running" || !got.Bosses[1].Kill || track.Pulls[1].EndMS != 0 {
		t.Fatalf("after the log went on: %s, bosses %+v, pulls %+v; want it running with the second boss killed",
			got.Status, got.Bosses, track.Pulls)
	}
	feed(t, keys, `11/3/2026 23:20:00.000-7  ZONE_CHANGE,0,"Silvermoon City",0`)
	got, _ = keys.Latest(0)
	if saved, _ := keys.SaveFinished(); got.Status != "abandoned" || got.End != killed || len(saved) != 1 ||
		len(keys.Saved()) != 1 {
		t.Fatalf("after leaving: %s, ended %d, saved %v, %d keys; want it abandoned at its last line, saved over its "+
			"copy", got.Status, got.End, saved, len(keys.Saved()))
	}
}

func TestASilentKeyStaysClosedWhenThePartyLeftOrStartedAnother(t *testing.T) {
	for _, next := range []string{`ZONE_CHANGE,0,"Silvermoon City",0`,
		`CHALLENGE_MODE_START,"Voidscar Arena",2923,585,17,[9]`} {
		keys, last := silentKey(t)
		keys.Abandon()
		// The next day's log: its header says nothing about the key; the line after it does.
		feed(t, keys, "11/4/2026 08:00:00.000-7  COMBAT_LOG_VERSION,22,ADVANCED_LOG_ENABLED,1",
			"11/4/2026 08:00:01.000-7  "+next,
			"11/4/2026 08:00:02.000-7  ENCOUNTER_START,3286,\"Second\",8,5,2923")
		if saved := keys.Saved(); len(saved) != 1 || saved[0].Status != "abandoned" || keys.history[0].End != last {
			t.Errorf("after %s: saved %+v; want the silent key still abandoned at its last line", next, saved)
		}
	}
}
