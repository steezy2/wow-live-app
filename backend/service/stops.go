package service

import (
	"cmp"
	"maps"
	"slices"

	"wow-live/backend/models"
)

const (
	// kickWindowMS: a kick with no interrupt this soon after it was wasted (the cast was already stopped, or immune).
	kickWindowMS = 200
	// dispelWindowMS: the game logs a dispelled debuff's removal just before the dispel, so a removal waits this long
	// for one before it counts as the debuff running out.
	dispelWindowMS = 50
	// castTimeoutMS: a must-stop cast with no outcome after this long was stopped somehow (moved, line of sight).
	castTimeoutMS = 15_000
	// spikeMS and spikeShare: a spike is spikeMS of party damage taken above this share of all such stretches so
	// far; at most maxSpikes are shown, after minSpikeStretches stretches of fighting.
	spikeMS           = 5_000
	spikeShare        = 0.95
	maxSpikes         = 8
	minSpikeStretches = 60
	// spikeCauseCount: a spike names this many of the abilities that hit the party hardest in it.
	spikeCauseCount = 3
	// overlapMS: two party cooldowns this close together on one spike overlap.
	overlapMS = 5_000
)

// openCast is a must-stop cast in progress: who is casting what since when, and a debuff a party member put on the
// caster meanwhile (a stun or knock that may stop it).
type openCast struct {
	at      int64
	pull    int
	mob     string
	spell   string
	spellID int
	cc      string
}

// pendingKick is a party member's kick waiting for its interrupt.
type pendingKick struct {
	at     int64
	player string
	spell  string
}

// stopEvent follows must-stop casts, kicks and dispellable debuffs. It runs for
// every line inside a key, after event's own handling.
func (k *LogTracker) stopEvent(at int64, event string, f []string, src, dst string) {
	run := k.current
	if p := run.kick; p != nil && at-p.at > kickWindowMS {
		run.key.WastedKicks = append(run.key.WastedKicks, models.KeyCast{Player: p.player, Ability: p.spell,
			Kind: "kick", ClockMS: k.clockAt(p.at)})
		run.kick = nil
	}
	// What ends in one line is saved in key order, the same on every read (this runs on every line: only the few
	// that end are sorted).
	var ranOut [][2]string
	for key, span := range run.removed {
		if at-span[1] > dispelWindowMS {
			ranOut = append(ranOut, key)
		}
	}
	slices.SortFunc(ranOut, func(a, b [2]string) int { return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1])) })
	for _, key := range ranOut {
		k.recordDebuff(key, run.removed[key], "")
	}
	var timedOut []string
	for guid, c := range run.casting {
		if at-c.at > castTimeoutMS {
			timedOut = append(timedOut, guid)
		}
	}
	slices.Sort(timedOut)
	for _, guid := range timedOut {
		k.endCast(guid, "stopped", run.casting[guid].cc, at)
	}
	if run.facts == nil || len(f) < 10 {
		return
	}
	spell := atoi(f[8])
	switch event {
	case "SPELL_CAST_START":
		if hasFlag(f[2], hostileFlag) && slices.Contains(run.facts.MustStop, spell) {
			if c, ok := run.casting[f[0]]; ok {
				k.endCast(f[0], "stopped", c.cc, at)
			}
			run.casting[f[0]] = &openCast{at: at, pull: run.pull, mob: f[1], spell: f[9], spellID: spell}
		}
	case "SPELL_CAST_SUCCESS":
		if c, ok := run.casting[f[0]]; ok && c.spellID == spell {
			k.endCast(f[0], "went off", "", at)
		}
		if src != "" && slices.Contains(k.rules.Kicks.Names, f[9]) && hasFlag(f[6], hostileFlag) &&
			at-run.lastKick[src] > kickWindowMS {
			run.kick = &pendingKick{at: at, player: src, spell: f[9]}
		}
	case "SPELL_INTERRUPT":
		if len(f) < 12 {
			return
		}
		run.lastKick[src] = at
		if p := run.kick; p != nil && p.player == src {
			run.kick = nil
		}
		if c, ok := run.casting[f[4]]; ok && atoi(f[11]) == c.spellID {
			k.endCast(f[4], "interrupted", src, at)
		}
	case "SPELL_AURA_APPLIED":
		if len(f) < 12 {
			return
		}
		if c, ok := run.casting[f[4]]; ok && src != "" && f[11] == "DEBUFF" {
			c.cc = f[9]
		}
		if dst != "" && f[11] == "DEBUFF" && slices.Contains(run.facts.Dispel, spell) {
			run.debuffs[[2]string{f[4], f[9]}] = at
		}
	case "SPELL_DISPEL":
		key := [2]string{f[4], ""}
		if len(f) > 12 {
			key[1] = f[12]
		}
		if span, ok := run.removed[key]; ok {
			k.recordDebuff(key, span, src)
		}
	case "SPELL_AURA_REMOVED":
		key := [2]string{f[4], f[9]}
		if from, ok := run.debuffs[key]; ok {
			delete(run.debuffs, key)
			run.removed[key] = [2]int64{from, at}
		}
	}
}

// endCast records a must-stop cast's outcome: interrupted (by a player), stopped (by a debuff, or unknown), went
// off, or the mob died first.
func (k *LogTracker) endCast(guid, outcome, by string, at int64) {
	run := k.current
	c := run.casting[guid]
	delete(run.casting, guid)
	stop := models.KeyStop{Pull: c.pull, ClockMS: k.clockAt(c.at), Mob: c.mob, Spell: c.spell, Outcome: outcome,
		By: by}
	if outcome == "interrupted" {
		stop.IntoMS = at - c.at
	}
	run.key.Stops = append(run.key.Stops, stop)
}

// recordDebuff keeps a dispellable debuff that came off a party member: dispelled by someone, or it ran out (by "").
func (k *LogTracker) recordDebuff(key [2]string, span [2]int64, by string) {
	run := k.current
	delete(run.removed, key)
	run.key.Debuffs = append(run.key.Debuffs, models.KeyDispel{Player: k.names[key[0]], Spell: key[1], By: by,
		AfterMS: span[1] - span[0], ClockMS: k.clockAt(span[0])})
}

// mobDied ends a must-stop cast whose caster died: stopped when a debuff landed during it, else the mob died first.
func (k *LogTracker) mobDied(guid string, at int64) {
	if c, ok := k.current.casting[guid]; ok {
		outcome := "died"
		if c.cc != "" {
			outcome = "stopped"
		}
		k.endCast(guid, outcome, c.cc, at)
	}
}

// spikeWindow is how long a spike lasts, in ms: spikeMS, or a raid's own (raid-rules.toml's spike_seconds, since a
// raid's damage comes in longer waves).
func (k *LogTracker) spikeWindow(run *keyRun) int64 {
	if run.raid != nil {
		return int64(k.raid.rules.Suggestions.SpikeSeconds * 1000)
	}
	return spikeMS
}

// spikes is the run's worst bursts of party damage taken (the top spikeShare of spikeWindow stretches), worst first,
// with each hit member's cover by the death rules and the party cooldowns used for it.
func (k *LogTracker) spikes(run *keyRun, casts []models.KeyCast, buffs []models.BuffUp, kit models.Kit) []models.KeySpike {
	party := map[int64]int64{}
	for _, bySecond := range run.taken {
		for s, n := range bySecond {
			party[s] += n
		}
	}
	window := k.spikeWindow(run)
	var stretches [][2]int64 // start second, damage
	for s := range party {
		sum := int64(0)
		for i := range window / 1000 {
			sum += party[s+i]
		}
		stretches = append(stretches, [2]int64{s, sum})
	}
	spikes := []models.KeySpike{}
	if len(stretches) < minSpikeStretches {
		return spikes
	}
	slices.SortFunc(stretches, func(a, b [2]int64) int { return cmp.Compare(b[1], a[1]) })
	threshold := stretches[int(float64(len(stretches))*(1-spikeShare))][1]
	var taken []int64
	for _, s := range stretches {
		if s[1] < threshold || len(spikes) == maxSpikes {
			break
		}
		if slices.ContainsFunc(taken, func(t int64) bool { return max(t-s[0], s[0]-t) < window/1000 }) {
			continue
		}
		taken = append(taken, s[0])
		spikes = append(spikes, k.spike(run, s[0], s[1], window, casts, buffs, kit))
	}
	return spikes
}

// spike is one burst starting at a second of the key (log time / 1000): who was hit and their cover, and the party
// cooldowns (externals) used from the death window before it to its end; in a raid, the raid cooldowns whose buff
// was up during it.
func (k *LogTracker) spike(run *keyRun, second, damage, span int64, casts []models.KeyCast, buffs []models.BuffUp,
	kit models.Kit) models.KeySpike {
	at := second * 1000
	sp := models.KeySpike{ClockMS: k.clockAt(at), Seconds: span / 1000, Damage: damage, PartyCDs: []string{}, Members: []models.KeySpikeMember{}}
	window := int64(k.rules.Defensives.WindowSeconds * 1000)
	covers := func(c models.KeyCast) bool {
		return (slices.Contains(k.rules.Defensives.Externals, c.Ability) ||
			slices.Contains(k.rules.Defensives.RaidCooldowns, c.Ability)) &&
			c.ClockMS >= sp.ClockMS-window && c.ClockMS <= sp.ClockMS+span
	}
	if run.raid != nil { // a raid's cover is its raid cooldowns (coversSpike)
		casts = run.raid.cooldowns
		covers = func(c models.KeyCast) bool {
			return coversSpike(c, sp.ClockMS, sp.ClockMS+span, window)
		}
	}
	var cdAt []int64
	for _, c := range casts {
		if covers(c) {
			sp.PartyCDs = append(sp.PartyCDs, c.Ability+" ("+c.Player+")")
			cdAt = append(cdAt, c.ClockMS)
		}
	}
	slices.Sort(cdAt)
	for i := 1; i < len(cdAt); i++ {
		sp.Overlap = sp.Overlap || cdAt[i]-cdAt[i-1] <= overlapMS
	}
	var hit []models.Death
	var amounts []int64
	for player, bySecond := range run.taken {
		n := int64(0)
		for i := range span / 1000 {
			n += bySecond[second+i]
		}
		if n > 0 {
			hit = append(hit, models.Death{Fight: run.pullOf[second], At: at + span, Player: player})
			amounts = append(amounts, n)
		}
	}
	ClassifyDeaths(hit, run.casts, buffs, kit, window)
	for i, h := range hit {
		sp.Members = append(sp.Members, models.KeySpikeMember{Player: h.Player, Taken: amounts[i],
			Defensive: h.Defensive, Used: h.Used, Ready: list(h.Ready)})
	}
	slices.SortFunc(sp.Members, func(a, b models.KeySpikeMember) int { return cmp.Compare(b.Taken, a.Taken) })
	sp.Unused = unusedDefensive(sp)
	sp.Causes = spikeCauses(run.takenAt, second, span)
	return sp
}

// tookFrom counts a hit at a second of the log by ability and source into takenAt (the party's, or one player's), for
// spikeCauses.
func tookFrom(takenAt map[int64]map[[2]string]*models.SpikeCause, second int64, ability, source string, amount int64) {
	if amount <= 0 {
		return
	}
	bySource := takenAt[second]
	if bySource == nil {
		bySource = map[[2]string]*models.SpikeCause{}
		takenAt[second] = bySource
	}
	c := bySource[[2]string{ability, source}]
	if c == nil {
		c = &models.SpikeCause{Ability: ability, Source: source}
		bySource[[2]string{ability, source}] = c
	}
	c.Damage, c.Hits = c.Damage+amount, c.Hits+1
}

// spikeCauses is what hit the party hardest over span ms from a second of the log, most damage first (ties by
// ability and source, so the order never follows the map's), at most spikeCauses of them.
func spikeCauses(takenAt map[int64]map[[2]string]*models.SpikeCause, second, span int64) []models.SpikeCause {
	sum := map[[2]string]models.SpikeCause{}
	for i := range span / 1000 {
		for id, c := range takenAt[second+i] {
			s := sum[id]
			s.Ability, s.Source, s.Damage, s.Hits = c.Ability, c.Source, s.Damage+c.Damage, s.Hits+c.Hits
			sum[id] = s
		}
	}
	causes := slices.SortedFunc(maps.Values(sum), func(a, b models.SpikeCause) int {
		return cmp.Or(cmp.Compare(b.Damage, a.Damage), cmp.Compare(a.Ability, b.Ability), cmp.Compare(a.Source, b.Source))
	})
	return causes[:min(len(causes), spikeCauseCount)]
}
