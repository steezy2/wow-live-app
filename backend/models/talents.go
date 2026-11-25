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
	// Order is every node the game's loadout string writes, in its order: the class's whole tree, other specs' nodes
	// and the hero tree choice too. Hash is that tree's hash (hex) the string carries, and Points the points each
	// section ("class", "spec", and "hero" for each hero tree) spends at the top level.
	Order  []int          `json:"order"`
	Hash   string         `json:"hash"`
	Points map[string]int `json:"points"`
	// HeroChoice is the node whose entries pick the hero tree; it is never drawn.
	HeroChoice TalentNode `json:"heroChoice"`
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
	Free     bool          `json:"free,omitempty"` // granted without a point (the loadout string writes it as not bought)
	Next     []int         `json:"next"`           // the nodes this one leads to
	Entries  []TalentEntry `json:"entries"`
	// ReqPoints is the points its section must have spent on nodes of a lower ReqPoints before it can be taken.
	ReqPoints int `json:"reqPoints,omitempty"`
}

// TalentEntry is a talent a node can hold: its ID, the spell it casts or gives and its icon (Wowhead's icon name).
// SubTree is the hero tree a TalentTree.HeroChoice entry picks (a hero node's SubTree).
type TalentEntry struct {
	ID      int    `json:"id"`
	SpellID int    `json:"spellID"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
	SubTree int    `json:"subTree,omitempty"`
}
