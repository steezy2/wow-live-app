package service

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"wow-live/backend/models"
)

// The heal model's stat rules, QuestionablyEpic's for level 90 (STAT.ts, ProfileUtilities.convertStatPercentages):
// ratings per 1% (simc agrees: 165 haste rating is 3.75%), base crit and mastery, and the diminishing returns that
// take 10% of a stat's rating above each of drPercents' worth of it.
const (
	critPerPoint    = 46
	hastePerPoint   = 44
	masteryPerPoint = 46
	versPerPoint    = 54
	baseCrit        = 5 // percent
	baseMastery     = 8 // mastery points, before the spec's multiplier
	drStep          = 0.1
	// baseIntellect is a level-90 caster's intellect with nothing on (simc's, a naked Restoration Druid), added to a
	// character's gear; a top log's combatant info already has it.
	baseIntellect = 715
)

// drPercents are the stat percents above which each further 10% of the rating is lost.
var drPercents = []float64{30, 40, 50, 60}

// healSpec is what the heal model needs of a healer spec beyond its logs: how much one mastery point (rating included,
// base 8) adds to its healing on average, and the share of its healing that grows with haste (the rest is mana-bound).
// Both are hand-set from QuestionablyEpic's mastery multipliers and not yet checked against logs (docs/tech-debt.md).
type healSpec struct{ mastery, haste float64 }

// healSpecs are the seven healer specs by spec ID.
var healSpecs = map[int]healSpec{
	105:  {mastery: 0.013, haste: 0.6}, // Restoration Druid
	1468: {mastery: 0.018, haste: 0.5}, // Preservation Evoker
	270:  {mastery: 0.012, haste: 0.6}, // Mistweaver Monk
	65:   {mastery: 0.012, haste: 0.4}, // Holy Paladin
	256:  {mastery: 0.010, haste: 0.6}, // Discipline Priest
	257:  {mastery: 0.008, haste: 0.6}, // Holy Priest
	264:  {mastery: 0.014, haste: 0.5}, // Restoration Shaman
}

// healSheet is a healer's stats as the model uses them: intellect, and crit, haste and versatility in percent and
// mastery in points, after diminishing returns and with the base crit and mastery.
type healSheet struct{ Int, Crit, Haste, Mastery, Vers float64 }

// sheetOf turns ratings into a healSheet.
func sheetOf(s models.RankedStats) healSheet {
	return healSheet{Int: float64(s.Main), Crit: baseCrit + diminished(s.Crit, critPerPoint),
		Haste: diminished(s.Haste, hastePerPoint), Mastery: baseMastery + diminished(s.Mastery, masteryPerPoint),
		Vers: diminished(s.Vers, versPerPoint)}
}

// diminished is a rating in percent after diminishing returns.
func diminished(rating, perPoint int) float64 {
	r := float64(rating)
	for _, p := range drPercents {
		r -= max(0, float64(rating)-p*float64(perPoint)) * drStep
	}
	return r / float64(perPoint)
}

// HealItems gives what an item gives at its level and bonus IDs (clients/wowhead).
type HealItems interface {
	ItemStats(ctx context.Context, id, level int, bonus []int) (models.ItemStats, error)
}

// gearStats are the ratings and intellect of the gear worn, with its gems and enchants (their stats from ref) and the
// base intellect, and each item's own stats in gear's order (a trinket's effects among them).
func gearStats(ctx context.Context, src HealItems, ref models.SimReference, gear []*models.SimItem) (
	models.RankedStats, []models.ItemStats, error) {
	total := models.RankedStats{Main: baseIntellect}
	items := make([]models.ItemStats, len(gear))
	for i, it := range gear {
		s, err := src.ItemStats(ctx, it.ID, it.Level, it.Opts["bonus_id"])
		if err != nil {
			return models.RankedStats{}, nil, fmt.Errorf("reading the stats of %s (%d): %w", it.Name, it.ID, err)
		}
		for _, id := range it.Opts["gem_id"] {
			if g := slices.IndexFunc(ref.Gems, func(g models.SimGem) bool { return g.ID == id }); g >= 0 {
				s.Stats.Add(refStats(ref.Gems[g].Stats))
			}
		}
		for _, id := range it.Opts["enchant_id"] {
			if e := slices.IndexFunc(ref.Enchants, func(e models.SimEnchant) bool { return e.ID == id }); e >= 0 {
				s.Stats.Add(refStats(ref.Enchants[e].Stats))
			}
		}
		items[i] = s
		total.Add(s.Stats)
	}
	return total, items, nil
}

// refStats reads a gem's or enchant's stats, "41 Int & 70 Crit" (cmd/simoptref's statsText): intellect, a main stat
// and the four secondaries count.
func refStats(text string) models.RankedStats {
	var s models.RankedStats
	for part := range strings.SplitSeq(text, " & ") {
		amount, stat, _ := strings.Cut(part, " ")
		n, _ := strconv.Atoi(amount)
		if stat == "Int" || stat == "Main stat" {
			s.Main += n
		} else if r := s.Rating(stat); r != nil {
			*r += n
		}
	}
	return s
}
