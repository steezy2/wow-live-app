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
		want    map[int]models.ReportLink
	}{
		{"same key", []models.CharacterReport{report("A", 100, t0, keyFight(3, 12, hour, length, testParty...))},
			map[int]models.ReportLink{0: link("A", 3)}},
		{"2 minutes shorter still matches",
			[]models.CharacterReport{report("A", 100, t0, keyFight(3, 12, hour, length-2*minute, testParty...))},
			map[int]models.ReportLink{0: link("A", 3)}},
		{"4 minutes longer doesn't",
			[]models.CharacterReport{report("A", 100, t0, keyFight(3, 12, hour, length+4*minute, testParty...))},
			map[int]models.ReportLink{}},
		{"another level doesn't",
			[]models.CharacterReport{report("A", 100, t0, keyFight(3, 13, hour, length, testParty...))},
			map[int]models.ReportLink{}},
		{"2 shared players don't",
			[]models.CharacterReport{report("A", 100, t0, keyFight(3, 12, hour, length, "Testmage", "Testpriest", "Other"))},
			map[int]models.ReportLink{}},
		{"two days later doesn't",
			[]models.CharacterReport{report("A", 100, t0+48*hour, keyFight(3, 12, hour, length, testParty...))},
			map[int]models.ReportLink{}},
		{"the closer length wins", []models.CharacterReport{report("A", 100, t0,
			keyFight(3, 12, hour, length-minute, testParty...), keyFight(4, 12, hour, length-20_000, testParty...))},
			map[int]models.ReportLink{0: link("A", 4)}},
		{"a tie within 5 s is no match", []models.CharacterReport{report("A", 100, t0,
			keyFight(3, 12, hour, length-10_000, testParty...), keyFight(4, 12, hour, length+10_000, testParty...))},
			map[int]models.ReportLink{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := matchReports([]models.Key{key}, nil, tc.reports, testZones.Encounters)
			if !reflect.DeepEqual(got.keys, tc.want) {
				t.Errorf("matched %v; want %v", got.keys, tc.want)
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
			if _, ok := got.pulls[0]; ok != tc.want {
				t.Errorf("matched %v; want %v", got.pulls, tc.want)
			}
		})
	}
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
	}
	saved := []models.Key{
		{Dungeon: "Test Dungeon", Level: 11, Report: &models.ReportLink{Code: "OLD", FightID: 2}, Imported: true},
		localKey(t0+10*hour, length, 12, testParty...), // the game wrote NEW's key itself
	}
	t.Run("everything that qualifies, oldest first", func(t *testing.T) {
		got := PickFights(reports, testZones, "Testmage-Somerealm", nil, nil)
		var codes []string
		for _, f := range got {
			codes = append(codes, f.Code+":"+string(f.Kind))
		}
		want := []string{"OLD:mplus", "OLD:mplus", "RAID:raid", "NEW:mplus"}
		if !reflect.DeepEqual(codes, want) {
			t.Errorf("picked %v; want %v (other zone, trash, a fight without the character and OLD's order matter)", codes, want)
		}
		if got[0].FightID != 1 || got[0].StartMS != t0 || got[2].StartMS != t0+5*hour {
			t.Errorf("picked %+v; want start times from the report's start plus the fight's", got)
		}
	})
	t.Run("not the ones saved or matched", func(t *testing.T) {
		got := PickFights(reports, testZones, "Testmage-Somerealm", saved, nil)
		var links []models.ReportLink
		for _, f := range got {
			links = append(links, f.ReportLink)
		}
		want := []models.ReportLink{link("OLD", 1), link("RAID", 1)}
		if !reflect.DeepEqual(links, want) {
			t.Errorf("picked %v; want %v: OLD#2 is imported and NEW#1 is a local key", links, want)
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
