package models

// RecapPeriod is a period the Recap tab offers: its name and its span in ms since the epoch (To is now for one still
// running).
type RecapPeriod struct {
	Label string `json:"label" example:"This month"`
	From  int64  `json:"from"`
	To    int64  `json:"to"`
}

// Recap is a period's Mythic+ keys and raid pulls read as a story, one character's or everyone's, in two halves that
// never count each other's runs.
type Recap struct {
	From  int64      `json:"from"`
	To    int64      `json:"to"`
	MPlus RecapMPlus `json:"mplus"`
	Raid  RecapRaid  `json:"raid"`
}

// RecapMPlus is a recap's keys: run, timed (and their share), the highest level timed, the time spent in them and the
// party's deaths; a row per key, newest first; each dungeon's runs; the character; who they ran with; and a few facts
// that each open a key.
type RecapMPlus struct {
	Keys         int            `json:"keys"`
	Timed        int            `json:"timed"`
	SuccessPct   float64        `json:"successPct"`
	HighestTimed int            `json:"highestTimed"` // 0 when none was timed
	TimeMS       int64          `json:"timeMS"`
	Deaths       int            `json:"deaths"`   // the party's
	KeyRows      []RecapKeyRow  `json:"keyRows"`  // newest first, the latest few only
	Dungeons     []RecapDungeon `json:"dungeons"` // most run first
	You          *RecapYou      `json:"you,omitempty"`
	Party        []RecapMate    `json:"party"` // most keys together first
	Facts        []RecapFact    `json:"facts"`
}

// RecapKeyRow is one key in a recap, opened by its start: the character's own numbers and the whole party's.
type RecapKeyRow struct {
	Start   int64          `json:"start"`
	Dungeon string         `json:"dungeon"`
	Level   int            `json:"level"`
	Status  string         `json:"status" enums:"running,timed,depleted,abandoned"`
	You     RecapKeyYou    `json:"you"`
	Party   RecapKeyCounts `json:"party"`
}

// RecapKeyCounts is a key's kicks, crowd control casts (mplus.toml's [cc]) and dispels, and the share of damage that
// went into minibosses and bosses (in %). Each is -1 when not known: the player wasn't in the key, or (PriorityPct
// only) the key was saved before priority damage was kept.
type RecapKeyCounts struct {
	Kicks       int     `json:"kicks"`
	CC          int     `json:"cc"`
	Dispels     int     `json:"dispels"`
	PriorityPct float64 `json:"priorityPct"`
}

// RecapKeyYou is the character's own numbers in a key: their DPS and HPS over the key and their counts, each -1 when
// they weren't in it (DPS and HPS also when the key kept no meter row for them).
type RecapKeyYou struct {
	DPS float64 `json:"dps"`
	HPS float64 `json:"hps"`
	RecapKeyCounts
}

// RecapRaid is a recap's raid pulls: pulls, kills, the bosses killed (by boss and difficulty), the time spent in
// pulls and the raid's deaths, the pulls the character was in and their deaths in them; and each boss.
type RecapRaid struct {
	Pulls      int         `json:"pulls"`
	Kills      int         `json:"kills"`
	Killed     int         `json:"killed"`
	TimeMS     int64       `json:"timeMS"`
	Deaths     int         `json:"deaths"`
	YourPulls  int         `json:"yourPulls"`
	YourDeaths int         `json:"yourDeaths"`
	Bosses     []RecapBoss `json:"bosses"` // in the order first pulled
}

// RecapDungeon is one dungeon in a recap: its runs, how many were timed and their share; and the highest level timed
// with its timed runs and their average, median and fastest time (all 0 when none was timed). Untimed and abandoned
// keys never count toward the level or the times.
type RecapDungeon struct {
	Dungeon    string  `json:"dungeon"`
	Runs       int     `json:"runs"`
	Timed      int     `json:"timed"`
	SuccessPct float64 `json:"successPct"`
	BestLevel  int     `json:"bestLevel"`
	BestRuns   int     `json:"bestRuns"`
	AvgMS      int64   `json:"avgMS"`
	MedianMS   int64   `json:"medianMS"`
	BestMS     int64   `json:"bestMS"` // the fastest
}

// RecapYou is the character a recap is for (the one picked, or the one whose game wrote most of the period's logs):
// the specs played, the median DPS (HPS for a healer spec) per dungeon, the deaths, and what killed them most, most
// first.
type RecapYou struct {
	Character string       `json:"character"`
	Specs     []string     `json:"specs"`
	Dungeons  []RecapRate  `json:"dungeons"`
	Keys      int          `json:"keys"` // the keys they played, which Deaths is over
	Deaths    int          `json:"deaths"`
	Killers   []RecapCount `json:"killers"`
}

// RecapRate is the owner's median damage or healing per second in one dungeon.
type RecapRate struct {
	Dungeon string  `json:"dungeon"`
	Metric  string  `json:"metric" enums:"dps,hps"`
	Median  float64 `json:"median"`
}

// RecapCount is a name and how many times it happened.
type RecapCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// RecapTopTime is the median time of a dungeon's top speed-ranked Warcraft Logs runs at a level (the highest the
// recap's character timed there) and how many ranked runs it's over; MedianMS is 0 when fewer than the recap needs.
type RecapTopTime struct {
	Dungeon  string `json:"dungeon"`
	Level    int    `json:"level"`
	MedianMS int64  `json:"medianMS"`
	Runs     int    `json:"runs"`
}

// RecapMate is a player the owner ran keys with: their realm and class ("" when unknown), the keys together and how
// many of them were timed.
type RecapMate struct {
	Player string `json:"player"`
	Realm  string `json:"realm"`
	Class  string `json:"class"`
	Keys   int    `json:"keys"`
	Timed  int    `json:"timed"`
}

// RecapFact is one line of a recap and the key or raid pull it opens (by its start).
type RecapFact struct {
	Text  string    `json:"text"`
	Kind  FightKind `json:"kind" enums:"mplus,raid"`
	Start int64     `json:"start"`
}

// RecapBoss is one raid boss and difficulty in a recap: its pulls and kills, the first kill (its start and how many
// pulls it took; 0 for none), the lowest health a wipe left (-1 for no wipe with advanced logging), the owner's
// deaths, and the ability that hit the raid hardest.
type RecapBoss struct {
	Boss          string  `json:"boss"`
	Difficulty    string  `json:"difficulty"`
	Pulls         int     `json:"pulls"`
	Kills         int     `json:"kills"`
	FirstKill     int64   `json:"firstKill"`
	FirstKillPull int     `json:"firstKillPull"`
	BestWipePct   float64 `json:"bestWipePct"`
	YourDeaths    int     `json:"yourDeaths"`
	MostHit       string  `json:"mostHit"`
}
