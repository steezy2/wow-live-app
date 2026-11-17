package service

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"wow-live/backend/models"
)

// DungeonSources is what cmd/mplusref gathered for one dungeon: Mythic Dungeon Tools' data, Wowhead's overview,
// Keystone.guru's total forces (0 when it has none) and its NPCs by ID (keystoneguru.Client.NPCs).
type DungeonSources struct {
	Name              string
	MDT               models.MDTDungeon
	Guide             models.DungeonGuide
	KeystoneGuruTotal int
	NPCs              map[int]models.DungeonMob
}

// tipKinds sorts the guide's boss tips by the words in their tag; a tip can be in several lists.
var tipKinds = map[string][]string{
	"avoidable": {"dodge", "avoid", "run away", "hide"},
	"soak":      {"soak"},
	"dispel":    {"dispel"},
	"interrupt": {"interrupt"},
}

// npcTwins are NPCs Mythic Dungeon Tools doesn't list that the game counts as one it does: a second ID of the same mob.
// Kings' Rest has four Half-Finished Mummies under each ID (a +17 on Nov 10 came to 587 of 608 without them).
var npcTwins = map[int]int{273050: 270502}

// DungeonFacts combines a dungeon's sources into its reference entry, counting each of npcTwins as its twin, and
// puts each mob's name and spells from Mythic Dungeon Tools on Keystone.guru's NPC (a boss when either says so). It
// fails when Mythic Dungeon Tools has no data for the dungeon, its forces total disagrees with Keystone.guru's or a mob
// has no health there.
func DungeonFacts(src DungeonSources) (models.DungeonFacts, error) {
	m, g := src.MDT, src.Guide
	if m.MapID == 0 {
		return models.DungeonFacts{}, fmt.Errorf("%s: Mythic Dungeon Tools has no data", src.Name)
	}
	if src.KeystoneGuruTotal > 0 && src.KeystoneGuruTotal != m.Total {
		return models.DungeonFacts{}, fmt.Errorf("%s: forces total %d in Mythic Dungeon Tools, %d on Keystone.guru",
			src.Name, m.Total, src.KeystoneGuruTotal)
	}
	tips := map[string][]int{}
	for id, tag := range g.Tips {
		for kind, words := range tipKinds {
			if slices.ContainsFunc(words, func(w string) bool { return strings.Contains(strings.ToLower(tag), w) }) {
				tips[kind] = append(tips[kind], id)
			}
		}
	}
	f := models.DungeonFacts{Name: src.Name, Guide: g.URL, Forces: models.Forces{Total: m.Total, NPCs: map[int]int{}},
		Avoidable: sorted(tips["avoidable"]), Soak: sorted(tips["soak"]),
		Dispel: sorted(append(slices.Collect(maps.Keys(g.Dispels)), tips["dispel"]...)),
		Purge:  sorted(slices.Collect(maps.Keys(g.Purges))), MustStop: []int{}, Review: []int{}}
	mobs := maps.Clone(m.Mobs)
	for twin, of := range npcTwins {
		if mob, ok := mobs[of]; ok {
			mobs[twin] = mob
		}
	}
	var counts []int64
	f.Mobs = map[int]models.DungeonMob{}
	for id, mob := range mobs {
		f.Forces.NPCs[id] = mob.Count
		if mob.Count > 0 {
			counts = append(counts, int64(mob.Count))
		}
		d, ok := src.NPCs[id]
		if !ok || d.Health == 0 {
			return models.DungeonFacts{}, fmt.Errorf("%s: Keystone.guru has no health for NPC %d (%s)", src.Name, id, mob.Name)
		}
		d.NPC, d.Name, d.Boss = id, mob.Name, d.Boss || mob.Boss
		for _, s := range mob.Spells {
			d.Spells = append(d.Spells, models.MobSpell{ID: s, Interruptible: m.Spells[s].Interruptible,
				Magic: m.Spells[s].Magic})
		}
		f.Mobs[id] = d
	}
	f.Bosses, f.Minibosses = []int{}, []int{}
	for id, mob := range mobs {
		switch {
		case mob.Boss:
			f.Bosses = append(f.Bosses, id)
		case len(counts) > 0 && float64(mob.Count) >= minibossRatio*float64(median(counts)):
			f.Minibosses = append(f.Minibosses, id)
		}
	}
	f.Bosses, f.Minibosses = sorted(f.Bosses), sorted(f.Minibosses)
	for _, id := range append(slices.Collect(maps.Keys(g.Interrupts)), tips["interrupt"]...) {
		if m.Spells[id].Interruptible {
			f.MustStop = append(f.MustStop, id)
		} else {
			f.Review = append(f.Review, id)
		}
	}
	f.MustStop, f.Review = sorted(f.MustStop), sorted(f.Review)
	return f, nil
}

// RouteEnemies places Mythic Dungeon Tools' clones on the dungeon's floors, sorted by (Index, Clone), where
// Keystone.guru's enemy with the same NPC (or the one it says Mythic Dungeon Tools lists) and clone index stands, and
// the clones Keystone.guru doesn't place on those floors.
func RouteEnemies(m models.MDTDungeon, floors []models.DungeonFloor, ksg []models.KeystoneGuruEnemy) (
	enemies []models.RouteEnemy, unplaced []models.MDTClone) {
	enemies, byClone := []models.RouteEnemy{}, ksgByClone(ksg) // a list in the file, never null
	for _, c := range m.Clones {
		e, ok := byClone[[2]int{c.NPC, c.Clone}]
		i := slices.IndexFunc(floors, func(f models.DungeonFloor) bool { return f.UIMapID == e.Map })
		if !ok || i < 0 {
			unplaced = append(unplaced, c)
			continue
		}
		x, y := ksgToLog(floors[i], e.Lat, e.Lng)
		mob := m.Mobs[c.NPC]
		enemies = append(enemies, models.RouteEnemy{Index: c.Index, Clone: c.Clone, NPC: c.NPC, Count: mob.Count,
			Boss: mob.Boss, Pack: cmp.Or(e.Pack, c.Group), X: x, Y: y, Map: e.Map})
	}
	slices.SortFunc(enemies, func(a, b models.RouteEnemy) int {
		return cmp.Or(cmp.Compare(a.Index, b.Index), cmp.Compare(a.Clone, b.Clone))
	})
	return enemies, unplaced
}

// ksgByClone is Keystone.guru's enemies by the Mythic Dungeon Tools (NPC, clone index) they stand for: their own NPC's
// and the one Mythic Dungeon Tools lists them as.
func ksgByClone(ksg []models.KeystoneGuruEnemy) map[[2]int]models.KeystoneGuruEnemy {
	byClone := map[[2]int]models.KeystoneGuruEnemy{}
	for _, e := range ksg {
		byClone[[2]int{e.NPC, e.MDTID}] = e
		if e.MDTNPC > 0 {
			byClone[[2]int{e.MDTNPC, e.MDTID}] = e
		}
	}
	return byClone
}

// A floor's MDTPlane is fitted again without the enemies more than planeOutlier times the median off the first fit
// (Mythic Dungeon Tools and Keystone.guru place a few differently), and kept when that fits to maxPlaneRMS on
// Keystone.guru's 384 by 256 map. Measured Nov 14: the season's floors fit to 0.01 (Midnight's, which Keystone.guru
// took from MDT) to 6.5 (Temple of Sethraliss' second); Magisters' Terrace, whose floors Keystone.guru lists twice
// under one ID, fits to 29 and more, or backwards.
const (
	planeOutlier = 3.0
	maxPlaneRMS  = 8.0
	minPlaneFit  = 3 // enemies a fit needs
)

// MDTPlanes is the floors, each placed on Mythic Dungeon Tools' map (MDTPlane) where its enemies (matched like
// RouteEnemies') stand there: on the sublevel most of them are on, with a scale and offset fitted from their places in
// both. A floor whose enemies fit none (see maxPlaneRMS) gets no plane.
func MDTPlanes(m models.MDTDungeon, floors []models.DungeonFloor, ksg []models.KeystoneGuruEnemy) (
	out []models.DungeonFloor) {
	byClone := ksgByClone(ksg)
	pairs := map[[2]int][][4]float64{} // (floor, sublevel) -> MDT x, y and Keystone.guru lng, lat
	for _, c := range m.Clones {
		if e, ok := byClone[[2]int{c.NPC, c.Clone}]; ok {
			k := [2]int{e.Map, c.Sublevel}
			pairs[k] = append(pairs[k], [4]float64{c.X, c.Y, e.Lng, e.Lat})
		}
	}
	keys := slices.SortedFunc(maps.Keys(pairs), func(a, b [2]int) int {
		return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
	})
	out = slices.Clone(floors)
	for i, f := range out {
		var best [][4]float64
		sublevel := 0
		for _, k := range keys {
			if k[0] == f.UIMapID && len(pairs[k]) > len(best) {
				best, sublevel = pairs[k], k[1]
			}
		}
		if plane, ok := fitPlane(best); ok {
			plane.Sublevel = sublevel
			out[i].MDT = &plane
		}
	}
	return out
}

// fitPlane fits lng = Scale·x + X and lat = Scale·y + Y to the pairs by least squares, once more without the
// outliers; Box is around the pairs it kept.
func fitPlane(pairs [][4]float64) (models.MDTPlane, bool) {
	var p models.MDTPlane
	solve := func(pairs [][4]float64) {
		var mean [4]float64
		for _, q := range pairs {
			for j := range mean {
				mean[j] += q[j] / float64(len(pairs))
			}
		}
		var num, den float64
		for _, q := range pairs {
			dx, dy := q[0]-mean[0], q[1]-mean[1]
			num += dx*(q[2]-mean[2]) + dy*(q[3]-mean[3])
			den += dx*dx + dy*dy
		}
		p.Scale = num / den
		p.X, p.Y = mean[2]-p.Scale*mean[0], mean[3]-p.Scale*mean[1]
	}
	off := func(q [4]float64) float64 { return math.Hypot(p.Scale*q[0]+p.X-q[2], p.Scale*q[1]+p.Y-q[3]) }
	if len(pairs) < minPlaneFit {
		return p, false
	}
	solve(pairs)
	offs := make([]float64, len(pairs))
	for i, q := range pairs {
		offs[i] = off(q)
	}
	median := slices.Clone(offs)
	slices.Sort(median)
	var kept [][4]float64
	for i, q := range pairs {
		if offs[i] <= planeOutlier*median[len(median)/2] || offs[i] < 0.5 {
			kept = append(kept, q)
		}
	}
	solve(kept)
	var sq float64
	p.Box = [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, q := range kept {
		sq += off(q) * off(q)
		p.Box = [4]float64{min(p.Box[0], q[0]), min(p.Box[1], q[1]), max(p.Box[2], q[0]), max(p.Box[3], q[1])}
	}
	to := func(v, per float64) float64 { return math.Round(v*per) / per } // a tidy file, far finer than a yard
	p.Scale, p.X, p.Y = to(p.Scale, 1e6), to(p.X, 1e3), to(p.Y, 1e3)
	for i := range p.Box {
		p.Box[i] = to(p.Box[i], 10)
	}
	return p, len(kept) >= minPlaneFit && p.Scale > 0 && math.Sqrt(sq/float64(len(kept))) <= maxPlaneRMS
}

// loopYards is how near two of a patrol's points are when they are the same place (Keystone.guru draws a loop's last
// point back on its first, a tenth of a yard or so off).
const loopYards = 2.0

// RoutePatrols puts Keystone.guru's patrols on the dungeon's floors in log yards, in its order, each with the route
// enemies walking it (matched to Keystone.guru's like RouteEnemies, so in (Index, Clone) order); a patrol on another
// floor or with fewer than two points is left out.
func RoutePatrols(floors []models.DungeonFloor, patrols []models.KeystoneGuruPatrol, ksg []models.KeystoneGuruEnemy,
	enemies []models.RouteEnemy) []models.Patrol {
	byClone, walkers := ksgByClone(ksg), map[int][]models.EnemyRef{}
	for _, e := range enemies {
		if p := byClone[[2]int{e.NPC, e.Clone}].Patrol; p > 0 {
			walkers[p] = append(walkers[p], models.EnemyRef{Index: e.Index, Clone: e.Clone})
		}
	}
	var out []models.Patrol
	for _, p := range patrols {
		i := slices.IndexFunc(floors, func(f models.DungeonFloor) bool { return f.UIMapID == p.Map })
		if i < 0 || len(p.Points) < 2 {
			continue
		}
		patrol := models.Patrol{Map: p.Map, Color: strings.TrimPrefix(p.Color, "#"),
			Enemies: append([]models.EnemyRef{}, walkers[p.ID]...)}
		for _, pt := range p.Points {
			x, y := ksgToLog(floors[i], pt[0], pt[1])
			patrol.Points = append(patrol.Points, [2]float64{x, y})
		}
		patrol.Loop = isLoop(patrol.Points)
		out = append(out, patrol)
	}
	return out
}

// isLoop is whether a path ends where it starts without coming back over its own points: Keystone.guru draws a walk
// back and forth as out and back again, which ends where it starts too.
func isLoop(points [][2]float64) bool {
	near := func(a, b [2]float64) bool { return math.Hypot(a[0]-b[0], a[1]-b[1]) <= loopYards }
	if len(points) < 3 || !near(points[0], points[len(points)-1]) {
		return false
	}
	inner := points[1 : len(points)-1]
	for i, p := range inner {
		if slices.ContainsFunc(inner[i+1:], func(q [2]float64) bool { return near(p, q) }) {
			return false
		}
	}
	return true
}

// Keystone.guru's map is ksgWidth by ksgHeight at zoom 0: lng runs right, lat down from 0 to -ksgHeight.
const ksgWidth, ksgHeight = 384.0, 256.0

// ksgToLog is where a Keystone.guru map point stands in log yards (to a tenth), the inverse of the map's projection
// (KeyMap.tsx's onTiles): across is the log's -y from the floor's maxX, down the log's x from its maxY.
func ksgToLog(f models.DungeonFloor, lat, lng float64) (x, y float64) {
	return math.Round((f.MaxY+lat/ksgHeight*(f.MaxY-f.MinY))*10) / 10,
		math.Round((lng/ksgWidth*(f.MaxX-f.MinX)-f.MaxX)*10) / 10
}

// logToKsg is where a point in log yards stands on a floor's Keystone.guru map: ksgToLog's inverse, unrounded.
func logToKsg(f models.DungeonFloor, x, y float64) (lat, lng float64) {
	return (x - f.MaxY) / (f.MaxY - f.MinY) * ksgHeight, (y + f.MaxX) / (f.MaxX - f.MinX) * ksgWidth
}

// sorted is the IDs once each, in order, never nil (so the file always has a list).
func sorted(ids []int) []int {
	out := append([]int{}, ids...)
	slices.Sort(out)
	return slices.Compact(out)
}

// UnevenHit is an enemy ability that hit some of a key's party or a raid often and another member never: a candidate
// for a dungeon's or a boss's avoidable list.
type UnevenHit struct {
	MapID   int // the key's map ID, or the raid encounter's ID
	SpellID int
	Name    string
	Keys    int // keys or raid pulls it was uneven in
}

// HitCounter counts, per key or raid pull, how often each enemy ability's direct hits landed on each party member, to
// find the uneven ones, and which mobs died in keys, to find those without a forces count (cmd/mplusref and
// cmd/raidref --check). A key's abilities are kept by its map ID, a raid pull's by its encounter ID.
type HitCounter struct {
	mapID  int
	raid   bool // the running segment is a raid pull
	party  []string
	hits   map[int]map[string]int // spell -> party GUID -> hits, in the running key
	names  map[int]string
	uneven map[[2]int]int // (map, spell) -> keys it was uneven in
	Kills  map[[2]int]int // (map, NPC) -> mobs of it that died in keys
	// Encounters names the raid encounters seen, by ID.
	Encounters map[int]string
}

// NewHitCounter is an empty HitCounter.
func NewHitCounter() *HitCounter {
	return &HitCounter{names: map[int]string{}, uneven: map[[2]int]int{}, Kills: map[[2]int]int{},
		Encounters: map[int]string{}}
}

// Add reads one line.
func (h *HitCounter) Add(line models.LogLine) {
	f := line.Fields
	switch {
	case line.Event == "CHALLENGE_MODE_START" && len(f) >= 3:
		h.mapID, h.raid, h.party, h.hits = atoi(f[2]), false, nil, map[int]map[string]int{}
	case line.Event == "ENCOUNTER_START" && h.hits == nil && len(f) >= 3 &&
		models.RaidDifficultyByLogID(f[2]).Name != "":
		h.mapID, h.raid, h.party, h.hits = atoi(f[0]), true, nil, map[int]map[string]int{}
		h.Encounters[h.mapID] = f[1]
	case h.hits == nil:
	case line.Event == "CHALLENGE_MODE_END" || h.raid && line.Event == "ENCOUNTER_END":
		h.endKey()
	case line.Event == "COMBATANT_INFO" && len(f) > 0:
		h.party = append(h.party, f[0])
	case line.Event == "UNIT_DIED" && !h.raid && len(f) > 4 && strings.HasPrefix(f[4], "Creature-"):
		if parts := strings.Split(f[4], "-"); len(parts) > 5 {
			h.Kills[[2]int{h.mapID, atoi(parts[5])}]++
		}
	case line.Event == "SPELL_DAMAGE" && len(f) > 9 && hasFlag(f[2], models.FlagHostile) &&
		slices.Contains(h.party, f[4]):
		spell := atoi(f[8])
		if h.hits[spell] == nil {
			h.hits[spell] = map[string]int{}
		}
		h.hits[spell][f[4]]++
		h.names[spell] = f[9]
	}
}

// endKey keeps the abilities that were uneven in the key that just ended: one member hit at least 3 times the median
// and at least 3 times, another never.
func (h *HitCounter) endKey() {
	for spell, byPlayer := range h.hits {
		counts := make([]int, len(h.party))
		for i, guid := range h.party {
			counts[i] = byPlayer[guid]
		}
		slices.Sort(counts)
		if len(counts) > 0 && counts[0] == 0 && counts[len(counts)-1] >= max(3, 3*counts[len(counts)/2]) {
			h.uneven[[2]int{h.mapID, spell}]++
		}
	}
	h.hits = nil
}

// Uneven is every ability that was uneven in at least one key, most keys first.
func (h *HitCounter) Uneven() []UnevenHit {
	var out []UnevenHit
	for k, n := range h.uneven {
		out = append(out, UnevenHit{MapID: k[0], SpellID: k[1], Name: h.names[k[1]], Keys: n})
	}
	slices.SortFunc(out, func(a, b UnevenHit) int {
		return cmp.Or(cmp.Compare(a.MapID, b.MapID), cmp.Compare(b.Keys, a.Keys), cmp.Compare(a.SpellID, b.SpellID))
	})
	return out
}
