package models

import "context"

// RankQuery picks a spec's ranked characters on one Warcraft Logs encounter: a Mythic+ dungeon at a keystone level
// (Level above 0), or a raid boss on a difficulty (Warcraft Logs' number: 5 Mythic, 4 Heroic, 3 Normal).
type RankQuery struct {
	Encounter  int
	Level      int
	Difficulty int
	Class      string
	Spec       string
	Metric     string // "dps" or "hps"
}

// RankedRun is one character's ranked run or kill: its report fight, how long it took, and who played it.
type RankedRun struct {
	Amount     float64 `json:"amount"` // damage or healing per second
	DurationMS int64   `json:"duration"`
	Code       string  `json:"code"`
	FightID    int     `json:"fightID"`
	Name       string  `json:"name"`
	Server     string  `json:"server"`
}

// RankedPlayer is one player's numbers over a fight or a stretch of it: from a Warcraft Logs report (RunPlayer), or
// from a saved key or pull through the same shape, so both sides of a comparison read alike.
type RankedPlayer struct {
	DurationMS int64 `json:"durationMS"` // the stretch the numbers cover
	Damage     int64 `json:"damage"`
	Healing    int64 `json:"healing"`
	Taken      int64 `json:"taken"`
	// ActiveMS is how long they were doing something; 0 when unknown.
	ActiveMS  int64           `json:"activeMS"`
	Deaths    int             `json:"deaths"`
	Abilities []RankedAbility `json:"abilities"`
	Buffs     []RankedBuff    `json:"buffs"`
}

// RankedAbility is what one ability did: its spell ID (0 when unknown, then the name matches), damage and healing,
// casts and hits.
type RankedAbility struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Damage  int64  `json:"damage"`
	Healing int64  `json:"healing"`
	Casts   int    `json:"casts"`
	Hits    int    `json:"hits"`
}

// RankedBuff is a buff on the player and how long it was up in the stretch.
type RankedBuff struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	UptimeMS int64  `json:"uptimeMS"`
}

// CompareBaselines is what a player review reads from Warcraft Logs: the dungeons' and bosses' encounter IDs, a page
// of a spec's ranked runs, one ranked player's numbers, and the points left this hour.
type CompareBaselines interface {
	MPlusEncounters(ctx context.Context) (map[string]int, error)
	RaidEncounters(ctx context.Context, bosses []string) (map[string]int, error)
	// RankedRuns is page (from 1) of the ranked runs and whether more pages follow.
	RankedRuns(ctx context.Context, q RankQuery, page int) ([]RankedRun, bool, error)
	// RunPlayer reads player (name on server) in a report fight: all of it, or only its first windowMS.
	RunPlayer(ctx context.Context, code string, fightID int, name, server string, windowMS int64) (RankedPlayer, error)
	PointsLeft(ctx context.Context) (float64, error)
}

// CompareStore keeps player reviews by kind, the fight's start and the player; Compare fails with fs.ErrNotExist
// when there's none.
type CompareStore interface {
	SaveCompare(c PlayerCompare) error
	Compare(kind FightKind, start int64, player string) (PlayerCompare, error)
}

// CompareRow is one line of a player review: the player's number against their own similar runs' median (Yours,
// null with none), the top logs' median and the best log's.
type CompareRow struct {
	Name  string   `json:"name"`
	ID    int      `json:"id,omitempty"`
	You   float64  `json:"you"`
	Yours *float64 `json:"yours,omitempty"`
	Top   float64  `json:"top"`
	Best  float64  `json:"best"`
	// Unit is "/s", "%", "per min" (a rotational ability), "casts" (one the top logs cast at most once a minute) or "".
	Unit string `json:"unit"`
	// Share is the ability's share of the player's damage or healing, in %.
	Share float64 `json:"share,omitempty"`
}

// CompareLog is one top log in a player review, linked to its Warcraft Logs fight.
type CompareLog struct {
	Name       string  `json:"name"`
	Server     string  `json:"server"`
	Code       string  `json:"code"`
	FightID    int     `json:"fightID"`
	DurationMS int64   `json:"durationMS"`
	Amount     float64 `json:"amount"` // its damage or healing per second
}

// PlayerCompare is a player's run against three baselines: their own similar runs, the top Warcraft Logs logs of
// their spec in a similar time, and their own parses there.
type PlayerCompare struct {
	Kind   FightKind `json:"kind" enums:"mplus,raid"`
	Start  int64     `json:"start"`
	End    int64     `json:"end"` // the fight's end when it was made: a key that went on afterwards is read again
	Player string    `json:"player"`
	Spec   string    `json:"spec"`
	// WindowMS is how much of the top logs was read: 0 for all of it, else the fight's length (a wipe or an
	// abandoned key is read over the same stretch).
	WindowMS int64 `json:"windowMS,omitempty"`
	// Baseline says which logs the top column is: "20 runs within 15% of 34:10", or why not.
	Baseline string `json:"baseline"`
	Verdict  string `json:"verdict"`
	// YourRuns is how many of the player's own earlier runs the Yours column is the median of.
	YourRuns  int              `json:"yourRuns"`
	Logs      []CompareLog     `json:"logs"`
	Parses    *CharacterParses `json:"parses,omitempty"`
	Parse     *CharacterParse  `json:"parse,omitempty"` // their parse of this dungeon or boss, from Parses
	Stats     []CompareRow     `json:"stats"`
	Abilities []CompareRow     `json:"abilities"`
	Buffs     []CompareRow     `json:"buffs"`
	OnlyYou   []string         `json:"onlyYou"`
	OnlyTop   []string         `json:"onlyTop"`
	Advice    []string         `json:"advice"`
	// Rules are the spec's rules the player fell short of, checked on their own log; none for a spec without rules.
	Rules []RuleResult `json:"rules"`
	// Points is what the review spent of Warcraft Logs' hourly points.
	Points float64 `json:"points"`
}

// SpecRules are the per-spec rules a player review checks, by the spec as the review names it ("Arcane Mage"):
// spec-rules.toml.
type SpecRules map[string]struct {
	Rules []SpecRule `toml:"rules"`
}

// SpecRule is one thing a spec should do. Kind "with": at least AtLeast (a share, 0 to 1) of the Cast casts happen
// while Buff is up. "together": at least AtLeast of the Cast casts are within Seconds of a cast of Other. "uptime":
// Buff is up at least AtLeast of the fight. Why says why it matters.
type SpecRule struct {
	Kind    string  `toml:"kind"`
	Cast    string  `toml:"cast"`
	Buff    string  `toml:"buff"`
	Other   string  `toml:"other"`
	Seconds float64 `toml:"seconds"`
	AtLeast float64 `toml:"at_least"`
	Why     string  `toml:"why"`
}

// RuleResult is a spec rule the player fell short of: what it checks, the share they reached and the share it asks
// for (0 to 1), and its why.
type RuleResult struct {
	Text    string  `json:"text"`
	Share   float64 `json:"share"`
	AtLeast float64 `json:"atLeast"`
	Why     string  `json:"why,omitempty"`
}
