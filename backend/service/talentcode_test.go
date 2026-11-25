package service

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"wow-live/backend/models"
)

// fixtureTalents are testdata/simc-export.txt's loadouts: the one worn, then the saved "M+" one.
func fixtureTalents(t *testing.T) []string {
	t.Helper()
	var out []string
	for line := range strings.Lines(fixtureSimC(t)) {
		if s, ok := strings.CutPrefix(strings.TrimPrefix(line, "# "), "talents="); ok {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func referenceTrees(t *testing.T) map[int]models.TalentTree {
	t.Helper()
	raw, err := os.ReadFile("../../reference/talent-trees.json")
	if err != nil {
		t.Fatal(err)
	}
	var trees models.TalentTrees
	if err := json.Unmarshal(raw, &trees); err != nil {
		t.Fatal(err)
	}
	return trees.BySpec()
}

// TestTalentStringRoundTrip: a real export's strings read into valid picks and write back the same.
func TestTalentStringRoundTrip(t *testing.T) {
	trees := referenceTrees(t)
	for _, s := range fixtureTalents(t) {
		picks := profileTalents(trees, s)
		if len(picks) == 0 {
			t.Fatalf("%s: no picks", s)
		}
		if err := validTalents(trees[262], picks); err != nil {
			t.Errorf("valid: %v", err)
		}
		if got, err := EncodeTalents(trees[262], picks); err != nil || got != s {
			t.Errorf("encoded %q, %v; want %q", got, err, s)
		}
	}
}

// TestDecodeTalentsRefuses: a string of another version, spec or tree (another node order), or cut or grown, errors,
// and an export carrying it has no picks.
func TestDecodeTalentsRefuses(t *testing.T) {
	trees := referenceTrees(t)
	elemental, wornTalents := trees[262], fixtureTalents(t)[0]
	reordered := elemental
	reordered.Hash = "00112233445566778899aabbccddeeff"
	for name, tc := range map[string]struct {
		tree models.TalentTree
		s    string
	}{
		"version 1":    {elemental, "B" + wornTalents[1:]},
		"another spec": {trees[263], wornTalents},
		"another tree": {reordered, wornTalents},
		"cut":          {elemental, wornTalents[:len(wornTalents)-4]},
		"grown":        {elemental, wornTalents + "AA"},
		"not base64":   {elemental, wornTalents[:20] + "!" + wornTalents[21:]},
	} {
		if picks, err := DecodeTalents(tc.tree, tc.s); err == nil {
			t.Errorf("%s: decoded %d picks", name, len(picks))
		}
	}
	if picks := profileTalents(trees, "B"+wornTalents[1:]); picks != nil {
		t.Errorf("profile picks %v", picks)
	}
}

// TestValidTalentsRefuses: a node taken without one leading to it at its full rank, or more points than a section
// has, is refused.
func TestValidTalentsRefuses(t *testing.T) {
	tree := models.TalentTree{Points: map[string]int{"class": 4}, Nodes: []models.TalentNode{
		{ID: 1, Tree: "class", MaxRanks: 1, Next: []int{2}}, {ID: 2, Tree: "class", MaxRanks: 2, Next: []int{4}},
		{ID: 4, Tree: "class", MaxRanks: 1},
		{ID: 3, Tree: "class", MaxRanks: 1, ReqPoints: 1}}}
	for name, tc := range map[string]struct {
		picks []models.TalentPick
		ok    bool
	}{
		"taken in order":  {[]models.TalentPick{{NodeID: 1, Rank: 1}, {NodeID: 2, Rank: 1}}, true},
		"no parent":       {[]models.TalentPick{{NodeID: 2, Rank: 1}, {NodeID: 3, Rank: 1}}, false},
		"parent not full": {[]models.TalentPick{{NodeID: 1, Rank: 1}, {NodeID: 2, Rank: 1}, {NodeID: 4, Rank: 1}}, false},
		"parent full":     {[]models.TalentPick{{NodeID: 1, Rank: 1}, {NodeID: 2, Rank: 2}, {NodeID: 4, Rank: 1}}, true},
		"over points": {[]models.TalentPick{{NodeID: 1, Rank: 1}, {NodeID: 2, Rank: 2}, {NodeID: 4, Rank: 1},
			{NodeID: 3, Rank: 1}}, false},
		"past its gate":   {[]models.TalentPick{{NodeID: 1, Rank: 1}, {NodeID: 3, Rank: 1}}, true},
		"before its gate": {[]models.TalentPick{{NodeID: 3, Rank: 1}}, false},
	} {
		if err := validTalents(tree, tc.picks); (err == nil) != tc.ok {
			t.Errorf("%s: %v", name, err)
		}
	}
}
