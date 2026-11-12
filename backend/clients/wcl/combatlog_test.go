package wcl

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wow-live/backend/clients/combatlog"
	"wow-live/backend/clients/keystore"
	"wow-live/backend/models"
	"wow-live/backend/service"
)

// golden is the combat log the fixture fight (testdata/meta.json, page1.json, page2.json) converts to.
const golden = "testdata/fight.log"

var fixtureOptions = FightOptions{Logger: "Testmage-TestRealm",
	Dungeons: map[int]models.DungeonFacts{777: {Name: "Test Hollow"}, 778: {Name: "Other Hollow"}}}

// fixture is a testdata file as the API's reply.
func fixture(t *testing.T, name string) func(w http.ResponseWriter, body, auth string) {
	payload, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data(string(payload))
}

func TestWriteFightLogWritesTheLinesTheGameWould(t *testing.T) {
	f := newFake(t, fixture(t, "meta.json"), fixture(t, "page1.json"), fixture(t, "page2.json"))
	var out bytes.Buffer
	if err := f.client(t).WriteFightLog(context.Background(), "TEST", 1, fixtureOptions, &out); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	got, wantLines := strings.Split(out.String(), "\n"), strings.Split(strings.ReplaceAll(string(want), "\r", ""), "\n")
	for i := range max(len(got), len(wantLines)) {
		if i >= len(got) || i >= len(wantLines) || got[i] != wantLines[i] {
			t.Fatalf("line %d:\n got %q\nwant %q", i+1, at(got, i), at(wantLines, i))
		}
	}
}

func at(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "(none)"
}

func TestImportFightResumesFromTheCachedPagesAfterARateLimit(t *testing.T) {
	f := newFake(t, fixture(t, "meta.json"), fixture(t, "page1.json"), status(http.StatusTooManyRequests))
	c := f.client(t)
	if _, err := c.ImportFight(context.Background(), "TEST", 1, fixtureOptions); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
	f.replies, f.queries = append(f.replies, fixture(t, "page2.json")), nil
	path, err := c.ImportFight(context.Background(), "TEST", 1, fixtureOptions)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.queries) != 1 || !strings.Contains(f.queries[0], `"s":40000`) {
		t.Fatalf("after the pause it asked %d queries, want only the second page: %v", len(f.queries), f.queries)
	}
	if want := filepath.Join(c.CacheDir, ImportDir, "TEST-1.zip"); path != want {
		t.Fatalf("zip at %s, want %s", path, want)
	}
	if pages, _ := filepath.Glob(filepath.Join(c.CacheDir, ImportDir, "pages", "*")); len(pages) != 0 {
		t.Fatalf("pages left after the zip: %v", pages)
	}
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	entry, err := r.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(entry)
	want, _ := os.ReadFile(golden)
	if r.File[0].Name != "TEST-1.txt" || string(got) != strings.ReplaceAll(string(want), "\r", "") {
		t.Fatalf("zip holds %s, %d bytes; want TEST-1.txt with the golden log", r.File[0].Name, len(got))
	}
}

func TestAConvertedFightSavesTheKeyItWas(t *testing.T) {
	store := keystore.Store{Dir: t.TempDir()}
	keys, err := service.NewLogTracker(nil, nil, models.MPlusRules{}, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := combatlog.EachLineIn(golden, keys.Add); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	saved, err := store.List()
	if err != nil || len(saved) != 1 {
		t.Fatalf("saved %d keys, %v; want 1", len(saved), err)
	}
	key := saved[0]
	if key.Dungeon != "Test Hollow" || key.MapID != 777 || key.Level != 12 || key.Status != "timed" ||
		key.FinalMS != 65000 || key.Logger != "Testmage" || key.Start != 1794153601000 {
		t.Fatalf("key = %s %d +%d %s in %d ms by %q from %d", key.Dungeon, key.MapID, key.Level, key.Status,
			key.FinalMS, key.Logger, key.Start)
	}
	if len(key.Deaths) != 1 || key.Deaths[0].Player != "Testpriest" || key.Deaths[0].Killer != "Trash Imp" ||
		key.Deaths[0].Ability != "Doom Bolt" {
		t.Fatalf("deaths = %+v, want Testpriest to the Trash Imp's Doom Bolt", key.Deaths)
	}
	if len(key.Interrupts) != 1 || len(key.Bosses) != 1 || !key.Bosses[0].Kill {
		t.Fatalf("interrupts %+v, bosses %+v; want one each", key.Interrupts, key.Bosses)
	}
	meter := map[string]models.KeyMeterRow{}
	for _, r := range key.Overall.Rows {
		meter[r.Player] = r
	}
	// Testmage: melee 100, Fireball 500 and 1000, Auto Shot 20, the Wolf's 80 (not the Friendly Guide's 7), the
	// absorbed 300 taken. Testpriest: 50 of Shadow Word: Pain, the killing 9000 taken (the meter leaves falling out),
	// heals 400 + 300 absorbed + a 250 tick, dispels once.
	mage, priest := meter["Testmage"], meter["Testpriest"]
	if mage.Damage != 1700 || mage.Interrupts != 1 || mage.Purges != 1 || priest.Damage != 50 ||
		priest.Healing != 950 || priest.Dispels != 1 || priest.DamageTaken != 9000 || mage.DamageTaken != 300 {
		t.Fatalf("meter = %+v", meter)
	}
}
