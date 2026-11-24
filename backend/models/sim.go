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
}

// SimItem is one piece of gear: its slot as simc names it ("finger1", "main_hand"; every carried ring is "finger1",
// as the export writes it), the name and item level the export's comment gives, the item ID, and its other numbers
// by simc's key ("bonus_id", "gem_id", "enchant_id", ...).
type SimItem struct {
	Slot  string           `json:"slot"`
	Name  string           `json:"name,omitempty"`
	Level int              `json:"level,omitempty"`
	ID    int              `json:"id"`
	Opts  map[string][]int `json:"opts,omitempty"`
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
	FightStyles []string   `json:"fightStyles"`
	Accuracies  []string   `json:"accuracies"`
	MinLength   int        `json:"minLength"`
	MaxLength   int        `json:"maxLength"`
	MaxTargets  int        `json:"maxTargets"`
	Defaults    SimOptions `json:"defaults"`
}

// SimRequest starts a sim: its Kind (one of SimKinds), the pasted SimC export (parsed again on every request) and
// the options.
type SimRequest struct {
	Kind    string     `json:"kind" binding:"required" example:"quick"`
	Export  string     `json:"export" binding:"required"`
	Options SimOptions `json:"options"`
}

// SimKinds are the kinds of sim: one character's DPS ("quick").
var SimKinds = []string{"quick"}

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
	Kind      string     `json:"kind" enums:"quick"`
	State     string     `json:"state" enums:"running,done,failed,cancelled"`
	Character string     `json:"character" example:"Steezy"`
	Spec      string     `json:"spec" example:"elemental"`
	Options   SimOptions `json:"options"`
	Progress  int        `json:"progress"`
	StartedMS int64      `json:"startedMS"`
	TookMS    int64      `json:"tookMS,omitempty"`
	Error     string     `json:"error,omitempty"`
	Result    *SimResult `json:"result,omitempty"`
}

// SimResult is what a finished sim found: the character's DPS (DPSError is the 1 standard error of the mean), each
// ability's part of it, biggest first, and the buffs that were up, longest first.
type SimResult struct {
	DPS       float64      `json:"dps"`
	DPSError  float64      `json:"dpsError"`
	Abilities []SimAbility `json:"abilities"`
	Buffs     []SimBuff    `json:"buffs"`
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
