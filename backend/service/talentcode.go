package service

import (
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"

	"wow-live/backend/models"
)

const (
	// loadoutVersion is the version of the game's talent loadout string this codec reads and writes.
	loadoutVersion = 2
	// loadoutChars is the string's base64 alphabet; each character holds 6 bits, the lowest first.
	loadoutChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	rankBits     = 6
	choiceBits   = 2
)

var errLoadoutLength = errors.New("talent string has the wrong length")

// DecodeTalents reads the game's talent loadout string (an export's talents=) against tree: each node taken, with its
// entry and rank. It errors on another version, spec or tree hash, a node the spec doesn't have, or the wrong length.
func DecodeTalents(tree models.TalentTree, s string) ([]models.TalentPick, error) {
	r := loadoutReader{s: s}
	if v := r.read(8); v != loadoutVersion {
		return nil, fmt.Errorf("talent string version %d, not %d", v, loadoutVersion)
	}
	if spec := r.read(16); spec != tree.SpecID {
		return nil, fmt.Errorf("talent string of spec %d, not %d", spec, tree.SpecID)
	}
	hash := make([]byte, 16)
	for i := range hash {
		hash[i] = byte(r.read(8))
	}
	if h := hex.EncodeToString(hash); h != tree.Hash && strings.Trim(h, "0") != "" {
		return nil, fmt.Errorf("talent string of another tree (%s, not %s)", h, tree.Hash)
	}
	nodes := loadoutNodes(tree)
	var picks []models.TalentPick
	for _, id := range tree.Order {
		if r.read(1) == 0 {
			continue
		}
		n, ok := nodes[id]
		if !ok || len(n.Entries) == 0 {
			return nil, fmt.Errorf("talent string takes node %d, which %s doesn't have", id, tree.Spec)
		}
		pick := models.TalentPick{NodeID: id, EntryID: n.Entries[0].ID, Rank: max(n.MaxRanks, 1)}
		if r.read(1) == 1 { // bought, not granted
			if r.read(1) == 1 {
				pick.Rank = r.read(rankBits)
			}
			if r.read(1) == 1 {
				i := r.read(choiceBits)
				if i >= len(n.Entries) {
					return nil, fmt.Errorf("talent string picks entry %d of node %d", i, id)
				}
				pick.EntryID = n.Entries[i].ID
			}
		}
		picks = append(picks, pick)
	}
	if r.err != nil || len(s) != (r.pos+5)/6 {
		return nil, errLoadoutLength
	}
	return picks, nil
}

// EncodeTalents is picks written as the game's talent loadout string for tree. It errors on a node the tree doesn't
// have, an entry its node doesn't hold, or a rank past the node's.
func EncodeTalents(tree models.TalentTree, picks []models.TalentPick) (string, error) {
	hash, err := hex.DecodeString(tree.Hash)
	if err != nil || len(hash) != 16 {
		return "", fmt.Errorf("tree hash %q of %s", tree.Hash, tree.Spec)
	}
	nodes := loadoutNodes(tree)
	byNode := map[int]models.TalentPick{}
	for _, p := range picks {
		n, ok := nodes[p.NodeID]
		if !ok || !slices.Contains(tree.Order, p.NodeID) || p.Rank < 1 || p.Rank > max(n.MaxRanks, 1) ||
			!slices.ContainsFunc(n.Entries, func(e models.TalentEntry) bool { return e.ID == p.EntryID }) {
			return "", fmt.Errorf("talent %d:%d rank %d isn't one of %s's", p.NodeID, p.EntryID, p.Rank, tree.Spec)
		}
		byNode[p.NodeID] = p
	}
	var w loadoutWriter
	w.write(8, loadoutVersion)
	w.write(16, tree.SpecID)
	for _, b := range hash {
		w.write(8, int(b))
	}
	for _, id := range tree.Order {
		p, taken := byNode[id]
		n := nodes[id]
		if w.flag(taken) && w.flag(!n.Free) {
			if w.flag(p.Rank < max(n.MaxRanks, 1)) {
				w.write(rankBits, p.Rank)
			}
			if w.flag(n.Choice) {
				w.write(choiceBits, slices.IndexFunc(n.Entries, func(e models.TalentEntry) bool { return e.ID == p.EntryID }))
			}
		}
	}
	return w.String(), nil
}

// validTalents says why picks can't all be taken in tree, nil when they can: no section ("class", "spec", each hero
// tree, which the string holds both of) spends more than its points, and each taken node that others lead to has one
// of those taken or free.
func validTalents(tree models.TalentTree, picks []models.TalentPick) error {
	taken := map[int]bool{}
	for _, p := range picks {
		taken[p.NodeID] = true
	}
	parents := map[int][]int{}
	for _, n := range tree.Nodes {
		for _, next := range n.Next {
			parents[next] = append(parents[next], n.ID)
		}
	}
	nodes := loadoutNodes(tree)
	type section struct {
		tree    string
		subTree int
	}
	spent := map[section]int{}
	// A free node is granted: the inactive hero tree's isn't in the string, yet its talents stay taken.
	reached := func(id int) bool { return taken[id] || nodes[id].Free }
	for _, p := range picks {
		n := nodes[p.NodeID]
		if !n.Free {
			s := section{n.Tree, n.SubTree}
			if spent[s] += p.Rank; spent[s] > tree.Points[n.Tree] {
				return fmt.Errorf("over the %d %s points", tree.Points[n.Tree], n.Tree)
			}
		}
		if ps := parents[n.ID]; len(ps) > 0 && !slices.ContainsFunc(ps, reached) {
			return fmt.Errorf("%s is taken without a talent that leads to it", n.Name)
		}
	}
	return nil
}

// profileTalents is an export's talents= read against its spec's tree in trees, nil when it can't be read.
func profileTalents(trees map[int]models.TalentTree, s string) []models.TalentPick {
	picks, err := DecodeTalents(loadoutTree(trees, s), s)
	if err != nil {
		return nil
	}
	return picks
}

// loadoutTree is the tree in trees of the spec loadout string s names (the zero tree when none is).
func loadoutTree(trees map[int]models.TalentTree, s string) models.TalentTree {
	r := loadoutReader{s: s}
	r.read(8)
	return trees[r.read(16)]
}

// talentLine is picks as a profileset's talents= line for tree.
func talentLine(tree models.TalentTree, picks []models.TalentPick) (string, error) {
	s, err := EncodeTalents(tree, picks)
	return "talents=" + s + "\n", err
}

// loadoutNodes is tree's nodes by ID, the hero tree choice among them.
func loadoutNodes(tree models.TalentTree) map[int]models.TalentNode {
	nodes := map[int]models.TalentNode{tree.HeroChoice.ID: tree.HeroChoice}
	for _, n := range tree.Nodes {
		nodes[n.ID] = n
	}
	return nodes
}

// loadoutReader reads a loadout string's values, the lowest bit first; err is set by a read past its end or a
// character outside loadoutChars.
type loadoutReader struct {
	s   string
	pos int // bits read
	err error
}

func (r *loadoutReader) read(bits int) int {
	v := 0
	for i := range bits {
		if r.pos/6 >= len(r.s) {
			r.err = errLoadoutLength
			return 0
		}
		c := strings.IndexByte(loadoutChars, r.s[r.pos/6])
		if c < 0 {
			r.err = fmt.Errorf("talent string character %q", r.s[r.pos/6])
			return 0
		}
		v |= (c >> (r.pos % 6) & 1) << i
		r.pos++
	}
	return v
}

// loadoutWriter writes a loadout string's values, the lowest bit first.
type loadoutWriter struct {
	b    strings.Builder
	char int // the bits of the character being filled
	n    int // how many it holds
}

func (w *loadoutWriter) write(bits, v int) {
	for i := range bits {
		w.char |= (v >> i & 1) << w.n
		if w.n++; w.n == 6 {
			w.b.WriteByte(loadoutChars[w.char])
			w.char, w.n = 0, 0
		}
	}
}

// flag writes b as one bit and returns it.
func (w *loadoutWriter) flag(b bool) bool {
	if b {
		w.write(1, 1)
	} else {
		w.write(1, 0)
	}
	return b
}

// String is what was written, the last character padded with zero bits.
func (w *loadoutWriter) String() string {
	if w.n > 0 {
		return w.b.String() + string(loadoutChars[w.char])
	}
	return w.b.String()
}
