package service

import (
	"cmp"
	"errors"
	"io/fs"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"wow-live/backend/models"
)

const (
	// trackEveryMS: a party member's position is kept at most this often.
	trackEveryMS = 1_000
	// mobEveryMS: an ordinary mob's position is kept at most this often, as a key has hundreds.
	mobEveryMS = 2_000
	// walkedGrid is the size in yards of the squares the walked area is drawn in.
	walkedGrid = 3.0
)

// savedRun is a finished key and its track, waiting to be saved.
type savedRun struct {
	key   models.Key
	track models.KeyTrack
}

// position keeps where the actor in a line's advanced-logging block was (x, y and map at offsets 14 to 16 of the
// block): a party member about once a second for the map, and every mob's last place for where it dies.
func (k *LogTracker) position(at int64, event string, f []string) {
	i := advancedAt(event)
	if i == 0 || len(f) < i+advancedFields {
		return
	}
	run, guid := k.current, f[i]
	x, errX := strconv.ParseFloat(f[i+14], 64)
	y, errY := strconv.ParseFloat(f[i+15], 64)
	if errX != nil || errY != nil {
		return
	}
	p := models.TrackPoint{ClockMS: k.clockAt(at), X: x, Y: y, Map: atoi(f[i+16])}
	switch {
	case slices.Contains(run.party, guid):
		if at-run.sampled[guid] >= trackEveryMS {
			run.sampled[guid] = at
			name := k.names[guid]
			run.track.Players[name] = append(run.track.Players[name], p)
		}
	case creature(guid):
		run.mobHP[guid] = int64(atoi(f[i+3]))
		p.Name = f[1]
		if guid == f[4] {
			p.Name = f[5]
		}
		run.mobAt[guid] = p
		r := run.raid
		watched := r != nil && (slices.Contains(r.boss.NPCs, npcOf(guid)) || slices.Contains(r.boss.Priority, p.Name))
		if r == nil {
			watched = run.bigUnit(guid, p.Name)
		}
		switch {
		case watched && at-run.sampled[guid] >= trackEveryMS:
			run.sampled[guid] = at
			if hp, maxHP := atoi(f[i+2]), atoi(f[i+3]); maxHP > 0 {
				p.HP = math.Round(1000*float64(hp)/float64(maxHP)) / 10
			}
			run.track.Bosses[guid] = append(run.track.Bosses[guid], p)
		case !watched && run.track.Bosses[guid] == nil && hostileActor(guid, f) && at-run.sampled[guid] >= mobEveryMS:
			run.sampled[guid] = at
			m := run.track.Mobs[guid]
			m.Name = p.Name
			m.Points = append(m.Points, models.TrackPoint{ClockMS: p.ClockMS, X: x, Y: y, Map: p.Map})
			run.track.Mobs[guid] = m
		}
	}
}

// meterEveryMS: how often the replay's meters are sampled.
const meterEveryMS = 2_000

// sampleMeters keeps each player's damage and healing so far about every meterEveryMS, so the replay can show the
// meters at any moment.
func (k *LogTracker) sampleMeters(at int64) {
	run := k.current
	if at-run.meterAt < meterEveryMS {
		return
	}
	run.meterAt = at
	clockMS := k.clockAt(at)
	for player, r := range run.total {
		run.track.Meters[player] = append(run.track.Meters[player], models.MeterPoint{ClockMS: clockMS,
			Damage: r.Damage, Healing: r.Healing, Taken: r.DamageTaken, Avoidable: r.Avoidable,
			Priority: r.PriorityDamage, Kicks: r.Interrupts, Dispels: r.Dispels + r.Purges})
	}
}

// abilitySampler is what the replay's focus bars keep of a run: when the ability totals were last kept and what they
// were then, each player's cast going (GUID -> its index in their track casts), and how long a stretch lasts.
type abilitySampler struct {
	every, at     int64
	abilitiesWere map[[2]string]models.KeyAbility
	takenWere     map[[2]string]int64
	targetsWere   map[[2]string]int64
	casting       map[string]int
}

// abilityEveryMS: how long a raid pull's ability totals are kept for the replay's focus bars, as stretches on the
// pull's clock.
const abilityEveryMS = 2_000

// mplusAbilityEveryMS is a stretch in a key: it lasts five times as long, so a half-hour key's track stays small.
const mplusAbilityEveryMS = 10_000

// sampleAbilities closes a stretch when a line crosses a boundary (abilityEveryMS in a raid pull, mplusAbilityEveryMS in
// a key) of the run's clock
// (so a quiet log makes one long stretch, not many empty ones): each raider's damage, healing and damage taken by
// ability, and damage into each enemy, since the last stretch (only what changed), with when it closed, so the
// replay can sum them up to any moment and share out the stretch it's in. Casts come from the track's casts; hits
// stay whole-pull.
func (k *LogTracker) sampleAbilities(at int64) {
	run := k.current
	s := &run.sample
	clockMS := k.clockAt(at)
	if clockMS/s.every <= s.at/s.every {
		return
	}
	s.at = clockMS
	run.track.AbilityTimes = append(run.track.AbilityTimes, clockMS)
	add := func(into map[string][]models.AbilitySeries, id [2]string, p [4]int64) {
		series := into[id[0]]
		i := slices.IndexFunc(series, func(s models.AbilitySeries) bool { return s.Ability == id[1] })
		if i < 0 {
			series, i = append(series, models.AbilitySeries{Ability: id[1]}), len(series)
		}
		series[i].Points = append(series[i].Points, p)
		into[id[0]] = series
	}
	for id, a := range run.abilities {
		if was := s.abilitiesWere[id]; a.Damage != was.Damage || a.Healing != was.Healing {
			add(run.track.Abilities, id, [4]int64{clockMS, a.Damage - was.Damage, a.Healing - was.Healing, 0})
			s.abilitiesWere[id] = *a
		}
	}
	for id, taken := range run.takenBy {
		if was := s.takenWere[id]; taken != was {
			add(run.track.Abilities, id, [4]int64{clockMS, 0, 0, taken - was})
			s.takenWere[id] = taken
		}
	}
	for id, damage := range targetDamage(run) {
		if was := s.targetsWere[id]; damage != was {
			add(run.track.TargetSeries, id, [4]int64{clockMS, damage - was, 0, 0})
			s.targetsWere[id] = damage
		}
	}
}

// targetDamage is each raider's damage into the enemies of each name over the run, by [raider, enemy].
func targetDamage(run *keyRun) map[[2]string]int64 {
	damage := map[[2]string]int64{}
	for _, pull := range run.pulls {
		for _, mob := range pull.mobs {
			for player, amount := range mob.byPlayer {
				damage[[2]string{player, mob.name}] += amount
			}
		}
	}
	return damage
}

// trackCast keeps a raider's or a hostile creature's cast for the replay (a raider's by name, a creature's in MobCasts
// by GUID): a cast's start, then its end (SPELL_CAST_SUCCESS of the same spell, or SPELL_CAST_FAILED with
// "Interrupted", not a key pressed again mid-cast), or an instant.
func (k *LogTracker) trackCast(at int64, event string, f []string) {
	run := k.current
	if len(f) < 10 {
		return
	}
	var casts map[string][]models.TrackCast
	who := f[0]
	switch {
	case slices.Contains(run.party, f[0]):
		casts, who = run.track.Casts, k.names[f[0]]
	case creature(f[0]) && hasFlag(f[2], hostileFlag):
		if run.track.MobCasts == nil {
			run.track.MobCasts = map[string][]models.TrackCast{}
		}
		casts = run.track.MobCasts
	default:
		return
	}
	clockMS := k.clockAt(at)
	open, casting := run.sample.casting[f[0]]
	switch {
	case event == "SPELL_CAST_START":
		casts[who] = append(casts[who], models.TrackCast{ClockMS: clockMS, Spell: f[9], Cut: true})
		run.sample.casting[f[0]] = len(casts[who]) - 1
	case casting && casts[who][open].Spell == f[9] &&
		(event == "SPELL_CAST_SUCCESS" || len(f) > 11 && f[11] == "Interrupted"):
		c := &casts[who][open]
		c.EndMS, c.Cut = max(clockMS, c.ClockMS+1), event != "SPELL_CAST_SUCCESS"
		delete(run.sample.casting, f[0])
	case event == "SPELL_CAST_SUCCESS":
		casts[who] = append(casts[who], models.TrackCast{ClockMS: clockMS, Spell: f[9]})
	}
}

// creature is whether a GUID is an NPC's: a mob's, a boss's, a vehicle's or a summoned pet's, not a player's.
func creature(guid string) bool {
	return strings.HasPrefix(guid, "Creature-") || strings.HasPrefix(guid, "Vehicle-")
}

// hostileActor is whether the advanced-logging block's actor is the line's hostile source or target.
func hostileActor(guid string, f []string) bool {
	return (guid == f[0] && hasFlag(f[2], hostileFlag)) || (guid == f[4] && hasFlag(f[6], hostileFlag))
}

// mobDeathAt puts a mob's death on the map where it was last seen, and ends its trail there.
func (k *LogTracker) mobDeathAt(at int64, guid string) {
	run := k.current
	if p, ok := run.mobAt[guid]; ok {
		p.ClockMS = k.clockAt(at)
		run.track.Deaths = append(run.track.Deaths, p)
		delete(run.mobAt, guid)
		if m, ok := run.track.Mobs[guid]; ok {
			m.Died = p.ClockMS
			run.track.Mobs[guid] = m
		}
	}
}

// trackOf is a copy of a run's track with each player's class, safe to hand out while the run goes on.
func (k *LogTracker) trackOf(run *keyRun) models.KeyTrack {
	t := models.KeyTrack{Players: map[string][]models.TrackPoint{}, Classes: k.classes(run),
		Deaths: list(run.track.Deaths), Meters: map[string][]models.MeterPoint{}, Pulls: list(run.track.Pulls),
		Recaps: k.recaps(run)}
	for name, points := range run.track.Players {
		t.Players[name] = list(points)
	}
	for name, points := range run.track.Meters {
		t.Meters[name] = list(points)
	}
	t.Bosses = bossTrails(run.track.Bosses)
	if len(run.track.Mobs) > 0 {
		t.Mobs = mobTrails(run.track.Mobs, run.key.Status == "running") // the live map shows only the mobs still up
	}
	if run.key.Status != "running" { // the focus bars and casts are for the replay of a finished run
		t.AbilityTimes = list(run.track.AbilityTimes)
		t.Casts = map[string][]models.TrackCast{}
		for name, casts := range run.track.Casts {
			t.Casts[name] = list(casts)
		}
		if len(run.track.MobCasts) > 0 {
			t.MobCasts = map[string][]models.TrackCast{}
			for guid, casts := range run.track.MobCasts {
				t.MobCasts[guid] = list(casts)
			}
		}
		t.Abilities, t.TargetSeries = copySeries(run.track.Abilities), copySeries(run.track.TargetSeries)
	}
	if r := run.raid; r == nil {
		t.Health = slices.SortedFunc(maps.Keys(t.Bosses), func(a, b string) int {
			return cmp.Or(cmp.Compare(t.Bosses[a][0].ClockMS, t.Bosses[b][0].ClockMS), cmp.Compare(a, b))
		})
	} else {
		t.Health = slices.Sorted(maps.Keys(r.bossUnits()))
		var adds []string
		for guid, u := range r.units {
			if slices.Contains(r.boss.Priority, u.name) && !slices.Contains(t.Health, guid) {
				adds = append(adds, guid)
			}
		}
		slices.Sort(adds)
		t.Health = append(t.Health, adds...)
	}
	return t
}

// bossTrails is a copy of a run's boss trails, nil when there's none.
func bossTrails(from map[string][]models.TrackPoint) map[string][]models.TrackPoint {
	if len(from) == 0 {
		return nil
	}
	out := map[string][]models.TrackPoint{}
	for guid, points := range from {
		out[guid] = list(points)
	}
	return out
}

// mobTrails is a copy of a run's mob trails, without the dead ones when skipDead.
func mobTrails(from map[string]models.MobTrail, skipDead bool) map[string]models.MobTrail {
	out := map[string]models.MobTrail{}
	for guid, m := range from {
		if !skipDead || m.Died == 0 {
			out[guid] = models.MobTrail{Points: list(m.Points), Died: m.Died}
		}
	}
	return out
}

// copySeries is a copy of ability series by raider, each raider's sorted by ability.
func copySeries(from map[string][]models.AbilitySeries) map[string][]models.AbilitySeries {
	out := map[string][]models.AbilitySeries{}
	for name, series := range from {
		for _, s := range series {
			out[name] = append(out[name], models.AbilitySeries{Ability: s.Ability, Points: slices.Clone(s.Points)})
		}
		slices.SortFunc(out[name], func(a, b models.AbilitySeries) int { return cmp.Compare(a.Ability, b.Ability) })
	}
	return out
}

// Track is a key's track by its start time: the running key's, the last one's, or a saved one's.
func (k *LogTracker) Track(start int64) (models.KeyTrack, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	switch {
	case k.current != nil && k.current.key.Start == start:
		return k.trackOf(k.current), nil
	case k.last != nil && k.last.Start == start && k.lastTrack != nil:
		return *k.lastTrack, nil
	}
	t, err := k.store.Track(start)
	if errors.Is(err, fs.ErrNotExist) {
		return t, ErrNoSuchKey
	}
	return t, err
}

// RouteTrack is a key's map ID and what comparing it with a route reads of its track (by its start time, as Track):
// for the running key its mobs with the dead ones (which Track leaves out while it runs), deaths, bosses and pulls; a
// key saved without a track gets an empty one.
func (k *LogTracker) RouteTrack(start int64) (int, models.KeyTrack, error) {
	k.mu.RLock()
	if run := k.current; run != nil && run.raid == nil && run.key.Start == start {
		defer k.mu.RUnlock()
		return run.key.MapID, models.KeyTrack{Mobs: mobTrails(run.track.Mobs, false), Deaths: list(run.track.Deaths),
			Bosses: bossTrails(run.track.Bosses), Pulls: list(run.track.Pulls)}, nil
	}
	mapID := 0
	if i := slices.IndexFunc(k.history, func(key models.Key) bool { return key.Start == start }); i >= 0 {
		mapID = k.history[i].MapID
	} else if k.last != nil && k.last.Start == start {
		mapID = k.last.MapID
	}
	k.mu.RUnlock()
	if mapID == 0 {
		return 0, models.KeyTrack{}, ErrNoSuchKey
	}
	track, err := k.Track(start)
	if errors.Is(err, ErrNoSuchKey) {
		err = nil
	}
	return mapID, track, err
}

// DungeonMap is a dungeon's floors with Keystone.guru tiles (from the dungeon file) and where parties have walked in
// its saved runs, one point per walkedGrid square (Name and ClockMS empty): the shape drawn for floors without tiles.
func (k *LogTracker) DungeonMap(mapID int) (models.DungeonMap, error) {
	walked, err := k.walked(mapID)
	return models.DungeonMap{Floors: list(k.facts[mapID].Floors), Walked: walked, Enemies: list(k.facts[mapID].Enemies)}, err
}

func (k *LogTracker) walked(mapID int) ([]models.TrackPoint, error) {
	k.mu.RLock()
	var starts []int64
	for _, key := range k.history {
		if key.MapID == mapID {
			starts = append(starts, key.Start)
		}
	}
	k.mu.RUnlock()
	seen := map[[3]int]bool{}
	walked := []models.TrackPoint{}
	for _, start := range starts {
		t, err := k.store.Track(start)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, points := range t.Players {
			for _, p := range points {
				cell := [3]int{int(math.Floor(p.X / walkedGrid)), int(math.Floor(p.Y / walkedGrid)), p.Map}
				if !seen[cell] {
					seen[cell] = true
					walked = append(walked, models.TrackPoint{X: (float64(cell[0]) + 0.5) * walkedGrid,
						Y: (float64(cell[1]) + 0.5) * walkedGrid, Map: p.Map})
				}
			}
		}
	}
	return walked, nil
}

// bigUnit is whether a key's mob is worth following on the map's health box: the boss being fought (a unit of its
// name) or one of the dungeon file's minibosses.
func (r *keyRun) bigUnit(guid, name string) bool {
	if n := len(r.key.Bosses); n > 0 && r.key.Bosses[n-1].EndMS == 0 && strings.EqualFold(r.key.Bosses[n-1].Name, name) {
		return true
	}
	return r.facts != nil && slices.Contains(r.facts.Minibosses, npcOf(guid))
}
