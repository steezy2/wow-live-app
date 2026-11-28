package service

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"wow-live/backend/models"
)

func TestHealProfileFromRankedPlayers(t *testing.T) {
	spec := healSpec{mastery: 0.01, haste: 0.5}
	stats := &models.RankedStats{Main: 1000}
	log := func(x, y int64) models.RankedPlayer {
		return models.RankedPlayer{DurationMS: 60000, Stats: stats, Abilities: []models.RankedAbility{
			{ID: 1, Name: "Mend", Healing: x}, {ID: 2, Name: "Rare Talent", Healing: y},
			{ID: 3, Name: "Trinket Heal", Healing: 999999}}}
	}
	tops := []models.RankedPlayer{log(600000, 0), log(600000, 0), log(900000, 60000), {DurationMS: 60000}}
	p := buildHealProfile(spec, tops, func(a models.RankedAbility) bool { return a.ID == 3 })
	// the stat-less log is left out, Rare Talent healed in 1 of 3, the trinket is healEffects'
	if p.Logs != 3 || len(p.Spells) != 1 || p.Spells[0].Name != "Mend" {
		t.Fatalf("profile = %+v, want Mend alone from 3 logs", p)
	}
	// at the logs' own stats the median log's 10000 HPS comes back
	if hps, parts := healHPS(p, *stats, nil); math.Abs(hps-10000) > 1e-6 || parts[0].Share != 1 {
		t.Errorf("score = %v (%+v), want 10000 all from Mend", hps, parts)
	}
	more := *stats
	more.Main = 2000
	if hps, _ := healHPS(p, more, nil); math.Abs(hps-20000) > 1e-6 {
		t.Errorf("score at twice the intellect = %v, want 20000", hps)
	}
}

func TestHealEffectsFromLogs(t *testing.T) {
	heal := wornEffect{item: models.SimItem{Slot: "trinket1", ID: 10, Name: "Healing Charm"},
		stats: models.ItemStats{Spells: []int{111}, Amount: 2000}}
	burst := wornEffect{item: models.SimItem{Slot: "trinket2", ID: 20, Name: "Burst Idol"},
		stats: models.ItemStats{Spells: []int{222}, Slugs: []string{"haste-burst"},
			Buff: models.RankedStats{Haste: 1000}}}
	unseen := wornEffect{item: models.SimItem{Slot: "finger1", ID: 30, Name: "Odd Ring"},
		stats: models.ItemStats{Spells: []int{333}}}
	top := models.RankedPlayer{DurationMS: 60000,
		Gear:      []models.RankedGear{{Slot: "trinket1", ID: 10, Level: 300}, {Slot: "trinket2", ID: 20, Level: 300}},
		Abilities: []models.RankedAbility{{ID: 111, Name: "Charm Heal", Healing: 120000}},
		Buffs:     []models.RankedBuff{{ID: 999, Name: "Haste Burst", UptimeMS: 15000}}}
	got := healEffects([]wornEffect{heal, burst, unseen}, []models.RankedPlayer{top, top},
		func(wornEffect, int) float64 { return 1.5 })
	// 2000 HPS in the logs, half again at the character's level; the burst's 1000 haste up a quarter of the fight
	if got[0].HPS != 3000 || got[1].Stats.Haste != 250 || got[1].HPS != 0 || got[2].Note == "" {
		t.Errorf("effects = %+v; want 3000 HPS, 250 haste on average and a note for the ring", got)
	}
}

// healerSims are suggestSims made a Restoration Shaman's (spec 264), whose ten top logs heal 10000 per second with one
// spell at the test items' stats.
func healerSims(t *testing.T) (*Sims, *fakeTops, models.SimRequest) {
	t.Helper()
	s, logs := suggestSims(t)
	s.Drops = models.SimDrops{Specs: map[int]models.SimSpec{264: {Class: "shaman", Spec: "restoration"}}}
	for i := range logs.builds.Tops {
		logs.builds.Tops[i].DurationMS = 60000
		logs.builds.Tops[i].Stats.Main = 3000
		logs.builds.Tops[i].Abilities = []models.RankedAbility{{ID: 1, Name: "Healing Wave", Healing: 600000}}
	}
	req := simRequest(t)
	req.Export = strings.Replace(req.Export, "spec=elemental", "spec=restoration", 1)
	req.Kind, req.Target = "stats", &models.SuggestTarget{Kind: models.FightMPlus}
	return s, logs, req
}

func TestHealerSimRunsTheModel(t *testing.T) {
	s, _, req := healerSims(t)
	plan, err := s.Plan(req)
	if err != nil || plan.Error != "" || plan.Seconds != healPlanSeconds || plan.Points != suggestPoints(req.Target) {
		t.Errorf("plan = %+v (%v), want the heal model's time and the top logs' points", plan, err)
	}
	started, err := s.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	sim := waitSim(t, s, started.ID)
	r := sim.Result
	if sim.State != models.SimDone || r.Metric != "hps" || r.DPS <= 0 || r.Abilities[0].Name != "Healing Wave" ||
		!slices.Contains(r.Notes, healModelNote) {
		t.Fatalf("sim = %+v (%+v), want the heal model's HPS", sim, r)
	}
	if len(r.Weights) != 5 || r.Weights[0].Stat != "Int" || slices.ContainsFunc(r.Weights,
		func(w models.SimStatWeight) bool { return w.Weight <= 0 }) || r.Pawn == "" {
		t.Errorf("weights = %+v, pawn %q; want intellect first and every stat worth something", r.Weights, r.Pawn)
	}
	if n := len(s.Engine.(*fakeEngine).inputs); n != 0 {
		t.Errorf("simc ran %d times, want none", n)
	}
	req.Kind = "suggest"
	if plan, _ := s.Plan(req); plan.Error != errHealerKind.Error() {
		t.Errorf("a healer's Suggest plan says %q, want errHealerKind", plan.Error)
	}
	if _, err := s.Start(req); !errors.Is(err, errHealerKind) {
		t.Errorf("a healer's Suggest = %v, want errHealerKind", err)
	}
}

// embellished are oneHanders with the bags' head (249979) embellished: spell 7 is its effect.
type embellished struct{ oneHanders }

func (embellished) ItemStats(ctx context.Context, id, level int, bonus []int) (models.ItemStats, error) {
	s, err := oneHanders{}.ItemStats(ctx, id, level, bonus)
	if id == 249979 {
		s.Spells = []int{7}
	}
	return s, err
}

func TestHealerTopGearCountsASwappedInEffectOnce(t *testing.T) {
	s, logs, req := healerSims(t)
	s.Items = embellished{}
	for i := range logs.builds.Tops {
		top := &logs.builds.Tops[i]
		top.Gear = append(top.Gear, models.RankedGear{Slot: "head", ID: 249979, Level: 300})
		top.Abilities = append(top.Abilities, models.RankedAbility{ID: 7, Name: "Embellished Mend", Healing: 60000})
	}
	req.Kind, req.Equipped, req.Bags = "topgear", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, []int{0}
	started, err := s.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	r := waitSim(t, s, started.ID).Result
	// the head's 1000 HPS (60000 over the logs' minute) is its row's gain alone, not a spell of the baseline too
	if r == nil || len(r.Combos) != 1 || math.Abs(r.Combos[0].Gain-1000) > 1e-6 ||
		slices.ContainsFunc(r.Abilities, func(a models.SimAbility) bool { return a.ID == 7 }) {
		t.Fatalf("result = %+v, want one combo 1000 HPS up and no head spell in the baseline", r)
	}
}
