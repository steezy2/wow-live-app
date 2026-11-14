package service

import (
	"cmp"
	"errors"
	"maps"
	"math"
	"slices"

	"wow-live/backend/models"
)

// How a key's kills are put on a route; tune after the owner's first keys.
const (
	// MatchYards is how far from a map enemy's place a mob of its NPC can die and still be that enemy.
	MatchYards = 20.0
	// PullDonePct is the share of a route pull's forces (in percent) that makes it done when not every enemy died.
	PullDonePct = 80
)

// kill is a mob's death: its GUID, its NPC and where it died, with when on the key's clock (ClockMS).
type kill struct {
	guid string
	npc  int
	at   models.TrackPoint
}

// kills are a track's mob deaths in order: each dead mob at its last trail point, and each boss or miniboss unit
// (followed in Bosses, so not in Mobs) at its place in Deaths, the first death of its name after its last point.
func kills(track models.KeyTrack) []kill {
	var out []kill
	for guid, m := range track.Mobs {
		if m.Died > 0 {
			at := m.Points[len(m.Points)-1]
			at.ClockMS = m.Died
			out = append(out, kill{guid, npcOf(guid), at})
		}
	}
	used := make([]bool, len(track.Deaths))
	for _, guid := range slices.Sorted(maps.Keys(track.Bosses)) {
		if track.Mobs[guid].Died > 0 { // a boss seen before its fight has a trail in Mobs too
			continue
		}
		last := track.Bosses[guid][len(track.Bosses[guid])-1]
		for i, d := range track.Deaths {
			if !used[i] && d.Name == last.Name && d.ClockMS >= last.ClockMS {
				used[i] = true
				out = append(out, kill{guid, npcOf(guid), d})
				break
			}
		}
	}
	slices.SortFunc(out, func(a, b kill) int {
		return cmp.Or(cmp.Compare(a.at.ClockMS, b.at.ClockMS), cmp.Compare(a.guid, b.guid))
	})
	return out
}

// matchEnemies is the map enemy (an index in enemies) each kill was, in death order: the nearest one not taken yet with
// the kill's NPC on its floor within MatchYards, or -1 for a kill off the map.
func matchEnemies(enemies []models.RouteEnemy, kills []kill) []int {
	taken := make([]bool, len(enemies))
	out := make([]int, len(kills))
	for i, k := range kills {
		out[i] = -1
		best := MatchYards
		for j, e := range enemies {
			if d := math.Hypot(e.X-k.at.X, e.Y-k.at.Y); !taken[j] && e.NPC == k.npc && e.Map == k.at.Map && d <= best {
				out[i], best = j, d
			}
		}
		if out[i] >= 0 {
			taken[out[i]] = true
		}
	}
	return out
}

// MatchKills is how a key's track went along a route of its dungeon (facts): each dead mob is matched to a map enemy
// (matchEnemies), then counted in its route pull, or off the route by the key pull it died in.
func MatchKills(route models.RouteView, facts models.DungeonFacts, track models.KeyTrack) models.RouteProgress {
	pullOf := map[models.EnemyRef]int{}
	for i, pull := range route.Pulls {
		for _, ref := range pull.Enemies {
			pullOf[ref] = i
		}
	}
	p := models.RouteProgress{Pulls: make([]models.RoutePullProgress, len(route.Pulls)), Current: -1, Next: -1,
		Off: []models.RouteOff{}, Skipped: []int{}, Kills: []models.EnemyRef{}}
	forces := make([]int, len(route.Pulls))
	byKeyPull := make([]map[int]int, len(route.Pulls)) // each route pull's kills by key pull
	off := map[int]*models.RouteOff{}
	offAt := func(keyPull int) *models.RouteOff {
		if off[keyPull] == nil {
			off[keyPull] = &models.RouteOff{KeyPull: keyPull, ClockMS: pullStart(track.Pulls, keyPull),
				Enemies: []models.EnemyRef{}}
		}
		return off[keyPull]
	}
	all := kills(track)
	for i, at := range matchEnemies(facts.Enemies, all) {
		keyPull := keyPullAt(track.Pulls, all[i].at.ClockMS)
		if at < 0 {
			if n := facts.Forces.NPCs[all[i].npc]; n > 0 {
				o := offAt(keyPull)
				o.OffMap++
				o.Forces += n
			}
			continue
		}
		e := facts.Enemies[at]
		ref := models.EnemyRef{Index: e.Index, Clone: e.Clone}
		p.Kills = append(p.Kills, ref)
		n, ok := pullOf[ref]
		if !ok {
			o := offAt(keyPull)
			o.Enemies = append(o.Enemies, ref)
			o.Forces += e.Count
			continue
		}
		p.Current = n
		p.Pulls[n].Killed++
		forces[n] += e.Count
		if byKeyPull[n] == nil {
			byKeyPull[n] = map[int]int{}
		}
		byKeyPull[n][keyPull]++
	}
	for i, pull := range route.Pulls {
		pp := &p.Pulls[i]
		pp.Of, pp.KeyPull = len(pull.Enemies), -1
		pp.Done = pp.Killed == pp.Of || pull.Forces > 0 && forces[i]*100 >= PullDonePct*pull.Forces
		for keyPull, n := range byKeyPull[i] {
			if most := byKeyPull[i][pp.KeyPull]; n > most || n == most && keyPull < pp.KeyPull {
				pp.KeyPull = keyPull
			}
		}
		pp.ClockMS = pullStart(track.Pulls, pp.KeyPull)
		if !pp.Done && p.Next < 0 {
			p.Next = i
		}
		if pp.Killed == 0 && pp.Of > 0 {
			p.Skipped = append(p.Skipped, i)
		}
	}
	for _, keyPull := range slices.Sorted(maps.Keys(off)) {
		o := *off[keyPull]
		o.Pct = forcesPct(o.Forces, facts.Forces.Total)
		p.Off = append(p.Off, o)
	}
	return p
}

// keyPullAt is the key pull (an index in pulls) a mob dying at clockMS died in: the last one started by then, -1 before
// the first.
func keyPullAt(pulls []models.PullSpan, clockMS int64) int {
	i, _ := slices.BinarySearchFunc(pulls, clockMS+1, func(p models.PullSpan, t int64) int { return cmp.Compare(p.StartMS, t) })
	return i - 1
}

// pullStart is a key pull's start on the key's clock, 0 for none (-1).
func pullStart(pulls []models.PullSpan, keyPull int) int64 {
	if keyPull < 0 {
		return 0
	}
	return pulls[keyPull].StartMS
}

// Compare is a route of the key's dungeon next to how the key's track went along it: the route id names, or the
// dungeon's starred one when id is "". ErrNoSuchKey for an unknown key, ErrNoSuchRoute when there's no route; a
// route of another dungeon is invalid.
func (r *Routes) Compare(start int64, id string) (models.RouteCompare, error) {
	mapID, track, err := r.Track(start)
	if err != nil {
		return models.RouteCompare{}, err
	}
	var route models.Route
	if id == "" {
		route, err = r.starred(mapID)
	} else {
		route, err = r.get(id)
	}
	switch {
	case err != nil:
		return models.RouteCompare{}, err
	case route.MapID != mapID:
		return models.RouteCompare{}, invalidf("that route is for another dungeon")
	}
	view := r.view(route)
	return models.RouteCompare{Route: view, RouteProgress: MatchKills(view, r.Facts[mapID], track),
		NoMobs: track.Mobs == nil}, nil
}

// Live is the key's dungeon's starred route followed with the kills so far; nil unless the key is running and the
// dungeon has a route.
func (r *Routes) Live(key models.KeyView) (*models.RouteLive, error) {
	if key.Status != "running" {
		return nil, nil
	}
	route, err := r.starred(key.MapID)
	switch {
	case errors.Is(err, ErrNoSuchRoute):
		return nil, nil
	case err != nil:
		return nil, err
	}
	_, track, err := r.Track(key.Start)
	if err != nil {
		return nil, err
	}
	view := r.view(route)
	p := MatchKills(view, r.Facts[key.MapID], track)
	step := func(i int) *models.RouteStep {
		if i < 0 {
			return nil
		}
		return &models.RouteStep{Number: i + 1, RoutePullView: view.Pulls[i]}
	}
	return &models.RouteLive{Name: view.Name, Current: step(p.Current), Next: step(p.Next), ForcesPct: view.TotalPct}, nil
}

// starred is a dungeon's starred route; ErrNoSuchRoute when it has none.
func (r *Routes) starred(mapID int) (models.Route, error) {
	routes, err := r.ofDungeon(mapID)
	if err != nil {
		return models.Route{}, err
	}
	if i := slices.IndexFunc(routes, func(x models.Route) bool { return x.Starred }); i >= 0 {
		return routes[i], nil
	}
	return models.Route{}, ErrNoSuchRoute
}
