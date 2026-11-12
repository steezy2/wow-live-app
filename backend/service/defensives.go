package service

import (
	"maps"
	"slices"

	"wow-live/backend/models"
)

// LearnKit is what each player carries and each item's cooldown, from their casts: the shortest gap between two
// uses.
func LearnKit(casts []models.Cast) models.Kit {
	kit := models.Kit{Carried: map[string]map[string]bool{}, Cooldown: map[[2]string]int64{}}
	times := map[[2]string][]int64{}
	for _, c := range casts {
		if kit.Carried[c.Player] == nil {
			kit.Carried[c.Player] = map[string]bool{}
		}
		kit.Carried[c.Player][c.Ability] = true
		key := [2]string{c.Player, c.Ability}
		times[key] = append(times[key], c.At)
	}
	for key, at := range times {
		slices.Sort(at)
		for i := 1; i < len(at); i++ {
			gap := at[i] - at[i-1]
			if seen, ok := kit.Cooldown[key]; !ok || gap < seen {
				kit.Cooldown[key] = gap
			}
		}
	}
	return kit
}

// addKit adds what another kit knows to kit: what each player carries, and each item's shortest cooldown.
func addKit(kit, other models.Kit) {
	for player, items := range other.Carried {
		if kit.Carried[player] == nil {
			kit.Carried[player] = map[string]bool{}
		}
		maps.Copy(kit.Carried[player], items)
	}
	for item, cooldown := range other.Cooldown {
		if seen, ok := kit.Cooldown[item]; !ok || cooldown < seen {
			kit.Cooldown[item] = cooldown
		}
	}
}

// castToBuffMS is how long after a player's cast its buff can land and still be theirs.
const castToBuffMS = 1000

// ClassifyDeaths sets each death's Defensive (the latest tracked cast in the window before it, or else a defensive
// buff up at the death, Used when the player cast it) or else its Ready list (what the player carries and could still have used). casts cover the whole run, so an item used in one fight can still be on
// cooldown in the next.
func ClassifyDeaths(deaths []models.Death, casts []models.Cast, buffs []models.BuffUp, kit models.Kit, windowMS int64) {
	for i := range deaths {
		d := &deaths[i]
		var mine []models.Cast
		var recent *models.Cast
		for _, c := range casts {
			if c.Player != d.Player || c.At > d.At {
				continue
			}
			mine = append(mine, c)
			if c.Fight == d.Fight && d.At-windowMS <= c.At && (recent == nil || c.At > recent.At) {
				recent = &c
			}
		}
		d.Defensive, d.Used, d.Ready = "", false, nil
		if recent != nil {
			d.Defensive, d.Used = recent.Ability, true
			continue
		}
		for _, b := range buffs {
			if b.Fight == d.Fight && b.Player == d.Player && b.Start <= d.At && d.At <= b.End {
				d.Defensive = b.Ability
				d.Used = slices.ContainsFunc(mine, func(c models.Cast) bool {
					return c.Ability == b.Ability && c.At <= b.Start && b.Start-c.At <= castToBuffMS
				})
			}
		}
		if d.Defensive != "" {
			continue
		}
		for _, ability := range slices.Sorted(maps.Keys(kit.Carried[d.Player])) {
			if stillReady(ability, *d, mine, kit) {
				d.Ready = append(d.Ready, ability)
			}
		}
	}
}

// stillReady is whether the player could still use ability when they died: ready once its learned cooldown has
// passed since the last use, or, with no cooldown learned (never seen used twice), used up for the rest of the
// fight it was used in.
func stillReady(ability string, d models.Death, used []models.Cast, kit models.Kit) bool {
	var last int64 = -1
	sameFight := false
	for _, c := range used {
		if c.Ability == ability {
			last = max(last, c.At)
			sameFight = sameFight || c.Fight == d.Fight
		}
	}
	if last < 0 {
		return true
	}
	cooldown, known := kit.Cooldown[[2]string{d.Player, ability}]
	if !known {
		return !sameFight
	}
	return d.At-last >= cooldown
}
