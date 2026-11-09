package wcl

import (
	"context"
	"fmt"
	"slices"

	"wow-live/backend/models"
)

// raidDifficulties are Warcraft Logs' raid difficulties tried for a boss's top kill, Mythic first.
var raidDifficulties = []int{5, 4, 3}

// RaidBosses is every boss of the raid that has the named encounters (the newest Warcraft Logs zone with one of
// them), by the encounter name both Warcraft Logs and the combat log use, so a boss the caller didn't name is in too.
// Each has its NPC IDs and the uiMaps it's fought on (Maps has only their IDs), from a top-ranked kill: the actors
// Warcraft Logs marks as bosses, and the fight's maps. A boss with no ranked kill has only its name.
func (c *Client) RaidBosses(ctx context.Context, names []string) (map[string]models.RaidBoss, error) {
	zones, err := c.zones(ctx)
	if err != nil {
		return nil, err
	}
	bosses := map[string]models.RaidBoss{}
	for _, z := range slices.Backward(zones) {
		if !slices.ContainsFunc(z.Encounters, func(e encounter) bool { return slices.Contains(names, e.Name) }) {
			continue
		}
		for _, e := range z.Encounters {
			code, fight, err := c.topKill(ctx, e.ID)
			if err != nil {
				return nil, fmt.Errorf("top kill of %s: %w", e.Name, err)
			}
			boss := models.RaidBoss{Name: e.Name}
			if code != "" {
				if boss, err = c.bossFight(ctx, code, fight); err != nil {
					return nil, fmt.Errorf("the top kill of %s: %w", e.Name, err)
				}
				boss.Name = e.Name
			}
			bosses[e.Name] = boss
		}
		break
	}
	return bosses, nil
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

// bossFight is a report fight's boss actors (NPC IDs) and its maps.
func (c *Client) bossFight(ctx context.Context, code string, fight int) (models.RaidBoss, error) {
	var data struct {
		ReportData struct {
			Report struct {
				Fights []struct {
					EnemyNPCs []struct {
						GameID int `json:"gameID"`
					} `json:"enemyNPCs"`
					Maps []struct {
						ID int `json:"id"`
					} `json:"maps"`
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
		enemyNPCs { gameID } maps { id } } masterData { actors(type: "NPC", subType: "Boss") { gameID } } } } }`
	file := fmt.Sprintf("raid-fight-%s-%d.json", code, fight)
	if err := c.cachedQuery(ctx, file, 0, gql, map[string]any{"c": code, "f": fight}, &data); err != nil {
		return models.RaidBoss{}, err
	}
	var fought []int
	var boss models.RaidBoss
	for _, f := range data.ReportData.Report.Fights {
		for _, n := range f.EnemyNPCs {
			fought = append(fought, n.GameID)
		}
		for _, m := range f.Maps {
			boss.Maps = append(boss.Maps, models.RaidMap{UIMapID: m.ID})
		}
	}
	for _, a := range data.ReportData.Report.MasterData.Actors {
		if slices.Contains(fought, a.GameID) && !slices.Contains(boss.NPCs, a.GameID) {
			boss.NPCs = append(boss.NPCs, a.GameID)
		}
	}
	return boss, nil
}
