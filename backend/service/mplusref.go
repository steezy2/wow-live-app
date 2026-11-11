package service

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"wow-live/backend/models"
)

// DungeonSources is what cmd/mplusref gathered for one dungeon: Mythic Dungeon Tools' data, Wowhead's overview and
// Keystone.guru's total forces (0 when it has none).
type DungeonSources struct {
	Name              string
	MDT               models.MDTDungeon
	Guide             models.DungeonGuide
	KeystoneGuruTotal int
}

// tipKinds sorts the guide's boss tips by the words in their tag; a tip can be in several lists.
var tipKinds = map[string][]string{
	"avoidable": {"dodge", "avoid", "run away", "hide"},
	"soak":      {"soak"},
	"dispel":    {"dispel"},
	"interrupt": {"interrupt"},
}

// npcTwins are NPCs Mythic Dungeon Tools doesn't list that the game counts as one it does: a second ID of the same mob.
// Kings' Rest has four Half-Finished Mummies under each ID (a +17 on Nov 10 came to 587 of 608 without them).
var npcTwins = map[int]int{273050: 270502}

// DungeonFacts combines a dungeon's sources into its reference entry, counting each of npcTwins as its twin. It fails
// when Mythic Dungeon Tools has no data for the dungeon or its forces total disagrees with Keystone.guru's.
func DungeonFacts(src DungeonSources) (models.DungeonFacts, error) {
	m, g := src.MDT, src.Guide
	if m.MapID == 0 {
		return models.DungeonFacts{}, fmt.Errorf("%s: Mythic Dungeon Tools has no data", src.Name)
	}
	if src.KeystoneGuruTotal > 0 && src.KeystoneGuruTotal != m.Total {
		return models.DungeonFacts{}, fmt.Errorf("%s: forces total %d in Mythic Dungeon Tools, %d on Keystone.guru",
			src.Name, m.Total, src.KeystoneGuruTotal)
	}
	tips := map[string][]int{}
	for id, tag := range g.Tips {
		for kind, words := range tipKinds {
			if slices.ContainsFunc(words, func(w string) bool { return strings.Contains(strings.ToLower(tag), w) }) {
				tips[kind] = append(tips[kind], id)
			}
		}
	}
	f := models.DungeonFacts{Name: src.Name, Guide: g.URL, Forces: models.Forces{Total: m.Total, NPCs: map[int]int{}},
		Avoidable: sorted(tips["avoidable"]), Soak: sorted(tips["soak"]),
		Dispel: sorted(append(slices.Collect(maps.Keys(g.Dispels)), tips["dispel"]...)),
		Purge:  sorted(slices.Collect(maps.Keys(g.Purges))), MustStop: []int{}, Review: []int{}}
	mobs := maps.Clone(m.Mobs)
	for twin, of := range npcTwins {
		if mob, ok := mobs[of]; ok {
			mobs[twin] = mob
		}
	}
	var counts []int64
	for id, mob := range mobs {
		f.Forces.NPCs[id] = mob.Count
		if mob.Count > 0 {
			counts = append(counts, int64(mob.Count))
		}
	}
	f.Bosses, f.Minibosses = []int{}, []int{}
	for id, mob := range mobs {
		switch {
		case mob.Boss:
			f.Bosses = append(f.Bosses, id)
		case len(counts) > 0 && float64(mob.Count) >= minibossRatio*float64(median(counts)):
			f.Minibosses = append(f.Minibosses, id)
		}
	}
	f.Bosses, f.Minibosses = sorted(f.Bosses), sorted(f.Minibosses)
	for _, id := range append(slices.Collect(maps.Keys(g.Interrupts)), tips["interrupt"]...) {
		if m.Spells[id].Interruptible {
			f.MustStop = append(f.MustStop, id)
		} else {
			f.Review = append(f.Review, id)
		}
	}
	f.MustStop, f.Review = sorted(f.MustStop), sorted(f.Review)
	return f, nil
}

// sorted is the IDs once each, in order, never nil (so the file always has a list).
func sorted(ids []int) []int {
	out := append([]int{}, ids...)
	slices.Sort(out)
	return slices.Compact(out)
}

// UnevenHit is an enemy ability that hit some of a key's party often and another member never: a candidate for a
// dungeon's avoidable list.
type UnevenHit struct {
	MapID   int
	SpellID int
	Name    string
	Keys    int // keys it was uneven in
}

// HitCounter counts, per key, how often each enemy ability's direct hits landed on each party member, to find the
// uneven ones, and which mobs died, to find those without a forces count (cmd/mplusref --check).
type HitCounter struct {
	mapID  int
	party  []string
	hits   map[int]map[string]int // spell -> party GUID -> hits, in the running key
	names  map[int]string
	uneven map[[2]int]int // (map, spell) -> keys it was uneven in
	Kills  map[[2]int]int // (map, NPC) -> mobs of it that died in keys
}

// NewHitCounter is an empty HitCounter.
func NewHitCounter() *HitCounter {
	return &HitCounter{names: map[int]string{}, uneven: map[[2]int]int{}, Kills: map[[2]int]int{}}
}

// Add reads one line.
func (h *HitCounter) Add(line models.LogLine) {
	f := line.Fields
	switch {
	case line.Event == "CHALLENGE_MODE_START" && len(f) >= 3:
		h.mapID, h.party, h.hits = atoi(f[2]), nil, map[int]map[string]int{}
	case h.hits == nil:
	case line.Event == "CHALLENGE_MODE_END":
		h.endKey()
	case line.Event == "COMBATANT_INFO" && len(f) > 0:
		h.party = append(h.party, f[0])
	case line.Event == "UNIT_DIED" && len(f) > 4 && strings.HasPrefix(f[4], "Creature-"):
		if parts := strings.Split(f[4], "-"); len(parts) > 5 {
			h.Kills[[2]int{h.mapID, atoi(parts[5])}]++
		}
	case line.Event == "SPELL_DAMAGE" && len(f) > 9 && hasFlag(f[2], hostileFlag) && slices.Contains(h.party, f[4]):
		spell := atoi(f[8])
		if h.hits[spell] == nil {
			h.hits[spell] = map[string]int{}
		}
		h.hits[spell][f[4]]++
		h.names[spell] = f[9]
	}
}

// endKey keeps the abilities that were uneven in the key that just ended: one member hit at least 3 times the median
// and at least 3 times, another never.
func (h *HitCounter) endKey() {
	for spell, byPlayer := range h.hits {
		counts := make([]int, len(h.party))
		for i, guid := range h.party {
			counts[i] = byPlayer[guid]
		}
		slices.Sort(counts)
		if len(counts) > 0 && counts[0] == 0 && counts[len(counts)-1] >= max(3, 3*counts[len(counts)/2]) {
			h.uneven[[2]int{h.mapID, spell}]++
		}
	}
	h.hits = nil
}

// Uneven is every ability that was uneven in at least one key, most keys first.
func (h *HitCounter) Uneven() []UnevenHit {
	var out []UnevenHit
	for k, n := range h.uneven {
		out = append(out, UnevenHit{MapID: k[0], SpellID: k[1], Name: h.names[k[1]], Keys: n})
	}
	slices.SortFunc(out, func(a, b UnevenHit) int {
		return cmp.Or(cmp.Compare(a.MapID, b.MapID), cmp.Compare(b.Keys, a.Keys), cmp.Compare(a.SpellID, b.SpellID))
	})
	return out
}
