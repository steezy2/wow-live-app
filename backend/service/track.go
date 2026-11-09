package service

import (
	"errors"
	"io/fs"
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
func (k *KeyTracker) position(at int64, event string, f []string) {
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
	}
}

// meterEveryMS: how often the replay's meters are sampled.
const meterEveryMS = 2_000

// sampleMeters keeps each player's damage and healing so far about every meterEveryMS, so the replay can show the
// meters at any moment.
func (k *KeyTracker) sampleMeters(at int64) {
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

// mobDeathAt puts a mob's death on the map where it was last seen.
func (k *KeyTracker) mobDeathAt(at int64, guid string) {
	run := k.current
	if p, ok := run.mobAt[guid]; ok {
		p.ClockMS = k.clockAt(at)
		run.track.Deaths = append(run.track.Deaths, p)
		delete(run.mobAt, guid)
	}
}

// trackOf is a copy of a run's track with each player's class, safe to hand out while the run goes on.
func (k *KeyTracker) trackOf(run *keyRun) models.KeyTrack {
	t := models.KeyTrack{Players: map[string][]models.TrackPoint{}, Classes: k.classes(run),
		Deaths: list(run.track.Deaths), Meters: map[string][]models.MeterPoint{}, Pulls: list(run.track.Pulls)}
	for name, points := range run.track.Players {
		t.Players[name] = list(points)
	}
	for name, points := range run.track.Meters {
		t.Meters[name] = list(points)
	}
	return t
}

// Track is a key's track by its start time: the running key's, the last one's, or a saved one's.
func (k *KeyTracker) Track(start int64) (models.KeyTrack, error) {
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
func (k *KeyTracker) DungeonMap(mapID int) (models.DungeonMap, error) {
	walked, err := k.walked(mapID)
	return models.DungeonMap{Floors: list(k.facts[mapID].Floors), Walked: walked}, err
}

func (k *KeyTracker) walked(mapID int) ([]models.TrackPoint, error) {
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
