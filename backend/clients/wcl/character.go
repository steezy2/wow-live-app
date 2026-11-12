package wcl

import (
	"context"
	"fmt"
	"strings"

	"wow-live/backend/models"
)

// Realms is a region's realms ("us"), by normalized name (models.NormalizeRealm) to Warcraft Logs' slug, cached for
// zonesMaxAge.
func (c *Client) Realms(ctx context.Context, region string) (map[string]string, error) {
	var regions struct {
		WorldData struct {
			Regions []struct {
				ID          int    `json:"id"`
				CompactName string `json:"compactName"`
			} `json:"regions"`
		} `json:"worldData"`
	}
	if err := c.cachedQuery(ctx, "regions.json", zonesMaxAge, "{ worldData { regions { id compactName } } }", nil,
		&regions); err != nil {
		return nil, fmt.Errorf("listing regions: %w", err)
	}
	id := 0
	for _, r := range regions.WorldData.Regions {
		if strings.EqualFold(r.CompactName, region) {
			id = r.ID
		}
	}
	if id == 0 {
		return nil, fmt.Errorf("Warcraft Logs has no region %q", region)
	}
	realms := map[string]string{}
	gql := `query($r: Int!, $p: Int!) { worldData { region(id: $r) { servers(limit: 100, page: $p) {
		data { name slug } has_more_pages } } } }`
	for page := 1; ; page++ {
		var data struct {
			WorldData struct {
				Region struct {
					Servers struct {
						Data []struct {
							Name string `json:"name"`
							Slug string `json:"slug"`
						} `json:"data"`
						HasMore bool `json:"has_more_pages"`
					} `json:"servers"`
				} `json:"region"`
			} `json:"worldData"`
		}
		file := fmt.Sprintf("realms-%s-%d.json", region, page)
		if err := c.cachedQuery(ctx, file, zonesMaxAge, gql, map[string]any{"r": id, "p": page}, &data); err != nil {
			return nil, fmt.Errorf("listing %s realms: %w", region, err)
		}
		for _, s := range data.WorldData.Region.Servers.Data {
			realms[models.NormalizeRealm(s.Name)] = s.Slug
		}
		if !data.WorldData.Region.Servers.HasMore {
			return realms, nil
		}
	}
}

// maxReportPages is the last page of a character's recent reports Warcraft Logs serves ("The page argument must be a
// value between 1 and 5"), so a listing holds at most the newest 150 reports.
const maxReportPages = 5

// RecentReports is one page (30) of the reports a character appears in, newest first, and whether another page
// follows (never after maxReportPages). Players are named from each report's player list. Cached for models.ReportListMaxAge. A page is 30 because
// 100 reports' fights and players are past Warcraft Logs' query complexity limit (50000; 100 came to 110702).
func (c *Client) RecentReports(ctx context.Context, region, slug, name string, page int) (
	[]models.CharacterReport, bool, error) {
	var data struct {
		CharacterData struct {
			Character *struct {
				RecentReports struct {
					Data []struct {
						Code      string `json:"code"`
						StartTime int64  `json:"startTime"`
						EndTime   int64  `json:"endTime"`
						Zone      *struct {
							ID int `json:"id"`
						} `json:"zone"`
						Fights []struct {
							models.ReportFight
							FriendlyPlayers []int `json:"friendlyPlayers"`
						} `json:"fights"`
						MasterData struct {
							Actors []struct {
								ID   int    `json:"id"`
								Name string `json:"name"`
							} `json:"actors"`
						} `json:"masterData"`
					} `json:"data"`
					HasMore bool `json:"has_more_pages"`
				} `json:"recentReports"`
			} `json:"character"`
		} `json:"characterData"`
	}
	gql := `query($n: String!, $s: String!, $r: String!, $p: Int!) { characterData {
		character(name: $n, serverSlug: $s, serverRegion: $r) { recentReports(limit: 30, page: $p) {
			data { code startTime endTime zone { id } fights { id encounterID keystoneLevel difficulty kill startTime
				endTime friendlyPlayers } masterData { actors(type: "Player") { id name } } } has_more_pages } } } }`
	file := fmt.Sprintf("reports-%s-%s-%s-%d.json", region, slug, name, page)
	if err := c.cachedQuery(ctx, file, models.ReportListMaxAge, gql,
		map[string]any{"n": name, "s": slug, "r": region, "p": page}, &data); err != nil {
		return nil, false, fmt.Errorf("listing %s's reports: %w", name, err)
	}
	character := data.CharacterData.Character
	if character == nil {
		return nil, false, fmt.Errorf("Warcraft Logs has no character %s on %s-%s", name, region, slug)
	}
	var reports []models.CharacterReport
	for _, r := range character.RecentReports.Data {
		names := map[int]string{}
		for _, a := range r.MasterData.Actors {
			names[a.ID] = a.Name
		}
		report := models.CharacterReport{Code: r.Code, StartTime: r.StartTime, EndTime: r.EndTime}
		if r.Zone != nil {
			report.ZoneID = r.Zone.ID
		}
		for _, f := range r.Fights {
			for _, id := range f.FriendlyPlayers {
				if name, ok := names[id]; ok {
					f.Players = append(f.Players, name)
				}
			}
			report.Fights = append(report.Fights, f.ReportFight)
		}
		reports = append(reports, report)
	}
	return reports, character.RecentReports.HasMore && page < maxReportPages, nil
}

// CurrentZones are the newest Mythic+ zone and the newest raid zone (the one with a boss named in raidBosses, as
// RaidBosses picks it), with their encounters by name; a zone not found is 0.
func (c *Client) CurrentZones(ctx context.Context, raidBosses []string) (models.WCLZones, error) {
	zones, err := c.zones(ctx)
	if err != nil {
		return models.WCLZones{}, err
	}
	current := models.WCLZones{Encounters: map[string]int{}}
	for _, pick := range []struct {
		match func(zone) bool
		id    *int
	}{{isMPlus, &current.MPlusID}, {hasEncounter(raidBosses), &current.RaidID}} {
		if z, ok := newestZone(zones, pick.match); ok {
			*pick.id = z.ID
			for name, id := range encounterIDs(z) {
				current.Encounters[name] = id
			}
		}
	}
	return current, nil
}
