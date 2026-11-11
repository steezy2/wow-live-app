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
	case strings.HasPrefix(guid, "Creature-") || strings.HasPrefix(guid, "Vehicle-"):
		run.mobHP[guid] = int64(atoi(f[i+3]))
		p.Name = f[1]
		if guid == f[4] {
			p.Name = f[5]
		}
		run.mobAt[guid] = p
		r := run.raid
		if r != nil && (slices.Contains(r.boss.NPCs, npcOf(guid)) || slices.Contains(r.boss.Priority, p.Name)) &&
			at-run.sampled[guid] >= trackEveryMS {
			run.sampled[guid] = at
			if hp, maxHP := atoi(f[i+2]), atoi(f[i+3]); maxHP > 0 {
				p.HP = math.Round(1000*float64(hp)/float64(maxHP)) / 10
			}
			run.track.Bosses[guid] = append(run.track.Bosses[guid], p)
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
			Damage: r.Damage, Healing: r.Healing, Taken: r.DamageTaken})
	}
}

// abilityEveryMS: how often a raid pull's ability totals are kept for the replay's focus bars.
const abilityEveryMS = 10_000

// sampleAbilities keeps, about every abilityEveryMS in a raid pull, the damage, healing and damage taken of each
// raider's abilities since the last time (only what changed) and when it was kept, so the replay can sum them up to
// any moment and share out the stretch it's in. Casts come from the track's casts; hits stay whole-pull.
func (k *LogTracker) sampleAbilities(at int64) {
	run := k.current
	r := run.raid
	if r == nil || at-r.abilitiesAt < abilityEveryMS {
		return
	}
	r.abilitiesAt = at
	clockMS := k.clockAt(at)
	run.track.AbilityTimes = append(run.track.AbilityTimes, clockMS)
	add := func(id [2]string, p [4]int64) {
		series := run.track.Abilities[id[0]]
		i := slices.IndexFunc(series, func(s models.AbilitySeries) bool { return s.Ability == id[1] })
		if i < 0 {
			series, i = append(series, models.AbilitySeries{Ability: id[1]}), len(series)
		}
		series[i].Points = append(series[i].Points, p)
		run.track.Abilities[id[0]] = series
	}
	for id, a := range run.abilities {
		if was := r.abilitiesWere[id]; a.Damage != was.Damage || a.Healing != was.Healing {
			add(id, [4]int64{clockMS, a.Damage - was.Damage, a.Healing - was.Healing, 0})
			r.abilitiesWere[id] = *a
		}
	}
	for id, taken := range run.takenBy {
		if was := r.takenWere[id]; taken != was {
			add(id, [4]int64{clockMS, 0, 0, taken - was})
			r.takenWere[id] = taken
		}
	}
}

// trackCast keeps a raider's cast for the replay: a cast's start, then its end (SPELL_CAST_SUCCESS of the same spell,
// or SPELL_CAST_FAILED with "Interrupted", not a key pressed again mid-cast), or an instant.
func (k *LogTracker) trackCast(at int64, event string, f []string) {
	run := k.current
	r := run.raid
	if len(f) < 10 || !slices.Contains(run.party, f[0]) {
		return
	}
	player, casts := k.names[f[0]], run.track.Casts
	clockMS := k.clockAt(at)
	open, casting := r.casting[f[0]]
	switch {
	case event == "SPELL_CAST_START":
		casts[player] = append(casts[player], models.TrackCast{ClockMS: clockMS, Spell: f[9], Cut: true})
		r.casting[f[0]] = len(casts[player]) - 1
	case casting && casts[player][open].Spell == f[9] &&
		(event == "SPELL_CAST_SUCCESS" || len(f) > 11 && f[11] == "Interrupted"):
		c := &casts[player][open]
		c.EndMS, c.Cut = max(clockMS, c.ClockMS+1), event != "SPELL_CAST_SUCCESS"
		delete(r.casting, f[0])
	case event == "SPELL_CAST_SUCCESS":
		casts[player] = append(casts[player], models.TrackCast{ClockMS: clockMS, Spell: f[9]})
	}
}

// mobDeathAt puts a mob's death on the map where it was last seen.
func (k *LogTracker) mobDeathAt(at int64, guid string) {
	run := k.current
	if p, ok := run.mobAt[guid]; ok {
		p.ClockMS = k.clockAt(at)
		run.track.Deaths = append(run.track.Deaths, p)
		delete(run.mobAt, guid)
	}
}

// trackOf is a copy of a run's track with each player's class, safe to hand out while the run goes on.
func (k *LogTracker) trackOf(run *keyRun) models.KeyTrack {
	t := models.KeyTrack{Players: map[string][]models.TrackPoint{}, Classes: k.classes(run),
		Deaths: list(run.track.Deaths), Meters: map[string][]models.MeterPoint{}, Pulls: list(run.track.Pulls)}
	for name, points := range run.track.Players {
		t.Players[name] = list(points)
	}
	for name, points := range run.track.Meters {
		t.Meters[name] = list(points)
	}
	if len(run.track.Bosses) > 0 {
		t.Bosses = map[string][]models.TrackPoint{}
		for guid, points := range run.track.Bosses {
			t.Bosses[guid] = list(points)
		}
	}
	if r := run.raid; r != nil {
		t.Health = slices.Sorted(maps.Keys(r.bossUnits()))
		var adds []string
		for guid, u := range r.units {
			if slices.Contains(r.boss.Priority, u.name) && !slices.Contains(t.Health, guid) {
				adds = append(adds, guid)
			}
		}
		slices.Sort(adds)
		t.Health = append(t.Health, adds...)
		t.AbilityTimes = list(run.track.AbilityTimes)
		t.Casts, t.Abilities = map[string][]models.TrackCast{}, map[string][]models.AbilitySeries{}
		for name, casts := range run.track.Casts {
			t.Casts[name] = list(casts)
		}
		for name, series := range run.track.Abilities {
			for _, s := range series {
				t.Abilities[name] = append(t.Abilities[name], models.AbilitySeries{Ability: s.Ability,
					Points: slices.Clone(s.Points)})
			}
			slices.SortFunc(t.Abilities[name], func(a, b models.AbilitySeries) int { return cmp.Compare(a.Ability, b.Ability) })
		}
	}
	return t
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

// DungeonMap is a dungeon's floors with Keystone.guru tiles (from the dungeon file) and where parties have walked in
// its saved runs, one point per walkedGrid square (Name and ClockMS empty): the shape drawn for floors without tiles.
func (k *LogTracker) DungeonMap(mapID int) (models.DungeonMap, error) {
	walked, err := k.walked(mapID)
	return models.DungeonMap{Floors: list(k.facts[mapID].Floors), Walked: walked}, err
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
