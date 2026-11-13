package models

// Key is one Mythic+ run as the combat log shows it: the dungeon, level and affixes, when it started (ms since the
// epoch, also its ID once saved), how it ended, and what happened on the key's clock.
type Key struct {
	Dungeon     string         `json:"dungeon"`
	MapID       int            `json:"mapID"`
	Level       int            `json:"level"`
	Affixes     []Affix        `json:"affixes"`
	Start       int64          `json:"start"`
	End         int64          `json:"end,omitempty"`
	Status      string         `json:"status" enums:"running,timed,depleted,abandoned"`
	FinalMS     int64          `json:"finalMS,omitempty"` // the game's own time for the key, once it ends
	TimerMS     int64          `json:"timerMS,omitempty"` // the dungeon's time limit; 0 when Raider.IO doesn't list it
	Deaths      []KeyDeath     `json:"deaths"`
	Bosses      []KeyBoss      `json:"bosses"`
	Players     []string       `json:"players"`
	Specs       map[string]int `json:"specs"` // player -> specialization ID, from COMBATANT_INFO
	Interrupts  []KeyInterrupt `json:"interrupts"`
	Casts       []KeyCast      `json:"casts"`
	Stops       []KeyStop      `json:"stops"`       // must-stop casts and what happened to each
	WastedKicks []KeyCast      `json:"wastedKicks"` // kicks that interrupted nothing
	Debuffs     []KeyDispel    `json:"debuffs"`     // dispellable debuffs on the party and how fast they came off
	Spikes      []KeySpike     `json:"spikes"`      // the worst bursts of party damage taken, worst first
	// BigHits is each time a player took a big share of their health fast, in order; missing in keys saved before
	// Nov 12.
	BigHits []KeyBigHit `json:"bigHits,omitempty"`
	// Lusts is when the party got Bloodlust or a copy, on the key's clock.
	Lusts []int64 `json:"lusts"`
	// BetweenPulls is the time between each pull and the one before it (from the second pull), on the key's clock.
	BetweenPulls []int64 `json:"betweenPulls"`
	// Activity is each player's share of their time alive in pulls with a cast going or a hit landing (in %).
	Activity map[string]float64 `json:"activity"`
	// AvoidableHits is each player's avoidable damage by ability, most first.
	AvoidableHits []KeyHit `json:"avoidableHits"`
	// Pulls is each pull that had priority mobs, in order.
	Pulls     []KeyPull      `json:"pulls"`
	Reference bool           `json:"reference"`        // reference/mplus-dungeons.json has the dungeon (avoidable list, forces)
	Forces    *KeyForces     `json:"forces,omitempty"` // nil when the dungeon file doesn't have the dungeon
	LastPull  KeyMeter       `json:"lastPull"`
	Overall   KeyMeter       `json:"overall"`
	Buffs     []KeyBuffCheck `json:"buffs"`
	// BuffGaps is each player's flask, food or party buff missing at the key's start (from 0) and each that came off them
	// for a while, in order: there from Nov 12, once the log is read again.
	BuffGaps []BuffGap `json:"buffGaps,omitempty"`
	// Abilities is each player's damage, healing and casts by ability, and Taken their damage taken by ability, most
	// first: there once the key is over, and missing in keys saved before Nov 10.
	Abilities []KeyAbility `json:"abilities,omitempty"`
	Taken     []KeyHit     `json:"taken,omitempty"`
	// Logger is the party member whose game wrote the log ("" in keys saved before Nov 11).
	Logger string `json:"logger,omitempty"`
	// Realms is each player's realm as the log names it ("MoonGuard"); missing in keys saved before Nov 12.
	Realms map[string]string `json:"realms,omitempty"`
	// Report is the Warcraft Logs fight this key is, when one is known (imported, or matched to a report).
	Report *ReportLink `json:"report,omitempty"`
	// Imported marks a key built from a Warcraft Logs report instead of a local combat log.
	Imported bool `json:"imported,omitempty"`
	// Lanes is the key's timeline for the replay (see keyLanes), and Suggestions what to fix next key, worked out when
	// the key is read (see keySuggestions); both empty in keys saved before Nov 11.
	Lanes       []RaidLane `json:"lanes,omitempty"`
	Suggestions []string   `json:"suggestions,omitempty"`
}

// KeyAbility is one ability in a player's key: its damage to enemies and healing done as the meters count them (pets
// under their owner, absorbs as healing), how often it was cast, how often it hit or healed, and its spell school as
// the combat log gives it (a bit per school: 1 physical, 2 holy, 4 fire, 8 nature, 16 frost, 32 shadow, 64 arcane; 0
// in keys saved before Nov 11).
type KeyAbility struct {
	Player  string `json:"player"`
	Ability string `json:"ability"`
	Damage  int64  `json:"damage"`
	Healing int64  `json:"healing"`
	Casts   int    `json:"casts"`
	Hits    int    `json:"hits"`
	School  int    `json:"school,omitempty"`
}

// KeySummary is a saved key in the list: when it started (its ID), the dungeon, level, how it ended, its time and
// the upgrade it gave (as KeyView's Chests; 0 when depleted or abandoned).
type KeySummary struct {
	Start   int64  `json:"start"`
	Dungeon string `json:"dungeon"`
	Level   int    `json:"level"`
	Status  string `json:"status"`
	FinalMS int64  `json:"finalMS"`
	Chests  int    `json:"chests"`
	// Report is the Warcraft Logs fight the key is, when one is known (imported, or matched to a report), and
	// Imported marks a key built from it instead of a local combat log.
	Report   *ReportLink `json:"report,omitempty"`
	Imported bool        `json:"imported,omitempty"`
}

// Affix is a keystone affix: its ID and name ("" when unknown).
type Affix struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// KeyDeath is a player dying during a key, at a time on the key's clock (penalty included): what killed them, and
// what covered the death (a defensive used just before, or one up) or else what they could still have used.
type KeyDeath struct {
	Player    string   `json:"player"`
	ClockMS   int64    `json:"clockMS"`
	Killer    string   `json:"killer"`
	Ability   string   `json:"ability"`
	Defensive string   `json:"defensive"`
	Used      bool     `json:"used,omitempty"` // they pressed Defensive themselves
	Ready     []string `json:"ready"`
}

// KeyInterrupt is a player (or their pet) interrupting a mob's spell.
type KeyInterrupt struct {
	Player  string `json:"player"`
	Kick    string `json:"kick"` // the player's spell that did it ("Wind Shear"); empty in keys saved before Nov 8
	Target  string `json:"target"`
	Spell   string `json:"spell"`
	ClockMS int64  `json:"clockMS"`
}

// KeyCast is a player using a defensive, healthstone, health potion or combat potion, crowd control, or a raid
// cooldown.
type KeyCast struct {
	Player  string `json:"player"`
	Ability string `json:"ability"`
	Kind    string `json:"kind" enums:"defensive,healthstone,health potion,combat potion,kick,cc,raid cooldown"`
	ClockMS int64  `json:"clockMS"`
	EndMS   int64  `json:"endMS,omitempty"`  // a raid cooldown: when its buff last left a raider (0 when none was seen to)
	Prepot  bool   `json:"prepot,omitempty"` // a combat potion drunk before the run started
}

// KeyPull is a trash pull with minibosses (the dungeon file's miniboss NPCs): the share of the party's damage that went
// into them in the pull's first 10 s, their share of the pull's health, where each fell in its own pack's
// kill order, and whether one died last of a pack with regular mobs (Late).
type KeyPull struct {
	Pull       int           `json:"pull"`
	ClockMS    int64         `json:"clockMS"`
	OpeningPct float64       `json:"openingPct"`
	HealthPct  float64       `json:"healthPct"`
	Minibosses []KeyMiniboss `json:"minibosses"`
	Late       bool          `json:"late"`
}

// KeyMiniboss is a miniboss in a pull: when it died on the key's clock, and its place among its pack's regular mobs
// (first hit within 10 s of it) that died: Of is itself plus those, Order is 1 plus those that died before it (0 if it
// didn't die). It's late when it died after all of them. AfterMS is how long after the last of those it died
// (0 when it wasn't last), DamagePct the share of the pull's damage that went into it; both 0 in keys saved before
// Nov 12.
type KeyMiniboss struct {
	Name      string  `json:"name"`
	DiedMS    int64   `json:"diedMS"`
	Order     int     `json:"order"`
	Of        int     `json:"of"`
	AfterMS   int64   `json:"afterMS,omitempty"`
	DamagePct float64 `json:"damagePct"`
}

// KeyHit is damage a player took from one ability: with its direct hits for an avoidable one, and its spell school
// in damage taken where the key has it (as KeyAbility's).
type KeyHit struct {
	Player  string `json:"player"`
	Ability string `json:"ability"`
	Damage  int64  `json:"damage"`
	Hits    int    `json:"hits"` // direct hits; 0 in keys saved before Nov 9
	School  int    `json:"school,omitempty"`
}

// KeyReview is a finished key next to its baselines: a review per party member, a
// line for the party, what the spec baselines are, and the Warcraft Logs points it spent. End is the key's end when
// it was reviewed (0 in reviews made before Nov 10): a key closed as abandoned that then went on is reviewed again.
type KeyReview struct {
	Start    int64          `json:"start"`
	End      int64          `json:"end"`
	Players  []PlayerReview `json:"players"`
	Party    string         `json:"party"`
	Notes    []string       `json:"notes"` // the party as a whole: deaths' cost, must-stop casts, time between pulls
	Baseline string         `json:"baseline"`
	Points   float64        `json:"points"`
	// TopBosses is how long each boss took in the similar top runs (the median, by boss name; missing in reviews
	// saved before Nov 11).
	TopBosses map[string]int64 `json:"topBosses,omitempty"`
}

// PlayerReview is one party member's numbers against their baselines, up to four things to work on with the
// evidence, and exactly one thing they did well.
type PlayerReview struct {
	Player   string         `json:"player"`
	Spec     string         `json:"spec" example:"Blood Death Knight"` // the spec and class, as the page shows them
	Metrics  []ReviewMetric `json:"metrics"`
	Advice   []string       `json:"advice"`
	Positive string         `json:"positive"`
}

// ReviewMetric is one number of a player's run: the median of the top runs of their spec at this level and a similar
// time (0 without; Warcraft Logs' rankings are the leaderboard, so a percentile among them would say little), their
// median in their earlier runs of the dungeon (-1 without), and their rank in the party (1 is best; lower is better
// for damage taken, avoidable damage, deaths and wasted kicks).
type ReviewMetric struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Top       float64 `json:"top"`
	Usual     float64 `json:"usual"`
	PartyRank int     `json:"partyRank"`
	Amount    bool    `json:"amount"`              // shown shortened ("2.1M"), else as a whole number
	Help      string  `json:"help"`                // what the metric counts, for the page to show on hover
	Group     bool    `json:"group,omitempty"`     // it starts a new group of columns (offense, defense, consumables, deaths)
	Total     float64 `json:"total,omitempty"`     // DPS and HPS: the key's damage or healing in all
	TotalName string  `json:"totalName,omitempty"` // what Total is: "damage" or "healing"
}

// MPlusRanking is one top run in Warcraft Logs' rankings for a spec in a dungeon at a keystone level.
type MPlusRanking struct {
	Amount     float64 `json:"amount"` // damage or healing per second
	DurationMS int64   `json:"duration"`
	Code       string  `json:"code"`
	FightID    int     `json:"fightID"`
}

// KeyTrack is where a key's party went and where its mobs died, for the map: each member's position about once a
// second (by name, with their class), and each mob's death where it was last seen (Name: the mob).
type KeyTrack struct {
	Players map[string][]TrackPoint `json:"players"`
	Classes map[string]string       `json:"classes"`
	Deaths  []TrackPoint            `json:"deaths"`
	Meters  map[string][]MeterPoint `json:"meters"` // each player's damage and healing so far, about every 2 s
	Pulls   []PullSpan              `json:"pulls"`  // each pull, in order; missing in keys saved before Nov 8
	// Bosses is a raid boss's units and the adds to burst (by GUID, each point named, with its health) about once a
	// second; a raid pull's or a key's, once it has ended.
	Bosses map[string][]TrackPoint `json:"bosses,omitempty"`
	// Health is the Bosses GUIDs the replay's health box shows: the units whose health is the boss's, then the adds
	// to burst.
	Health []string `json:"health,omitempty"`
	// Mobs is every other hostile mob (by GUID) the replay draws as a small dot: about every 2 s while it acts, and
	// when it died; none in tracks saved before Nov 11.
	Mobs map[string]MobTrail `json:"mobs,omitempty"`
	// Casts is each raider's casts in order, and Abilities what each one's abilities did in each abilityEveryMS
	// stretch, so the replay shows what a raider was casting and their bars up to its moment; none in pulls saved
	// before Nov 11.
	Casts     map[string][]TrackCast     `json:"casts,omitempty"`
	Abilities map[string][]AbilitySeries `json:"abilities,omitempty"`
	// AbilityTimes is when Abilities' stretches ended, in order: a stretch starts where the one before it ended.
	AbilityTimes []int64 `json:"abilityTimes,omitempty"`
	// TargetSeries is each raider's damage into each enemy (by name, as Ability) in the same stretches.
	TargetSeries map[string][]AbilitySeries `json:"targetSeries,omitempty"`
	// Recaps is each death's and big hit's recap, in order; MobCasts each enemy's casts (by GUID, as Mobs and
	// Bosses). Both missing in tracks saved before Nov 12.
	Recaps   []HitRecap             `json:"recaps,omitempty"`
	MobCasts map[string][]TrackCast `json:"mobCasts,omitempty"`
}

// MobTrail is a mob's name, where it went (its points unnamed) and when it died on the clock (0 while alive); the
// name is missing in tracks saved before Nov 12.
type MobTrail struct {
	Name   string       `json:"n,omitempty"`
	Points []TrackPoint `json:"p"`
	Died   int64        `json:"died,omitempty"`
}

// TrackCast is a cast on the pull's clock: when it started, the spell, for one with a cast time when it ended
// (finished, or stopped; 0 for an instant), and whether it never finished (stopped, or the pull ended mid-cast).
type TrackCast struct {
	ClockMS int64  `json:"t"`
	Spell   string `json:"s"`
	EndMS   int64  `json:"e,omitempty"`
	Cut     bool   `json:"x,omitempty"`
}

// AbilitySeries is what one of a raider's abilities did, or what they took damage from, in each stretch it did
// something in: each point is the stretch's end on the pull's clock, then its damage, healing and damage taken
// (numbers only, as a long pull has thousands).
type AbilitySeries struct {
	Ability string     `json:"a"`
	Points  [][4]int64 `json:"p"` // [end clockMS, damage, healing, damage taken]
}

// PullSpan is a pull's start and end on the key's clock; EndMS is 0 while it's being fought.
type PullSpan struct {
	StartMS int64 `json:"startMS"`
	EndMS   int64 `json:"endMS"`
}

// MeterPoint is a player's damage, healing and damage taken so far at a time on the key's clock, and (missing in
// tracks saved before Nov 11) their avoidable damage taken, damage into priority targets, kicks, and dispels and
// purges so far.
type MeterPoint struct {
	ClockMS   int64 `json:"t"`
	Damage    int64 `json:"d"`
	Healing   int64 `json:"h"`
	Taken     int64 `json:"tk"` // damage taken so far; 0 in keys saved before Nov 8
	Avoidable int64 `json:"av,omitempty"`
	Priority  int64 `json:"pd,omitempty"`
	Kicks     int   `json:"k,omitempty"`
	Dispels   int   `json:"ds,omitempty"`
}

// TrackPoint is a place at a time on the key's clock: x and y in yards as the combat log gives them (x north, y
// west), and the map (floor) ID.
type TrackPoint struct {
	ClockMS int64   `json:"t"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Map     int     `json:"map"`
	Name    string  `json:"name,omitempty"`
	HP      float64 `json:"hp,omitempty"` // a raid boss's or add's health left (%), 0 when unknown
}

// KeyStop is a must-stop cast (the dungeon file's mustStop list) and its outcome: interrupted (By the player, IntoMS
// into the cast), stopped (By a debuff that landed during it, or "" when it just didn't finish), went off, or the mob
// died first.
type KeyStop struct {
	Pull    int    `json:"pull"`
	ClockMS int64  `json:"clockMS"`
	Mob     string `json:"mob"`
	Spell   string `json:"spell"`
	Outcome string `json:"outcome" enums:"interrupted,stopped,went off,died"`
	By      string `json:"by"`
	IntoMS  int64  `json:"intoMS,omitempty"`
	Wipe    bool   `json:"wipe,omitempty"` // a raid boss's must-stop that wipes the raid when it goes off
}

// KeyDispel is a dispellable debuff (the dungeon file's dispel list) on a party member: who removed it and how long
// after it landed, or By "" when it ran out instead.
type KeyDispel struct {
	Player  string `json:"player"`
	Spell   string `json:"spell"`
	By      string `json:"by"`
	AfterMS int64  `json:"afterMS"`
	ClockMS int64  `json:"clockMS"`
}

// KeySpike is a burst of party damage taken over Seconds (5 s; a raid pull's: raid-rules.toml's spike_seconds): when,
// how much, the party cooldowns used for it (Overlap when two came within 5 s of each other), and each member hit with
// their cover.
type KeySpike struct {
	ClockMS  int64            `json:"clockMS"`
	Seconds  int64            `json:"seconds"`
	Damage   int64            `json:"damage"`
	PartyCDs []string         `json:"partyCDs"`
	Overlap  bool             `json:"overlap"`
	Members  []KeySpikeMember `json:"members"`
	// Unused is the defensive the member hit hardest had ready and didn't use ("" when they used one or had none
	// ready); missing in keys saved before Nov 11.
	Unused string `json:"unused,omitempty"`
	// Causes is what hit the party hardest in it, most damage first (at most three); missing in keys saved
	// before Nov 12.
	Causes []SpikeCause `json:"causes,omitempty"`
}

// KeyBigHit is a player taking a big share of their health fast ([big_hits] in mplus.toml), kept only when their health
// fell under low_hp_pct in it: when it started on the clock, how much they took (and what % of their max HP), what
// hit them, their cover (as a death's), and whether they died within the recap's length.
type KeyBigHit struct {
	Player    string       `json:"player"`
	ClockMS   int64        `json:"clockMS"`
	Taken     int64        `json:"taken"`
	Pct       float64      `json:"pct"`
	Causes    []SpikeCause `json:"causes"`
	Defensive string       `json:"defensive"`
	Used      bool         `json:"used,omitempty"`
	Ready     []string     `json:"ready"`
	Died      bool         `json:"died,omitempty"`
	RecapMS   int64        `json:"recapMS,omitempty"`
}

// HitRecap is a player's last seconds ([big_hits] recap_seconds) before a death or a big hit ended (ClockMS): what
// hit them, healed them or shielded them, and the buttons pressed for them, oldest first.
type HitRecap struct {
	Player  string       `json:"player"`
	ClockMS int64        `json:"clockMS"`
	MaxHP   int64        `json:"maxHP"`
	Events  []RecapEvent `json:"events"`
}

// RecapEvent is one thing that happened to a player: a hit (Amount with what was absorbed), a heal (overheal left
// out), a shield soaking a hit, or a button pressed (their own, or an external on them); HPPct is their health after
// it, 0 when unknown.
type RecapEvent struct {
	ClockMS int64   `json:"t"`
	Kind    string  `json:"k" enums:"damage,heal,absorb,cast"`
	Ability string  `json:"a"`
	Source  string  `json:"s"`
	Amount  int64   `json:"n,omitempty"`
	HPPct   float64 `json:"hp,omitempty"`
}

// SpikeCause is an ability that hit the party during a spike: who cast it ("" when the log names no one), its damage
// to the party and its hits.
type SpikeCause struct {
	Ability string `json:"ability"`
	Source  string `json:"source"`
	Damage  int64  `json:"damage"`
	Hits    int    `json:"hits"`
}

// KeySpikeMember is a party member hit in a spike: what they took and, by the death rules, the defensive they used or
// had up, else what they still had ready.
type KeySpikeMember struct {
	Player    string   `json:"player"`
	Taken     int64    `json:"taken"`
	Defensive string   `json:"defensive"`
	Used      bool     `json:"used,omitempty"` // they pressed Defensive themselves
	Ready     []string `json:"ready"`
}

// KeyMeter is each player's numbers over a stretch of the key (a pull, or all of it), highest damage first.
type KeyMeter struct {
	StartMS int64         `json:"startMS"`
	EndMS   int64         `json:"endMS"`
	Ongoing bool          `json:"ongoing"` // the pull is still being fought (no pullGapMS pause yet)
	Rows    []KeyMeterRow `json:"rows"`
}

// KeyMeterRow is one player's numbers over a stretch: damage done to enemies (absorbed included) and healing done
// (overheal left out, shields included), in total and per second, pets counted to their owner; interrupts; dispels
// (off the party) and purges (off enemies); damage taken (absorbed included, pets left out) and the part of it from
// the dungeon's avoidable abilities (direct hits only).
type KeyMeterRow struct {
	Player         string  `json:"player"`
	Class          string  `json:"class"` // "Mage", "DeathKnight"; "" when unknown
	Damage         int64   `json:"damage"`
	Healing        int64   `json:"healing"`
	DPS            float64 `json:"dps"`
	HPS            float64 `json:"hps"`
	Interrupts     int     `json:"interrupts"`
	Dispels        int     `json:"dispels"`
	Purges         int     `json:"purges"`
	DamageTaken    int64   `json:"damageTaken"`
	Avoidable      int64   `json:"avoidable"`
	PriorityDamage int64   `json:"priorityDamage"` // damage into trash pulls' minibosses and into bosses
}

// KeyBuffCheck is what a player lacked when the key started: a flask, food, or a party buff a class in the group
// gives.
type KeyBuffCheck struct {
	Player  string   `json:"player"`
	Missing []string `json:"missing"`
}

// KeyProjection is the key's expected final time from earlier runs of the dungeon (runs within two levels count
// double): the weighted median and the 20th to 80th percentile, the upgrade the median gives (3, 2, 1, or 0 for
// depleted), the share of runs pointing at each result (index 0 depleted to 3), how many runs and what it used, and
// the best timed run at the key's level boss by boss.
type KeyProjection struct {
	FinalMS int64      `json:"finalMS"`
	LowMS   int64      `json:"lowMS"`
	HighMS  int64      `json:"highMS"`
	Chests  int        `json:"chests"`
	Odds    []float64  `json:"odds"`
	Runs    int        `json:"runs"`
	Source  string     `json:"source"` // "3 of your runs, forces and bosses"
	Best    []KeyGhost `json:"best"`
}

// KeyGhost is a boss in your best timed run at this level: when you killed it then, and the latest you can kill it
// now and still time the key if the rest goes as it did then.
type KeyGhost struct {
	Name     string `json:"name"`
	KillMS   int64  `json:"killMS"`
	NeedByMS int64  `json:"needByMS"`
}

// KeyForces is a key's enemy forces: the count killed of the dungeon's total, and its share (it can pass 100; 100 once
// a key is finished, since the game needs it), each counted kill's share (saved with the key), the pace against
// your best run at the level (running keys only, when there is one), whether a finished key's kills fell short enough that the dungeon file
// needs checking, and the enemies that died without being in the dungeon file at all.
type KeyForces struct {
	Count      int            `json:"count"`
	Total      int            `json:"total"`
	Pct        float64        `json:"pct"`
	At         []ForcesPoint  `json:"at"`
	Pace       *ForcesPace    `json:"pace,omitempty"`
	NeedsCheck bool           `json:"needsCheck"`
	Uncounted  []UncountedNPC `json:"uncounted"`
}

// UncountedNPC is an enemy that died in a key without a forces count in the dungeon file: its NPC ID, name and how
// many died.
type UncountedNPC struct {
	NPC    int    `json:"npc"`
	Name   string `json:"name"`
	Deaths int    `json:"deaths"`
}

// ForcesPace is how a running key's forces compare with the best run of its dungeon at its level (the fastest timed
// one): the best run's forces at the same clock, and how much sooner (+) or later (-) the key reached its forces than
// the best run did, as a share of the best run's time to get there.
type ForcesPace struct {
	BestPct   float64 `json:"bestPct"`
	FasterPct float64 `json:"fasterPct"`
}

// ForcesPoint is the forces share after a kill, at a time on the key's clock.
type ForcesPoint struct {
	ClockMS int64   `json:"clockMS"`
	Pct     float64 `json:"pct"`
}

// MPlusRules is what the M+ page checks ([defensives], [consumables], [buffs], [kicks], [lust] and [cc] in mplus.toml).
type MPlusRules struct {
	Kicks struct {
		Names []string `toml:"names"`
	} `toml:"kicks"`
	Lust struct {
		Names []string `toml:"names"`
	} `toml:"lust"`
	CC struct {
		Names []string `toml:"names"`
	} `toml:"cc"`
	BigHits     BigHitRules        `toml:"big_hits"`
	Defensives  Defensives         `toml:"defensives"`
	Consumables Consumables        `toml:"consumables"`
	Buffs       map[string]string  `toml:"buffs"` // party buff -> the class that gives it
	Suggestions KeySuggestionRules `toml:"suggestions"`
}

// KeySuggestionRules are mplus.toml's [suggestions]: how many "Fix next key" lines, how much longer (in % and in
// seconds) than in the best run a pull is slow, and how long a wait between pulls is long.
type KeySuggestionRules struct {
	Most            int     `toml:"most"`
	SlowPullPct     float64 `toml:"slow_pull_pct"`
	SlowPullSeconds float64 `toml:"slow_pull_seconds"`
	GapSeconds      float64 `toml:"gap_seconds"`
}

// BigHitRules are mplus.toml's [big_hits]: a big hit is a player taking TankPct (a tank) or OtherPct (anyone else) of
// their max HP within Seconds, kept only when their health fell under LowHPPct in it (0: always; a death
// in it counts as 0); a death's or big hit's recap keeps RecapSeconds.
type BigHitRules struct {
	Seconds      float64 `toml:"seconds"`
	TankPct      float64 `toml:"tank_pct"`
	OtherPct     float64 `toml:"other_pct"`
	RecapSeconds float64 `toml:"recap_seconds"`
	LowHPPct     float64 `toml:"low_hp_pct"`
}

// Defensives are the tracked survival buttons: class defensives, externals, healthstones and health potions, and
// how long before a death a use still counts as covering it.
type Defensives struct {
	WindowSeconds float64  `toml:"window_seconds"`
	Names         []string `toml:"names"`
	Externals     []string `toml:"externals"`
	RaidCooldowns []string `toml:"raid_cooldowns"` // externals the raid lanes also show
	Healthstones  []string `toml:"healthstones"`
	HealthPotions []string `toml:"health_potions"`
}

// Consumables are name patterns for the buffs everyone should have, and the combat potions by name and by their
// buff's aura ID (for a log whose first sighting of one is a pull-start aura list, which gives no names).
type Consumables struct {
	Flask           []string `toml:"flask"`
	Food            []string `toml:"food"`
	CombatPotions   []string `toml:"combat_potions"`
	CombatPotionIDs []int    `toml:"combat_potion_ids"`
}

// RunSummary is a ranked Warcraft Logs run's party damage taken and each boss kill's length, by boss name.
type RunSummary struct {
	DamageTaken int64
	BossMS      map[string]int64
}

// KeyBoss is a boss fight in a key: when it started and ended on the key's clock, and whether it died.
type KeyBoss struct {
	Name    string `json:"name"`
	StartMS int64  `json:"startMS"`
	EndMS   int64  `json:"endMS,omitempty"`
	Kill    bool   `json:"kill"`
}

// KeyView is a key as the page shows it at one moment: the clock with the death penalty (and the penalty on its
// own), the +2 and +3 marks, the most chests still in reach (3, 2, 1, or 0 once over time; -1 without a timer), and
// the projected result when earlier runs of the dungeon give one, and how long each boss took in the best earlier run
// at the level (by name, when there is one).
type KeyView struct {
	Key
	ClockMS    int64            `json:"clockMS"`
	PenaltyMS  int64            `json:"penaltyMS"`
	Plus2MS    int64            `json:"plus2MS"`
	Plus3MS    int64            `json:"plus3MS"`
	Chests     int              `json:"chests"`
	Projection *KeyProjection   `json:"projection,omitempty"`
	BestBosses map[string]int64 `json:"bestBosses,omitempty"`
	// BestFinalMS is the best other timed run's time, for the pace line under the result.
	BestFinalMS int64 `json:"bestFinalMS,omitempty"`
	// SilentMS is how long ago the last line of a running key was logged (0 once it's over). The game writes its log
	// in batches as fights start and end, so a long silence is a long fight, or a party that left.
	SilentMS int64 `json:"silentMS"`
}

// Cast is a player pressing a tracked button (a defensive, healthstone or potion) at a time (ms) in a fight.
type Cast struct {
	Fight   int
	At      int64
	Player  string
	Ability string
	OnOther bool // an external put on someone else: it doesn't cover the caster
}

// BuffUp is a defensive buff on a player from Start to End (ms) in a fight.
type BuffUp struct {
	Fight      int
	Start, End int64
	Player     string
	Ability    string
}

// Death is a player dying at a time (ms) in a fight to an ability, with what covered it (a defensive used just
// before or a defensive buff up) or else what the player could still have used.
type Death struct {
	Fight     int
	At        int64
	Player    string
	Ability   string
	Defensive string
	Used      bool // the player pressed Defensive (it isn't an external or raid cooldown someone else put up)
	Ready     []string
}

// Kit is what each player is known to carry (used at least once) and the shortest time seen between two uses of
// one item (its cooldown, ms), by player and item.
type Kit struct {
	Carried  map[string]map[string]bool
	Cooldown map[[2]string]int64
}

// Settings are the desktop app's choices: the game's combat log folder, the region (for affix names) and the overlay.
type Settings struct {
	LogsDir string          `json:"logsDir"`
	Region  string          `json:"region" enums:"us,eu,kr,tw"`
	Overlay OverlaySettings `json:"overlay"`
	// Character is the player's own character as "Name-Realm" (the realm as the player writes it), whose Warcraft
	// Logs reports can be imported; empty for none.
	Character string `json:"character"`
}

// OverlaySettings are when the overlay shows (during keys, always or never) and its background's opacity (in %; the
// text stays solid).
type OverlaySettings struct {
	Show    string `json:"show" enums:"keys,always,off"`
	Opacity int    `json:"opacity" minimum:"0" maximum:"100"`
	// OneCard shows the pull and the whole key in one card instead of two, one above the other (the default).
	OneCard bool `json:"oneCard"`
	// Rows is how many players each card shows, the top ones (0 for everyone); Columns the cards' columns in order
	// (saving none sets DPS, HPS and kicks).
	Rows    int      `json:"rows" minimum:"0"`
	Columns []string `json:"columns" enums:"dps,hps,kicks,pullDps,taken,avoidable,priority,dispels,deaths"`
	// ClickThrough lets clicks on the overlay through to the game; off (the default), a click on a player's row
	// shows their abilities in the pull.
	ClickThrough bool `json:"clickThrough"`
}

// SettingsView is the settings with what the page shows beside them: whether the log folder exists, when the game
// last wrote a combat log in it (0 for never), and the choices to pick from.
type SettingsView struct {
	Settings
	LogsDirFound bool  `json:"logsDirFound"`
	LastLogMS    int64 `json:"lastLogMS"`
	// Regions are the regions to pick from, OverlayShows when the overlay can show, and MinOpacity its least opacity.
	Regions      []string `json:"regions"`
	OverlayShows []string `json:"overlayShows"`
	// OverlayColumns are the columns the overlay's cards can show.
	OverlayColumns []string `json:"overlayColumns"`
	MinOpacity     int      `json:"minOpacity"`
	// CharacterGuess is the name of the character that wrote most saved keys' logs, to prefill Character with ("" for
	// none); the logs don't name its realm.
	CharacterGuess string `json:"characterGuess"`
}

// LogFile is one combat log in the game's log folder, or one archived as a zip in its WoW Live archive folder, as
// Settings lists them for archiving.
type LogFile struct {
	Name      string `json:"name"` // the log's file name (an archived one without .zip)
	Path      string `json:"-"`    // where it is: the log, or its zip
	Bytes     int64  `json:"bytes"`
	WrittenMS int64  `json:"writtenMS"` // last written (Unix ms)
	FirstMS   int64  `json:"-"`         // its first line's time, 0 when none parses
	Keys      int    `json:"keys"`      // saved keys that started in it
	Pulls     int    `json:"pulls"`     // saved raid pulls that started in it
	Read      bool   `json:"read"`      // read by the current reader (Read old logs), so archiving loses nothing
	Archived  bool   `json:"archived"`
	Active    bool   `json:"active"`          // the game may still write it: it can't be archived
	Suggested bool   `json:"suggested"`       // ticked at first: old, read and not active
	Error     string `json:"error,omitempty"` // why the last archive or delete of it failed
}
