package service

import (
	"cmp"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"wow-live/backend/models"
)

// The heal model is QuestionablyEpic's way of valuing a healer's gear (docs/sim.md, Healers) with its hand-kept spell
// lists and trinket formulas replaced by the spec's top logs: each spell's healing at the logger's stats is brought
// back to a base (healFactor), scored again at the character's stats, and each trinket's or embellishment's own
// healing and stat buff is measured in the logs of players who wore it.

// healSpell is one spell of a healProfile: its base healing per second (at 1 intellect and no secondary stats), the
// median over the top logs that cast it times the share of logs that did.
type healSpell struct {
	Name string
	ID   int
	Base float64
}

// healProfile is a healer spec's healing as its top logs did it.
type healProfile struct {
	Spec   healSpec
	Spells []healSpell
	Logs   int
}

// healFactor is how much a sheet multiplies a spell's base: intellect, crit (a crit heals double), versatility,
// mastery at the spec's rate and haste on the share of healing that grows with it.
func healFactor(spec healSpec, s healSheet) float64 {
	return s.Int * (1 + s.Crit/100) * (1 + s.Vers/100) * (1 + s.Mastery*spec.mastery) * (1 + s.Haste/100*spec.haste)
}

// buildHealProfile is the profile of tops (a log whose main stat isn't known is left out). A spell is left out when it
// healed in fewer than half the logs (a talent some took) or excluded says it is a trinket's or an embellishment's,
// which healEffects values.
func buildHealProfile(spec healSpec, tops []models.RankedPlayer, excluded func(models.RankedAbility) bool) healProfile {
	type seen struct {
		name  string
		id    int
		bases []float64
	}
	spells := map[string]*seen{}
	logs := 0
	for _, top := range tops {
		if top.Stats == nil || top.Stats.Main == 0 || top.DurationMS <= 0 {
			continue
		}
		logs++
		f := healFactor(spec, sheetOf(*top.Stats))
		for _, a := range top.Abilities {
			if a.Healing <= 0 || excluded(a) {
				continue
			}
			key := a.Name
			if a.ID != 0 {
				key = strconv.Itoa(a.ID)
			}
			if spells[key] == nil {
				spells[key] = &seen{name: a.Name, id: a.ID}
			}
			spells[key].bases = append(spells[key].bases, float64(a.Healing)*1000/float64(top.DurationMS)/f)
		}
	}
	p := healProfile{Spec: spec, Logs: logs}
	for _, key := range slices.Sorted(maps.Keys(spells)) {
		s := spells[key]
		if 2*len(s.bases) < logs {
			continue
		}
		p.Spells = append(p.Spells, healSpell{Name: s.name, ID: s.id,
			Base: median(s.bases) * float64(len(s.bases)) / float64(logs)})
	}
	return p
}

// healHPS is the healing per second of gear worth stats on the profile, with the effects' own healing and their stat
// buffs on average, and each spell's part of it, biggest first (as SimResult.Abilities, its DPS the HPS).
func healHPS(p healProfile, stats models.RankedStats, effects []healEffect) (float64, []models.SimAbility) {
	for _, e := range effects {
		stats.Add(e.Stats)
	}
	f := healFactor(p.Spec, sheetOf(stats))
	var total float64
	var parts []models.SimAbility
	for _, sp := range p.Spells {
		parts = append(parts, models.SimAbility{Name: sp.Name, ID: sp.ID, DPS: sp.Base * f})
		total += sp.Base * f
	}
	for _, e := range effects {
		if e.HPS > 0 {
			parts = append(parts, models.SimAbility{Name: e.Name, ID: e.ID, DPS: e.HPS})
			total += e.HPS
		}
	}
	for i := range parts {
		parts[i].Share = parts[i].DPS / max(total, 1)
	}
	slices.SortStableFunc(parts, func(a, b models.SimAbility) int { return cmp.Compare(b.DPS, a.DPS) })
	return total, parts
}

// healWeights are each stat's healing per second per point of rating at stats (from one percent's worth more), as
// simc's scale factors are read into weights.
func healWeights(p healProfile, stats models.RankedStats, effects []healEffect) []models.SimStatWeight {
	base, _ := healHPS(p, stats, effects)
	factors := map[string]float64{}
	for stat, n := range map[string]int{"Int": 100, "Crit": critPerPoint, "Haste": hastePerPoint,
		"Mastery": masteryPerPoint, "Vers": versPerPoint} {
		more := stats
		if r := more.Rating(stat); r != nil {
			*r += n
		} else {
			more.Main += n
		}
		hps, _ := healHPS(p, more, effects)
		factors[stat] = (hps - base) / float64(n)
	}
	return statWeights(factors)
}

// healEffect is what a worn trinket's or embellishment's effect is worth as the top logs show it (ID is its first
// spell, for the breakdown's icon): its own healing per second and the stats its buff gives on average (amount times
// uptime), or a Note when it couldn't be measured.
type healEffect struct {
	Slot, Name string
	ID         int
	HPS        float64
	Stats      models.RankedStats
	Note       string
}

// wornEffect is a worn item with an effect: the export's item and what its tooltip says at its level.
type wornEffect struct {
	item  models.SimItem
	stats models.ItemStats
}

// matches says whether a logged ability or buff (its spell ID and name) is the effect's: one of its spells, or named
// as one of them or the item.
func (w wornEffect) matches(id int, name string) bool {
	if id != 0 && slices.Contains(w.stats.Spells, id) {
		return true
	}
	slug := slugOf(name)
	return slug != "" && (slices.Contains(w.stats.Slugs, slug) || slug == slugOf(w.item.Name))
}

// nonSlug is what Wowhead's slugs leave out of a name.
var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugOf is a name as Wowhead writes it in a link: "Gebbo's Bottomless Bag" is "gebbos-bottomless-bag".
func slugOf(name string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ReplaceAll(strings.ToLower(name), "'", ""), "-"), "-")
}

// healEffects measures each worn effect in the top logs that wore the same item (by ID): the median healing per second
// of its spells, scaled by its level (scale gives how much stronger the character's effect is than at a top player's
// level), and its stat buff's median uptime times the amount the character's tooltip states. An effect no top log
// wore, or whose buff states no amount, keeps only its stats, with a note.
func healEffects(worn []wornEffect, tops []models.RankedPlayer, scale func(wornEffect, int) float64) []healEffect {
	var out []healEffect
	for _, w := range worn {
		e := healEffect{Slot: w.item.Slot, Name: w.item.Name, ID: w.stats.Spells[0]}
		var hps, uptimes []float64
		for _, top := range tops {
			at := slices.IndexFunc(top.Gear, func(g models.RankedGear) bool { return g.ID == w.item.ID })
			if at < 0 || top.DurationMS <= 0 {
				continue
			}
			var healed int64
			for _, a := range top.Abilities {
				if w.matches(a.ID, a.Name) {
					healed += a.Healing
				}
			}
			hps = append(hps, float64(healed)*1000/float64(top.DurationMS)*scale(w, top.Gear[at].Level))
			var up int64
			for _, b := range top.Buffs {
				if w.matches(b.ID, b.Name) {
					up = max(up, b.UptimeMS)
				}
			}
			uptimes = append(uptimes, float64(up)/float64(top.DurationMS))
		}
		if len(hps) == 0 {
			e.Note = "no top log wore it: counted by its stats only"
			out = append(out, e)
			continue
		}
		e.HPS = median(hps)
		if up := median(uptimes); up > 0 {
			if w.stats.Buff != (models.RankedStats{}) {
				e.Stats = scaled(w.stats.Buff, up)
			} else {
				e.Note = "its buff states no amount: its buff isn't counted"
			}
		}
		out = append(out, e)
	}
	return out
}

// scaled is s times f, rounded.
func scaled(s models.RankedStats, f float64) models.RankedStats {
	r := func(n int) int { return int(float64(n)*f + 0.5) }
	return models.RankedStats{Main: r(s.Main), Crit: r(s.Crit), Haste: r(s.Haste), Mastery: r(s.Mastery),
		Vers: r(s.Vers)}
}
