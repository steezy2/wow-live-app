// Package raiderio reads Raider.IO's public API (https://raider.io/api): the season's dungeons with their Mythic+
// timers and total enemy forces, and the week's affixes. Answers are cached in CacheDir.
package raiderio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"wow-live/backend/clients/cachefile"
)

const (
	// API is Raider.IO's API root.
	API = "https://raider.io/api/v1"
	// Expansion is Raider.IO's ID for the current expansion (Midnight), whose newest season lists the dungeons.
	Expansion = 11
	// seasonMaxAge and affixMaxAge: how long the cached season and affix answers are reused (affixes change weekly).
	seasonMaxAge = 7 * 24 * time.Hour
	affixMaxAge  = 6 * time.Hour
)

// Client calls Raider.IO, caching answers in CacheDir. BaseURL defaults to API; HTTP may be nil.
type Client struct {
	HTTP     *http.Client
	CacheDir string
	BaseURL  string
}

// Dungeon is a dungeon of the current season.
type Dungeon struct {
	MapID        int    `json:"challenge_mode_id"` // the combat log's challenge mode ID
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	TimerSeconds int    `json:"keystone_timer_seconds"`
}

// Season is the current season's slug ("season-mn-2") and dungeons.
func (c Client) Season(ctx context.Context) (string, []Dungeon, error) {
	var data struct {
		Seasons []struct {
			Slug     string    `json:"slug"`
			Dungeons []Dungeon `json:"dungeons"`
		} `json:"seasons"`
	}
	params := url.Values{"expansion_id": {fmt.Sprint(Expansion)}}
	if err := c.get(ctx, "mythic-plus/static-data", params, fmt.Sprintf("static-%d.json", Expansion), seasonMaxAge,
		&data); err != nil {
		return "", nil, err
	}
	if len(data.Seasons) == 0 {
		return "", nil, fmt.Errorf("Raider.IO lists no season for expansion %d", Expansion)
	}
	return data.Seasons[0].Slug, data.Seasons[0].Dungeons, nil
}

// Timers is each dungeon's Mythic+ time limit in ms, by its map ID, for the current season.
func (c Client) Timers(ctx context.Context) (map[int]int64, error) {
	_, dungeons, err := c.Season(ctx)
	if err != nil {
		return nil, err
	}
	timers := map[int]int64{}
	for _, d := range dungeons {
		timers[d.MapID] = int64(d.TimerSeconds) * 1000
	}
	return timers, nil
}

// TotalForces is the enemy forces a key of the dungeon needs, from the details of its top logged runs (0 when none
// of the first few was logged).
func (c Client) TotalForces(ctx context.Context, season, slug string) (int, error) {
	var top struct {
		Rankings []struct {
			Run struct {
				ID int `json:"keystone_run_id"`
			} `json:"run"`
		} `json:"rankings"`
	}
	params := url.Values{"season": {season}, "region": {"world"}, "dungeon": {slug}, "page": {"0"}}
	if err := c.get(ctx, "mythic-plus/runs", params, "runs-"+slug+".json", seasonMaxAge, &top); err != nil {
		return 0, err
	}
	for _, r := range top.Rankings[:min(5, len(top.Rankings))] {
		var run struct {
			Logged struct {
				Total int `json:"total_enemy_forces"`
			} `json:"logged_details"`
		}
		params := url.Values{"season": {season}, "id": {fmt.Sprint(r.Run.ID)}}
		if err := c.get(ctx, "mythic-plus/run-details", params, fmt.Sprintf("run-%d.json", r.Run.ID), seasonMaxAge,
			&run); err != nil {
			return 0, err
		}
		if run.Logged.Total > 0 {
			return run.Logged.Total, nil
		}
	}
	return 0, nil
}

// Affixes is this week's affix names by ID, for a region ("us").
func (c Client) Affixes(ctx context.Context, region string) (map[int]string, error) {
	var data struct {
		Details []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"affix_details"`
	}
	params := url.Values{"region": {region}, "locale": {"en"}}
	if err := c.get(ctx, "mythic-plus/affixes", params, "affixes-"+region+".json", affixMaxAge, &data); err != nil {
		return nil, err
	}
	names := map[int]string{}
	for _, a := range data.Details {
		names[a.ID] = a.Name
	}
	return names, nil
}

// get decodes an API answer, from the cache file when it's fresh enough.
func (c Client) get(ctx context.Context, path string, params url.Values, file string, maxAge time.Duration,
	out any) error {
	base := c.BaseURL
	if base == "" {
		base = API
	}
	cached := filepath.Join(c.CacheDir, file)
	if err := cachefile.Fresh(ctx, c.HTTP, base+"/"+path+"?"+params.Encode(), cached, maxAge); err != nil {
		return fmt.Errorf("Raider.IO %s: %w", path, err)
	}
	data, err := os.ReadFile(cached)
	if err != nil {
		return fmt.Errorf("reading %s: %w", cached, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("reading Raider.IO's %s answer: %w", path, err)
	}
	return nil
}
