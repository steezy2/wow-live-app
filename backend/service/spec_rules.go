package service

import (
	"fmt"
	"math"

	"wow-live/backend/models"
)

// failedRules are the rules (a spec's, from spec-rules.toml) the player's own casts and buffs fell short of over a
// fight of durationMS; a rule with nothing to measure (no such cast) is left out.
func failedRules(rules []models.SpecRule, casts []models.TrackCast, buffs []models.BuffSpan, durationMS int64) []models.RuleResult {
	failed := []models.RuleResult{}
	for _, r := range rules {
		share, ok := ruleShare(r, casts, buffs, durationMS)
		if ok && share < r.AtLeast {
			failed = append(failed, models.RuleResult{Text: ruleText(r), Share: share, AtLeast: r.AtLeast, Why: r.Why})
		}
	}
	return failed
}

// ruleShare is the share (0 to 1) the player reached on a rule, and false when there was nothing to measure.
func ruleShare(r models.SpecRule, casts []models.TrackCast, buffs []models.BuffSpan, durationMS int64) (float64, bool) {
	if r.Kind == "uptime" {
		var up int64
		for _, b := range buffs {
			if b.Name == r.Buff {
				up += max(0, min(b.ToMS, durationMS)-max(b.FromMS, 0))
			}
		}
		return float64(up) / float64(max(1, durationMS)), durationMS > 0
	}
	var made, good int
	for _, c := range casts {
		if c.Spell != r.Cast || c.Cut {
			continue
		}
		made++
		switch r.Kind {
		case "with":
			for _, b := range buffs {
				if b.Name == r.Buff && b.FromMS <= c.ClockMS && c.ClockMS <= b.ToMS {
					good++
					break
				}
			}
		case "together":
			for _, o := range casts {
				if o.Spell == r.Other && math.Abs(float64(o.ClockMS-c.ClockMS)) <= r.Seconds*1000 {
					good++
					break
				}
			}
		}
	}
	return float64(good) / float64(max(1, made)), made > 0
}

// ruleText says what a rule checks, as the review words it.
func ruleText(r models.SpecRule) string {
	switch r.Kind {
	case "with":
		return fmt.Sprintf("%s while %s was up", r.Cast, r.Buff)
	case "together":
		return fmt.Sprintf("%s within %g s of %s", r.Cast, r.Seconds, r.Other)
	default:
		return fmt.Sprintf("%s up", r.Buff)
	}
}
