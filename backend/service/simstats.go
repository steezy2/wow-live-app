package service

import (
	"fmt"
	"strings"

	"wow-live/backend/models"
)

// secondaryStats are the secondary stats a stat weights sim reports, by simc's name for each, and pawnNames how Pawn
// spells each stat (the main stat, Intellect, Strength or Agility, goes first).
var (
	secondaryStats = []string{"Crit", "Haste", "Mastery", "Vers"}
	pawnNames      = map[string]string{"Int": "Intellect", "Str": "Strength", "Agi": "Agility", "Crit": "CritRating",
		"Haste": "HasteRating", "Mastery": "MasteryRating", "Vers": "Versatility"}
)

// statWeights are the main stat's scale factor and each secondary stat's (simc's DPS per point of rating), each
// weighted against the main stat's; none when simc gave no main stat or it adds no DPS.
func statWeights(factors map[string]float64) []models.SimStatWeight {
	main := ""
	for _, stat := range []string{"Int", "Str", "Agi"} {
		if factors[stat] > factors[main] {
			main = stat
		}
	}
	if main == "" {
		return nil
	}
	out := []models.SimStatWeight{}
	for _, stat := range append([]string{main}, secondaryStats...) {
		if value, ok := factors[stat]; ok {
			out = append(out, models.SimStatWeight{Stat: stat, Weight: value / factors[main], DPS: value})
		}
	}
	return out
}

// pawnString carries weights into the Pawn addon: ( Pawn: v1: "name": Intellect=1, CritRating=0.52, ... ).
func pawnString(name string, weights []models.SimStatWeight) string {
	parts := make([]string, len(weights))
	for i, w := range weights {
		parts[i] = fmt.Sprintf("%s=%.2f", pawnNames[w.Stat], w.Weight)
	}
	return fmt.Sprintf("( Pawn: v1: %q: %s )", name, strings.Join(parts, ", "))
}

// scaleLine is the simc line that makes a "stats" sim work out its scale factors, and nothing for any other kind.
func scaleLine(kind string) string {
	if kind == "stats" {
		return "calculate_scale_factors=1\n"
	}
	return ""
}

// errStatsOneSetup is why a stat weights sim refuses a comparison.
var errStatsOneSetup = fmt.Errorf("%w: stat weights are for one setup: pick one value of each consumable and rune",
	ErrBadSimOptions)
