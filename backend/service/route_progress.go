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
	// matchYards is how far from a map enemy's place a mob of its NPC can die and still be that enemy.
	matchYards = 20.0
	// pullDonePct is the share of a route pull's forces (in percent) that makes it done when not every enemy died.
	pullDonePct = 80
	// livePullsAhead is how many unfinished pulls after the last one reached are shown as next: a fight's kills reach
	// the log only when it ends.
	livePullsAhead = 2
	// behindPct is how many points of forces the key may trail the route's plan before it is flagged.
	behindPct = 1.0
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
// the kill's NPC on its floor within matchYards, or -1 for a kill off the map.
func matchEnemies(enemies []models.RouteEnemy, kills []kill) []int {
	taken := make([]bool, len(enemies))
	out := make([]int, len(kills))
	for i, k := range kills {
		out[i] = -1
		best := matchYards
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
		Off: []models.RouteOff{}, Skipped: []int{}, Kills: []models.EnemyRef{}, Last: -1, Missed: []models.RouteMiss{}}
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
		pp.Done = pp.Killed == pp.Of || pull.Forces > 0 && forces[i]*100 >= pullDonePct*pull.Forces
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
		if pp.Killed > 0 {
			p.Last = i
		}
	}
	for _, keyPull := range slices.Sorted(maps.Keys(off)) {
		o := *off[keyPull]
		o.Pct = share(o.Forces, facts.Forces.Total)
		p.Off = append(p.Off, o)
	}
	markPulls(&p, route, facts, forces)
	return p
}

// markPulls gives each route pull its state and finds the missed pulls and how far the forces trail the plan; forces
// are the key's kills' forces by route pull.
func markPulls(p *models.RouteProgress, route models.RouteView, facts models.DungeonFacts, forces []int) {
	count := map[models.EnemyRef]int{}
	for _, e := range facts.Enemies {
		count[models.EnemyRef{Index: e.Index, Clone: e.Clone}] = e.Count
	}
	killed := map[models.EnemyRef]bool{}
	for _, ref := range p.Kills {
		killed[ref] = true
	}
	ahead := 0
	for i := range p.Pulls {
		pp := &p.Pulls[i]
		switch {
		case pp.Done:
			pp.State = models.PullDone
		case i < p.Last:
			pp.State = models.PullMissed
			m := models.RouteMiss{Pull: i, Enemies: []models.EnemyRef{}}
			for _, ref := range route.Pulls[i].Enemies {
				if !killed[ref] {
					m.Enemies = append(m.Enemies, ref)
					m.Forces += count[ref]
				}
			}
			m.Pct = share(m.Forces, facts.Forces.Total)
			p.Missed = append(p.Missed, m)
		case ahead < livePullsAhead:
			pp.State, ahead = models.PullNext, ahead+1
		default:
			pp.State = models.PullLater
		}
	}
	if p.Last < 0 {
		return
	}
	have := 0
	for _, n := range forces {
		have += n
	}
	for _, o := range p.Off {
		have += o.Forces
	}
	planned, actual := route.Pulls[p.Last].TotalPct, share(have, facts.Forces.Total)
	if planned-actual >= behindPct {
		p.Behind = &models.RouteBehind{PlannedPct: planned, ActualPct: actual}
	}
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
	if key.Status != models.StatusRunning {
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
	return &models.RouteLive{Name: view.Name, Current: step(p.Current), Next: step(p.Next), ForcesPct: view.TotalPct,
		Missed: len(p.Missed), Behind: p.Behind != nil}, nil
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
