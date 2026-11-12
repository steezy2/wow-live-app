package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"wow-live/backend/models"
)

const (
	testDungeon = 12001
	testBoss    = 13001
	minute      = int64(60_000)
	hour        = 60 * minute
	t0          = 1_700_000_000_000 // when the fixtures' reports start
)

var (
	testParty = []string{"Testmage", "Testpriest", "Testrogue", "Testhunter", "Testdruid"}
	testZones = models.WCLZones{MPlusID: 100, RaidID: 200,
		Encounters: map[string]int{"Test Dungeon": testDungeon, "Test Boss": testBoss}}
)

// keyFight is a key fight of the test dungeon that starts at offset into its report and lasts length.
func keyFight(id int, level int, offset, length int64, players ...string) models.ReportFight {
	return models.ReportFight{ID: id, EncounterID: testDungeon, KeystoneLevel: level, StartTime: offset,
		EndTime: offset + length, Players: players}
}

func bossFight(id int, kill bool, offset, length int64, players ...string) models.ReportFight {
	return models.ReportFight{ID: id, EncounterID: testBoss, Difficulty: 5, Kill: kill, StartTime: offset,
		EndTime: offset + length, Players: players}
}

func report(code string, zone int, start int64, fights ...models.ReportFight) models.CharacterReport {
	return models.CharacterReport{Code: code, StartTime: start, EndTime: start + 2*hour, ZoneID: zone, Fights: fights}
}

func localKey(start, length int64, level int, players ...string) models.Key {
	return models.Key{Dungeon: "Test Dungeon", Level: level, Start: start, End: start + length, Players: players}
}

func link(code string, fight int) models.ReportLink {
	return models.ReportLink{Code: code, FightID: fight}
}

func TestMatchReportsKeys(t *testing.T) {
	const length = 30 * minute
	key := localKey(t0+hour, length, 12, testParty...)
	for _, tc := range []struct {
		name    string
		reports []models.CharacterReport
		want    map[models.ReportLink]bool
	}{
		{"same key", []models.CharacterReport{report("A", 100, t0, keyFight(3, 12, hour, length, testParty...))},
			map[models.ReportLink]bool{link("A", 3): true}},
		{"2 minutes shorter still matches",
			[]models.CharacterReport{report("A", 100, t0, keyFight(3, 12, hour, length-2*minute, testParty...))},
			map[models.ReportLink]bool{link("A", 3): true}},
		{"4 minutes longer doesn't",
			[]models.CharacterReport{report("A", 100, t0, keyFight(3, 12, hour, length+4*minute, testParty...))},
			map[models.ReportLink]bool{}},
		{"another level doesn't",
			[]models.CharacterReport{report("A", 100, t0, keyFight(3, 13, hour, length, testParty...))},
			map[models.ReportLink]bool{}},
		{"2 shared players don't",
			[]models.CharacterReport{report("A", 100, t0, keyFight(3, 12, hour, length, "Testmage", "Testpriest", "Other"))},
			map[models.ReportLink]bool{}},
		{"two days later doesn't",
			[]models.CharacterReport{report("A", 100, t0+48*hour, keyFight(3, 12, hour, length, testParty...))},
			map[models.ReportLink]bool{}},
		{"the closer length wins in a report", []models.CharacterReport{report("A", 100, t0,
			keyFight(3, 12, hour, length-minute, testParty...), keyFight(4, 12, hour, length-20_000, testParty...))},
			map[models.ReportLink]bool{link("A", 4): true}},
		{"the same key in two reports is both", []models.CharacterReport{
			report("A", 100, t0, keyFight(3, 12, hour, length-10_000, testParty...)),
			report("B", 100, t0+20_000, keyFight(4, 12, hour, length+10_000, testParty...))},
			map[models.ReportLink]bool{link("A", 3): true, link("B", 4): true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := matchReports([]models.Key{key}, nil, tc.reports, testZones.Encounters)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("matched %v; want %v", got, tc.want)
			}
		})
	}
}

func TestMatchReportsPulls(t *testing.T) {
	const length = 5 * minute
	raiders := make([]string, 10)
	for i := range raiders {
		raiders[i] = string(rune('A'+i)) + "raider"
	}
	pull := models.RaidPull{Boss: "Test Boss", Difficulty: "Mythic", Kill: true, Start: t0 + hour, End: t0 + hour + length,
		Players: raiders}
	for _, tc := range []struct {
		name  string
		fight models.ReportFight
		want  bool
	}{
		{"same pull", bossFight(5, true, hour, length, raiders...), true},
		{"a wipe isn't a kill", bossFight(5, false, hour, length, raiders...), false},
		{"half the raid is enough", bossFight(5, true, hour, length, raiders[:5]...), true},
		{"fewer than half isn't", bossFight(5, true, hour, length, raiders[:4]...), false},
		{"another difficulty", func() models.ReportFight {
			f := bossFight(5, true, hour, length, raiders...)
			f.Difficulty = 4
			return f
		}(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := matchReports(nil, []models.RaidPull{pull}, []models.CharacterReport{report("R", 200, t0, tc.fight)},
				testZones.Encounters)
			if ok := len(got) == 1; ok != tc.want {
				t.Errorf("matched %v; want %v", got, tc.want)
			}
		})
	}
}

func TestReportLinks(t *testing.T) {
	const length = 30 * minute
	reports := []models.CharacterReport{
		report("OLD", 100, t0, keyFight(1, 12, hour, length, testParty...)),
		report("NEW", 100, t0+30_000, keyFight(4, 12, hour, length, testParty...)), // the same key, uploaded again
		report("RAID", 200, t0+2*hour, bossFight(2, true, 0, 5*minute, testParty...)),
	}
	linked := localKey(t0+hour, length, 12, testParty...)
	linked.Report = &models.ReportLink{Code: "OLD", FightID: 1}
	imported := localKey(t0+hour, length, 12, testParty...)
	imported.Imported = true
	pull := models.RaidPull{Boss: "Test Boss", Difficulty: "Mythic", Kill: true, Start: t0 + 2*hour,
		End: t0 + 2*hour + 5*minute, Players: testParty}
	for _, tc := range []struct {
		name      string
		keys      []models.Key
		wantKeys  map[int64]models.ReportLink
		wantPulls map[int64]models.ReportLink
	}{
		{"a local key gets the newest report's fight", []models.Key{localKey(t0+hour, length, 12, testParty...)},
			map[int64]models.ReportLink{t0 + hour: link("NEW", 4)}, map[int64]models.ReportLink{t0 + 2*hour: link("RAID", 2)}},
		{"a linked or imported key is left alone", []models.Key{linked, imported}, map[int64]models.ReportLink{},
			map[int64]models.ReportLink{t0 + 2*hour: link("RAID", 2)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys, pulls := ReportLinks(reports, testZones.Encounters, tc.keys, []models.RaidPull{pull})
			if !reflect.DeepEqual(keys, tc.wantKeys) || !reflect.DeepEqual(pulls, tc.wantPulls) {
				t.Errorf("linked keys %v, pulls %v; want %v, %v", keys, pulls, tc.wantKeys, tc.wantPulls)
			}
		})
	}
}

func TestLinkedReportSurvivesReadingTheLogAgain(t *testing.T) {
	t.Run("a key", func(t *testing.T) {
		keys := newTracker(t, nil)
		lines := []string{at(0, `CHALLENGE_MODE_START,"Voidscar Arena",2923,585,10,[9]`),
			"11/4/2026 20:03:00.000-7  CHALLENGE_MODE_END,2923,1,10,180000,100.0,3000.0"}
		start := feed(t, keys, lines...)[0].Time
		if n, err := keys.LinkReports(map[int64]models.ReportLink{start: link("A", 3)}, nil); n != 1 || err != nil {
			t.Fatalf("linked %d, %v; want the key", n, err)
		}
		feed(t, keys, lines...)
		if got := keys.history; len(got) != 1 || got[0].Report == nil || *got[0].Report != link("A", 3) {
			t.Fatalf("saved %+v; want the one key, still linked to A#3", got)
		}
	})
	t.Run("a raid pull", func(t *testing.T) {
		keys, _ := newRaidTracker(t)
		lines := pullLines(3, 60, 60, 50, false)
		read := func() {
			feed(t, keys, lines...)
			if _, err := keys.SaveFinished(); err != nil {
				t.Fatal(err)
			}
		}
		read()
		start := keys.pulls[0].Start
		if n, err := keys.LinkReports(nil, map[int64]models.ReportLink{start: link("R", 5)}); n != 1 || err != nil {
			t.Fatalf("linked %d, %v; want the pull", n, err)
		}
		read()
		if got := keys.pulls; len(got) != 1 || got[0].Report == nil || *got[0].Report != link("R", 5) {
			t.Fatalf("saved %+v; want the one pull, still linked to R#5", got)
		}
	})
}

func TestPickFights(t *testing.T) {
	const length = 30 * minute
	reports := []models.CharacterReport{
		report("NEW", 100, t0+10*hour, keyFight(1, 12, 0, length, testParty...)),
		report("OLD", 100, t0, keyFight(1, 10, 0, length, testParty...), keyFight(2, 11, hour, length, testParty...)),
		report("ELSEWHERE", 99, t0, keyFight(1, 10, 0, length, testParty...)),
		report("RAID", 200, t0+5*hour, bossFight(1, false, 0, 4*minute, testParty...),
			models.ReportFight{ID: 2, StartTime: 10 * minute, EndTime: 20 * minute, Players: testParty}, // trash
			bossFight(3, true, 30*minute, 5*minute, "Testrogue")),
		report("STRANGER", 100, t0+20*hour, keyFight(1, 12, 0, length, "Testrogue", "Testhunter")),
		report("COPY", 100, t0+30_000, keyFight(1, 10, 0, length, testParty...)),         // OLD#1, uploaded again
		report("NEW2", 100, t0+10*hour+20_000, keyFight(1, 12, 0, length, testParty...)), // NEW#1, uploaded again
	}
	saved := []models.Key{
		{Dungeon: "Test Dungeon", Level: 11, Report: &models.ReportLink{Code: "OLD", FightID: 2}, Imported: true},
		localKey(t0+10*hour, length, 12, testParty...), // the game wrote NEW's key itself
	}
	t.Run("everything that qualifies, newest first, a fight in several reports once", func(t *testing.T) {
		got := PickFights(reports, testZones, "Testmage-Somerealm", nil, nil)
		var codes []string
		for _, f := range got {
			codes = append(codes, f.Code+":"+string(f.Kind))
		}
		want := []string{"NEW2:mplus", "RAID:raid", "OLD:mplus", "COPY:mplus"}
		if !reflect.DeepEqual(codes, want) {
			t.Errorf("picked %v; want %v (other zone, trash, a fight without the character and OLD's order matter)", codes, want)
		}
		if got[3].FightID != 1 || got[3].StartMS != t0+30_000 || got[1].StartMS != t0+5*hour {
			t.Errorf("picked %+v; want start times from the report's start plus the fight's", got)
		}
	})
	t.Run("not the ones saved or matched", func(t *testing.T) {
		got := PickFights(reports, testZones, "Testmage-Somerealm", saved, nil)
		var links []models.ReportLink
		for _, f := range got {
			links = append(links, f.ReportLink)
		}
		want := []models.ReportLink{link("RAID", 1), link("COPY", 1)}
		if !reflect.DeepEqual(links, want) {
			t.Errorf("picked %v; want %v: OLD#2 is imported, NEW#1 and its copy a local key, OLD#1 COPY's twin",
				links, want)
		}
	})
}

type fakeRealms struct{ err error }

func (f fakeRealms) Realms(context.Context, string) (map[string]string, error) {
	return map[string]string{"area52": "area-52", "somerealm": "some-realm"}, f.err
}

func TestResolveChecksNameAndRealm(t *testing.T) {
	for _, tc := range []struct {
		character, name, slug string
		bad                   bool
	}{
		{"Testmage-Somerealm", "Testmage", "some-realm", false},
		{"Testmage-Area 52", "Testmage", "area-52", false},
		{"Àbc-area-52", "Àbc", "area-52", false},
		{"T-Somerealm", "", "", true},
		{"Toolongcharname-Somerealm", "", "", true},
		{"Test1-Somerealm", "", "", true},
		{"Testmage-Nowhere", "", "", true},
		{"Testmage", "", "", true},
	} {
		name, slug, err := Characters{Realms: fakeRealms{}}.Resolve(context.Background(), "us", tc.character)
		if name != tc.name || slug != tc.slug || errors.Is(err, ErrBadCharacter) != tc.bad || (err != nil) != tc.bad {
			t.Errorf("%q: %q, %q, %v; want %q, %q, bad %v", tc.character, name, slug, err, tc.name, tc.slug, tc.bad)
		}
	}
	boom := errors.New("down")
	if _, _, err := (Characters{Realms: fakeRealms{boom}}).Resolve(context.Background(), "us", "Testmage-Area52"); !errors.Is(err, boom) {
		t.Errorf("Warcraft Logs failing gave %v; want its error through", err)
	}
}

type fakeKeyList []models.Key

func (f fakeKeyList) List() ([]models.Key, error) { return f, nil }

func TestGuessIsTheMostFrequentLogger(t *testing.T) {
	keys := fakeKeyList{{Logger: "Testrogue"}, {Logger: "Testmage"}, {}, {Logger: "Testmage"}, {Logger: "Testrogue"},
		{Logger: "Testmage"}}
	if got := (Characters{Keys: keys}).Guess(); got != "Testmage" {
		t.Errorf("guessed %q; want Testmage, the logger of 3 of 5 keys", got)
	}
	if got := (Characters{Keys: fakeKeyList{{}}}).Guess(); got != "" {
		t.Errorf("guessed %q from keys with no logger; want none", got)
	}
}

func TestQuickRepullsAreNotCopies(t *testing.T) {
	reports := []models.CharacterReport{report("RAID", 200, t0,
		bossFight(1, false, 0, 20_000, testParty...), bossFight(2, false, 120_000, 23_000, testParty...))}
	if got := PickFights(reports, testZones, "Testmage-Somerealm", nil, nil); len(got) != 2 {
		t.Errorf("picked %+v; want both wipes: one report never holds a fight twice", got)
	}
}
