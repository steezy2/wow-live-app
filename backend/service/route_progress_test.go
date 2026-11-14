package service

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"wow-live/backend/models"
)

// mobGUID is a creature GUID of an NPC (n tells units apart).
func mobGUID(npc, n int) string { return fmt.Sprintf("Creature-0-1-2-3-%d-%08X", npc, n) }

// died is a mob trail that ended at x, y on floor m at clockMS.
func died(x, y float64, m int, clockMS int64) models.MobTrail {
	return models.MobTrail{Points: []models.TrackPoint{{X: x - 30, Y: y, Map: m}, {X: x, Y: y, Map: m}}, Died: clockMS}
}

// progressOf is MatchKills of a route (pulls of enemy refs) on a dungeon's facts.
func progressOf(facts models.DungeonFacts, track models.KeyTrack, pulls ...[]models.EnemyRef) models.RouteProgress {
	route := models.Route{MapID: 1}
	for _, enemies := range pulls {
		route.Pulls = append(route.Pulls, models.RoutePull{Color: "ff3eff", Enemies: enemies})
	}
	view := (&Routes{Facts: map[int]models.DungeonFacts{1: facts}}).view(route)
	return MatchKills(view, facts, track)
}

func TestMatchKills(t *testing.T) {
	a, b, c := models.EnemyRef{Index: 1, Clone: 1}, models.EnemyRef{Index: 1, Clone: 2}, models.EnemyRef{Index: 2, Clone: 1}
	facts := models.DungeonFacts{Forces: models.Forces{Total: 100, NPCs: map[int]int{100: 5, 200: 5}},
		Enemies: []models.RouteEnemy{
			{Index: 1, Clone: 1, NPC: 100, Count: 5, Pack: 1, Map: 1},
			{Index: 1, Clone: 2, NPC: 100, Count: 5, Pack: 2, X: 10, Map: 1},
			{Index: 2, Clone: 1, NPC: 200, Count: 5, Boss: true, X: 100, Y: 100, Map: 2},
		}}
	for _, tc := range []struct {
		name   string
		track  models.KeyTrack
		kills  []models.EnemyRef
		offMap int
	}{
		{"same NPC in two packs close together: each kill takes the nearest one left, in death order",
			models.KeyTrack{Mobs: map[string]models.MobTrail{
				mobGUID(100, 1): died(4, 0, 1, 1000), mobGUID(100, 2): died(1, 0, 1, 2000)}},
			[]models.EnemyRef{a, b}, 0},
		{"out of reach is off the map",
			models.KeyTrack{Mobs: map[string]models.MobTrail{mobGUID(100, 1): died(0, 30, 1, 1000)}},
			[]models.EnemyRef{}, 1},
		{"another floor is off the map",
			models.KeyTrack{Mobs: map[string]models.MobTrail{mobGUID(200, 1): died(100, 100, 1, 1000)}},
			[]models.EnemyRef{}, 1},
		{"a boss followed in Bosses dies where Deaths puts it",
			models.KeyTrack{Mobs: map[string]models.MobTrail{},
				Bosses: map[string][]models.TrackPoint{mobGUID(200, 1): {{ClockMS: 500, X: 50, Y: 50, Map: 2, Name: "Boss"}}},
				Deaths: []models.TrackPoint{{ClockMS: 400, Name: "Boss"}, {ClockMS: 900, X: 99, Y: 100, Map: 2, Name: "Boss"}}},
			[]models.EnemyRef{c}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := progressOf(facts, tc.track, []models.EnemyRef{a}, []models.EnemyRef{b}, []models.EnemyRef{c})
			offMap := 0
			for _, o := range p.Off {
				offMap += o.OffMap
			}
			if !slices.Equal(p.Kills, tc.kills) || offMap != tc.offMap {
				t.Fatalf("kills %v, %d off the map; want %v, %d", p.Kills, offMap, tc.kills, tc.offMap)
			}
		})
	}
}

func TestRouteProgress(t *testing.T) {
	enemy := func(index, npc, count int, x float64) models.RouteEnemy {
		return models.RouteEnemy{Index: index, Clone: 1, NPC: npc, Count: count, X: x, Map: 1}
	}
	ref := func(index int) models.EnemyRef { return models.EnemyRef{Index: index, Clone: 1} }
	facts := models.DungeonFacts{Forces: models.Forces{Total: 100, NPCs: map[int]int{1: 8, 2: 2, 3: 5, 4: 5, 5: 3, 6: 2}},
		Enemies: []models.RouteEnemy{enemy(1, 1, 8, 0), enemy(2, 2, 2, 100), enemy(3, 3, 5, 200), enemy(4, 4, 5, 300),
			enemy(5, 5, 3, 400)}}
	track := models.KeyTrack{Pulls: []models.PullSpan{{StartMS: 0, EndMS: 10_000}, {StartMS: 20_000, EndMS: 30_000}},
		Mobs: map[string]models.MobTrail{
			mobGUID(1, 1): died(0, 0, 1, 5_000),                            // 8 of pull 1's 10 forces: done
			mobGUID(4, 1): died(300, 0, 1, 21_000),                         // all of pull 3, in key pull 2
			mobGUID(5, 1): died(400, 0, 1, 22_000),                         // on the map, in no route pull
			mobGUID(6, 1): died(900, 0, 1, 23_000),                         // worth forces, off the map
			mobGUID(7, 1): died(1000, 0, 1, 23_500),                        // worth nothing, off the map: left out
			mobGUID(3, 1): {Points: []models.TrackPoint{{X: 200, Map: 1}}}, // pull 2's, still up
		}}
	got := progressOf(facts, track, []models.EnemyRef{ref(1), ref(2)}, []models.EnemyRef{ref(3)}, []models.EnemyRef{ref(4)})
	want := models.RouteProgress{
		Pulls: []models.RoutePullProgress{
			{Killed: 1, Of: 2, Done: true, KeyPull: 0},
			{Killed: 0, Of: 1, KeyPull: -1},
			{Killed: 1, Of: 1, Done: true, KeyPull: 1, ClockMS: 20_000},
		},
		Current: 2, Next: 1, Skipped: []int{1}, Kills: []models.EnemyRef{ref(1), ref(4), ref(5)},
		Off: []models.RouteOff{{KeyPull: 1, ClockMS: 20_000, Enemies: []models.EnemyRef{ref(5)}, OffMap: 1, Forces: 5, Pct: 5}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("progress = %+v\nwant       %+v", got, want)
	}
}

// routeTrack is a key's track where the enemies of newRoutes at these x died, a second apart.
func routeTrack(xs ...float64) models.KeyTrack {
	track := models.KeyTrack{Mobs: map[string]models.MobTrail{}}
	for i, x := range xs {
		track.Mobs[mobGUID(100, i)] = died(x, 0, 0, int64(i+1)*1000)
	}
	return track
}

func TestCompareRoute(t *testing.T) {
	r := newRoutes(t)
	pull := models.MDTPull{Color: "ff3eff", Enemies: map[int][]int{1: {1}}}
	for _, p := range []models.MDTPreset{preset("abc", 163, pull), preset("def", 163), preset("other", 161)} {
		if _, err := r.ImportPreset(p); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, id   string
		track      models.KeyTrack
		wantID     string
		wantNoMobs bool
		wantErr    error
	}{
		{"the starred route by default", "", routeTrack(0), "abc", false, nil},
		{"a route by its id", "def", routeTrack(0), "def", false, nil},
		{"a route of another dungeon", "other", routeTrack(0), "", false, ErrInvalidRoute},
		{"a key saved without mobs", "", models.KeyTrack{}, "abc", true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r.Track = func(int64) (int, models.KeyTrack, error) { return voidscar, tc.track, nil }
			got, err := r.Compare(1, tc.id)
			if !errors.Is(err, tc.wantErr) || err == nil && (got.Route.ID != tc.wantID || got.NoMobs != tc.wantNoMobs) {
				t.Fatalf("compare = route %q, noMobs %v, %v; want %q, %v, %v", got.Route.ID, got.NoMobs, err, tc.wantID,
					tc.wantNoMobs, tc.wantErr)
			}
		})
	}
}

func TestLiveRoute(t *testing.T) {
	r := newRoutes(t)
	running := models.KeyView{Key: models.Key{Start: 1, MapID: voidscar, Status: "running"}}
	r.Track = func(int64) (int, models.KeyTrack, error) { return voidscar, routeTrack(), nil }
	if live, err := r.Live(running); live != nil || err != nil {
		t.Fatalf("live without a route = %+v, %v; want nil", live, err)
	}
	first := models.MDTPull{Color: "ff3eff", Enemies: map[int][]int{1: {1, 2}}}
	second := models.MDTPull{Color: "ff3eff", Enemies: map[int][]int{2: {1}}}
	if _, err := r.ImportPreset(preset("abc", 163, first, second)); err != nil {
		t.Fatal(err)
	}
	if live, err := r.Live(models.KeyView{Key: models.Key{MapID: voidscar, Status: "done"}}); live != nil || err != nil {
		t.Fatalf("live of a finished key = %+v, %v; want nil", live, err)
	}
	for _, tc := range []struct {
		name          string
		track         models.KeyTrack
		current, next int // pull numbers, 0 for none
	}{
		{"half the first pull", routeTrack(0), 1, 1},
		{"every pull", routeTrack(0, 100, 200), 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r.Track = func(int64) (int, models.KeyTrack, error) { return voidscar, tc.track, nil }
			live, err := r.Live(running)
			number := func(s *models.RouteStep) int {
				if s == nil {
					return 0
				}
				return s.Number
			}
			if err != nil || number(live.Current) != tc.current || number(live.Next) != tc.next || live.ForcesPct != 40 {
				t.Fatalf("live = %+v, %v; want current %d, next %d and the route's 40%%", live, err, tc.current, tc.next)
			}
		})
	}
}
