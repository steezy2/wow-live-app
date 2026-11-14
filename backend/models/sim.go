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

// SimEngine is the SimulationCraft program the app downloaded: its file name on simulationcraft.org, the version and
// game patch it printed, when it was built (ms since the epoch; the nightly list's time) and its size.
type SimEngine struct {
	Installed bool   `json:"installed"`
	File      string `json:"file,omitempty" example:"simc-1210.01.73a372b-win64.7z"`
	Version   string `json:"version,omitempty" example:"1210-01"`
	Patch     string `json:"patch,omitempty" example:"12.1.5.70529"`
	BuiltMS   int64  `json:"builtMS,omitempty"`
	Size      int64  `json:"size,omitempty"`
}
