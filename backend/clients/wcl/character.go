package wcl

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"wow-live/backend/clients/cachefile"
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
// follows (never after maxReportPages). Players are named from each report's player list. The page's list is cached
// for models.ReportListMaxAge and each report's fights and players for good (by its end, which a report still being
// logged moves), since they cost most of the points: so listing again only spends points on new reports.
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
					} `json:"data"`
					HasMore bool `json:"has_more_pages"`
				} `json:"recentReports"`
			} `json:"character"`
		} `json:"characterData"`
	}
	gql := `query($n: String!, $s: String!, $r: String!, $p: Int!) { characterData {
		character(name: $n, serverSlug: $s, serverRegion: $r) { recentReports(limit: 30, page: $p) {
			data { code startTime endTime zone { id } } has_more_pages } } } }`
	file := fmt.Sprintf("reportlist-%s-%s-%s-%d.json", region, slug, name, page)
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
		report := models.CharacterReport{Code: r.Code, StartTime: r.StartTime, EndTime: r.EndTime}
		if r.Zone != nil {
			report.ZoneID = r.Zone.ID
		}
		reports = append(reports, report)
	}
	if err := c.addFights(ctx, reports); err != nil {
		return nil, false, fmt.Errorf("listing %s's fights: %w", name, err)
	}
	return reports, character.RecentReports.HasMore && page < maxReportPages, nil
}

// reportFights is a report's fights and players, as Warcraft Logs answers them.
type reportFights struct {
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
}

// addFights fills in reports' fights with their players' names, from the cache or, for those not in it, from one
// query.
func (c *Client) addFights(ctx context.Context, reports []models.CharacterReport) error {
	path := func(r models.CharacterReport) string {
		return filepath.Join(c.CacheDir, "wcl-mplus", "reports", fmt.Sprintf("%s-%d.json", r.Code, r.EndTime))
	}
	found := make([]reportFights, len(reports))
	var params, fields []string
	vars := map[string]any{}
	for i, r := range reports {
		if !readCached(path(r), 0, &found[i]) {
			params = append(params, fmt.Sprintf("$c%d: String!", i))
			fields = append(fields, fmt.Sprintf(`r%d: report(code: $c%d) { fights { id encounterID keystoneLevel
				difficulty kill startTime endTime friendlyPlayers } masterData { actors(type: "Player") { id name } } }`,
				i, i))
			vars[fmt.Sprintf("c%d", i)] = r.Code
		}
	}
	if len(fields) > 0 {
		var data struct {
			ReportData map[string]reportFights `json:"reportData"`
		}
		gql := fmt.Sprintf("query(%s) { reportData { %s } }", strings.Join(params, ", "), strings.Join(fields, " "))
		if err := c.Query(ctx, gql, vars, &data); err != nil {
			return err
		}
		for i, r := range reports {
			if got, ok := data.ReportData[fmt.Sprintf("r%d", i)]; ok {
				found[i] = got
				if err := cachefile.WriteJSON(path(r), got); err != nil {
					return err
				}
			}
		}
	}
	for i, f := range found {
		names := map[int]string{}
		for _, a := range f.MasterData.Actors {
			names[a.ID] = a.Name
		}
		for _, fight := range f.Fights {
			for _, id := range fight.FriendlyPlayers {
				if name, ok := names[id]; ok {
					fight.Players = append(fight.Players, name)
				}
			}
			reports[i].Fights = append(reports[i].Fights, fight.ReportFight)
		}
	}
	return nil
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
