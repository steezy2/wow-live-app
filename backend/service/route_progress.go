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
	// matchYards is how far from a map enemy's place a mob of its NPC can first be seen and still be that enemy.
	matchYards = 30.0
	// pullDonePct is the share of a route pull's forces (in percent) that makes it done when not every enemy died.
	pullDonePct = 80
	// livePullsAhead is how many unfinished pulls after the last one reached are shown as next: a fight's kills reach
	// the log only when it ends.
	livePullsAhead = 2
	// behindPct is how many points of forces the key may trail the route's plan before it is flagged.
	behindPct = 1.0
)

// kill is a mob's death: its GUID, its NPC and where it stood, with when it died on the key's clock (ClockMS).
type kill struct {
	guid string
	npc  int
	at   models.TrackPoint
}

// kills are a track's mob deaths in order: each dead mob at its first trail point (a pulled mob runs to the party, so
// where it died is not where the route put it), and each boss or miniboss unit
// (followed in Bosses, so not in Mobs) at its place in Deaths, the first death of its name after its last point.
func kills(track models.KeyTrack) []kill {
	var out []kill
	for guid, m := range track.Mobs {
		if m.Died > 0 {
			at := m.Points[0]
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

// matchEnemies is the map enemy (an index in enemies) each kill was, in death order, or -1 for a kill off the map: the
// pairs of a kill and an enemy of its NPC on its floor within maxYards are taken closest first, so a pack's far kill
// doesn't take the enemy a nearer kill stood at.
func matchEnemies(enemies []models.RouteEnemy, kills []kill, maxYards float64) []int {
	type pair struct {
		kill, enemy int
		yards       float64
	}
	var pairs []pair
	for i, k := range kills {
		for j, e := range enemies {
			if d := math.Hypot(e.X-k.at.X, e.Y-k.at.Y); e.NPC == k.npc && e.Map == k.at.Map && d <= maxYards {
				pairs = append(pairs, pair{i, j, d})
			}
		}
	}
	slices.SortStableFunc(pairs, func(a, b pair) int { return cmp.Compare(a.yards, b.yards) })
	out, taken := make([]int, len(kills)), make([]bool, len(enemies))
	for i := range out {
		out[i] = -1
	}
	for _, p := range pairs {
		if out[p.kill] < 0 && !taken[p.enemy] {
			out[p.kill], taken[p.enemy] = p.enemy, true
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
	for i, at := range matchEnemies(facts.Enemies, all, matchYards) {
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

// madePull is the map enemies one key pull killed (KeyPull an index in the track's Pulls).
type madePull struct {
	KeyPull int
	Enemies []models.EnemyRef
}

// madePulls are the pulls a key made: each kill takes the nearest map enemy of its NPC on its floor that no closer
// kill took (a recording has no plan to hold the kill to, so a slightly different pull still counts), grouped by the
// key pull it died in, in pull order. Kills before the first pull join it. Dropped counts the kills worth forces that no
// enemy was left for.
func madePulls(facts models.DungeonFacts, track models.KeyTrack) (pulls []madePull, dropped int) {
	all := kills(track)
	byPull := map[int][]models.EnemyRef{}
	for i, at := range matchEnemies(facts.Enemies, all, math.Inf(1)) {
		if at < 0 {
			if facts.Forces.NPCs[all[i].npc] > 0 {
				dropped++
			}
			continue
		}
		e := facts.Enemies[at]
		pull := max(keyPullAt(track.Pulls, all[i].at.ClockMS), 0)
		byPull[pull] = append(byPull[pull], models.EnemyRef{Index: e.Index, Clone: e.Clone})
	}
	for _, pull := range slices.Sorted(maps.Keys(byPull)) {
		pulls = append(pulls, madePull{pull, byPull[pull]})
	}
	return pulls, dropped
}

// enemyForces is the forces each map enemy gives.
func enemyForces(facts models.DungeonFacts) map[models.EnemyRef]int {
	out := map[models.EnemyRef]int{}
	for _, e := range facts.Enemies {
		out[models.EnemyRef{Index: e.Index, Clone: e.Clone}] = e.Count
	}
	return out
}

// recordPulls are madePulls as the pulls of a route to record, with the forces share they hold.
func recordPulls(facts models.DungeonFacts, track models.KeyTrack) models.RecordedRoute {
	made, dropped := madePulls(facts, track)
	count, forces := enemyForces(facts), 0
	out := models.RecordedRoute{Pulls: [][]models.EnemyRef{}, Dropped: dropped}
	for _, m := range made {
		out.Pulls = append(out.Pulls, m.Enemies)
		for _, ref := range m.Enemies {
			forces += count[ref]
		}
	}
	out.Pct = share(forces, facts.Forces.Total)
	return out
}

// buildRun is how a key's own pulls (madePulls) sit against a route: each key pull's route pull is the one most of its
// enemies are in, and it matches that pull, differs from it (enemies of other pulls killed in it, or of the pull not
// killed in it) or is off the route. Unseen are the route's enemies no key pull killed, by route pull; they are not
// known missed, since the log can fail to show a death.
func buildRun(route models.RouteView, facts models.DungeonFacts, track models.KeyTrack) models.RouteRun {
	made, _ := madePulls(facts, track)
	count := enemyForces(facts)
	planOf := map[models.EnemyRef]int{}
	for i, pull := range route.Pulls {
		for _, ref := range pull.Enemies {
			planOf[ref] = i
		}
	}
	run := models.RouteRun{Pulls: []models.RunPull{}, Unseen: []models.RouteMiss{}}
	killed := map[models.EnemyRef]bool{}
	for _, m := range made {
		overlap := map[int]int{}
		forces := 0
		for _, ref := range m.Enemies {
			killed[ref] = true
			forces += count[ref]
			if i, ok := planOf[ref]; ok {
				overlap[i]++
			}
		}
		rp := models.RunPull{KeyPull: m.KeyPull, ClockMS: pullStart(track.Pulls, m.KeyPull), Enemies: m.Enemies, Forces: forces,
			Pct: share(forces, facts.Forces.Total), Plan: -1, State: models.RunOff}
		for _, i := range slices.Sorted(maps.Keys(overlap)) {
			if rp.Plan < 0 || overlap[i] > overlap[rp.Plan] {
				rp.Plan = i
			}
		}
		if rp.Plan >= 0 {
			rp.State = models.RunMatches
			rp.Extra = len(m.Enemies) - overlap[rp.Plan]
			for _, ref := range route.Pulls[rp.Plan].Enemies {
				if !slices.Contains(m.Enemies, ref) {
					rp.Missing++
				}
			}
			if rp.Extra > 0 || rp.Missing > 0 {
				rp.State = models.RunDiffers
			}
		}
		run.Pulls = append(run.Pulls, rp)
	}
	for i, pull := range route.Pulls {
		miss := models.RouteMiss{Pull: i, Enemies: []models.EnemyRef{}}
		for _, ref := range pull.Enemies {
			if !killed[ref] {
				miss.Enemies = append(miss.Enemies, ref)
				miss.Forces += count[ref]
			}
		}
		if len(miss.Enemies) > 0 {
			miss.Pct = share(miss.Forces, facts.Forces.Total)
			run.Unseen = append(run.Unseen, miss)
		}
	}
	return run
}

// Recorded is the pulls a key made as route pulls' enemies, for a route recorded from it (ErrNoSuchKey for an unknown
// key); none for a key saved before mob positions were kept.
func (r *Routes) Recorded(start int64) (models.RecordedRoute, error) {
	mapID, track, err := r.Track(start)
	if err != nil {
		return models.RecordedRoute{}, err
	}
	return recordPulls(r.Facts[mapID], track), nil
}

// markPulls gives each route pull its state and finds the missed pulls and how far the forces trail the plan; forces
// are the key's kills' forces by route pull.
func markPulls(p *models.RouteProgress, route models.RouteView, facts models.DungeonFacts, forces []int) {
	count := enemyForces(facts)
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
	// a pull still being killed is not behind plan yet: plan only up to the pull before it
	last := route.Pulls[p.Last]
	planned, actual := last.TotalPct, share(have, facts.Forces.Total)
	if !p.Pulls[p.Last].Done {
		planned -= last.Pct
	}
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

// pullStart is a key pull's start on the key's clock, 0 for none (-1, or a track with no pulls).
func pullStart(pulls []models.PullSpan, keyPull int) int64 {
	if keyPull < 0 || keyPull >= len(pulls) {
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
		Run: buildRun(view, r.Facts[mapID], track), NoMobs: track.Mobs == nil}, nil
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
