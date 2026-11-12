// Package raiderio reads Raider.IO's public API (https://raider.io/api): the season's dungeons with their Mythic+
// timers, and the week's affixes. Answers are cached in CacheDir.
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

// season is the current season as Raider.IO's static data has it: its slug, dungeons, and when it starts and ends
// by region (null for one not started or not ended).
type season struct {
	Slug     string                `json:"slug"`
	Dungeons []Dungeon             `json:"dungeons"`
	Starts   map[string]*time.Time `json:"starts"`
	Ends     map[string]*time.Time `json:"ends"`
}

// current is the expansion's newest season.
func (c Client) current(ctx context.Context) (season, error) {
	var data struct {
		Seasons []season `json:"seasons"`
	}
	params := url.Values{"expansion_id": {fmt.Sprint(Expansion)}}
	if err := c.get(ctx, "mythic-plus/static-data", params, fmt.Sprintf("static-%d.json", Expansion), seasonMaxAge,
		&data); err != nil {
		return season{}, err
	}
	if len(data.Seasons) == 0 {
		return season{}, fmt.Errorf("Raider.IO lists no season for expansion %d", Expansion)
	}
	return data.Seasons[0], nil
}

// Season is the current season's slug ("season-mn-2") and dungeons.
func (c Client) Season(ctx context.Context) (string, []Dungeon, error) {
	s, err := c.current(ctx)
	return s.Slug, s.Dungeons, err
}

// SeasonDates is when the current season starts and ends in region ("us"); the end is zero while it runs.
func (c Client) SeasonDates(ctx context.Context, region string) (start, end time.Time, err error) {
	s, err := c.current(ctx)
	if err != nil {
		return start, end, err
	}
	if s.Starts[region] == nil {
		return start, end, fmt.Errorf("Raider.IO has no start of %s in %s", s.Slug, region)
	}
	if s.Ends[region] != nil {
		end = *s.Ends[region]
	}
	return *s.Starts[region], end, nil
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
