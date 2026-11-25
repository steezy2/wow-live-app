package models

// SuggestTarget is the content a Suggest sim reads the top logs of: Mythic+ overall (Kind "mplus", the season's
// first dungeons) or one raid boss (Kind "raid": Encounter, the boss as ENCOUNTER_START names it, on Difficulty,
// Warcraft Logs' number). It is one of SimChoices.SuggestTargets.
type SuggestTarget struct {
	Kind       FightKind `json:"kind" enums:"mplus,raid" example:"mplus"`
	Encounter  string    `json:"encounter,omitempty" example:"Sszorak"`
	Difficulty int       `json:"difficulty,omitempty" example:"4"`
}

// SuggestTargetChoice is a target a Suggest sim can be started with and the name the page lists it by.
type SuggestTargetChoice struct {
	Name   string        `json:"name" example:"Mythic+ overall"`
	Target SuggestTarget `json:"target"`
}

// Suggestion is one change the Sim tab's Suggest proposes, with the evidence for it. An enchant or gem has a Variant,
// a consumable its Consumables, a talent swap its Talent and the talent it Replaces, gear from the bags and a drop to
// farm their Swaps; a stat is advice only. A talent swap the sim shows as a gain is Kind "talent"; one it shows as no
// gain or a loss is "talentOther": what the top players take for something besides DPS, never a DPS tip.
type Suggestion struct {
	Kind string `json:"kind" enums:"enchant,gem,stat,consumable,talent,talentOther,gear,farm"`
	// Slot is the gear slot an enchant or gem goes in, as simc names it (a ring's is "finger1" or "finger2").
	Slot string `json:"slot,omitempty" example:"finger1"`
	What string `json:"what" example:"Enchant Amani Mastery on finger1"`
	// Evidence says why: how many of the top players do it, or which sim found it.
	Evidence    string         `json:"evidence" example:"14 of 20 top players"`
	Variant     *SimVariant    `json:"variant,omitempty"`
	Consumables SimConsumables `json:"consumables,omitempty"`
	Talent      *TalentShare   `json:"talent,omitempty"`
	// Replaces is the character's talent a talent swap gives up for Talent.
	Replaces *TalentPick `json:"replaces,omitempty"`
	// Swaps are what gear from the bags or a drop changes, and Source and Boss where a drop to farm drops.
	Swaps  []SimSwap `json:"swaps,omitempty"`
	Source string    `json:"source,omitempty" example:"The Venomous Abyss"`
	Boss   string    `json:"boss,omitempty"`
	// Expected is a stat suggestion's DPS gain if the gap closed, from the sim's stat weights.
	Expected float64 `json:"expected,omitempty" example:"410"`
	// Gain is the sim's DPS gain of the change over the unchanged character on one target and DPSError the standard
	// error of that difference; AoEGain and AoEError are the same on the AoE fight. A gain the sim doesn't show on a
	// fight is 0 there, and all are 0 for a stat, which the sim doesn't test, and for a "talentOther" unless the sim
	// shows a loss.
	Gain     float64 `json:"gain,omitempty" example:"812"`
	DPSError float64 `json:"dpsError,omitempty" example:"95"`
	AoEGain  float64 `json:"aoeGain,omitempty" example:"1450"`
	AoEError float64 `json:"aoeError,omitempty" example:"120"`
	// Combined is whether the change is in the report's combined row.
	Combined bool `json:"combined,omitempty"`
	// Group is the report's group the suggestion is ranked in: 0 a change the sim tests, 1 a stat, 2 a talent swap
	// that adds no DPS.
	Group int `json:"group,omitempty" enums:"0,1,2"`
}

// SimSuggestReport is what a Suggest sim found besides the stat weights (SimResult.Weights): the changes, best first
// (those the sim showed as a gain on either fight by the larger of their Gain and AoEGain, then the stats by their
// Expected gain, then the "talentOther" talent swaps), the gain of every change that fits at once (nil with fewer
// than two), how many tested changes other than a
// talent swap the sim showed as no gain (under 0.05% of the DPS, within their error or a loss) and so left out, which
// top logs the suggestions come from and the Warcraft Logs points reading them spent. Notes say what was left out and
// why (no top logs, no talent string, changes not tried, talent swaps the tree doesn't allow, gear over the Top Gear
// limit).
type SimSuggestReport struct {
	Suggestions []Suggestion `json:"suggestions"`
	Combined    *SimCombo    `json:"combined,omitempty"`
	Dropped     int          `json:"dropped"`
	Baseline    string       `json:"baseline,omitempty" example:"20 ranked Arcane Mage runs within 15% of 34:10"`
	Points      float64      `json:"points,omitempty"`
	Notes       []string     `json:"notes"`
}
