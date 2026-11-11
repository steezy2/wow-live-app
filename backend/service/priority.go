package service

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"wow-live/backend/models"
)

const (
	// openingMS is the start of a pull whose damage into its minibosses is shown on its own.
	openingMS = 10_000
	// minibossRatio: a miniboss is worth this many times the dungeon's median forces count (the dungeon file), or
	// without the file has this many times the pull's median health.
	minibossRatio = 2.0
	// packMS: mobs first hit within this long of a miniboss are its pack, for its place in the kill order (a pull can
	// chain several packs).
	packMS = 10_000
)

// pullStat is what a pull did to each mob it hit: the party's damage in all and in the opening, and each mob. A boss
// pull has no minibosses.
type pullStat struct {
	startAt, clockMS   int64
	boss               bool
	damage, openDamage int64
	mobs               map[string]*mobStat
}

// mobStat is one mob in a pull: its name, the party's damage into it by player and in the opening, and whether and
// when it died on the key's clock.
type mobStat struct {
	name     string
	byPlayer map[string]int64
	opening  int64
	firstHit int64
	dead     bool
	diedMS   int64
}

// npcOf is a creature's NPC ID from its GUID (Creature-0-3882-2859-27547-245336-00006BE4EA: 245336), or 0.
func npcOf(guid string) int {
	parts := strings.Split(guid, "-")
	if len(parts) < 7 || (parts[0] != "Creature" && parts[0] != "Vehicle") {
		return 0
	}
	return atoi(parts[5])
}

// hitMob counts a party member's hit on a mob for the pull's miniboss numbers; the mob belongs to the pull it was
// first hit in.
func (k *LogTracker) hitMob(at int64, guid, name, player string, amount int64) {
	run := k.current
	stat := run.pulls[run.pull]
	if stat == nil {
		stat = &pullStat{startAt: run.pullFrom, clockMS: run.pullAt, boss: run.bossPull[run.pull],
			mobs: map[string]*mobStat{}}
		run.pulls[run.pull] = stat
	}
	if _, ok := run.mobPull[guid]; !ok {
		run.mobPull[guid] = run.pull
	}
	mob := stat.mobs[guid]
	if mob == nil {
		mob = &mobStat{name: name, byPlayer: map[string]int64{}, firstHit: k.clockAt(at)}
		stat.mobs[guid] = mob
	}
	stat.damage += amount
	mob.byPlayer[player] += amount
	if at-stat.startAt <= openingMS {
		stat.openDamage += amount
		mob.opening += amount
	}
}

// mobDown records when a mob the party hit died, in the pull it was first hit in.
func (k *LogTracker) mobDown(at int64, guid string) {
	run := k.current
	n, ok := run.mobPull[guid]
	if stat := run.pulls[n]; ok && stat != nil && stat.mobs[guid] != nil && !stat.mobs[guid].dead {
		stat.mobs[guid].dead, stat.mobs[guid].diedMS = true, k.clockAt(at)
	}
}

// minibossesOf is a trash pull's minibosses: mobs of the dungeon file's miniboss NPCs, or without the file every mob
// with minibossRatio times the pull's median health (else its single biggest mob when it stands out). A pull can have
// several. None for a boss pull or a pack of equals.
func minibossesOf(stat *pullStat, run *keyRun) map[string]bool {
	minis := map[string]bool{}
	if stat.boss {
		return minis
	}
	if run.facts != nil && len(run.facts.Minibosses) > 0 {
		for guid := range stat.mobs {
			minis[guid] = slices.Contains(run.facts.Minibosses, npcOf(guid))
		}
		return minis
	}
	var hps []int64
	for guid := range stat.mobs {
		if run.mobHP[guid] > 0 {
			hps = append(hps, run.mobHP[guid])
		}
	}
	if len(hps) < 2 {
		return minis
	}
	mid, top := median(hps), slices.Max(hps)
	for guid := range stat.mobs {
		minis[guid] = float64(run.mobHP[guid]) >= minibossRatio*float64(mid)
	}
	if !slices.Contains(slices.Collect(maps.Values(minis)), true) && top > mid {
		for guid := range stat.mobs {
			minis[guid] = run.mobHP[guid] == top
		}
	}
	return minis
}

// priorityDamage is each player's damage into the priority mobs of the given pulls: the minibosses of trash pulls,
// and in boss pulls the bosses themselves (the dungeon file's boss NPCs; their adds don't count).
func priorityDamage(run *keyRun, pulls ...int) map[string]int64 {
	out := map[string]int64{}
	for _, n := range pulls {
		stat := run.pulls[n]
		if stat == nil {
			continue
		}
		minis := minibossesOf(stat, run)
		for guid, mob := range stat.mobs {
			if minis[guid] || stat.boss && run.facts != nil && slices.Contains(run.facts.Bosses, npcOf(guid)) {
				for player, d := range mob.byPlayer {
					out[player] += d
				}
			}
		}
	}
	return out
}

// priorityPulls is each trash pull with minibosses: the share of damage into them in its opening and in all, their
// share of the pack's health, and where each fell in the pull's kill order. A miniboss is fine as long as it dies
// before most of the pack; Late says one didn't.
func priorityPulls(run *keyRun) []models.KeyPull {
	pulls := []models.KeyPull{}
	for n, s := range run.pulls {
		minis := minibossesOf(s, run)
		var into, opening, health, packHealth int64
		for guid, mob := range s.mobs {
			packHealth += run.mobHP[guid]
			if minis[guid] {
				health += run.mobHP[guid]
				opening += mob.opening
				for _, d := range mob.byPlayer {
					into += d
				}
			}
		}
		p := models.KeyPull{Pull: n, ClockMS: s.clockMS, PriorityPct: share(into, s.damage),
			OpeningPct: share(opening, s.openDamage), HealthPct: share(health, packHealth), Minibosses: []models.KeyMiniboss{}}
		for _, guid := range slices.Sorted(maps.Keys(s.mobs)) { // GUID order, so a tie in the sort below is always the same
			if mob := s.mobs[guid]; minis[guid] {
				m := packOrder(s, minis, mob)
				p.Late = p.Late || m.Order-1 > (m.Of-1)/2 && m.Order > 0
				p.Minibosses = append(p.Minibosses, m)
			}
		}
		if len(p.Minibosses) == 0 {
			continue
		}
		slices.SortFunc(p.Minibosses, func(a, b models.KeyMiniboss) int {
			return cmp.Or(cmp.Compare(a.Order, b.Order), cmp.Compare(a.Name, b.Name))
		})
		pulls = append(pulls, p)
	}
	slices.SortFunc(pulls, func(a, b models.KeyPull) int { return cmp.Compare(a.Pull, b.Pull) })
	return pulls
}

// packOrder is a miniboss's place among the regular mobs of its pack (first hit within packMS of it) that died: Of is
// itself plus those, Order is 1 plus those that died before it (0 if it didn't die).
func packOrder(s *pullStat, minis map[string]bool, mini *mobStat) models.KeyMiniboss {
	m := models.KeyMiniboss{Name: mini.name, DiedMS: mini.diedMS, Of: 1}
	before := 0
	for guid, mob := range s.mobs {
		if minis[guid] || !mob.dead || max(mob.firstHit-mini.firstHit, mini.firstHit-mob.firstHit) > packMS {
			continue
		}
		m.Of++
		if mini.dead && mob.diedMS < mini.diedMS {
			before++
		}
	}
	if mini.dead {
		m.Order = before + 1
	}
	return m
}

func share(part, whole int64) float64 {
	if whole == 0 {
		return 0
	}
	return 100 * float64(part) / float64(whole)
}
