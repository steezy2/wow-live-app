package wcl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"wow-live/backend/clients/cachefile"
)

// eventsPerPage is how many events one events query asks for, Warcraft Logs' most.
const eventsPerPage = 10000

// fightMeta is what a report fight's combat log needs besides its events: the report's start (ms since the epoch)
// and region, the fight, and the report's actors and abilities.
type fightMeta struct {
	StartTime int64 `json:"startTime"`
	Region    struct {
		CompactName string `json:"compactName"`
	} `json:"region"`
	Fights     []fightInfo `json:"fights"`
	MasterData struct {
		Actors    []actor   `json:"actors"`
		Abilities []ability `json:"abilities"`
	} `json:"masterData"`
}

// fightInfo is one report fight: a raid pull, or a key with its dungeon pulls. Times are ms since the report's start.
type fightInfo struct {
	ID              int     `json:"id"`
	EncounterID     int     `json:"encounterID"`
	Name            string  `json:"name"`
	StartTime       int64   `json:"startTime"`
	EndTime         int64   `json:"endTime"`
	Kill            bool    `json:"kill"`
	Difficulty      int     `json:"difficulty"`
	Size            int     `json:"size"`
	KeystoneLevel   int     `json:"keystoneLevel"`
	KeystoneAffixes []int   `json:"keystoneAffixes"`
	KeystoneTime    int64   `json:"keystoneTime"`
	KeystoneBonus   int     `json:"keystoneBonus"`
	GameZone        idName  `json:"gameZone"`
	DungeonPulls    []pull  `json:"dungeonPulls"`
	FriendlyNPCs    []actor `json:"friendlyNPCs"`
	EnemyPets       []actor `json:"enemyPets"`
}

// idName is a Warcraft Logs object's ID and name (a zone).
type idName struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// pull is one pull of a key; a boss pull has an encounter ID.
type pull struct {
	EncounterID int    `json:"encounterID"`
	Name        string `json:"name"`
	StartTime   int64  `json:"startTime"`
	EndTime     int64  `json:"endTime"`
	Kill        bool   `json:"kill"`
}

// actor is a unit of a report: a player, a pet (with its owner's actor ID) or an NPC (gameID is its NPC ID).
type actor struct {
	ID       int    `json:"id"`
	GameID   int64  `json:"gameID"`
	Name     string `json:"name"`
	Server   string `json:"server"`
	Type     string `json:"type"`
	PetOwner int    `json:"petOwner"`
}

// ability is a spell of a report; type is its school, a decimal number.
type ability struct {
	GameID int    `json:"gameID"`
	Name   string `json:"name"`
	Type   string `json:"type"`
}

// eventPage is one page of a fight's events as Warcraft Logs answers it, kept as fetched; no next page is the last.
type eventPage struct {
	Data              json.RawMessage `json:"data"`
	NextPageTimestamp *int64          `json:"nextPageTimestamp"`
}

// fightMeta is a report fight's metadata, from the page cache when it's there.
func (c *Client) fightMeta(ctx context.Context, code string, fightID int) (fightMeta, error) {
	var data struct {
		ReportData struct {
			Report *fightMeta `json:"report"`
		} `json:"reportData"`
	}
	gql := `query($c: String!, $f: Int!) { reportData { report(code: $c) { startTime region { compactName }
		fights(fightIDs: [$f]) { id encounterID name startTime endTime kill difficulty size keystoneLevel
			keystoneAffixes keystoneTime keystoneBonus gameZone { id name } friendlyNPCs { id } enemyPets { id }
			dungeonPulls { encounterID name startTime endTime kill } }
		masterData { actors { id gameID name server type petOwner } abilities { gameID name type } } } } }`
	path := c.pagePath(code, fightID, "meta")
	if err := c.cachedPage(ctx, path, gql, map[string]any{"c": code, "f": fightID}, &data); err != nil {
		return fightMeta{}, fmt.Errorf("fight %d of report %s: %w", fightID, code, err)
	}
	if r := data.ReportData.Report; r != nil && len(r.Fights) == 1 {
		return *r, nil
	}
	os.Remove(path)
	return fightMeta{}, fmt.Errorf("report %s has no fight %d", code, fightID)
}

// fightEvents hands each page of a fight's events to use, in order. Every page is cached under the page cache as
// it's fetched, so a fetch stopped by ErrRateLimited (returned wrapped, for errors.Is) or a restart starts again at
// the first page not fetched yet.
func (c *Client) fightEvents(ctx context.Context, code string, meta fightMeta, use func([]event) error) error {
	fight := meta.Fights[0]
	gql := `query($c: String!, $f: Int!, $s: Float!, $e: Float!) { reportData { report(code: $c) {
		events(fightIDs: [$f], startTime: $s, endTime: $e, includeResources: true, limit: ` +
		fmt.Sprint(eventsPerPage) + `) { data nextPageTimestamp } } } }`
	for from := fight.StartTime; ; {
		var data struct {
			ReportData struct {
				Report struct {
					Events eventPage `json:"events"`
				} `json:"report"`
			} `json:"reportData"`
		}
		vars := map[string]any{"c": code, "f": fight.ID, "s": from, "e": fight.EndTime}
		if err := c.cachedPage(ctx, c.pagePath(code, fight.ID, fmt.Sprint(from)), gql, vars, &data); err != nil {
			return fmt.Errorf("events of fight %d of report %s from %d: %w", fight.ID, code, from, err)
		}
		page := data.ReportData.Report.Events
		var events []event
		if err := json.Unmarshal(page.Data, &events); err != nil {
			return fmt.Errorf("reading events of fight %d of report %s from %d: %w", fight.ID, code, from, err)
		}
		if err := use(events); err != nil {
			return err
		}
		next := page.NextPageTimestamp
		if next == nil || *next <= from || *next >= fight.EndTime {
			return nil
		}
		from = *next
	}
}

// cachedPage answers a query from the page file at path when it's there, else asks Warcraft Logs and keeps the
// answer there.
func (c *Client) cachedPage(ctx context.Context, path, gql string, variables map[string]any, out any) error {
	if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, out) == nil {
		return nil
	}
	if err := c.Query(ctx, gql, variables, out); err != nil {
		return err
	}
	return cachefile.WriteJSON(path, out)
}

// pagePath is a fight's cached page file: its metadata ("meta") or its events from a time.
func (c *Client) pagePath(code string, fightID int, part string) string {
	return filepath.Join(c.CacheDir, ImportDir, "pages", fmt.Sprintf("%s-%d-%s.json", code, fightID, part))
}

// removePages deletes a fight's cached pages, once its zip is written.
func (c *Client) removePages(code string, fightID int) error {
	paths, err := filepath.Glob(c.pagePath(code, fightID, "*"))
	if err != nil {
		return fmt.Errorf("listing the pages of fight %d of report %s: %w", fightID, code, err)
	}
	var errs []error
	for _, path := range paths {
		errs = append(errs, os.Remove(path))
	}
	return errors.Join(errs...)
}
