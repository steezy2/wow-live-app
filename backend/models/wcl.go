package models

import (
	"errors"
	"time"
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
)

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

// SecretStore keeps one secret per name (in the desktop app, Windows Credential Manager); Get is "" when none is
// stored.
type SecretStore interface {
	Get(name string) (string, error)
	Set(name, secret string) error
	Delete(name string) error
}
