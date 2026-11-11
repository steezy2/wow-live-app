package wcl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

// zonesMaxAge is how long the list of zones (and so the season's dungeons) is reused. Rankings and run totals are
// kept for good: delete logs/cache/wcl-mplus/ for a new season.
const zonesMaxAge = 7 * 24 * time.Hour

// MPlusEncounters is the newest Mythic+ zone's dungeons by name, as Warcraft Logs encounter IDs.
func (c *Client) MPlusEncounters(ctx context.Context) (map[string]int, error) {
	var data struct {
		WorldData struct {
			Zones []struct {
				ID         int    `json:"id"`
				Name       string `json:"name"`
				Encounters []struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				} `json:"encounters"`
			} `json:"zones"`
		} `json:"worldData"`
	}
	if err := c.cachedQuery(ctx, "zones.json", zonesMaxAge, "{ worldData { zones { id name encounters { id name } } } }",
		nil, &data); err != nil {
		return nil, err
	}
	newest := -1
	for i, z := range data.WorldData.Zones {
		if strings.HasPrefix(z.Name, "Mythic+") && (newest < 0 || z.ID > data.WorldData.Zones[newest].ID) {
			newest = i
		}
	}
	if newest < 0 {
		return nil, fmt.Errorf("Warcraft Logs lists no Mythic+ zone")
	}
	encounters := map[string]int{}
	for _, e := range data.WorldData.Zones[newest].Encounters {
		encounters[e.Name] = e.ID
	}
	return encounters, nil
}

// MPlusRankings is the top runs of a class and spec ("Paladin", "Holy") in a dungeon at a keystone level by "dps" or
// "hps". Warcraft Logs' bracket for level L is L-1 (checked Nov 5: bracket 13 gives +14 runs).
func (c *Client) MPlusRankings(ctx context.Context, encounter, level int, class, spec, metric string) (
	[]models.MPlusRanking, error) {
	var data struct {
		WorldData struct {
			Encounter struct {
				Rankings struct {
					Rankings []struct {
						Amount   float64 `json:"amount"`
						Duration int64   `json:"duration"`
						Report   struct {
							Code    string `json:"code"`
							FightID int    `json:"fightID"`
						} `json:"report"`
					} `json:"rankings"`
				} `json:"characterRankings"`
			} `json:"encounter"`
		} `json:"worldData"`
	}
	gql := `query($e: Int!, $c: String!, $s: String!, $b: Int!) { worldData { encounter(id: $e) {
		characterRankings(className: $c, specName: $s, metric: ` + metric + `, bracket: $b) } } }`
	file := fmt.Sprintf("rankings-%d-%d-%s-%s-%s.json", encounter, level, class, spec, metric)
	if err := c.cachedQuery(ctx, file, 0, gql, map[string]any{"e": encounter, "c": class, "s": spec, "b": level - 1},
		&data); err != nil {
		return nil, err
	}
	var runs []models.MPlusRanking
	for _, r := range data.WorldData.Encounter.Rankings.Rankings {
		runs = append(runs, models.MPlusRanking{Amount: r.Amount, DurationMS: r.Duration, Code: r.Report.Code,
			FightID: r.Report.FightID})
	}
	return runs, nil
}

// Run is a ranked run's party damage taken (from its fight's summary table) and how long each boss it killed took
// (its dungeon pulls with an encounter), in one query.
func (c *Client) Run(ctx context.Context, code string, fightID int) (models.RunSummary, error) {
	var data struct {
		ReportData struct {
			Report struct {
				Table struct {
					Data struct {
						DamageTaken []struct {
							Total int64 `json:"total"`
						} `json:"damageTaken"`
					} `json:"data"`
				} `json:"table"`
				Fights []struct {
					DungeonPulls []struct {
						Name        string `json:"name"`
						EncounterID int    `json:"encounterID"`
						StartTime   int64  `json:"startTime"`
						EndTime     int64  `json:"endTime"`
						Kill        bool   `json:"kill"`
					} `json:"dungeonPulls"`
				} `json:"fights"`
			} `json:"report"`
		} `json:"reportData"`
	}
	gql := `query($r: String!, $f: Int!) { reportData { report(code: $r) { table(fightIDs: [$f], dataType: Summary) ` +
		`fights(fightIDs: [$f]) { dungeonPulls { name encounterID startTime endTime kill } } } } }`
	if err := c.cachedQuery(ctx, fmt.Sprintf("run-%s-%d.json", code, fightID), 0, gql,
		map[string]any{"r": code, "f": fightID}, &data); err != nil {
		return models.RunSummary{}, err
	}
	run := models.RunSummary{BossMS: map[string]int64{}}
	for _, a := range data.ReportData.Report.Table.Data.DamageTaken {
		run.DamageTaken += a.Total
	}
	for _, f := range data.ReportData.Report.Fights {
		for _, p := range f.DungeonPulls {
			if p.EncounterID != 0 && p.Kill {
				run.BossMS[p.Name] = p.EndTime - p.StartTime
			}
		}
	}
	return run, nil
}

// PointsLeft is how many of this hour's points the account has left.
func (c *Client) PointsLeft(ctx context.Context) (float64, error) {
	budget, err := c.RateLimit(ctx)
	if err != nil {
		return 0, err
	}
	return budget.LimitPerHour - budget.PointsSpentThisHour, nil
}

// cachedQuery answers a query from logs/cache/wcl-mplus/file when it's younger than maxAge (0: any age), else asks
// Warcraft Logs and keeps the answer.
func (c *Client) cachedQuery(ctx context.Context, file string, maxAge time.Duration, gql string,
	variables map[string]any, out any) error {
	path := filepath.Join(c.CacheDir, "wcl-mplus", file)
	if info, err := os.Stat(path); err == nil && (maxAge == 0 || time.Since(info.ModTime()) < maxAge) {
		if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, out) == nil {
			return nil
		}
	}
	if err := c.Query(ctx, gql, variables, out); err != nil {
		return err
	}
	return cachefile.WriteJSON(path, out)
}
