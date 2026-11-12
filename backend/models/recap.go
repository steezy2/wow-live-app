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
// party's deaths; the per-key averages; each dungeon's runs; the character; who they ran with; and a few facts that
// each open a key.
type RecapMPlus struct {
	Keys         int            `json:"keys"`
	Timed        int            `json:"timed"`
	SuccessPct   float64        `json:"successPct"`
	HighestTimed int            `json:"highestTimed"` // 0 when none was timed
	TimeMS       int64          `json:"timeMS"`
	Deaths       int            `json:"deaths"` // the party's
	Averages     RecapAverages  `json:"averages"`
	Dungeons     []RecapDungeon `json:"dungeons"` // most run first
	You          *RecapYou      `json:"you,omitempty"`
	Party        []RecapMate    `json:"party"` // most keys together first
	Facts        []RecapFact    `json:"facts"`
}

// RecapAverages is a key's kicks, crowd control, dispels and priority damage on average, the party's and the
// character's own (in the keys they played).
type RecapAverages struct {
	Party RecapPerKey `json:"party"`
	You   RecapPerKey `json:"you"`
}

// RecapPerKey is an average per key: kicks, crowd control casts (mplus.toml's [cc]), dispels, and the share of
// damage that went into minibosses and bosses (in %). Each is -1 when no key in the period had it (keys saved before
// it was kept).
type RecapPerKey struct {
	Kicks       float64 `json:"kicks"`
	CC          float64 `json:"cc"`
	Dispels     float64 `json:"dispels"`
	PriorityPct float64 `json:"priorityPct"`
}

// RecapRaid is a recap's raid pulls: pulls, kills, the bosses killed (by boss and difficulty), the time spent in
// pulls and the raid's deaths; each boss; and the facts (none yet).
type RecapRaid struct {
	Pulls  int         `json:"pulls"`
	Kills  int         `json:"kills"`
	Killed int         `json:"killed"`
	TimeMS int64       `json:"timeMS"`
	Deaths int         `json:"deaths"`
	Bosses []RecapBoss `json:"bosses"` // in the order first pulled
	Facts  []RecapFact `json:"facts"`
}

// RecapDungeon is one dungeon in a recap: its runs, how many were timed and their share; the highest level timed and
// the fastest time at that level (0 for both when none was); and the highest level run, its runs and timed runs, and
// the average, median and fastest time of its finished runs (0 when none finished).
type RecapDungeon struct {
	Dungeon    string  `json:"dungeon"`
	Runs       int     `json:"runs"`
	Timed      int     `json:"timed"`
	SuccessPct float64 `json:"successPct"`
	BestLevel  int     `json:"bestLevel"`
	BestMS     int64   `json:"bestMS"`
	TopLevel   int     `json:"topLevel"`
	TopRuns    int     `json:"topRuns"`
	TopTimed   int     `json:"topTimed"`
	AvgMS      int64   `json:"avgMS"`
	MedianMS   int64   `json:"medianMS"`
	FastestMS  int64   `json:"fastestMS"`
}

// RecapYou is the character a recap is for (the one picked, or the one whose game wrote most of the period's logs): the specs played,
// the median DPS (HPS for a healer spec) per dungeon, the deaths, and what killed them most, most first.
type RecapYou struct {
	Character string       `json:"character"`
	Specs     []string     `json:"specs"`
	Dungeons  []RecapRate  `json:"dungeons"`
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
// recap's character ran there) and how many ranked runs it's over; MedianMS is 0 when fewer than the recap needs.
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
