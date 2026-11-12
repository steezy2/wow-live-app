package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"wow-live/backend/models"
)

const (
	// matchSlackMS is how far a report fight's length may differ from a saved key's or pull's: an uploader can trim or
	// split a log.
	matchSlackMS = 3 * 60_000
	// matchTieMS is how close the second-best candidate may be to the best before neither is trusted.
	matchTieMS = 5000
	// matchPlayers is how many of a key's players a report fight must have.
	matchPlayers = 3
	// matchDayMS is how far apart in time of day a key and its report fight may start: the log's times are the PC's
	// local clock and Warcraft Logs' are UTC, so only the date is trusted.
	matchDayMS = 24 * 3600_000
	// nameMin and nameMax are the length of a character's name.
	nameMin, nameMax = 2, 12
)

// ErrBadCharacter means a character isn't "Name-Realm" with a name of 2 to 12 letters and a realm Warcraft Logs knows.
var ErrBadCharacter = errors.New("a character is a name of 2 to 12 letters, a dash and a realm Warcraft Logs knows")

// raidDifficultyNames are Warcraft Logs' raid difficulty numbers as RaidPull.Difficulty names them.
var raidDifficultyNames = map[int]string{1: "LFR", 3: "Normal", 4: "Heroic", 5: "Mythic"}

// RealmSource lists a region's realms by normalized name (models.NormalizeRealm) to Warcraft Logs' slug.
type RealmSource interface {
	Realms(ctx context.Context, region string) (map[string]string, error)
}

// KeyLister lists the saved keys.
type KeyLister interface {
	List() ([]models.Key, error)
}

// Characters checks the character Settings keeps and guesses it from the saved keys.
type Characters struct {
	Realms RealmSource
	Keys   KeyLister
}

// Resolve splits character ("Name-Realm") into its name and its realm's Warcraft Logs slug in region. The error
// wraps ErrBadCharacter for a name or realm that doesn't fit; any other error is Warcraft Logs' (no keys, down).
func (c Characters) Resolve(ctx context.Context, region, character string) (name, slug string, err error) {
	name, realm, _ := strings.Cut(character, "-")
	if n := len([]rune(name)); n < nameMin || n > nameMax || strings.IndexFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r)
	}) >= 0 {
		return "", "", fmt.Errorf("%q: %w", character, ErrBadCharacter)
	}
	realms, err := c.Realms.Realms(ctx, region)
	if err != nil {
		return "", "", err
	}
	slug, ok := realms[models.NormalizeRealm(realm)]
	if !ok {
		return "", "", fmt.Errorf("realm %q: %w", realm, ErrBadCharacter)
	}
	return name, slug, nil
}

// Guess is the name of the character whose game wrote most saved keys' logs ("" for none). The logs don't keep its
// realm, so the owner adds that.
func (c Characters) Guess() string {
	keys, err := c.Keys.List()
	if err != nil {
		return ""
	}
	counts := map[string]int{}
	for _, k := range keys {
		if k.Logger != "" {
			counts[k.Logger]++
		}
	}
	best := ""
	for name, n := range counts {
		if n > counts[best] || n == counts[best] && name < best {
			best = name
		}
	}
	return best
}

// reportMatches are the report fight each saved key and pull is, by its index in the lists matchReports was given.
type reportMatches struct {
	keys, pulls map[int]models.ReportLink
}

// matchReports finds the report fight of each saved key and raid pull, if one is sure. A fight is a candidate for a
// key when it has the same dungeon and level, a length within matchSlackMS, a start on the same date and at least
// matchPlayers of the key's players; for a pull, the same boss, difficulty and result, a length within matchSlackMS
// and half the raid. The candidate with the closest length wins, unless the next is within matchTieMS of it (never
// guess). encounters are the dungeon and boss names' Warcraft Logs encounter IDs.
func matchReports(keys []models.Key, pulls []models.RaidPull, reports []models.CharacterReport,
	encounters map[string]int) reportMatches {
	found := reportMatches{keys: map[int]models.ReportLink{}, pulls: map[int]models.ReportLink{}}
	for i, k := range keys {
		wanted := func(f models.ReportFight) bool {
			return f.KeystoneLevel == k.Level && f.EncounterID == encounters[k.Dungeon] && f.EncounterID != 0 &&
				sharedPlayers(f.Players, k.Players) >= matchPlayers
		}
		if link, ok := closestFight(reports, k.Start, k.End-k.Start, wanted); ok {
			found.keys[i] = link
		}
	}
	for i, p := range pulls {
		wanted := func(f models.ReportFight) bool {
			return f.KeystoneLevel == 0 && f.EncounterID == encounters[p.Boss] && f.EncounterID != 0 &&
				raidDifficultyNames[f.Difficulty] == p.Difficulty && f.Kill == p.Kill &&
				2*sharedPlayers(f.Players, p.Players) >= len(p.Players)
		}
		if link, ok := closestFight(reports, p.Start, p.End-p.Start, wanted); ok {
			found.pulls[i] = link
		}
	}
	return found
}

// closestFight is the fight wanted says yes to that started within a day of start and has the length closest to
// durationMS (within matchSlackMS), unless the runner-up is within matchTieMS of it.
func closestFight(reports []models.CharacterReport, start, durationMS int64, wanted func(models.ReportFight) bool) (
	models.ReportLink, bool) {
	type candidate struct {
		link models.ReportLink
		off  int64
	}
	var candidates []candidate
	for _, r := range reports {
		for _, f := range r.Fights {
			off := abs(f.EndTime - f.StartTime - durationMS)
			if off <= matchSlackMS && abs(r.StartTime+f.StartTime-start) <= matchDayMS && wanted(f) {
				candidates = append(candidates, candidate{models.ReportLink{Code: r.Code, FightID: f.ID}, off})
			}
		}
	}
	slices.SortFunc(candidates, func(a, b candidate) int { return cmp.Compare(a.off, b.off) })
	if len(candidates) == 0 || len(candidates) > 1 && candidates[1].off-candidates[0].off < matchTieMS {
		return models.ReportLink{}, false
	}
	return candidates[0].link, true
}

// sharedPlayers is how many of a's names are in b.
func sharedPlayers(a, b []string) int {
	n := 0
	for _, name := range a {
		if slices.Contains(b, name) {
			n++
		}
	}
	return n
}

// PickFights are the report fights to import, oldest first so estimates build up in order: Mythic+ keys of the
// current Mythic+ zone's reports and boss pulls of the current raid zone's, that the character is in, that aren't
// saved yet (keys and pulls with a Report, imported or matched) and that no saved local key or pull is (matchReports).
func PickFights(reports []models.CharacterReport, zones models.WCLZones, character string, keys []models.Key,
	pulls []models.RaidPull) []models.FightRef {
	name, _, _ := strings.Cut(character, "-")
	taken := map[models.ReportLink]bool{}
	for _, k := range keys {
		if k.Report != nil {
			taken[*k.Report] = true
		}
	}
	for _, p := range pulls {
		if p.Report != nil {
			taken[*p.Report] = true
		}
	}
	matched := matchReports(keys, pulls, reports, zones.Encounters)
	for _, links := range []map[int]models.ReportLink{matched.keys, matched.pulls} {
		for _, link := range links {
			taken[link] = true
		}
	}
	var picked []models.FightRef
	for _, r := range reports {
		for _, f := range r.Fights {
			kind := models.FightKind("")
			switch {
			case r.ZoneID == zones.MPlusID && f.KeystoneLevel > 0:
				kind = models.FightMPlus
			case r.ZoneID == zones.RaidID && f.KeystoneLevel == 0 && f.EncounterID > 0:
				kind = models.FightRaid
			}
			link := models.ReportLink{Code: r.Code, FightID: f.ID}
			if kind != "" && !taken[link] && slices.Contains(f.Players, name) {
				picked = append(picked, models.FightRef{ReportLink: link, StartMS: r.StartTime + f.StartTime, Kind: kind})
			}
		}
	}
	slices.SortFunc(picked, func(a, b models.FightRef) int {
		return cmp.Or(cmp.Compare(a.StartMS, b.StartMS), cmp.Compare(a.Code, b.Code), cmp.Compare(a.FightID, b.FightID))
	})
	return picked
}
