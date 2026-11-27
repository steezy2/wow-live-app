package service

import (
	"context"
	"fmt"
	"slices"

	"wow-live/backend/models"
)

var (
	// errHealerKind is why a healer's Suggest sim, custom simc lines or compared consumables are refused.
	errHealerKind = fmt.Errorf("%w: simc doesn't heal, so a healer is scored on the spec's top logs: Quick Sim, Top "+
		"Gear, stat weights and the Droptimizer work, Suggest, custom simc lines and compared consumables don't",
		ErrBadSimOptions)
	// errHealerLogs is why a healer's sim is refused without Warcraft Logs, and errHealerTarget without a target.
	errHealerLogs = fmt.Errorf("%w: a healer is scored on the top Warcraft Logs players of the spec (simc doesn't "+
		"heal): set Warcraft Logs up in Settings", ErrBadSimOptions)
	errHealerTarget = fmt.Errorf("%w: pick the top logs a healer is scored on: Mythic+ overall or a raid boss",
		ErrBadSimOptions)
)

// healPlanSeconds is about how long a healer's sim takes: reading the top logs the first time (later reads are kept)
// and the gear's tooltips; scoring is instant.
const healPlanSeconds = 30

// healModelNote is what every healer's result says it assumes.
const healModelNote = "HPS from the top logs' spells at your stats (simc doesn't heal); trinkets and embellishments " +
	"measured in the top logs that wore them"

// healerSpec is the heal model's numbers for the export's spec, false for a spec that isn't a healer.
func (s *Sims) healerSpec(p models.SimProfile) (healSpec, bool) {
	spec, ok := healSpecs[specIDOf(s.Drops, p)]
	return spec, ok
}

// healerRefusal is why a healer's request can't run on the heal model, nil when it can.
func (s *Sims) healerRefusal(req models.SimRequest, custom []string, dims []compareDim) error {
	switch {
	case req.Kind == "suggest" || len(custom) > 0 || len(dims) > 0:
		return errHealerKind
	case !s.logsReady():
		return errHealerLogs
	case !s.validTarget(req.Target):
		return errHealerTarget
	}
	return nil
}

// memoItems asks src for each item once per run (Top Gear asks for the same items in every combination).
type memoItems struct {
	src  HealItems
	seen map[string]models.ItemStats
}

// ItemStats is src's answer, kept.
func (m memoItems) ItemStats(ctx context.Context, id, level int, bonus []int) (models.ItemStats, error) {
	key := fmt.Sprint(id, level, bonus)
	if s, ok := m.seen[key]; ok {
		return s, nil
	}
	s, err := m.src.ItemStats(ctx, id, level, bonus)
	if err == nil {
		m.seen[key] = s
	}
	return s, err
}

// heal runs the running healer's sim with this ID on the heal model and saves it as any sim's result.
func (s *Sims) heal(ctx context.Context, id string, spec healSpec, p models.SimProfile, kind string,
	combos []gearCombo, target models.SuggestTarget) {
	result, err := s.healResult(ctx, id, spec, p, kind, combos, target)
	s.finish(ctx, id, result, "", err, p.Equipped)
}

// healResult scores the equipped gear and each combination on the spec's top logs in target: a "stats" sim also
// gets its stat weights.
func (s *Sims) healResult(ctx context.Context, id string, spec healSpec, p models.SimProfile, kind string,
	combos []gearCombo, target models.SuggestTarget) (*models.SimResult, error) {
	s.setStage(id, "Reading the top logs")
	b, err := s.topBuilds(ctx, p, target)
	if err != nil {
		return nil, err
	}
	s.setStage(id, "Scoring the gear")
	items := memoItems{src: s.Items, seen: map[string]models.ItemStats{}}
	tops, err := topEffects(ctx, items, b.Tops)
	if err != nil {
		return nil, err
	}
	equipped := wornAfter(p, nil)
	_, own, err := gearStats(ctx, items, s.Ref, equipped)
	if err != nil {
		return nil, err
	}
	theirs := append(tops, withEffects(equipped, own)...) // the character's own embellishments are healEffects' too
	profile := buildHealProfile(spec, b.Tops, func(a models.RankedAbility) bool {
		return slices.ContainsFunc(theirs, func(w wornEffect) bool { return w.matches(a.ID, a.Name) })
	})
	if len(profile.Spells) == 0 {
		return nil, fmt.Errorf("%w: none of the %d top logs read had the healer's stats", ErrNoTopLogs, len(b.Tops))
	}
	score := func(swaps []models.SimSwap) (float64, []models.SimAbility, []healEffect, models.RankedStats, error) {
		worn := wornAfter(p, swaps)
		stats, each, err := gearStats(ctx, items, s.Ref, worn)
		if err != nil {
			return 0, nil, nil, stats, err
		}
		effects := healEffects(withEffects(worn, each), b.Tops, func(id, level int) float64 {
			return effectScale(ctx, items, id, level, worn, each)
		})
		hps, abilities := healHPS(profile, stats, effects)
		return hps, abilities, effects, stats, nil
	}
	hps, abilities, effects, stats, err := score(nil)
	if err != nil {
		return nil, err
	}
	r := &models.SimResult{Metric: "hps", DPS: hps, Abilities: abilities, Buffs: []models.SimBuff{},
		Notes: append([]string{healModelNote, b.Baseline}, b.Notes...)}
	for _, e := range effects {
		if e.Note != "" {
			r.Notes = append(r.Notes, e.Name+": "+e.Note)
		}
	}
	if kind == "stats" {
		r.Weights = healWeights(profile, stats, effects)
	}
	var sets []simSetJSON
	for _, c := range combos {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		combo, _, _, _, err := score(c.swaps)
		if err != nil {
			return nil, err
		}
		sets = append(sets, simSetJSON{Name: c.name, Mean: combo})
	}
	if len(combos) > 0 {
		r.Combos = gearResults(sets, hps, combos)
	}
	return r, nil
}

// withEffects are the worn items that have a Use: or Equip: effect, with their stats.
func withEffects(worn []*models.SimItem, each []models.ItemStats) []wornEffect {
	var out []wornEffect
	for i, it := range worn {
		if len(each[i].Spells) > 0 {
			out = append(out, wornEffect{item: *it, stats: each[i]})
		}
	}
	return out
}

// topEffects are the trinkets the top players wore, at their levels (their spells leave the spec's profile: a
// trinket's healing is its own, valued by healEffects).
func topEffects(ctx context.Context, items HealItems, tops []models.RankedPlayer) ([]wornEffect, error) {
	var out []wornEffect
	seen := map[[2]int]bool{}
	for _, top := range tops {
		for _, g := range top.Gear {
			if (g.Slot != "trinket1" && g.Slot != "trinket2") || seen[[2]int{g.ID, g.Level}] {
				continue
			}
			seen[[2]int{g.ID, g.Level}] = true
			s, err := items.ItemStats(ctx, g.ID, g.Level, nil)
			if err != nil {
				return nil, fmt.Errorf("reading a top log's trinket %d: %w", g.ID, err)
			}
			out = append(out, wornEffect{item: models.SimItem{Slot: g.Slot, ID: g.ID, Level: g.Level}, stats: s})
		}
	}
	return out, nil
}

// effectScale is how much stronger the worn item id's effect is at the character's level than at level (a top
// player's): the ratio of the first numbers their tooltips state, 1 when either states none or can't be read.
func effectScale(ctx context.Context, items HealItems, id, level int, worn []*models.SimItem,
	each []models.ItemStats) float64 {
	at := slices.IndexFunc(worn, func(it *models.SimItem) bool { return it.ID == id })
	theirs, err := items.ItemStats(ctx, id, level, nil)
	mine, their := firstNumber(each[at].Effect), firstNumber(theirs.Effect)
	if err != nil || mine == 0 || their == 0 {
		return 1
	}
	return mine / their
}
