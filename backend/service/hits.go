package service

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"wow-live/backend/models"
)

// Which unit the advanced-logging block names, measured on the owner's logs of Nov 12 (party members as targets):
// on SPELL_DAMAGE, SPELL_PERIODIC_DAMAGE, SPELL_HEAL and SPELL_PERIODIC_HEAL it is the target, so those lines give a
// party member's health right after the hit or heal; on SWING_DAMAGE it is the attacker, and on SPELL_CAST_SUCCESS
// the caster; SPELL_ABSORBED and the _MISSED lines have none. So a melee swing's health is unknown, and a party
// member's max HP comes from the spells that hit or heal them and their own casts.

// recapEvents is the most events a recap keeps, newest kept.
const recapEvents = 80

// tankSpecs are the tank specializations (by spec ID), whose big hits have a higher bar.
var tankSpecs = map[int]bool{250: true, 581: true, 104: true, 268: true, 66: true, 73: true}

// recapLine is a recap event at its log time: a recap's events are put on its clock counting back from its end in
// real time, since a key's clock jumps at every death.
type recapLine struct {
	at    int64
	event models.RecapEvent
}

// bigHit is a big hit running or finished: whose, the second of the log its stretch starts (as a spike's), its start
// on the run's clock and the max HP its bar came from; once finished, what it was, the player's lowest health in it
// (%, 0 when unknown) and its recap.
type bigHit struct {
	player  string
	second  int64
	clockMS int64
	maxHP   int64
	done    bool
	lowHP   float64
	hit     models.KeyBigHit
	recap   models.HitRecap
}

// hitWatch is what a run keeps for recaps and big hits, by player name.
type hitWatch struct {
	last    map[string][]recapLine                                // the player's last events, oldest first
	maxHP   map[string]int64                                      // the player's max HP, as last seen
	takenAt map[string]map[int64]map[[2]string]*models.SpikeCause // the player's own takenAt, their last seconds only
	over    map[string]bool                                       // the player's damage over the last seconds is at their bar
	all     []*bigHit                                             // the run's big hits, in order
	open    []*bigHit                                             // those still running, in order
}

func newHitWatch() hitWatch {
	return hitWatch{last: map[string][]recapLine{}, maxHP: map[string]int64{},
		takenAt: map[string]map[int64]map[[2]string]*models.SpikeCause{}, over: map[string]bool{}}
}

// add keeps an event in a player's buffer. A full buffer first drops what no recap needs (older than keepMS before
// the event, or past recapEvents), so the buffer never grows past twice recapEvents and trimming costs O(1) per event.
func (w *hitWatch) add(player string, line recapLine, keepMS int64) {
	buf := w.last[player]
	switch {
	case buf == nil:
		buf = make([]recapLine, 0, 2*recapEvents)
	case len(buf) == cap(buf):
		cut := max(0, len(buf)-recapEvents)
		for cut < len(buf) && buf[cut].at < line.at-keepMS {
			cut++
		}
		buf = buf[:copy(buf, buf[cut:])]
	}
	w.last[player] = append(buf, line)
}

// recap is a player's events in the keepMS up to end (log time), at most recapEvents of them, on the run's clock
// reading endClock at end.
func (w *hitWatch) recap(player string, end, endClock, keepMS int64) models.HitRecap {
	buf := w.last[player]
	from := len(buf)
	for from > 0 && len(buf)-from < recapEvents && buf[from-1].at >= end-keepMS {
		from--
	}
	r := models.HitRecap{Player: player, ClockMS: endClock, MaxHP: w.maxHP[player], Events: []models.RecapEvent{}}
	for _, l := range buf[from:] {
		e := l.event
		e.ClockMS = endClock - (end - l.at)
		r.Events = append(r.Events, e)
	}
	return r
}

// hpAfter is a party member's health after a line, in % (0 when the line doesn't say), from the advanced block when
// it is theirs (guid); it keeps their max HP.
func (w *hitWatch) hpAfter(event string, f []string, guid, player string) float64 {
	i := advancedAt(event)
	if i == 0 || len(f) < i+advancedFields || f[i] != guid || !strings.HasPrefix(guid, "Player-") {
		return 0
	}
	hp, maxHP := atoi(f[i+2]), atoi(f[i+3])
	if maxHP <= 0 {
		return 0
	}
	w.maxHP[player] = int64(maxHP)
	return math.Round(1000*float64(hp)/float64(maxHP)) / 10
}

// recapMS is how long a recap is, 0 without rules.
func (k *LogTracker) recapMS() int64 {
	return int64(k.rules.BigHits.RecapSeconds * 1000)
}

// recapEvent keeps an event in a party member's recap buffer.
func (k *LogTracker) recapEvent(run *keyRun, at int64, player string, e models.RecapEvent) {
	if keep := k.recapMS(); keep > 0 {
		run.hits.add(player, recapLine{at, e}, keep)
	}
}

// recapCast keeps a survival button pressed (a defensive, healthstone, health potion or raid cooldown) in the
// caster's recap, or an external put on a party member in theirs.
func (k *LogTracker) recapCast(run *keyRun, at int64, event string, f []string, caster, ability, kind string) {
	if kind != "defensive" && kind != "healthstone" && kind != "health potion" && kind != "raid cooldown" {
		return
	}
	player, guid := caster, f[0]
	if f[4] != f[0] && slices.Contains(k.rules.Defensives.Externals, f[9]) {
		if player, guid = k.player(f[4], f[6]), f[4]; player == "" || !strings.HasPrefix(guid, "Player-") {
			return
		}
	}
	k.recapEvent(run, at, player, models.RecapEvent{Kind: "cast", Ability: ability, Source: caster,
		HPPct: run.hits.hpAfter(event, f, guid, player)})
}

// tookHit keeps a hit on a party member (f[4]) in their recap, and starts a big hit when their damage taken over the
// last [big_hits] seconds (run.taken, this hit counted) first reaches their bar: the start is that stretch's first
// second. Their next big hit waits until the sum before a hit is back under the bar.
func (k *LogTracker) tookHit(run *keyRun, at int64, event string, f []string, player, ability, source string,
	amount int64) {
	if amount <= 0 {
		return
	}
	w := &run.hits
	k.recapEvent(run, at, player, models.RecapEvent{Kind: "damage", Ability: ability, Source: source, Amount: amount,
		HPPct: w.hpAfter(event, f, f[4], player)})
	n, s := int64(k.rules.BigHits.Seconds), at/1000
	if n == 0 {
		return
	}
	bySecond := w.takenAt[player]
	if bySecond == nil {
		bySecond = map[int64]map[[2]string]*models.SpikeCause{}
		w.takenAt[player] = bySecond
	}
	if bySecond[s] == nil { // a running hit's stretch is within the last n seconds
		for old := range bySecond {
			if old <= s-n {
				delete(bySecond, old)
			}
		}
	}
	tookFrom(bySecond, s, ability, source, amount)
	maxHP := w.maxHP[player]
	if maxHP == 0 {
		return
	}
	pct := k.rules.BigHits.OtherPct
	if tankSpecs[k.specs[f[4]]] {
		pct = k.rules.BigHits.TankPct
	}
	bar, sum := int64(pct/100*float64(maxHP)), int64(0)
	for i := range n {
		sum += run.taken[player][s-i]
	}
	if sum-amount < bar {
		w.over[player] = false
	}
	if sum >= bar && !w.over[player] {
		w.over[player] = true
		h := &bigHit{player: player, second: s - n + 1, clockMS: run.clock((s - n + 1) * 1000), maxHP: maxHP}
		w.all, w.open = append(w.all, h), append(w.open, h)
	}
}

// bigHitEnd is when a big hit's stretch ends, in log time.
func (k *LogTracker) bigHitEnd(h *bigHit) int64 {
	return (h.second + int64(k.rules.BigHits.Seconds)) * 1000
}

// finishBigHits finishes the running big hits whose stretch ended by at, with their recap (the recap's length up to
// the stretch's end); closing, all of them, cut at at.
func (k *LogTracker) finishBigHits(run *keyRun, at int64, closing bool) {
	w := &run.hits
	for len(w.open) > 0 {
		h, end := w.open[0], k.bigHitEnd(w.open[0])
		if end > at {
			if !closing {
				return
			}
			end = at
		}
		h.hit, h.done = k.measure(run, h), true
		h.recap = w.recap(h.player, end, run.clock(end), k.recapMS())
		h.lowHP = w.lowest(h.player, h.second*1000, end)
		w.open = w.open[1:]
	}
}

// measure is a big hit as it stands: what the player took over its stretch, the % of their max HP, and what hit
// them hardest.
func (k *LogTracker) measure(run *keyRun, h *bigHit) models.KeyBigHit {
	n, taken := int64(k.rules.BigHits.Seconds), int64(0)
	for i := range n {
		taken += run.taken[h.player][h.second+i]
	}
	return models.KeyBigHit{Player: h.player, ClockMS: h.clockMS, Taken: taken,
		Pct: math.Round(1000*float64(taken)/float64(h.maxHP)) / 10, Causes: spikeCauses(run.hits.takenAt[h.player],
			h.second, n*1000)}
}

// diedAfter says whether a big hit's player died within the recap's length of its start.
func (k *LogTracker) diedAfter(run *keyRun, h *bigHit) bool {
	from := h.second * 1000
	return slices.ContainsFunc(run.deaths, func(d models.Death) bool {
		return d.Player == h.player && d.At >= from && d.At <= from+k.recapMS()
	})
}

// bigHits is the run's kept big hits by clock (then player), running ones as they stand, each with the player's cover
// by the death rules at its end (as a spike's) and whether they died within the recap's length.
func (k *LogTracker) bigHits(run *keyRun, buffs []models.BuffUp, kit models.Kit) []models.KeyBigHit {
	var hits []models.KeyBigHit
	var cover []models.Death
	for _, h := range run.hits.all {
		if !k.kept(run, h) {
			continue
		}
		hit := h.hit
		if !h.done {
			hit = k.measure(run, h)
		}
		hit.Died = k.diedAfter(run, h)
		hits = append(hits, hit)
		cover = append(cover, models.Death{Fight: run.pullOf[h.second], At: k.bigHitEnd(h), Player: h.player})
	}
	ClassifyDeaths(cover, run.casts, buffs, kit, int64(k.rules.Defensives.WindowSeconds*1000))
	for i, c := range cover {
		hits[i].Defensive, hits[i].Used, hits[i].Ready = c.Defensive, c.Used, list(c.Ready)
	}
	slices.SortStableFunc(hits, func(a, b models.KeyBigHit) int {
		return cmp.Or(cmp.Compare(a.ClockMS, b.ClockMS), cmp.Compare(a.Player, b.Player))
	})
	return hits
}

// deathRecap keeps a party member's recap when they die at at (log time), on the clock of their KeyDeath, and
// empties their buffer so a death after a battle res starts fresh.
func (k *LogTracker) deathRecap(run *keyRun, at int64, player string, clockMS int64) {
	if keep := k.recapMS(); keep > 0 {
		run.track.Recaps = append(run.track.Recaps, run.hits.recap(player, at, clockMS, keep))
		delete(run.hits.last, player)
	}
}

// recaps is a run's recaps by clock, then player: each death's, and each finished big hit kept whose player didn't die
// within the recap's length (that hit's recap is the death's).
func (k *LogTracker) recaps(run *keyRun) []models.HitRecap {
	out := list(run.track.Recaps)
	for _, h := range run.hits.all {
		if h.done && k.kept(run, h) && !k.diedAfter(run, h) {
			out = append(out, h.recap)
		}
	}
	slices.SortStableFunc(out, func(a, b models.HitRecap) int {
		return cmp.Or(cmp.Compare(a.ClockMS, b.ClockMS), cmp.Compare(a.Player, b.Player))
	})
	return out
}

// lowest is a player's lowest health (%) in their buffer from..to (log time), 0 when no event in it says.
func (w *hitWatch) lowest(player string, from, to int64) float64 {
	low := 0.0
	for i := len(w.last[player]) - 1; i >= 0 && w.last[player][i].at >= from; i-- {
		if l := w.last[player][i]; l.at <= to && l.event.HPPct > 0 && (low == 0 || l.event.HPPct < low) {
			low = l.event.HPPct
		}
	}
	return low
}

// kept says whether a big hit counts: with [big_hits] low_hp_pct set, only when the player's health fell under it
// during the hit (a death in it counts as 0); a running hit is judged on what it has so far.
func (k *LogTracker) kept(run *keyRun, h *bigHit) bool {
	bar := k.rules.BigHits.LowHPPct
	if bar == 0 {
		return true
	}
	from, to := h.second*1000, k.bigHitEnd(h)
	if slices.ContainsFunc(run.deaths, func(d models.Death) bool {
		return d.Player == h.player && d.At >= from && d.At <= to
	}) {
		return true
	}
	low := h.lowHP
	if !h.done {
		low = run.hits.lowest(h.player, from, to)
	}
	return low > 0 && low < bar
}
