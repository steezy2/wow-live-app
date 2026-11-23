package models

// SimReference is reference/sim-options.json (written by cmd/simoptref, from Raidbots' public game data): what the Sim
// tab offers besides the pasted gear. Names that go into a simc input are looked up here, never taken from a request.
type SimReference struct {
	// Build is the game build the data is of, and Source where it was read.
	Build  string `json:"build" example:"12.1.5.70783"`
	Source string `json:"source"`
	// Consumables are the current expansion's, by kind (SimConsumableKinds).
	Consumables map[string][]SimConsumable `json:"consumables"`
	Enchants    []SimEnchant               `json:"enchants"`
	Gems        []SimGem                   `json:"gems"`
	// Upgrades are the upgrade tracks of the current seasons: the bonus ID of each level of each.
	Upgrades []SimUpgradeTrack `json:"upgrades"`
	// SocketBonus is the bonus ID that gives an item one more socket.
	SocketBonus int `json:"socketBonus" example:"10397"`
	// Omnium is the Omnium Folio's rune tree (nodes of one or more choices; picks are entry IDs).
	Omnium []TalentNode `json:"omnium"`
}

// SimConsumableKinds are the consumables a sim can be told to use, in the order the page lists them
// (SimChoices.ConsumableKinds).
var SimConsumableKinds = []string{"flask", "food", "potion", "augment", "weapon"}

// SimConsumable is a flask, food, potion, augment rune or weapon oil: the name simc takes ("flask_of_..._2"), the
// item's name and icon (a name for GET /api/icons/{name}), its item ID and, for a crafted one, its quality (1 to 3).
type SimConsumable struct {
	Value   string `json:"value" example:"flask_of_the_shattered_sun_2"`
	Name    string `json:"name" example:"Flask of the Shattered Sun (Quality 2)"`
	Icon    string `json:"icon"`
	ItemID  int    `json:"itemID"`
	Quality int    `json:"quality,omitempty"`
}

// SimEnchant is an enchant: its ID (simc's enchant_id), the item or spell that gives it, its stats, its crafting
// quality (1 to 3, 0 for one with none) and the slots it can go on.
type SimEnchant struct {
	ID      int      `json:"id" example:"7964"`
	Name    string   `json:"name" example:"Amani Mastery"`
	Stats   string   `json:"stats,omitempty" example:"32 Int & 70 Sta"`
	Icon    string   `json:"icon"`
	Quality int      `json:"quality,omitempty"`
	Slots   []string `json:"slots"`
}

// SimGem is a gem: its item ID (simc's gem_id), name, icon, stats and crafting quality.
type SimGem struct {
	ID      int    `json:"id" example:"240883"`
	Name    string `json:"name" example:"Quick Lapis"`
	Stats   string `json:"stats,omitempty" example:"11 Vers & 4 Haste"`
	Icon    string `json:"icon"`
	Quality int    `json:"quality,omitempty"`
}

// SimUpgradeTrack is a track items upgrade along ("Myth 1/8" to "Myth 8/8"): the group Blizzard keys it by, its name
// and its levels, lowest first.
type SimUpgradeTrack struct {
	Group  int               `json:"group" example:"630"`
	Name   string            `json:"name" example:"Myth"`
	Levels []SimUpgradeLevel `json:"levels"`
}

// SimUpgradeLevel is one level of a track: the bonus ID an item carries at it, its number (from 1) and the item level
// it gives.
type SimUpgradeLevel struct {
	BonusID   int `json:"bonusID" example:"12897"`
	Level     int `json:"level" example:"1"`
	ItemLevel int `json:"itemLevel" example:"388"`
}

// SimConsumables are the consumables a sim uses instead of simc's defaults: a SimConsumable.Value by its kind (one of
// SimConsumableKinds; a kind left out, or empty, keeps simc's default).
type SimConsumables map[string]string

// SimVariant is a changed copy of a Top Gear candidate, sim'd beside it: the item at Index of the profile's Equipped
// (Worn) or Bags, moved to upgrade level Upgrade of its track (0 keeps it), given one more socket (Socket), a gem
// (Gem, an item ID; it goes in the added socket, else the first one) and an enchant (Enchant, an enchant ID).
type SimVariant struct {
	Worn    bool `json:"worn,omitempty"`
	Index   int  `json:"index"`
	Upgrade int  `json:"upgrade,omitempty"`
	Socket  bool `json:"socket,omitempty"`
	Gem     int  `json:"gem,omitempty" example:"240883"`
	Enchant int  `json:"enchant,omitempty" example:"7964"`
}

// SimCompare is the consumable a "consumables" sim compares and the values it tries, each a SimConsumable.Value of
// Kind (one of SimConsumableKinds).
type SimCompare struct {
	Kind   string   `json:"kind" example:"flask"`
	Values []string `json:"values"`
}

// SimPlan is what a Top Gear request would run: how many gear combinations it makes (Max is the most one runs) and
// about how long that takes on this PC, for its fight length and accuracy. Error is why a sim of them would be refused
// (none, or over Max), in the words starting it gives; Notes say what each of the request's variants changes, in its
// order.
type SimPlan struct {
	Combos  int      `json:"combos"`
	Max     int      `json:"max" example:"100"`
	Seconds int      `json:"seconds"`
	Error   string   `json:"error,omitempty"`
	Notes   []string `json:"notes"`
}

// SimOptionRow is one value of a "consumables" sim: its simc name and display name, its DPS, the standard error of
// that mean and its gain over the sim's default (negative for a loss).
type SimOptionRow struct {
	Value    string  `json:"value"`
	Name     string  `json:"name"`
	DPS      float64 `json:"dps"`
	DPSError float64 `json:"dpsError"`
	Gain     float64 `json:"gain"`
}
