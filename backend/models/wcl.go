package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// ReportURL is where a report is on Warcraft Logs; add the code (and "#fight=N" for one pull).
const ReportURL = "https://www.warcraftlogs.com/reports/"

// ReportListMaxAge is how long a fetched report list is reused, so the tools in one run share one search.
const ReportListMaxAge = 10 * time.Minute

// Report is one Warcraft Logs report with its mythic pulls. Times are ms: StartTime and EndTime since the epoch,
// fight times since the report's start.
type Report struct {
	Code      string            `json:"code"`
	StartTime int64             `json:"startTime"`
	EndTime   int64             `json:"endTime"`
	Phases    []EncounterPhases `json:"phases"`
	Fights    []Fight           `json:"fights"`
}

// EncounterPhases is a boss's phase list.
type EncounterPhases struct {
	EncounterID int     `json:"encounterID"`
	Phases      []Phase `json:"phases"`
}

// Phase is one phase of a boss, which may be an intermission.
type Phase struct {
	ID             int  `json:"id"`
	IsIntermission bool `json:"isIntermission"`
}

// Fight is one pull.
type Fight struct {
	ID                      int               `json:"id"`
	EncounterID             int               `json:"encounterID"`
	Name                    string            `json:"name"`
	Kill                    bool              `json:"kill"`
	StartTime               int64             `json:"startTime"`
	EndTime                 int64             `json:"endTime"`
	FightPercentage         float64           `json:"fightPercentage"`
	LastPhase               int               `json:"lastPhase"`
	LastPhaseIsIntermission bool              `json:"lastPhaseIsIntermission"`
	FriendlyPlayers         []int             `json:"friendlyPlayers"`
	PhaseTransitions        []PhaseTransition `json:"phaseTransitions"`
}

// PhaseTransition is when a phase started (ms since the report's start).
type PhaseTransition struct {
	ID        int   `json:"id"`
	StartTime int64 `json:"startTime"`
}

// RateLimit is a Warcraft Logs account's points budget this hour.
type RateLimit struct {
	LimitPerHour        float64 `json:"limitPerHour"`
	PointsSpentThisHour float64 `json:"pointsSpentThisHour"`
	PointsResetIn       float64 `json:"pointsResetIn"`
}

// Stream is one kind of events to fetch: a data type such as "Deaths", "Casts" or "All", optionally only abilities
// with these exact names, and any other Warcraft Logs filter expression.
type Stream struct {
	DataType string
	Names    []string
	Where    string
}

// Event is one Warcraft Logs event as the API returns it (timestamp, type, sourceID, targetID, abilityGameID,
// fight, and more by type). Kept as decoded JSON so it caches exactly as fetched.
type Event map[string]any

// LogLine is one line of the game's combat log: when it happened (ms since the epoch), its event name
// ("SPELL_DAMAGE") and the comma-separated fields after it, unquoted, with bracketed groups kept whole.
type LogLine struct {
	Time   int64
	Event  string
	Fields []string
}

// Credentials are one Warcraft Logs API client's ID and secret.
type Credentials struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

// ErrNoCredentials means no Warcraft Logs client ID and secret were given; ErrBadCredentials that Warcraft Logs
// turned them down.
var (
	ErrNoCredentials  = errors.New("no Warcraft Logs keys: add them in Settings")
	ErrBadCredentials = errors.New("Warcraft Logs turned down the client ID and secret")
	// ErrRateLimited means the account's hourly points are used up until the hour resets.
	ErrRateLimited = errors.New("Warcraft Logs rate limit reached (the API allows so many points per hour); " +
		"try again later")
)

// ImportStatus is how an import of a character's Warcraft Logs reports stands: listing the reports, fetching fights,
// waiting for the hour's points (until ResumeAtMS), done, or failed (Problem says why; a failed fight leaves it
// running, with the fight's error in Problem). PointsPerFight is the average so far; Imported is how many saved keys
// and pulls came from Warcraft Logs, whichever import saved them.
type ImportStatus struct {
	State          string  `json:"state" enums:"idle,listing,running,waiting,done,failed"`
	Character      string  `json:"character"`
	Fights         int     `json:"fights"`
	FightsDone     int     `json:"fightsDone"`
	FightsFailed   int     `json:"fightsFailed"`
	PointsSpent    float64 `json:"pointsSpent"`
	PointsPerFight float64 `json:"pointsPerFight"`
	ResumeAtMS     int64   `json:"resumeAtMS"`
	Problem        string  `json:"problem"`
	Imported       int     `json:"imported"`
}

// KeyStatus is what the page shows about the saved Warcraft Logs keys, never the secret: whether they're set, the
// client ID's last 4 characters, and the last test (when, whether it worked, the points left of the hour's limit or
// why it failed).
type KeyStatus struct {
	Set          bool    `json:"set"`
	IDEnd        string  `json:"idEnd"`
	TestedMS     int64   `json:"testedMS"`
	Works        bool    `json:"works"`
	PointsLeft   float64 `json:"pointsLeft"`
	LimitPerHour float64 `json:"limitPerHour"`
	Problem      string  `json:"problem"`
}

// ReportLink is a fight of a Warcraft Logs report: its code and the fight's ID in it.
type ReportLink struct {
	Code    string `json:"code"`
	FightID int    `json:"fightID"`
}

// URL is the fight's page on Warcraft Logs.
func (l ReportLink) URL() string {
	return fmt.Sprintf("%s%s#fight=%d", ReportURL, l.Code, l.FightID)
}

// CharacterReport is one of a character's recent Warcraft Logs reports: when it ran (ms since the epoch), its zone
// and its fights.
type CharacterReport struct {
	Code      string        `json:"code"`
	StartTime int64         `json:"startTime"`
	EndTime   int64         `json:"endTime"`
	ZoneID    int           `json:"zoneID"`
	Fights    []ReportFight `json:"fights"`
}

// ReportFight is one fight of a CharacterReport: a Mythic+ key (KeystoneLevel above 0), a boss pull (EncounterID
// above 0) or trash. Times are ms since the report's start; Difficulty is Warcraft Logs' number (5 Mythic, 4 Heroic,
// 3 Normal, 1 LFR); Players are the friendly players' names without their realms.
type ReportFight struct {
	ID            int      `json:"id"`
	EncounterID   int      `json:"encounterID"`
	KeystoneLevel int      `json:"keystoneLevel"`
	Difficulty    int      `json:"difficulty"`
	Kill          bool     `json:"kill"`
	StartTime     int64    `json:"startTime"`
	EndTime       int64    `json:"endTime"`
	Players       []string `json:"players"`
}

// WCLZones are the Warcraft Logs zones an import reads: the newest Mythic+ zone and the newest raid zone, with their
// dungeons' and bosses' encounter IDs by name.
type WCLZones struct {
	MPlusID    int            `json:"mplusID"`
	RaidID     int            `json:"raidID"`
	Encounters map[string]int `json:"encounters"`
}

// FightKind says whether an imported fight is a Mythic+ key or a raid boss pull.
type FightKind string

// The kinds of fight an import reads.
const (
	FightMPlus FightKind = "mplus"
	FightRaid  FightKind = "raid"
)

// FightRef is one report fight to import: its report and ID, when it started (ms since the epoch), its kind, how
// long it lasted, how many players were in it and whether it was a kill.
type FightRef struct {
	ReportLink
	StartMS    int64     `json:"startMS"`
	Kind       FightKind `json:"kind" enums:"mplus,raid"`
	DurationMS int64     `json:"durationMS"`
	Players    int       `json:"players"`
	Kill       bool      `json:"kill"`
}

// ImportKind is what the owner picks to import: Mythic+ keys, raid kills or raid wipes.
type ImportKind string

// The kinds of fight the owner picks from.
const (
	ImportKeys      ImportKind = "keys"
	ImportRaidKills ImportKind = "raidKills"
	ImportRaidWipes ImportKind = "raidWipes"
)

// ImportKinds are every ImportKind, in the order the page lists them.
var ImportKinds = []ImportKind{ImportKeys, ImportRaidKills, ImportRaidWipes}

// ImportKind is which of the owner's choices the fight falls in.
func (f FightRef) ImportKind() ImportKind {
	switch {
	case f.Kind == FightMPlus:
		return ImportKeys
	case f.Kill:
		return ImportRaidKills
	}
	return ImportRaidWipes
}

// ImportPlan is what an import of a character's fights of this season would fetch, by kind in ImportKinds' order,
// with the points an import spends an hour (the hour's limit less what it leaves for key reviews) and what listing
// the reports cost.
type ImportPlan struct {
	Character     string           `json:"character"`
	Kinds         []ImportKindPlan `json:"kinds"`
	PointsPerHour float64          `json:"pointsPerHour"`
	ListedPoints  float64          `json:"listedPoints"`
}

// ImportKindPlan is one kind's fights in an ImportPlan: how many, their minutes, an estimate of their points, and
// whether the page ticks it at first.
type ImportKindPlan struct {
	Kind    ImportKind `json:"kind" enums:"keys,raidKills,raidWipes"`
	Fights  int        `json:"fights"`
	Minutes int        `json:"minutes"`
	Points  float64    `json:"points"`
	Default bool       `json:"default"`
}

// NormalizeRealm is a realm's name in lower case with only its letters and digits, so "Area 52", "area-52" and
// "Area52" are one realm.
func NormalizeRealm(realm string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, realm)
}

// SecretStore keeps one secret per name (in the desktop app, Windows Credential Manager); Get is "" when none is
// stored.
type SecretStore interface {
	Get(name string) (string, error)
	Set(name, secret string) error
	Delete(name string) error
}
