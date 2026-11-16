package service

import (
	"cmp"
	"maps"
	"math"
	"slices"

	"wow-live/backend/models"
)

// Key level scaling as Keystone.guru has it (Npc::getScalingFactor, config/keystoneguru.php), which matched every
// trash and boss health in the owner's +17 Temple of Sethraliss log to the point (Nov 12, 2026): each level through
// +10 multiplies by keyScalePerLevel and each level from +11 by keyScalePerLevelPast10 (Xal'atath's Guile), the
// product rounded to two decimals; then from +10 Fortified multiplies trash and Tyrannical bosses (between +7 and
// +9 one of them does, by week, which the app doesn't know, so neither is applied there), and up to +5
// lowKeyTrashHealth multiplies trash health (Lindormi's Guidance).
const (
	keyScalePerLevel       = 1.07
	keyScalePerLevelPast10 = 1.10
	fortifiedHealth        = 1.2
	tyrannicalHealth       = 1.25
	bothAffixesFrom        = 10
	lowKeyTrashHealth      = 0.95
	lowKeyTrashTo          = 5
	// defaultMobLevel is the mobs' key level in a dungeon without a saved key.
	defaultMobLevel = 10
	// MinMobLevel and MaxMobLevel are the key levels Mobs scales to.
	MinMobLevel, MaxMobLevel = 2, 40
)

// keyMult is the health and damage multiplier of a key level over +1, before affixes.
func keyMult(level int) float64 {
	mult := 1.0
	for i := 1; i < level; i++ {
		if i < 10 {
			mult *= keyScalePerLevel
		} else {
			mult *= keyScalePerLevelPast10
		}
	}
	return math.Round(mult*100) / 100
}

// scaleMob is the mob at a key level: its base health times the level's multiplier and the affixes it takes there.
func scaleMob(m models.DungeonMob, level int) models.DungeonMob {
	mult := keyMult(level)
	switch {
	case level >= bothAffixesFrom && m.Boss:
		mult *= tyrannicalHealth
	case level >= bothAffixesFrom:
		mult *= fortifiedHealth
	case !m.Boss && level <= lowKeyTrashTo:
		mult *= lowKeyTrashHealth
	}
	m.Health = int(math.Round(float64(m.Health) * mult))
	return m
}

// MobCasts is whether a mob in the reference file casts the spell.
func (k *LogTracker) MobCasts(id int) bool {
	for _, f := range k.facts {
		for _, m := range f.Mobs {
			if slices.ContainsFunc(m.Spells, func(s models.MobSpell) bool { return s.ID == id }) {
				return true
			}
		}
	}
	return false
}

// Mobs is the dungeon's mobs at a key level, by NPC ID; level 0 is the default: the level of the newest saved key in
// the dungeon, else defaultMobLevel. A dungeon the reference file has no mobs for has none.
func (k *LogTracker) Mobs(mapID, level int) models.DungeonMobs {
	k.mu.RLock()
	def := defaultMobLevel
	if i := slices.IndexFunc(k.history, func(key models.Key) bool { return key.MapID == mapID }); i >= 0 {
		def = k.history[i].Level
	}
	k.mu.RUnlock()
	level = cmp.Or(level, def)
	out := models.DungeonMobs{Level: level, MinLevel: MinMobLevel, MaxLevel: MaxMobLevel, DamageMult: keyMult(level), Mobs: []models.DungeonMob{}}
	mobs := k.facts[mapID].Mobs
	for _, npc := range slices.Sorted(maps.Keys(mobs)) {
		out.Mobs = append(out.Mobs, scaleMob(mobs[npc], level))
	}
	return out
}
