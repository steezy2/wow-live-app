package models

// SimCrafting is reference/sim-crafting.json (written by cmd/simcraftref, from Raidbots' public game data): what each
// of the season's crafted items can be recrafted with, and the Catalyst's conversions.
type SimCrafting struct {
	// Build is the game build the data is of, and Source where it was read.
	Build  string `json:"build" example:"12.1.5.70783"`
	Source string `json:"source"`
	// Items are the season's crafted items by item ID.
	Items map[int]CraftedItem `json:"items"`
	// Reagents are the items' reagents by item (or, for a crest, currency) ID: one per set of bonus IDs, so a
	// reagent's other qualities are left out.
	Reagents map[int]CraftingReagent `json:"reagents"`
	// Conversions are the season's Catalyst conversions by Raidbots' conversion ID.
	Conversions map[int]ItemConversion `json:"conversions"`
}

// CraftedItem is a crafted item and the IDs of the reagents (SimCrafting.Reagents) its recipe's slots take.
type CraftedItem struct {
	Name     string `json:"name" example:"Aln'hara Lantern"`
	Reagents []int  `json:"reagents" example:"244604,245792,3347"`
}

// The kinds of CraftingReagent: an embellishment or a recraft to a secondary stat pair.
const (
	ReagentEmbellishment = "embellishment"
	ReagentRecraft       = "recraft"
)

// CraftingReagent is a reagent: its kind, name, icon and the bonus IDs a crafted item carries when made with it; a
// recraft's also has its two stats (32 crit, 36 haste, 40 versatility, 49 mastery).
type CraftingReagent struct {
	Kind     string `json:"kind" example:"embellishment"`
	Name     string `json:"name" example:"Blessed Pango Charm"`
	Icon     string `json:"icon,omitempty"`
	BonusIDs []int  `json:"bonusIDs" example:"12686,8960"`
	Stats    []int  `json:"stats,omitempty" example:"32,40"`
}

// ItemConversion is a Catalyst conversion: an item with one of BonusIDs converts to the item of Items with the
// player's class and the item's inventory type, keeping its bonus IDs.
type ItemConversion struct {
	BonusIDs []int          `json:"bonusIDs" example:"13662"`
	Items    []CatalystItem `json:"items"`
}

// CatalystItem is an item the Catalyst makes, with the class that wears it and its inventory type (5 and 20 are both
// chests).
type CatalystItem struct {
	ID            int    `json:"id" example:"271477"`
	Name          string `json:"name" example:"Baleful Grave-Knight's Breastplate"`
	Class         int    `json:"class" example:"6"`
	InventoryType int    `json:"inventoryType" example:"5"`
}
