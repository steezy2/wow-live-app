package models

import "errors"

// ErrNoSuchIcon is returned for an icon name that isn't one, or that Wowhead doesn't have.
var ErrNoSuchIcon = errors.New("no such icon")

// TalentTrees is reference/talent-trees.json: every spec's talent tree, written by cmd/talentref.
type TalentTrees struct {
	Trees []TalentTree `json:"trees"`
}

// BySpec is the trees by spec ID.
func (t TalentTrees) BySpec() map[int]TalentTree {
	out := make(map[int]TalentTree, len(t.Trees))
	for _, tree := range t.Trees {
		out[tree.SpecID] = tree
	}
	return out
}

// TalentTree is one spec's talent layout: its class tree, spec tree and hero trees.
type TalentTree struct {
	SpecID int          `json:"specID"`
	Class  string       `json:"class"`
	Spec   string       `json:"spec"`
	Nodes  []TalentNode `json:"nodes"`
}

// TalentNode is a place in a talent tree. Picks name it by ID (COMBATANT_INFO's node) and the entry taken in it: one
// entry for a plain talent, two for a choice.
type TalentNode struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	// Tree is "class", "spec" or "hero" ("omnium" for the Omnium Folio's); SubTree says which of the hero trees a hero
	// node is in.
	Tree     string        `json:"tree" enums:"class,spec,hero,omnium"`
	SubTree  int           `json:"subTree,omitempty"`
	Choice   bool          `json:"choice,omitempty"`
	X        int           `json:"x"` // the tree's own grid, in thousandths of a node
	Y        int           `json:"y"`
	MaxRanks int           `json:"maxRanks"`
	Next     []int         `json:"next"` // the nodes this one leads to
	Entries  []TalentEntry `json:"entries"`
}

// TalentEntry is a talent a node can hold: its ID, the spell it casts or gives and its icon (Wowhead's icon name).
type TalentEntry struct {
	ID      int    `json:"id"`
	SpellID int    `json:"spellID"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
}
