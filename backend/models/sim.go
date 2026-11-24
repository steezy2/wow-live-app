package models

// SimProfile is a character read from the SimulationCraft addon's export (`/simc`): who they are, their talents and
// the gear they wear and carry. Only what ParseSimC validated is kept, so a profile is safe to write back into a
// simc input file.
type SimProfile struct {
	Class  string `json:"class" example:"shaman"`
	Name   string `json:"name" example:"Steezy"`
	Level  int    `json:"level" example:"90"`
	Race   string `json:"race" example:"dwarf"`
	Region string `json:"region" example:"us"`
	Server string `json:"server" example:"sargeras"`
	Role   string `json:"role,omitempty" example:"spell"`
	Spec   string `json:"spec" example:"elemental"`
	// Professions is the export's "enchanting=26/jewelcrafting=1", and Omnium its omnium_talents, kept as written.
	Professions string `json:"professions,omitempty"`
	Omnium      string `json:"omnium,omitempty"`
	Talents     string `json:"talents"`
	// Equipped is what the character wears, and Bags the other gear they carry (each in the export's order).
	Equipped []SimItem `json:"equipped"`
	Bags     []SimItem `json:"bags"`
	// ItemLevel is the equipped item level as the game counts it: the 16 slots without shirt and tabard, a main hand
	// with nothing in the off-hand (a two-hander) counted twice, rounded down.
	ItemLevel int `json:"itemLevel" example:"623"`
}

// SimItem is one piece of gear: its slot as simc names it ("finger1", "main_hand"; every carried ring is "finger1",
// as the export writes it), the slots' group Top Gear lists it under ("finger" for both rings, one of
// SimChoices.GearGroups), the name and item level the export's comment gives, the item ID, and its other numbers by
// simc's key ("bonus_id", "gem_id", "enchant_id", ...). A Top Gear variant says what it changed in Note.
type SimItem struct {
	Slot  string           `json:"slot"`
	Group string           `json:"group" example:"finger"`
	Name  string           `json:"name,omitempty"`
	Level int              `json:"level,omitempty"`
	ID    int              `json:"id"`
	Opts  map[string][]int `json:"opts,omitempty"`
	Note  string           `json:"note,omitempty" example:"Myth 3/8, socket"`
}

// SimOptions are the fight a sim runs: its style (one of SimChoices.FightStyles), its length in seconds, how many
// targets and its accuracy (one of SimChoices.Accuracies).
type SimOptions struct {
	FightStyle string `json:"fightStyle" example:"Patchwerk"`
	Length     int    `json:"length" example:"300"`
	Targets    int    `json:"targets" example:"1"`
	Accuracy   string `json:"accuracy" example:"normal"`
}

// SimChoices are what the sim page picks its options from, and the options it starts with.
type SimChoices struct {
	FightStyles []string `json:"fightStyles"`
	Accuracies  []string `json:"accuracies"`
	MinLength   int      `json:"minLength"`
	MaxLength   int      `json:"maxLength"`
	MaxTargets  int      `json:"maxTargets"`
	// MaxCombos is the most gear combinations a Top Gear sim runs; a request over it is refused.
	MaxCombos int `json:"maxCombos" example:"100"`
	// MaxCompare is the most combinations of compared consumables one sim runs.
	MaxCompare int `json:"maxCompare" example:"40"`
	// ConsumableKinds are the kinds of consumable a sim can be told to use, in the order the page lists them.
	ConsumableKinds []string `json:"consumableKinds"`
	// MaxDrops is the most drops a Droptimizer sims.
	MaxDrops int `json:"maxDrops" example:"300"`
	// Difficulties are the raid difficulties a Droptimizer can sim the raid at.
	Difficulties []string `json:"difficulties"`
	// KeyLevels are the Mythic+ key levels a Droptimizer can sim the dungeons at.
	KeyLevels []int      `json:"keyLevels"`
	Defaults  SimOptions `json:"defaults"`
	// DefaultDifficulty and DefaultKeyLevel are what the Droptimizer starts on.
	DefaultDifficulty string `json:"defaultDifficulty" example:"heroic"`
	DefaultKeyLevel   int    `json:"defaultKeyLevel" example:"10"`
	// MaxItemLookups is the most item IDs one GET /api/sim/items takes; the page asks in chunks of it.
	MaxItemLookups int `json:"maxItemLookups" example:"120"`
	// GearGroups are the items' groups (SimItem.Group) in wearing order.
	GearGroups []string `json:"gearGroups"`
}

// SimRequest starts a sim: its Kind (one of SimKinds), the pasted SimC export (parsed again on every request) and
// the options. A Top Gear sim also sends the items it may put on, and a Droptimizer sim the Difficulty and KeyLevel
// its drops are at.
type SimRequest struct {
	Kind    string     `json:"kind" binding:"required" example:"quick"`
	Export  string     `json:"export" binding:"required"`
	Options SimOptions `json:"options"`
	// Equipped and Bags are the items a Top Gear sim may put on, as indexes into the parsed profile's Equipped and
	// Bags. A slot with nothing ticked keeps its item; rings and trinkets with fewer than two ticked are made up from
	// the equipped ones.
	Equipped []int `json:"equipped"`
	Bags     []int `json:"bags"`
	// Variants are changed copies of a Top Gear sim's candidates.
	Variants []SimVariant `json:"variants,omitempty"`
	// Difficulty is the raid difficulty a Droptimizer's drops are at, one of SimChoices.Difficulties.
	Difficulty string `json:"difficulty,omitempty" enums:"lfr,normal,heroic,mythic" example:"heroic"`
	// KeyLevel is the Mythic+ key level a Droptimizer's dungeon drops are at, one of SimChoices.KeyLevels.
	KeyLevel int `json:"keyLevel,omitempty" example:"10"`
	// MaxUpgrade sims every drop of a Droptimizer fully upgraded: at the top rank of its upgrade track, and a Hero or Myth
	// weapon, trinket or neck also with its Ascendant Venomstones.
	MaxUpgrade bool `json:"maxUpgrade,omitempty"`
	// Consumables are what every kind of sim uses instead of simc's defaults, and what it compares.
	Consumables SimConsumables `json:"consumables"`
	// Omnium is the Omnium Folio's runes (entry:rank pairs, as the export writes them): nil keeps the export's, empty
	// is none.
	Omnium *string `json:"omnium,omitempty" example:"136822:1/136817:1"`
}

// SimKinds are the kinds of sim: one character's DPS ("quick"), the best combination of the gear they carry
// ("topgear") and the gain of each piece the season's raid and dungeons drop ("drops"). Any of them can compare
// several flasks, foods, potions, augment runes or weapon oils (SimRequest.Consumables). A sim saved by an earlier
// version can also be "consumables", a quick sim that only compared them.
var SimKinds = []string{"quick", "topgear", "drops"}

// The states a sim is in (Sim.State).
const (
	SimRunning   = "running"
	SimDone      = "done"
	SimFailed    = "failed"
	SimCancelled = "cancelled"
)

// Sim is one run: who, what kind, how far along it is (Progress 0 to 100), when it started (ms since the epoch) and
// how long it took, and, once State is "done", its Result; a failed one says why in Error. The list leaves Result out.
type Sim struct {
	ID        string     `json:"id" example:"9f3a1c5e7b2d4a60"`
	Kind      string     `json:"kind" enums:"quick,topgear,drops,consumables"`
	State     string     `json:"state" enums:"running,done,failed,cancelled"`
	Character string     `json:"character" example:"Steezy"`
	Spec      string     `json:"spec" example:"elemental"`
	Options   SimOptions `json:"options"`
	Progress  int        `json:"progress"`
	StartedMS int64      `json:"startedMS"`
	TookMS    int64      `json:"tookMS,omitempty"`
	Error     string     `json:"error,omitempty"`
	// DPS is the result's DPS, kept when the list leaves Result out.
	DPS    float64    `json:"dps,omitempty"`
	Result *SimResult `json:"result,omitempty"`
}

// SimResult is what a finished sim found: the character's DPS (DPSError is the 1 standard error of the mean), each
// ability's part of it, biggest first, and the buffs that were up, longest first. A Top Gear sim's DPS is the
// equipped gear's, and Combos the other combinations, best first.
type SimResult struct {
	DPS       float64      `json:"dps"`
	DPSError  float64      `json:"dpsError"`
	Abilities []SimAbility `json:"abilities"`
	Buffs     []SimBuff    `json:"buffs"`
	Combos    []SimCombo   `json:"combos,omitempty"`
	// Options are the values a Quick Sim compared, best first.
	Options []SimOptionRow `json:"options,omitempty"`
}

// SimCombo is one gear combination a Top Gear sim ran: its DPS, the standard error of that mean, its gain over the
// equipped gear (negative for a loss) and the slots it changes. A Droptimizer row says where the item drops: its
// Source (the raid's or dungeon's name, as "The Venomous Abyss") and Boss. A sim that compares consumables runs each
// combination once per value and names it in Consumable.
type SimCombo struct {
	DPS        float64   `json:"dps"`
	DPSError   float64   `json:"dpsError"`
	Gain       float64   `json:"gain"`
	Swaps      []SimSwap `json:"swaps"`
	Source     string    `json:"source,omitempty"`
	Boss       string    `json:"boss,omitempty"`
	Consumable string    `json:"consumable,omitempty" example:"Flask of Tempered Aggression"`
}

// SimSwap is one slot a combination changes: the equipped item taken off (Out, none when the slot was empty) and the
// item put on (In, its Slot the one it goes in; none empties the slot, as an off-hand beside a two-handed weapon).
type SimSwap struct {
	Slot string   `json:"slot" example:"finger2"`
	Out  *SimItem `json:"out,omitempty"`
	In   *SimItem `json:"in,omitempty"`
}

// SimAbility is one ability's damage or healing: its spell ID (for the tooltip), its DPS, its share of the total
// (0 to 1), casts per fight and the average amount per cast.
type SimAbility struct {
	Name    string  `json:"name"`
	ID      int     `json:"id"`
	School  string  `json:"school,omitempty"`
	DPS     float64 `json:"dps"`
	Share   float64 `json:"share"`
	Casts   float64 `json:"casts"`
	PerCast float64 `json:"perCast"`
}

// SimBuff is a buff and the share of the fight it was up, in percent.
type SimBuff struct {
	Name   string  `json:"name"`
	ID     int     `json:"id"`
	Uptime float64 `json:"uptime"`
}

// SimEngineStatus is the engine and what the app is doing about it: Installing while a download runs, Error when the
// last one failed, Stale when the installed build is older than the page should trust (a patch makes an old one wrong).
type SimEngineStatus struct {
	SimEngine
	Installing bool   `json:"installing"`
	Stale      bool   `json:"stale"`
	Error      string `json:"error,omitempty"`
}

// SimEngine is the SimulationCraft program the app installed: its file name (the nightly's, or the owner's own
// simc.exe), the version and game patch it printed, when it was built (ms since the epoch; the owner's file's date)
// and its size.
type SimEngine struct {
	Installed bool   `json:"installed"`
	File      string `json:"file,omitempty" example:"simc-1210.01.73a372b-win64.7z"`
	Version   string `json:"version,omitempty" example:"1210-01"`
	Patch     string `json:"patch,omitempty" example:"12.1.5.70529"`
	BuiltMS   int64  `json:"builtMS,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

// SimItemInfo is what the Sim tab draws an item with: its name, its icon (a name for GET /api/icons/{name}) and its
// quality (0 poor, 1 common, 2 uncommon, 3 rare, 4 epic, 5 legendary).
type SimItemInfo struct {
	ID      int    `json:"id" example:"212401"`
	Name    string `json:"name" example:"Ansurek's Final Judgment"`
	Icon    string `json:"icon" example:"inv_axe_1h_nerubianraid_d_01"`
	Quality int    `json:"quality" example:"4"`
}
