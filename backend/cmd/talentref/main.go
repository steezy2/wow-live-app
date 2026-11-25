// Command talentref writes reference/talent-trees.json, every spec's talent tree (nodes, positions, links and the
// talents each holds) from Raidbots' talents.json, for the player review's talent tree, plus the node order, tree hash
// and points the game's loadout string needs (service/talentcode.go). Run it each patch and review the diff.
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

	"wow-live/backend/clients/raidbots"
	"wow-live/backend/config"
	"wow-live/backend/models"
)

// raidbotsSpec is a spec's trees in Raidbots' file.
type raidbotsSpec struct {
	ClassName  string          `json:"className"`
	SpecName   string          `json:"specName"`
	SpecID     int             `json:"specId"`
	ClassNodes []raidbots.Node `json:"classNodes"`
	SpecNodes  []raidbots.Node `json:"specNodes"`
	HeroNodes  []raidbots.Node `json:"heroNodes"`
	// SubTreeNodes is the hero tree choice; FullNodeOrder, TreeHash and PointLevels (the level each point comes at, by
	// section) are what the game's loadout string needs.
	SubTreeNodes  []raidbots.Node  `json:"subTreeNodes"`
	FullNodeOrder []int            `json:"fullNodeOrder"`
	TreeHash      string           `json:"treeHash"`
	PointLevels   map[string][]int `json:"pointLevels"`
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
	var specs []raidbotsSpec
	if err := raidbots.Get(&http.Client{Timeout: 60 * time.Second}, "talents.json", &specs); err != nil {
		return err
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
	t := models.TalentTree{SpecID: s.SpecID, Class: s.ClassName, Spec: s.SpecName, Nodes: []models.TalentNode{},
		Order: s.FullNodeOrder, Hash: s.TreeHash, Points: map[string]int{}}
	for section, levels := range s.PointLevels {
		t.Points[section] = len(levels)
	}
	if len(s.SubTreeNodes) == 1 {
		t.HeroChoice = s.SubTreeNodes[0].TalentNode("hero")
		t.HeroChoice.Choice = true
	}
	for tree, nodes := range map[string][]raidbots.Node{"class": s.ClassNodes, "spec": s.SpecNodes, "hero": s.HeroNodes} {
		for _, n := range nodes {
			t.Nodes = append(t.Nodes, n.TalentNode(tree))
		}
	}
	slices.SortFunc(t.Nodes, func(a, b models.TalentNode) int { return a.ID - b.ID })
	return t
}
