package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"unicode"

	"wow-live/backend/models"
)

const (
	// matchSlackMS is how far a report fight's length and start may differ from a saved key's or pull's: an uploader
	// can trim or split a log, and Warcraft Logs' times follow the uploader's clock (a minute off seen).
	matchSlackMS = 3 * 60_000
	// copyLengthMS is how far two reports' copies of one fight may differ in length (the same log lines, cut alike),
	// so two quick pulls of a boss aren't taken for one. Two equal quick wipes that are each in only one uploader's
	// report still are; it hasn't been seen.
	copyLengthMS = 5000
	// matchPlayers is how many of a key's players a report fight must have.
	matchPlayers = 3
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

// ParseSource is Warcraft Logs as the character's parses read it (wcl.Client).
type ParseSource interface {
	CurrentZones(ctx context.Context, raidBosses []string) (models.WCLZones, error)
	CharacterParses(ctx context.Context, region, slug, name string, zones models.WCLZones) (models.CharacterParses, error)
}

// KeyLister lists the saved keys.
type KeyLister interface {
	List() ([]models.Key, error)
}

// Characters checks the character Settings keeps, guesses it from the saved keys, and reads its parses (none
// without Logs).
type Characters struct {
	Realms RealmSource
	Keys   KeyLister
	Logs   ParseSource
	Bosses []string // the raid's boss names, to find its Warcraft Logs zone
}

// Parses is character's Warcraft Logs parses in this season's dungeons and the current raid's bosses; the errors are
// Resolve's, and models.ErrNoCredentials without Logs.
func (c Characters) Parses(ctx context.Context, region, character string) (models.CharacterParses, error) {
	if c.Logs == nil {
		return models.CharacterParses{}, models.ErrNoCredentials
	}
	name, slug, err := c.Resolve(ctx, region, character)
	if err != nil {
		return models.CharacterParses{}, err
	}
	zones, err := c.Logs.CurrentZones(ctx, c.Bosses)
	if err != nil {
		return models.CharacterParses{}, err
	}
	return c.Logs.CharacterParses(ctx, region, slug, name, zones)
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

// Guess is the character ("Name-Realm", or a bare name for keys saved before realms were kept) whose game wrote most
// saved keys' logs ("" for none).
func (c Characters) Guess() string {
	keys, err := c.Keys.List()
	if err != nil {
		return ""
	}
	if played := playedCharacters(keys, nil, ""); len(played) > 0 {
		return played[0].ID
	}
	return ""
}

// matchReports is the report fights that are saved keys and raid pulls. A fight is a candidate for a key when it has
// the same dungeon and level, a length and start within matchSlackMS and at least matchPlayers of the key's players;
// for a pull, the same boss, difficulty and result, a length and start within matchSlackMS and half the raid. Each
// report's closest-length candidate is the key or pull (sameFights). encounters are the dungeon and boss names'
// Warcraft Logs encounter IDs.
func matchReports(keys []models.Key, pulls []models.RaidPull, reports []models.CharacterReport,
	encounters map[string]int) map[models.ReportLink]bool {
	found := map[models.ReportLink]bool{}
	for _, k := range keys {
		wanted := func(f models.ReportFight) bool {
			return f.KeystoneLevel == k.Level && f.EncounterID == encounters[k.Dungeon] && f.EncounterID != 0 &&
				sharedPlayers(f.Players, k.Players) >= matchPlayers
		}
		sameFights(found, reports, k.Start, k.End-k.Start, wanted)
	}
	for _, p := range pulls {
		wanted := func(f models.ReportFight) bool {
			return f.KeystoneLevel == 0 && f.EncounterID == encounters[p.Boss] && f.EncounterID != 0 &&
				raidDifficultyNames[f.Difficulty] == p.Difficulty && f.Kill == p.Kill &&
				2*sharedPlayers(f.Players, p.Players) >= len(p.Players)
		}
		sameFights(found, reports, p.Start, p.End-p.Start, wanted)
	}
	return found
}

// ReportLinks are the saved keys and raid pulls read from a local log and not linked yet that a report fight is
// (matchReports), by start time, each with its fight in the newest report that has it.
func ReportLinks(reports []models.CharacterReport, encounters map[string]int, keys []models.Key,
	pulls []models.RaidPull) (keyLinks, pullLinks map[int64]models.ReportLink) {
	started := map[string]int64{}
	for _, r := range reports {
		started[r.Code] = r.StartTime
	}
	newest := func(found map[models.ReportLink]bool) (models.ReportLink, bool) {
		links := slices.SortedFunc(maps.Keys(found), func(a, b models.ReportLink) int {
			return cmp.Or(cmp.Compare(started[b.Code], started[a.Code]), cmp.Compare(a.Code, b.Code))
		})
		if len(links) == 0 {
			return models.ReportLink{}, false
		}
		return links[0], true
	}
	keyLinks, pullLinks = map[int64]models.ReportLink{}, map[int64]models.ReportLink{}
	for _, k := range keys {
		if k.Imported || k.Report != nil {
			continue
		}
		if link, ok := newest(matchReports([]models.Key{k}, nil, reports, encounters)); ok {
			keyLinks[k.Start] = link
		}
	}
	for _, p := range pulls {
		if p.Imported || p.Report != nil {
			continue
		}
		if link, ok := newest(matchReports(nil, []models.RaidPull{p}, reports, encounters)); ok {
			pullLinks[p.Start] = link
		}
	}
	return keyLinks, pullLinks
}

// sameFights adds to found, in each report, the fight wanted says yes to whose start and length are within
// matchSlackMS of start and durationMS, the closest length if several are: the same fight can be in several reports,
// uploaded by different players, but only once in each, so its neighbours (the next quick wipe) stay unmatched.
func sameFights(found map[models.ReportLink]bool, reports []models.CharacterReport, start, durationMS int64,
	wanted func(models.ReportFight) bool) {
	for _, r := range reports {
		best, off := 0, int64(matchSlackMS+1)
		for _, f := range r.Fights {
			if o := abs(f.EndTime - f.StartTime - durationMS); o < off && abs(r.StartTime+f.StartTime-start) <= matchSlackMS &&
				wanted(f) {
				best, off = f.ID, o
			}
		}
		if off <= matchSlackMS {
			found[models.ReportLink{Code: r.Code, FightID: best}] = true
		}
	}
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

// PickFights are the report fights to import, newest first so the latest keys and pulls show up first: Mythic+
// keys of the current Mythic+ zone's reports and boss pulls of the current raid zone's, that the character is in
// (any case), that aren't saved yet (keys and pulls with a Report, imported or matched) and that no saved local key
// or pull is (matchReports), with their length, players and result.
func PickFights(reports []models.CharacterReport, zones models.WCLZones, character string, keys []models.Key,
	pulls []models.RaidPull) []models.FightRef {
	name, _, _ := strings.Cut(character, "-")
	taken := matchReports(keys, pulls, reports, zones.Encounters)
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
	type candidate struct {
		ref   models.FightRef
		fight models.ReportFight
	}
	length := func(c candidate) int64 { return c.fight.EndTime - c.fight.StartTime }
	var candidates []candidate
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
			in := slices.ContainsFunc(f.Players, func(p string) bool { return strings.EqualFold(p, name) })
			if kind != "" && in && !taken[link] {
				ref := models.FightRef{ReportLink: link, StartMS: r.StartTime + f.StartTime, Kind: kind,
					DurationMS: f.EndTime - f.StartTime, Players: len(f.Players), Kill: f.Kill}
				candidates = append(candidates, candidate{ref, f})
			}
		}
	}
	slices.SortFunc(candidates, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(b.ref.StartMS, a.ref.StartMS), cmp.Compare(b.ref.Code, a.ref.Code),
			cmp.Compare(b.ref.FightID, a.ref.FightID))
	})
	var picked []models.FightRef
	for i, c := range candidates {
		copied := slices.ContainsFunc(candidates[:i], func(o candidate) bool { // the same fight in another report
			return o.ref.Code != c.ref.Code && o.fight.EncounterID == c.fight.EncounterID && o.fight.KeystoneLevel == c.fight.KeystoneLevel &&
				o.fight.Difficulty == c.fight.Difficulty && o.fight.Kill == c.fight.Kill &&
				abs(o.ref.StartMS-c.ref.StartMS) <= matchSlackMS && abs(length(o)-length(c)) <= copyLengthMS
		})
		if !copied {
			picked = append(picked, c.ref)
		}
	}
	return picked
}

// SetSource marks the keys and raid pulls that end from now on as imported from link's Warcraft Logs fight; nil
// goes back to the game's own log.
func (k *LogTracker) SetSource(link *models.ReportLink) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.source = link
}

// LinkReports saves each Warcraft Logs fight in keyLinks and pullLinks (by start time, from ReportLinks) as its
// saved key's or raid pull's Report, and says how many it linked; one linked already keeps its link.
func (k *LogTracker) LinkReports(keyLinks, pullLinks map[int64]models.ReportLink) (n int, err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	defer func() {
		if n > 0 {
			k.version++
		}
	}()
	for i, key := range k.history {
		if link, ok := keyLinks[key.Start]; ok && !key.Imported && key.Report == nil {
			key.Report = &link
			if err := k.store.Save(key); err != nil {
				return n, fmt.Errorf("linking a key to its report: %w", err)
			}
			k.history[i] = key
			n++
		}
	}
	if k.raid != nil {
		for i, p := range k.pulls {
			if link, ok := pullLinks[p.Start]; ok && !p.Imported && p.Report == nil {
				p.Report = &link
				if err := k.raid.store.SavePull(p); err != nil {
					return n, fmt.Errorf("linking a raid pull to its report: %w", err)
				}
				k.pulls[i] = p
				n++
			}
		}
	}
	return n, nil
}

// ImportedCount is how many saved keys and raid pulls came from Warcraft Logs.
func (k *LogTracker) ImportedCount() int {
	k.mu.RLock()
	defer k.mu.RUnlock()
	n := 0
	for _, key := range k.history {
		if key.Imported {
			n++
		}
	}
	for _, p := range k.pulls {
		if p.Imported {
			n++
		}
	}
	return n
}

// RemoveImported deletes every saved key and raid pull that came from Warcraft Logs.
func (k *LogTracker) RemoveImported() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, key := range k.history {
		if key.Imported {
			if err := k.store.Delete(key.Start); err != nil {
				return fmt.Errorf("removing imported keys: %w", err)
			}
		}
	}
	k.history = slices.DeleteFunc(k.history, func(key models.Key) bool { return key.Imported })
	k.version++
	if k.raid == nil {
		return nil
	}
	for _, p := range k.pulls {
		if p.Imported {
			if err := k.raid.store.DeletePull(p.Start); err != nil {
				return fmt.Errorf("removing imported raid pulls: %w", err)
			}
		}
	}
	k.pulls = slices.DeleteFunc(k.pulls, func(p models.RaidPull) bool { return p.Imported })
	return nil
}

// sameKey is whether a and b are one key read twice, from a local log and from Warcraft Logs, whose uploader's
// clock can be a minute off.
func sameKey(a, b models.Key) bool {
	return a.Imported != b.Imported && a.Dungeon == b.Dungeon && a.Level == b.Level &&
		abs(a.Start-b.Start) <= matchSlackMS
}

// samePull is sameKey for raid pulls.
func samePull(a, b models.RaidPull) bool {
	return a.Imported != b.Imported && a.Boss == b.Boss && a.Difficulty == b.Difficulty && a.Kill == b.Kill &&
		abs(a.Start-b.Start) <= matchSlackMS
}

// keepLocal settles an item read both from a local log and from Warcraft Logs, before item is saved: the local copy
// wins. An imported item with a local twin in list is taken off it and not saved (true); a local one's imported
// twins are deleted (del) and taken off. mu is the tracker's lock.
func keepLocal[T any](mu *sync.RWMutex, list *[]T, item T, same func(a, b T) bool, imported func(T) bool,
	start func(T) int64, del func(int64) error) (bool, error) {
	mu.Lock()
	defer mu.Unlock()
	twin := func(other T) bool { return same(item, other) }
	if imported(item) {
		if !slices.ContainsFunc(*list, twin) {
			return false, nil
		}
		*list = slices.DeleteFunc(*list, func(other T) bool { return start(other) == start(item) && imported(other) })
		return true, nil
	}
	for _, other := range *list {
		if twin(other) {
			if err := del(start(other)); err != nil {
				return false, fmt.Errorf("deleting an imported copy: %w", err)
			}
		}
	}
	*list = slices.DeleteFunc(*list, twin)
	return false, nil
}
