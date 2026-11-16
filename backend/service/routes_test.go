package service

import (
	"errors"
	"fmt"
	"math"
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
// and 5 of 100 forces, on floor 2000 (log x 0 to 400, y -100 to 0), and a clock that moves on a second each time it's
// read.
func newRoutes(t *testing.T) *Routes {
	t.Helper()
	now := time.Unix(1000, 0)
	// Its x bounds run backwards, as Keystone.guru's often do.
	floor := models.DungeonFloor{RaidMap: models.RaidMap{UIMapID: 2000, MinX: 100, MaxX: 0, MinY: 0, MaxY: 400}}
	facts := map[int]models.DungeonFacts{
		voidscar: {MDTIndex: 163, Forces: models.Forces{Total: 100}, Floors: []models.DungeonFloor{floor},
			Enemies: []models.RouteEnemy{
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
	t.Run("more than maxPulls pulls are refused", func(t *testing.T) {
		if _, err := newRoutes(t).ImportPreset(preset("abc", 163, make([]models.MDTPull, maxPulls+1)...)); !errors.Is(err,
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
	long := make([]models.RoutePull, maxPulls+1)
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
		{"exactly the most pulls", "Fine", long[:maxPulls], true},
		{"bad color", "Fine", []models.RoutePull{{Color: "red"}}, false},
		{"empty name", "", nil, false},
		{"name of only spaces", "   ", nil, false},
		{"name too long", strings.Repeat("x", 61), nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.Update("abc", models.RouteUpdate{Name: tc.route, Pulls: tc.pulls})
			if tc.wantOK != (err == nil) || (err != nil && !errors.Is(err, ErrInvalidRoute)) {
				t.Fatalf("err = %v, want ok %v", err, tc.wantOK)
			}
		})
	}
	if _, err := r.Update("nope", models.RouteUpdate{Name: "Fine"}); !errors.Is(err, ErrNoSuchRoute) {
		t.Fatalf("update of an unknown route: %v", err)
	}
}

func TestUpdateRouteDrawings(t *testing.T) {
	r := newRoutes(t)
	if _, err := r.ImportPreset(preset("abc", 163)); err != nil {
		t.Fatal(err)
	}
	line := models.Drawing{ID: "a1", Map: 2000, Kind: models.DrawLine, Color: "ff3eff",
		Points: [][2]float64{{10, -10}, {390, -90}}}
	with := func(change func(*models.Drawing)) []models.Drawing {
		d := line
		d.Points = slices.Clone(line.Points)
		change(&d)
		return []models.Drawing{d}
	}
	many := func(n int) []models.Drawing {
		out := make([]models.Drawing, n)
		for i := range out {
			out[i] = line
			out[i].ID = fmt.Sprint("d", i)
		}
		return out
	}
	point := [][2]float64{{50, -50}}
	for _, tc := range []struct {
		name     string
		drawings []models.Drawing
		wantOK   bool
	}{
		{"a line", with(func(*models.Drawing) {}), true},
		{"a marker", with(func(d *models.Drawing) { d.Kind, d.Points = models.DrawMarker, point }), true},
		{"a note", with(func(d *models.Drawing) { d.Kind, d.Points, d.Text = models.DrawNote, point, "Lust" }), true},
		{"bad kind", with(func(d *models.Drawing) { d.Kind = "circle" }), false},
		{"bad color", with(func(d *models.Drawing) { d.Color = "red" }), false},
		{"bad id", with(func(d *models.Drawing) { d.ID = "../x" }), false},
		{"repeated id", append(with(func(*models.Drawing) {}), line), false},
		{"NaN", with(func(d *models.Drawing) { d.Points[0][0] = math.NaN() }), false},
		{"infinite", with(func(d *models.Drawing) { d.Points[1][1] = math.Inf(-1) }), false},
		{"off the floor", with(func(d *models.Drawing) { d.Points[1] = [2]float64{410, -50} }), false},
		{"unknown floor", with(func(d *models.Drawing) { d.Map = 2001 }), false},
		{"line of one point", with(func(d *models.Drawing) { d.Points = point }), false},
		{"line of too many points", with(func(d *models.Drawing) {
			d.Points = slices.Repeat(point, drawingLimits.LinePoints+1)
		}), false},
		{"marker of two points", with(func(d *models.Drawing) { d.Kind = models.DrawMarker }), false},
		{"note without text", with(func(d *models.Drawing) { d.Kind, d.Points, d.Text = models.DrawNote, point, " " }),
			false},
		{"long note", with(func(d *models.Drawing) {
			d.Kind, d.Points, d.Text = models.DrawNote, point, strings.Repeat("é", drawingLimits.NoteText+1)
		}), false},
		{"text on a line", with(func(d *models.Drawing) { d.Text = "hi" }), false},
		{"the most drawings", many(drawingLimits.Drawings), true},
		{"too many drawings", many(drawingLimits.Drawings + 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.Update("abc", models.RouteUpdate{Name: "Fine", Drawings: tc.drawings})
			if tc.wantOK != (err == nil) || (err != nil && !errors.Is(err, ErrInvalidRoute)) {
				t.Fatalf("err = %v, want ok %v", err, tc.wantOK)
			}
		})
	}
	t.Run("saved drawings come back with the route, not in the list", func(t *testing.T) {
		if _, err := r.Update("abc", models.RouteUpdate{Name: "Fine", Drawings: []models.Drawing{line}}); err != nil {
			t.Fatal(err)
		}
		view, err := r.Get("abc")
		if err != nil || !reflect.DeepEqual(view.Drawings, []models.Drawing{line}) || view.Limits != drawingLimits {
			t.Fatalf("view = %+v, %v; want the line and the limits", view, err)
		}
		list, _ := r.List(voidscar)
		if len(list) != 1 || list[0].Drawings != nil {
			t.Fatalf("list = %+v; want no drawings", list)
		}
	})
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
	view, err := r.Update("abc", models.RouteUpdate{Name: "Fine", Pulls: pulls})
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
	if _, err := r.Update(view.ID, models.RouteUpdate{Name: "Fresh", Pulls: []models.RoutePull{pull}}); err != nil {
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

// altar is Altar of Fangs' first floor with its plane on MDT's map, as the reference has them.
var altar = models.DungeonFloor{RaidMap: models.RaidMap{UIMapID: 2588, MinX: -1662.5, MaxX: -2552.08, MinY: 1414.58,
	MaxY: 2008.33}, MDT: &models.MDTPlane{Sublevel: 1, Scale: 0.480366, X: 124.045, Y: 0.024,
	Box: [4]float64{58.5, -429.4, 207, -100.5}}}

// withPlanes is newRoutes with floor 2000 on all of MDT's sublevel 1 at half scale, and altar beside it on sublevel 3.
func withPlanes(t *testing.T) *Routes {
	t.Helper()
	r := newRoutes(t)
	f := r.Facts[voidscar]
	f.Floors[0].MDT = &models.MDTPlane{Sublevel: 1, Scale: 0.5, Box: [4]float64{0, -555, 840, 0}}
	a := altar
	plane := *altar.MDT
	plane.Sublevel = 3
	a.MDT = &plane
	f.Floors = append(f.Floors, a)
	r.Facts[voidscar] = f
	return r
}

func TestMDTPlaneYards(t *testing.T) {
	// Two of Altar of Fangs' enemies: where Mythic Dungeon Tools has them and where the reference does, in yards.
	r := withPlanes(t)
	for _, tc := range []struct{ mdt, yards [2]float64 }{
		{[2]float64{65.5, -205.5}, [2]float64{1779.4, 2191.8}},
		{[2]float64{201.1, -204.8}, [2]float64{1780.2, 2040.9}},
	} {
		got, kept, _ := r.importDrawings(voidscar, []models.MDTDrawing{{Sublevel: 3, Note: true, Points: [][2]float64{tc.mdt}}})
		if len(kept) > 0 || len(got) != 1 || got[0].Map != 2588 || got[0].Points[0] != tc.yards {
			t.Errorf("MDT %v = %+v (kept %v); want a marker on 2588 at %v", tc.mdt, got, kept, tc.yards)
		}
		back := r.exportDrawings(models.Route{MapID: voidscar, Drawings: []models.Drawing{{Map: 2588,
			Kind: models.DrawMarker, Points: [][2]float64{tc.yards}}}})
		if p := back[0].Points[0]; back[0].Sublevel != 3 || math.Abs(p[0]-tc.mdt[0]) > 0.1 || math.Abs(p[1]-tc.mdt[1]) > 0.1 {
			t.Errorf("yards %v = MDT %+v; want sublevel 3 at %v", tc.yards, back[0], tc.mdt)
		}
	}
}

func TestMDTObjectsRoundTrip(t *testing.T) {
	r := withPlanes(t)
	foreign := models.MDTDrawing{Sublevel: 2, Color: "ff0000", Points: [][2]float64{{1, -1}, {2, -2}}}
	code, err := mdt.EncodeRoute(models.MDTPreset{UID: "abc", DungeonIdx: 163, Drawings: []models.MDTDrawing{foreign}})
	if err != nil {
		t.Fatal(err)
	}
	if view, err := r.Import(code); err != nil || view.DroppedDrawings != 1 {
		t.Fatalf("import = %+v, %v; want MDT's sublevel 2 line dropped", view, err)
	}
	drawings := []models.Drawing{
		{ID: "a", Map: 2000, Kind: models.DrawLine, Color: "ff3eff", Points: [][2]float64{{10.5, -10}, {390, -90.2}}},
		{ID: "b", Map: 2000, Kind: models.DrawNote, Color: "ffffff", Points: [][2]float64{{200, -50}}, Text: "Lust"},
		{ID: "c", Map: 2588, Kind: models.DrawMarker, Color: "ffffff", Points: [][2]float64{{1779.4, 2191.8}}},
	}
	if _, err := r.Update("abc", models.RouteUpdate{Name: "Drawn", Drawings: drawings}); err != nil {
		t.Fatal(err)
	}
	if code, err = r.Export("abc"); err != nil {
		t.Fatal(err)
	}
	exported, err := mdt.DecodeRoute(code)
	if err != nil {
		t.Fatal(err)
	}
	if last := exported.Drawings[len(exported.Drawings)-1]; len(exported.Drawings) != 4 || last.Raw == nil ||
		last.Sublevel != 2 {
		t.Fatalf("exported drawings = %+v; want the 3 drawn and MDT's own line last", exported.Drawings)
	}
	view, err := r.Import(code)
	if err != nil {
		t.Fatal(err)
	}
	for i := range view.Drawings {
		view.Drawings[i].ID = drawings[i].ID // an import names them anew
	}
	if !reflect.DeepEqual(view.Drawings, drawings) || view.DroppedDrawings != 1 {
		t.Fatalf("imported again = %+v, %d dropped\nwant %+v, 1", view.Drawings, view.DroppedDrawings, drawings)
	}
}

func TestImportDrawingsDrops(t *testing.T) {
	line := func(sublevel int, color string, points ...[2]float64) models.MDTDrawing {
		return models.MDTDrawing{Sublevel: sublevel, Color: color, Points: points}
	}
	note := func(text string) models.MDTDrawing {
		return models.MDTDrawing{Sublevel: 1, Note: true, Points: [][2]float64{{100, -100}}, Text: text}
	}
	many := slices.Repeat([]models.MDTDrawing{note("x")}, drawingLimits.Drawings+2)
	for _, tc := range []struct {
		name      string
		drawings  []models.MDTDrawing
		kinds     []string
		droppedOf int
	}{
		{"a line, a note and a pin without text", []models.MDTDrawing{line(1, "fcfbff", [2]float64{0, 0},
			[2]float64{10, -10}), note("Kick"), note(" ")}, []string{"line", "note", "marker"}, 0},
		{"a sublevel no floor is on", []models.MDTDrawing{line(2, "fcfbff", [2]float64{0, 0}, [2]float64{1, -1})}, nil, 1},
		{"a point off the floor", []models.MDTDrawing{line(1, "fcfbff", [2]float64{0, 0}, [2]float64{0, -900})}, nil, 1},
		{"a bad color", []models.MDTDrawing{line(1, "white", [2]float64{0, 0}, [2]float64{1, -1})}, nil, 1},
		{"a long note", []models.MDTDrawing{note(strings.Repeat("x", drawingLimits.NoteText+1))}, nil, 1},
		{"past the most drawings", many, slices.Repeat([]string{"note"}, drawingLimits.Drawings), 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := withPlanes(t)
			view, err := r.ImportPreset(models.MDTPreset{UID: "abc", DungeonIdx: 163, Drawings: tc.drawings})
			if err != nil {
				t.Fatal(err)
			}
			var kinds []string
			for _, d := range view.Drawings {
				kinds = append(kinds, d.Kind)
			}
			if !slices.Equal(kinds, tc.kinds) || view.DroppedDrawings != tc.droppedOf {
				t.Fatalf("kinds = %v, %d dropped; want %v, %d", kinds, view.DroppedDrawings, tc.kinds, tc.droppedOf)
			}
		})
	}
}

func TestExportKeepsAnImportedDrawingAsMDTWroteItUntilItMoves(t *testing.T) {
	r := withPlanes(t)
	was := models.MDTDrawing{Sublevel: 3, Note: true, Points: [][2]float64{{65.5, -205.5}}, Raw: []byte{1, 2, 3}}
	for _, tc := range []struct {
		name string
		at   [2]float64
		want []byte
	}{
		{"untouched", [2]float64{1779.4, 2191.8}, []byte{1, 2, 3}},
		{"moved", [2]float64{1700, 2100}, nil},
	} {
		route := models.Route{MapID: voidscar, MDTImported: map[string]models.MDTDrawing{"mdt1": was},
			Drawings: []models.Drawing{{ID: "mdt1", Map: 2588, Kind: models.DrawMarker, Points: [][2]float64{tc.at}}}}
		if got := r.exportDrawings(route)[0].Raw; !slices.Equal(got, tc.want) {
			t.Errorf("%s: exported raw %v; want %v", tc.name, got, tc.want)
		}
	}
}
