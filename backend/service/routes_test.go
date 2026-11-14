package service

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"wow-live/backend/clients/mdt"
	"wow-live/backend/clients/routestore"
	"wow-live/backend/models"
)

const voidscar = 585

// newRoutes is Routes on a temp folder over a dungeon with four enemies of NPC 100, 100 yards apart, worth 10, 10, 20
// and 5 of 100 forces, and a clock that moves on a second each time it's read.
func newRoutes(t *testing.T) *Routes {
	t.Helper()
	now := time.Unix(1000, 0)
	facts := map[int]models.DungeonFacts{
		voidscar: {MDTIndex: 163, Forces: models.Forces{Total: 100}, Enemies: []models.RouteEnemy{
			{Index: 1, Clone: 1, NPC: 100, Count: 10}, {Index: 1, Clone: 2, NPC: 100, X: 100, Count: 10},
			{Index: 2, Clone: 1, NPC: 100, X: 200, Count: 20}, {Index: 3, Clone: 1, NPC: 100, X: 300, Count: 5}}},
		586: {MDTIndex: 161},
	}
	return &Routes{Store: routestore.Store{Dir: t.TempDir()}, Facts: facts, Decode: mdt.DecodeRoute, Encode: mdt.EncodeRoute,
		Now: func() time.Time { now = now.Add(time.Second); return now }}
}

func preset(uid string, dungeon int, pulls ...models.MDTPull) models.MDTPreset {
	return models.MDTPreset{Name: "Route " + uid, UID: uid, DungeonIdx: dungeon, Pulls: pulls}
}

func TestImportRoute(t *testing.T) {
	pull := models.MDTPull{Color: "ff3eff", Enemies: map[int][]int{1: {2, 1}, 9: {1}, 2: {1, 7}}}
	t.Run("keeps the known enemies in order, counts the unknown ones and starts the dungeon's star", func(t *testing.T) {
		r := newRoutes(t)
		view, err := r.ImportPreset(preset("abc", 163, pull))
		if err != nil {
			t.Fatal(err)
		}
		want := []models.EnemyRef{{Index: 1, Clone: 1}, {Index: 1, Clone: 2}, {Index: 2, Clone: 1}}
		if view.ID != "abc" || view.MapID != voidscar || !view.Starred || view.Dropped != 2 ||
			!slices.Equal(view.Pulls[0].Enemies, want) {
			t.Fatalf("view = %+v; want route abc on %d, starred, 2 dropped, enemies %v", view, voidscar, want)
		}
	})
	t.Run("a second route is not starred and a made-up id is valid", func(t *testing.T) {
		r := newRoutes(t)
		if _, err := r.ImportPreset(preset("abc", 163)); err != nil {
			t.Fatal(err)
		}
		view, err := r.ImportPreset(preset("bad id!", 163))
		if err != nil || view.Starred || !models.ValidRouteID(view.ID) {
			t.Fatalf("view = %+v, %v; want an unstarred route with a valid new id", view, err)
		}
	})
	t.Run("the same uid replaces the route and keeps its star", func(t *testing.T) {
		r := newRoutes(t)
		for _, uid := range []string{"abc", "def"} {
			if _, err := r.ImportPreset(preset(uid, 163)); err != nil {
				t.Fatal(err)
			}
		}
		if err := r.Star("def"); err != nil {
			t.Fatal(err)
		}
		again := preset("def", 163, pull)
		again.Name = "Renamed"
		if _, err := r.ImportPreset(again); err != nil {
			t.Fatal(err)
		}
		routes, _ := r.List(voidscar)
		if len(routes) != 2 || !routes[0].Starred || routes[0].Name != "Renamed" || routes[1].Starred {
			t.Fatalf("routes = %+v; want def replaced, still starred, and abc unstarred", routes)
		}
	})
	t.Run("the same uid in another dungeon doesn't bring its star", func(t *testing.T) {
		r := newRoutes(t)
		for _, p := range []models.MDTPreset{preset("abc", 163), preset("def", 161), preset("abc", 161)} {
			if _, err := r.ImportPreset(p); err != nil {
				t.Fatal(err)
			}
		}
		routes, _ := r.List(586)
		if len(routes) != 2 || routes[0].ID != "abc" || routes[0].Starred || !routes[1].Starred {
			t.Fatalf("routes = %+v; want abc moved in unstarred and def still starred", routes)
		}
	})
	for _, tc := range []struct {
		name    string
		pulls   []models.MDTPull
		want    []models.RoutePull
		dropped int
	}{
		{"a color that isn't 6 hex digits gets the default",
			[]models.MDTPull{{Color: "fff;background:url(x)", Enemies: map[int][]int{1: {1}}}},
			[]models.RoutePull{{Color: defaultPullColor, Enemies: []models.EnemyRef{{Index: 1, Clone: 1}}}}, 0},
		{"an enemy an earlier pull has is dropped",
			[]models.MDTPull{{Color: "ff3eff", Enemies: map[int][]int{1: {1}}},
				{Color: "ff3eff", Enemies: map[int][]int{1: {1, 2}}}},
			[]models.RoutePull{{Color: "ff3eff", Enemies: []models.EnemyRef{{Index: 1, Clone: 1}}},
				{Color: "ff3eff", Enemies: []models.EnemyRef{{Index: 1, Clone: 2}}}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view, err := newRoutes(t).ImportPreset(preset("abc", 163, tc.pulls...))
			if err != nil {
				t.Fatal(err)
			}
			var got []models.RoutePull
			for _, p := range view.Pulls {
				got = append(got, p.RoutePull)
			}
			if !reflect.DeepEqual(got, tc.want) || view.Dropped != tc.dropped {
				t.Fatalf("pulls = %+v, %d dropped; want %+v, %d", got, view.Dropped, tc.want, tc.dropped)
			}
		})
	}
	t.Run("more than MaxPulls pulls are refused", func(t *testing.T) {
		if _, err := newRoutes(t).ImportPreset(preset("abc", 163, make([]models.MDTPull, MaxPulls+1)...)); !errors.Is(err,
			ErrInvalidRoute) {
			t.Fatalf("err = %v, want ErrInvalidRoute", err)
		}
	})
	t.Run("a dungeon outside the season is refused", func(t *testing.T) {
		if _, err := newRoutes(t).ImportPreset(preset("abc", 99)); !errors.Is(err, ErrInvalidRoute) {
			t.Fatalf("err = %v, want ErrInvalidRoute", err)
		}
	})
	t.Run("a bad string is refused with the decode step", func(t *testing.T) {
		_, err := newRoutes(t).Import("hello")
		if !errors.Is(err, ErrInvalidRoute) {
			t.Fatalf("err = %v, want ErrInvalidRoute", err)
		}
	})
}

func TestStarRules(t *testing.T) {
	r := newRoutes(t)
	starred := func() []string {
		routes, err := r.List(voidscar)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, route := range routes {
			if route.Starred {
				ids = append(ids, route.ID)
			}
		}
		return ids
	}
	for _, uid := range []string{"a", "b", "c"} {
		if _, err := r.ImportPreset(preset(uid, 163)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.ImportPreset(preset("other", 161)); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		do   func() error
		want string
	}{
		{func() error { return nil }, "a"},               // the first route of the dungeon
		{func() error { return r.Star("c") }, "c"},       // starring moves the star
		{func() error { return r.Delete("a") }, "c"},     // deleting another keeps it
		{func() error { return r.Delete("c") }, "b"},     // deleting the starred one stars the newest left
		{func() error { return r.Delete("other") }, "b"}, // another dungeon's routes don't count
	} {
		err := step.do()
		if got := starred(); !slices.Equal(got, []string{step.want}) {
			t.Fatalf("starred = %v (err %v), want only %s", got, err, step.want)
		}
	}
	if err := r.Star("nope"); !errors.Is(err, ErrNoSuchRoute) {
		t.Fatalf("star of an unknown route: %v", err)
	}
}

func TestUpdateRouteValidates(t *testing.T) {
	r := newRoutes(t)
	if _, err := r.ImportPreset(preset("abc", 163)); err != nil {
		t.Fatal(err)
	}
	pull := func(refs ...models.EnemyRef) models.RoutePull {
		return models.RoutePull{Color: "ff3eff", Enemies: refs}
	}
	one, two := models.EnemyRef{Index: 1, Clone: 1}, models.EnemyRef{Index: 1, Clone: 2}
	long := make([]models.RoutePull, MaxPulls+1)
	for i := range long {
		long[i] = pull()
	}
	for _, tc := range []struct {
		name   string
		route  string
		pulls  []models.RoutePull
		wantOK bool
	}{
		{"valid", "Fine", []models.RoutePull{pull(one), pull(two)}, true},
		{"enemy in two pulls", "Fine", []models.RoutePull{pull(one), pull(one)}, false},
		{"unknown enemy", "Fine", []models.RoutePull{pull(models.EnemyRef{Index: 9, Clone: 1})}, false},
		{"too many pulls", "Fine", long, false},
		{"exactly the most pulls", "Fine", long[:MaxPulls], true},
		{"bad color", "Fine", []models.RoutePull{{Color: "red"}}, false},
		{"empty name", "", nil, false},
		{"name of only spaces", "   ", nil, false},
		{"name too long", strings.Repeat("x", 61), nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.Update("abc", tc.route, tc.pulls)
			if tc.wantOK != (err == nil) || (err != nil && !errors.Is(err, ErrInvalidRoute)) {
				t.Fatalf("err = %v, want ok %v", err, tc.wantOK)
			}
		})
	}
	if _, err := r.Update("nope", "Fine", nil); !errors.Is(err, ErrNoSuchRoute) {
		t.Fatalf("update of an unknown route: %v", err)
	}
}

func TestRouteViewForces(t *testing.T) {
	r := newRoutes(t)
	if _, err := r.ImportPreset(preset("abc", 163)); err != nil {
		t.Fatal(err)
	}
	pulls := []models.RoutePull{
		{Color: "ff3eff", Enemies: []models.EnemyRef{{Index: 1, Clone: 1}, {Index: 1, Clone: 2}}},
		{Color: "ff3eff"},
		{Color: "ff3eff", Enemies: []models.EnemyRef{{Index: 2, Clone: 1}}},
	}
	view, err := r.Update("abc", "Fine", pulls)
	if err != nil {
		t.Fatal(err)
	}
	type forces struct {
		Forces, Total int
		Pct, TotalPct float64
	}
	var got []forces
	for _, p := range view.Pulls {
		got = append(got, forces{p.Forces, p.Total, p.Pct, p.TotalPct})
	}
	want := []forces{{20, 20, 20, 20}, {0, 20, 0, 20}, {20, 40, 20, 40}}
	if !slices.Equal(got, want) || view.TotalPct != 40 {
		t.Fatalf("forces = %v, total %v; want %v, 40", got, view.TotalPct, want)
	}
	list, _ := r.List(0)
	if len(list) != 1 || list[0].Pulls[0].Enemies != nil || list[0].Pulls[0].Forces != 20 {
		t.Fatalf("list = %+v; want pulls with forces and no enemies", list)
	}
}

func TestExportRoute(t *testing.T) {
	r := newRoutes(t)
	view, err := r.Create(voidscar, "Fresh")
	if err != nil {
		t.Fatal(err)
	}
	pull := models.RoutePull{Color: "ff3eff", Enemies: []models.EnemyRef{{Index: 1, Clone: 2}, {Index: 1, Clone: 1}}}
	if _, err := r.Update(view.ID, "Fresh", []models.RoutePull{pull}); err != nil {
		t.Fatal(err)
	}
	code, err := r.Export(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := mdt.DecodeRoute(code)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Fresh" || got.UID != view.ID || got.DungeonIdx != 163 || len(got.Pulls) != 1 ||
		got.Pulls[0].Color != "ff3eff" || !slices.Equal(got.Pulls[0].Enemies[1], []int{2, 1}) {
		t.Fatalf("decoded export = %+v; want the route's name, uid, dungeon and pull", got)
	}
}

func TestSavedRoutes(t *testing.T) {
	r := newRoutes(t)
	unnamed := preset("def", 163)
	unnamed.Name = ""
	r.Saves = func() ([]models.MDTPreset, error) {
		return []models.MDTPreset{preset("abc", 163, models.MDTPull{}), unnamed, preset("old", 11), preset("", 163)}, nil
	}
	if _, err := r.ImportSaved("abc"); err != nil {
		t.Fatal(err)
	}
	saved, err := r.Saved()
	if err != nil {
		t.Fatal(err)
	}
	want := []models.SavedRoute{{UID: "abc", Name: "Route abc", MapID: voidscar, Pulls: 1, Imported: true},
		{UID: "def", Name: "Default", MapID: voidscar}}
	if !slices.Equal(saved, want) {
		t.Fatalf("saved = %+v; want only the season's routes with a uid, the unnamed one named, abc imported: %+v",
			saved, want)
	}
	if _, err := r.ImportSaved("nope"); !errors.Is(err, ErrNoSuchRoute) {
		t.Fatalf("ImportSaved(nope) = %v; want ErrNoSuchRoute", err)
	}
}
