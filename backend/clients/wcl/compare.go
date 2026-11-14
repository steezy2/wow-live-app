package wcl

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"wow-live/backend/models"
)

// RaidEncounters is the bosses' Warcraft Logs encounter IDs by name: the newest raid with one of the named bosses.
func (c *Client) RaidEncounters(ctx context.Context, bosses []string) (map[string]int, error) {
	zones, err := c.zones(ctx)
	if err != nil {
		return nil, err
	}
	z, ok := newestZone(zones, hasEncounter(bosses))
	if !ok {
		return map[string]int{}, nil
	}
	return encounterIDs(z), nil
}

// RankedRuns is page (from 1) of a spec's ranked characters on an encounter, 100 to a page, best first, and whether
// another page follows. Warcraft Logs' bracket for keystone level L is L-1; a raid asks by difficulty. Ranked reports
// don't change, so each page is kept for good.
func (c *Client) RankedRuns(ctx context.Context, q models.RankQuery, page int) ([]models.RankedRun, bool, error) {
	var data struct {
		WorldData struct {
			Encounter struct {
				Rankings struct {
					HasMorePages bool `json:"hasMorePages"`
					Rankings     []struct {
						Amount   float64 `json:"amount"`
						Duration int64   `json:"duration"`
						Name     string  `json:"name"`
						Server   struct {
							Name string `json:"name"`
						} `json:"server"`
						Report struct {
							Code    string `json:"code"`
							FightID int    `json:"fightID"`
						} `json:"report"`
					} `json:"rankings"`
				} `json:"characterRankings"`
			} `json:"encounter"`
		} `json:"worldData"`
	}
	bracket, which := "difficulty: $b", fmt.Sprintf("raid-%d", q.Difficulty)
	vars := map[string]any{"e": q.Encounter, "c": q.Class, "s": q.Spec, "p": page, "b": q.Difficulty}
	if q.Level > 0 {
		bracket, which, vars["b"] = "bracket: $b", fmt.Sprintf("mplus-%d", q.Level), q.Level-1
	}
	gql := `query($e: Int!, $c: String!, $s: String!, $b: Int!, $p: Int!) { worldData { encounter(id: $e) {
		characterRankings(className: $c, specName: $s, metric: ` + q.Metric + `, ` + bracket + `, page: $p) } } }`
	file := fmt.Sprintf("ranked-%d-%s-%s-%s-%s-%d.json", q.Encounter, which, q.Class, q.Spec, q.Metric, page)
	if err := c.cachedQuery(ctx, file, topTimesMaxAge, gql, vars, &data); err != nil {
		return nil, false, err
	}
	rankings := data.WorldData.Encounter.Rankings
	runs := make([]models.RankedRun, 0, len(rankings.Rankings))
	for _, r := range rankings.Rankings {
		runs = append(runs, models.RankedRun{Amount: r.Amount, DurationMS: r.Duration, Code: r.Report.Code,
			FightID: r.Report.FightID, Name: r.Name, Server: r.Server.Name})
	}
	return runs, rankings.HasMorePages, nil
}

// playerActor is a player of a report.
type playerActor struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Server string `json:"server"`
}

// findActor is the report's player called name on server, or the only one called name (the report may spell the
// server another way); false when there's none, or several and none of server.
func findActor(actors []playerActor, name, server string) (int, bool) {
	var named []playerActor
	for _, a := range actors {
		if a.Name == name {
			named = append(named, a)
		}
	}
	for _, a := range named {
		if a.Server == server {
			return a.ID, true
		}
	}
	if len(named) == 1 {
		return named[0].ID, true
	}
	return 0, false
}

// table is a report table: its entries (abilities), auras (buffs) and the time it covers.
type table struct {
	Data struct {
		TotalTime int64 `json:"totalTime"`
		Entries   []struct {
			GUID       int    `json:"guid"`
			Name       string `json:"name"`
			Total      int64  `json:"total"`
			HitCount   int    `json:"hitCount"`
			ActiveTime int64  `json:"activeTime"`
		} `json:"entries"`
		Auras []struct {
			GUID        int    `json:"guid"`
			Name        string `json:"name"`
			TotalUptime int64  `json:"totalUptime"`
		} `json:"auras"`
	} `json:"data"`
}

// RunPlayer is one player's numbers in a report fight (found by name on server): all of it, or its first windowMS.
// Two queries, both kept for good: the fight's span and its players, then one with a table each for the player's
// damage, healing, casts, buffs on themselves, damage taken and deaths.
func (c *Client) RunPlayer(ctx context.Context, code string, fightID int, name, server string, windowMS int64) (
	models.RankedPlayer, error) {
	var who struct {
		ReportData struct {
			Report struct {
				Fights []struct {
					StartTime int64 `json:"startTime"`
					EndTime   int64 `json:"endTime"`
				} `json:"fights"`
				MasterData struct {
					Actors []playerActor `json:"actors"`
				} `json:"masterData"`
			} `json:"report"`
		} `json:"reportData"`
	}
	gql := `query($r: String!, $f: Int!) { reportData { report(code: $r) { fights(fightIDs: [$f]) { startTime endTime }
		masterData { actors(type: "Player") { id name server } } } } }`
	if err := c.cachedQuery(ctx, fmt.Sprintf("players-%s-%d.json", code, fightID), 0, gql,
		map[string]any{"r": code, "f": fightID}, &who); err != nil {
		return models.RankedPlayer{}, err
	}
	report := who.ReportData.Report
	if len(report.Fights) == 0 {
		return models.RankedPlayer{}, fmt.Errorf("report %s has no fight %d", code, fightID)
	}
	actor, ok := findActor(report.MasterData.Actors, name, server)
	if !ok {
		return models.RankedPlayer{}, fmt.Errorf("%s of %s isn't in report %s", name, server, code)
	}
	from, to := report.Fights[0].StartTime, report.Fights[0].EndTime
	if windowMS > 0 {
		to = min(to, from+windowMS)
	}
	var data struct {
		ReportData struct {
			Report map[string]table `json:"report"`
		} `json:"reportData"`
	}
	args := `fightIDs: [$f], startTime: $from, endTime: $to`
	gql = `query($r: String!, $f: Int!, $a: Int!, $from: Float!, $to: Float!) { reportData { report(code: $r) {
		damage: table(dataType: DamageDone, sourceID: $a, ` + args + `)
		healing: table(dataType: Healing, sourceID: $a, ` + args + `)
		casts: table(dataType: Casts, sourceID: $a, ` + args + `)
		buffs: table(dataType: Buffs, sourceID: $a, targetID: $a, ` + args + `)
		taken: table(dataType: DamageTaken, targetID: $a, ` + args + `)
		deaths: table(dataType: Deaths, targetID: $a, ` + args + `) } } }`
	file := fmt.Sprintf("player-%s-%d-%d-%d.json", code, fightID, actor, to-from)
	vars := map[string]any{"r": code, "f": fightID, "a": actor, "from": from, "to": to}
	if err := c.cachedQuery(ctx, file, 0, gql, vars, &data); err != nil {
		return models.RankedPlayer{}, err
	}
	return rankedPlayer(data.ReportData.Report, to-from), nil
}

// rankedPlayer reads the six tables of a player's stretch into their numbers: abilities by spell ID with the hits of
// a cast table counting casts.
func rankedPlayer(tables map[string]table, durationMS int64) models.RankedPlayer {
	p := models.RankedPlayer{DurationMS: durationMS}
	abilities := map[int]*models.RankedAbility{}
	ability := func(id int, name string) *models.RankedAbility {
		if abilities[id] == nil {
			abilities[id] = &models.RankedAbility{ID: id, Name: name}
		}
		return abilities[id]
	}
	for _, e := range tables["damage"].Data.Entries {
		a := ability(e.GUID, e.Name)
		a.Damage, a.Hits = a.Damage+e.Total, a.Hits+e.HitCount
		p.Damage += e.Total
	}
	for _, e := range tables["healing"].Data.Entries {
		a := ability(e.GUID, e.Name)
		a.Healing, a.Hits = a.Healing+e.Total, a.Hits+e.HitCount
		p.Healing += e.Total
	}
	for _, e := range tables["casts"].Data.Entries {
		ability(e.GUID, e.Name).Casts += e.HitCount
	}
	for _, e := range tables["taken"].Data.Entries {
		p.Taken += e.Total
		p.ActiveMS = max(p.ActiveMS, e.ActiveTime)
	}
	p.Deaths = len(tables["deaths"].Data.Entries)
	for _, a := range tables["buffs"].Data.Auras {
		p.Buffs = append(p.Buffs, models.RankedBuff{ID: a.GUID, Name: a.Name, UptimeMS: a.TotalUptime})
	}
	for _, a := range abilities {
		p.Abilities = append(p.Abilities, *a)
	}
	slices.SortFunc(p.Abilities, func(a, b models.RankedAbility) int { return strings.Compare(a.Name, b.Name) })
	return p
}
