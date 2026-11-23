package service

import (
	"testing"

	"wow-live/backend/models"
)

// A reloaded tracker keeps its fights slim, and what shows a fight or saves it again reads the whole one.
func TestSlimFightsAreWholeWhereAPageOrAFileNeedsThem(t *testing.T) {
	keys, pulls := newRaidTracker(t)
	feed(t, keys, shortKeyLines()...)
	feed(t, keys, pullLines(3, 0, 120, 40, false,
		hitOn(3, 9, "SPELL_DAMAGE", shamanUnit, 1001, "Caustic Waves"))...)
	if _, err := keys.SaveFinished(); err != nil {
		t.Fatal(err)
	}
	keyStore := keys.store.(*memStore)
	start, pullStart := keyStore.keys[0].Start, pulls.pulls[0].Start
	whole, wholePull := keyStore.keys[0], pulls.pulls[0]
	if len(whole.Abilities) == 0 || len(whole.Taken) == 0 || len(wholePull.Abilities) == 0 || len(wholePull.Taken) == 0 {
		t.Fatalf("the fixtures have no abilities: %d, %d", len(whole.Abilities), len(wholePull.Abilities))
	}
	if err := keys.Reload(); err != nil {
		t.Fatal(err)
	}
	if k, p := keys.history[0], keys.pulls[0]; !k.IsSlim() || k.Abilities != nil || k.Taken != nil || !k.HasAbilities() ||
		!p.IsSlim() || p.Abilities != nil || p.Targets != nil || !p.HasAbilities() ||
		p.TakenByAbility()["Caustic Waves"] != wholePull.TakenByAbility()["Caustic Waves"] {
		t.Fatalf("reloaded key %+v, pull %+v; want both slim, with their totals", k, p)
	}
	view, err := keys.SavedKey(start)
	if err != nil || len(view.Abilities) != len(whole.Abilities) || len(view.Taken) != len(whole.Taken) {
		t.Fatalf("saved key page = %d abilities, %v; want the whole key's %d", len(view.Abilities), err, len(whole.Abilities))
	}
	pull, err := keys.RaidPull(pullStart)
	if err != nil || len(pull.Abilities) != len(wholePull.Abilities) || len(pull.Taken) != len(wholePull.Taken) {
		t.Fatalf("pull page = %d abilities, %v; want the whole pull's %d", len(pull.Abilities), err, len(wholePull.Abilities))
	}
	n, err := keys.LinkReports(map[int64]models.ReportLink{start: {Code: "A"}},
		map[int64]models.ReportLink{pullStart: {Code: "R"}})
	if n != 2 || err != nil {
		t.Fatalf("linked %d, %v", n, err)
	}
	if saved := keyStore.keys[len(keyStore.keys)-1]; saved.IsSlim() || len(saved.Abilities) == 0 || saved.Report == nil {
		t.Fatalf("saved key after linking = %+v; want the whole key with its report", saved)
	}
	if saved := pulls.pulls[len(pulls.pulls)-1]; saved.IsSlim() || len(saved.Abilities) == 0 || saved.Report == nil {
		t.Fatalf("saved pull after linking = %+v; want the whole pull with its report", saved)
	}
}
