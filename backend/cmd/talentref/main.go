// Command talentref writes reference/talent-trees.json, every spec's talent tree (nodes, positions, links and the
// talents each holds) from Raidbots' talents.json, for the player review's talent tree. Run it each patch and review
// the diff.
//
// Usage, from backend/:
//
//	go run ./cmd/talentref
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"time"

	"wow-live/backend/config"
	"wow-live/backend/models"
)

const source = "https://www.raidbots.com/static/data/live/talents.json"

// raidbotsNode is a node of a Raidbots tree; only what the review draws is kept.
type raidbotsNode struct {
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

// raidbotsSpec is a spec's trees in Raidbots' file.
type raidbotsSpec struct {
	ClassName  string         `json:"className"`
	SpecName   string         `json:"specName"`
	SpecID     int            `json:"specId"`
	ClassNodes []raidbotsNode `json:"classNodes"`
	SpecNodes  []raidbotsNode `json:"specNodes"`
	HeroNodes  []raidbotsNode `json:"heroNodes"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	root, err := config.RootFromWorkingDir()
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(source)
	if err != nil {
		return fmt.Errorf("reading %s: %w", source, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reading %s: %s", source, resp.Status)
	}
	var specs []raidbotsSpec
	if err := json.NewDecoder(resp.Body).Decode(&specs); err != nil {
		return fmt.Errorf("reading %s: %w", source, err)
	}
	slices.SortFunc(specs, func(a, b raidbotsSpec) int { return a.SpecID - b.SpecID })
	var out bytes.Buffer
	out.WriteString("{\"trees\":[\n")
	for i, s := range specs {
		line, err := json.Marshal(treeOf(s))
		if err != nil {
			return err
		}
		out.Write(line)
		if i < len(specs)-1 {
			out.WriteString(",")
		}
		out.WriteString("\n")
	}
	out.WriteString("]}\n")
	path := config.TalentTreesPath(root)
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Printf("wrote %s: %d specs\n", path, len(specs))
	return nil
}

// treeOf is a spec's tree in the shape the app reads: one node list, each node saying which tree it is in.
func treeOf(s raidbotsSpec) models.TalentTree {
	t := models.TalentTree{SpecID: s.SpecID, Class: s.ClassName, Spec: s.SpecName, Nodes: []models.TalentNode{}}
	for tree, nodes := range map[string][]raidbotsNode{"class": s.ClassNodes, "spec": s.SpecNodes, "hero": s.HeroNodes} {
		for _, n := range nodes {
			node := models.TalentNode{ID: n.ID, Name: n.Name, Tree: tree, SubTree: n.SubTree, Choice: n.Type == "choice",
				X: n.PosX, Y: n.PosY, MaxRanks: n.MaxRanks, Next: append([]int{}, n.Next...), Entries: []models.TalentEntry{}}
			for _, e := range n.Entries {
				node.Entries = append(node.Entries, models.TalentEntry{ID: e.ID, SpellID: e.SpellID, Name: e.Name, Icon: e.Icon})
			}
			t.Nodes = append(t.Nodes, node)
		}
	}
	slices.SortFunc(t.Nodes, func(a, b models.TalentNode) int { return a.ID - b.ID })
	return t
}
