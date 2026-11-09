package wcl

import (
	"context"
	"fmt"
	"slices"
)

// raidDifficulties are Warcraft Logs' raid difficulties tried for a boss's top kill, Mythic first.
var raidDifficulties = []int{5, 4, 3}

// RaidBossNPCs is each boss's NPC IDs, by the encounter name both Warcraft Logs and the combat log use: the actors
// Warcraft Logs marks as bosses in a top-ranked kill of the encounter. A boss Warcraft Logs doesn't list is left out.
func (c *Client) RaidBossNPCs(ctx context.Context, names []string) (map[string][]int, error) {
	zones, err := c.zones(ctx)
	if err != nil {
		return nil, err
	}
	npcs := map[string][]int{}
	for _, name := range names {
		id := 0
		for _, z := range zones { // newest zone last, so its encounter wins
			for _, e := range z.Encounters {
				if e.Name == name {
					id = e.ID
				}
			}
		}
		if id == 0 {
			continue
		}
		code, fight, err := c.topKill(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("top kill of %s: %w", name, err)
		}
		if code != "" {
			if npcs[name], err = c.bossActors(ctx, code, fight); err != nil {
				return nil, fmt.Errorf("boss actors of %s: %w", name, err)
			}
		}
	}
	return npcs, nil
}

// topKill is the report and fight of an encounter's top-ranked kill, on the hardest difficulty that has one; "" when
// none is ranked.
func (c *Client) topKill(ctx context.Context, encounter int) (string, int, error) {
	for _, difficulty := range raidDifficulties {
		var data struct {
			WorldData struct {
				Encounter struct {
					Rankings struct {
						Rankings []struct {
							Report struct {
								Code    string `json:"code"`
								FightID int    `json:"fightID"`
							} `json:"report"`
						} `json:"rankings"`
					} `json:"fightRankings"`
				} `json:"encounter"`
			} `json:"worldData"`
		}
		gql := `query($e: Int!, $d: Int!) { worldData { encounter(id: $e) { fightRankings(difficulty: $d) } } }`
		file := fmt.Sprintf("raid-kills-%d-%d.json", encounter, difficulty)
		if err := c.cachedQuery(ctx, file, 0, gql, map[string]any{"e": encounter, "d": difficulty}, &data); err != nil {
			return "", 0, err
		}
		if r := data.WorldData.Encounter.Rankings.Rankings; len(r) > 0 {
			return r[0].Report.Code, r[0].Report.FightID, nil
		}
	}
	return "", 0, nil
}

// bossActors is the NPC IDs of a report's boss actors that fought in the fight.
func (c *Client) bossActors(ctx context.Context, code string, fight int) ([]int, error) {
	var data struct {
		ReportData struct {
			Report struct {
				Fights []struct {
					EnemyNPCs []struct {
						GameID int `json:"gameID"`
					} `json:"enemyNPCs"`
				} `json:"fights"`
				MasterData struct {
					Actors []struct {
						GameID int `json:"gameID"`
					} `json:"actors"`
				} `json:"masterData"`
			} `json:"report"`
		} `json:"reportData"`
	}
	gql := `query($c: String!, $f: Int!) { reportData { report(code: $c) { fights(fightIDs: [$f]) {
		enemyNPCs { gameID } } masterData { actors(type: "NPC", subType: "Boss") { gameID } } } } }`
	file := fmt.Sprintf("raid-bosses-%s-%d.json", code, fight)
	if err := c.cachedQuery(ctx, file, 0, gql, map[string]any{"c": code, "f": fight}, &data); err != nil {
		return nil, err
	}
	var fought []int
	for _, f := range data.ReportData.Report.Fights {
		for _, n := range f.EnemyNPCs {
			fought = append(fought, n.GameID)
		}
	}
	var ids []int
	for _, a := range data.ReportData.Report.MasterData.Actors {
		if slices.Contains(fought, a.GameID) && !slices.Contains(ids, a.GameID) {
			ids = append(ids, a.GameID)
		}
	}
	return ids, nil
}
