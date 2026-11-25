// Package raidbots reads Raidbots' public game data, which cmd/talentref, cmd/simoptref and cmd/simcraftref turn
// into reference files.
package raidbots

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"wow-live/backend/models"
)

// Source is where Raidbots serves the live game's data.
const Source = "https://www.raidbots.com/static/data/live/"

// Get reads Raidbots' file name ("talents.json") into v.
func Get(client *http.Client, name string, v any) error {
	body, err := Open(client, name)
	if err != nil {
		return err
	}
	defer body.Close()
	if err := json.NewDecoder(body).Decode(v); err != nil {
		return fmt.Errorf("reading %s: %w", name, err)
	}
	return nil
}

// Open is Raidbots' file name, for one too big to read whole (the caller closes it).
func Open(client *http.Client, name string) (io.ReadCloser, error) {
	resp, err := client.Get(Source + name)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", name, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("reading %s: %s", name, resp.Status)
	}
	return resp.Body, nil
}

// Node is a talent node of Raidbots' files (a spec's trees, the Omnium Folio); only what the app draws is kept.
type Node struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	PosX     int    `json:"posX"`
	PosY     int    `json:"posY"`
	MaxRanks int    `json:"maxRanks"`
	FreeNode bool   `json:"freeNode"`
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
		X: n.PosX, Y: n.PosY, MaxRanks: n.MaxRanks, Free: n.FreeNode, Next: append([]int{}, n.Next...),
		Entries: []models.TalentEntry{}}
	for _, e := range n.Entries {
		node.Entries = append(node.Entries, models.TalentEntry{ID: e.ID, SpellID: e.SpellID, Name: e.Name, Icon: e.Icon})
	}
	return node
}
