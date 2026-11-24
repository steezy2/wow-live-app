// Package raidbots reads Raidbots' public game data, which cmd/talentref and cmd/simoptref turn into reference files.
package raidbots

import (
	"encoding/json"
	"fmt"
	"net/http"

	"wow-live/backend/models"
)

// Source is where Raidbots serves the live game's data.
const Source = "https://www.raidbots.com/static/data/live/"

// Get reads Raidbots' file name ("talents.json") into v.
func Get(client *http.Client, name string, v any) error {
	resp, err := client.Get(Source + name)
	if err != nil {
		return fmt.Errorf("reading %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reading %s: %s", name, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("reading %s: %w", name, err)
	}
	return nil
}

// Node is a talent node of Raidbots' files (a spec's trees, the Omnium Folio); only what the app draws is kept.
type Node struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	PosX     int    `json:"posX"`
	PosY     int    `json:"posY"`
	MaxRanks int    `json:"maxRanks"`
	SubTree  int    `json:"subTreeId"`
	Next     []int  `json:"next"`
	Entries  []struct {
		ID      int    `json:"id"`
		SpellID int    `json:"spellId"`
		Name    string `json:"name"`
		Icon    string `json:"icon"`
	} `json:"entries"`
}

// TalentNode is n in the shape the app draws, as a node of tree ("class", "spec", "hero" or "omnium").
func (n Node) TalentNode(tree string) models.TalentNode {
	node := models.TalentNode{ID: n.ID, Name: n.Name, Tree: tree, SubTree: n.SubTree, Choice: n.Type == "choice",
		X: n.PosX, Y: n.PosY, MaxRanks: n.MaxRanks, Next: append([]int{}, n.Next...), Entries: []models.TalentEntry{}}
	for _, e := range n.Entries {
		node.Entries = append(node.Entries, models.TalentEntry{ID: e.ID, SpellID: e.SpellID, Name: e.Name, Icon: e.Icon})
	}
	return node
}
