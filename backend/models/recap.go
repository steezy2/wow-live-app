package models

// RecapPeriod is a period the Recap tab offers: its name and its span in ms since the epoch (To is now for one still
// running).
type RecapPeriod struct {
	Label string `json:"label" example:"This month"`
	From  int64  `json:"from"`
	To    int64  `json:"to"`
}

// Recap is a period's Mythic+ keys and raid pulls read as a story: the keys run and timed, the highest level timed,
// the time spent in keys and the deaths; each dungeon's runs; the owner's character; who they ran with; a few facts
// that each open a key or pull; and the raid by boss.
type Recap struct {
	From         int64          `json:"from"`
	To           int64          `json:"to"`
	Keys         int            `json:"keys"`
	Timed        int            `json:"timed"`
	HighestTimed int            `json:"highestTimed"` // 0 when none was timed
	TimeMS       int64          `json:"timeMS"`
	Deaths       int            `json:"deaths"`
	Dungeons     []RecapDungeon `json:"dungeons"` // most run first
	You          *RecapYou      `json:"you,omitempty"`
	Party        []RecapMate    `json:"party"` // most keys together first
	Facts        []RecapFact    `json:"facts"`
	Raid         []RecapBoss    `json:"raid"`
}

// RecapDungeon is one dungeon in a recap: its runs, how many were timed, the highest level timed and the fastest time
// at that level (0 for both when none was).
type RecapDungeon struct {
	Dungeon   string `json:"dungeon"`
	Runs      int    `json:"runs"`
	Timed     int    `json:"timed"`
	BestLevel int    `json:"bestLevel"`
	BestMS    int64  `json:"bestMS"`
}

// RecapYou is the owner's character in a recap (the one whose game wrote most of the period's logs): the specs played,
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

// RecapMate is a player the owner ran keys with: the keys together and how many of them were timed.
type RecapMate struct {
	Player string `json:"player"`
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
