package service

import (
	"cmp"
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
	spikeMS           = 3_000
	spikeShare        = 0.95
	maxSpikes         = 8
	minSpikeStretches = 60
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

// stopEvent follows must-stop casts, kicks and dispellable debuffs (docs/specs/mplus-v2.md 7A and 7B). It runs for
// every line inside a key, after event's own handling.
func (k *KeyTracker) stopEvent(at int64, event string, f []string, src, dst string) {
	run := k.current
	if p := run.kick; p != nil && at-p.at > kickWindowMS {
		run.key.WastedKicks = append(run.key.WastedKicks, models.KeyCast{Player: p.player, Ability: p.spell,
			Kind: "kick", ClockMS: k.clockAt(p.at)})
		run.kick = nil
	}
	for key, span := range run.removed {
		if at-span[1] > dispelWindowMS {
			k.recordDebuff(key, span, "")
		}
	}
	for guid, c := range run.casting {
		if at-c.at > castTimeoutMS {
			k.endCast(guid, "stopped", c.cc, at)
		}
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
func (k *KeyTracker) endCast(guid, outcome, by string, at int64) {
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
func (k *KeyTracker) recordDebuff(key [2]string, span [2]int64, by string) {
	run := k.current
	delete(run.removed, key)
	run.key.Debuffs = append(run.key.Debuffs, models.KeyDispel{Player: k.names[key[0]], Spell: key[1], By: by,
		AfterMS: span[1] - span[0], ClockMS: k.clockAt(span[0])})
}

// mobDied ends a must-stop cast whose caster died: stopped when a debuff landed during it, else the mob died first.
func (k *KeyTracker) mobDied(guid string, at int64) {
	if c, ok := k.current.casting[guid]; ok {
		outcome := "died"
		if c.cc != "" {
			outcome = "stopped"
		}
		k.endCast(guid, outcome, c.cc, at)
	}
}

// spikes is the run's worst bursts of party damage taken (the top spikeShare of spikeMS stretches), worst first,
// with each hit member's cover by the death rules and the party cooldowns used for it.
func (k *KeyTracker) spikes(run *keyRun, casts []models.KeyCast, buffs []models.BuffUp, kit models.Kit) []models.KeySpike {
	party := map[int64]int64{}
	for _, bySecond := range run.taken {
		for s, n := range bySecond {
			party[s] += n
		}
	}
	var stretches [][2]int64 // start second, damage
	for s := range party {
		sum := int64(0)
		for i := range int64(spikeMS / 1000) {
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
		if slices.ContainsFunc(taken, func(t int64) bool { return max(t-s[0], s[0]-t) < spikeMS/1000 }) {
			continue
		}
		taken = append(taken, s[0])
		spikes = append(spikes, k.spike(run, s[0], s[1], casts, buffs, kit))
	}
	return spikes
}

// spike is one burst starting at a second of the key (log time / 1000): who was hit and their cover, and the party
// cooldowns used from the death window before it to its end.
func (k *KeyTracker) spike(run *keyRun, second, damage int64, casts []models.KeyCast, buffs []models.BuffUp,
	kit models.Kit) models.KeySpike {
	at := second * 1000
	sp := models.KeySpike{ClockMS: k.clockAt(at), Damage: damage, PartyCDs: []string{}, Members: []models.KeySpikeMember{}}
	window := int64(k.rules.Defensives.WindowSeconds * 1000)
	var cdAt []int64
	for _, c := range casts {
		if slices.Contains(k.rules.Defensives.Externals, c.Ability) && c.ClockMS >= sp.ClockMS-window &&
			c.ClockMS <= sp.ClockMS+spikeMS {
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
		n := bySecond[second] + bySecond[second+1] + bySecond[second+2]
		if n > 0 {
			hit = append(hit, models.Death{Fight: run.pullOf[second], At: at + spikeMS, Player: player})
			amounts = append(amounts, n)
		}
	}
	ClassifyDeaths(hit, run.casts, buffs, kit, window)
	for i, h := range hit {
		sp.Members = append(sp.Members, models.KeySpikeMember{Player: h.Player, Taken: amounts[i],
			Defensive: h.Defensive, Ready: list(h.Ready)})
	}
	slices.SortFunc(sp.Members, func(a, b models.KeySpikeMember) int { return cmp.Compare(b.Taken, a.Taken) })
	return sp
}
