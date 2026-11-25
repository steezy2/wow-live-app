package models

// Suggestion is one change the Sim tab's Suggest proposes, with the evidence for it. An enchant or gem has a Variant,
// a consumable its Consumables, a talent its Talent, gear from the bags and a drop to farm their Swaps; a stat is
// advice only.
type Suggestion struct {
	Kind string `json:"kind" enums:"enchant,gem,stat,consumable,talent,gear,farm"`
	// Slot is the gear slot an enchant or gem goes in, as simc names it (a ring's is "finger1" or "finger2").
	Slot string `json:"slot,omitempty" example:"finger1"`
	What string `json:"what" example:"Enchant Amani Mastery on finger1"`
	// Evidence says why: how many of the top players do it, or which sim found it.
	Evidence    string         `json:"evidence" example:"14 of 20 top players"`
	Variant     *SimVariant    `json:"variant,omitempty"`
	Consumables SimConsumables `json:"consumables,omitempty"`
	Talent      *TalentShare   `json:"talent,omitempty"`
	// Swaps are what gear from the bags or a drop changes, and Source and Boss where a drop to farm drops.
	Swaps  []SimSwap `json:"swaps,omitempty"`
	Source string    `json:"source,omitempty" example:"The Venomous Abyss"`
	Boss   string    `json:"boss,omitempty"`
	// Expected is a stat suggestion's DPS gain if the gap closed, from the sim's stat weights.
	Expected float64 `json:"expected,omitempty" example:"410"`
	// Gain is the sim's DPS gain of the change over the unchanged character and DPSError the standard error of that
	// difference; both 0 for a talent or stat, which the sim doesn't test.
	Gain     float64 `json:"gain,omitempty" example:"812"`
	DPSError float64 `json:"dpsError,omitempty" example:"95"`
	// Combined is whether the change is in the report's combined row.
	Combined bool `json:"combined,omitempty"`
}

// SimSuggestReport is what a Suggest sim found besides the stat weights (SimResult.Weights): the changes, best first
// (those the sim tested by their Gain, then the stats by their Expected gain, then the talents), the gain of every
// change that fits at once (nil with fewer than two), how many tested changes the sim showed as no gain (under 0.05%
// of the DPS, within their error or a loss) and so left out, which top logs the suggestions come from and the Warcraft
// Logs points reading them spent. Notes say what was left out and why (no top logs, gear over the Top Gear limit).
type SimSuggestReport struct {
	Suggestions []Suggestion `json:"suggestions"`
	Combined    *SimCombo    `json:"combined,omitempty"`
	Dropped     int          `json:"dropped"`
	Baseline    string       `json:"baseline,omitempty" example:"20 ranked Arcane Mage runs within 15% of 34:10"`
	Points      float64      `json:"points,omitempty"`
	Notes       []string     `json:"notes"`
}
