package models

// Suggestion is one change the Sim tab's Suggest proposes, with the evidence for it. An enchant or gem has a Variant,
// a consumable its Consumables, a talent its Talent; a stat is advice only.
type Suggestion struct {
	Kind string `json:"kind" enums:"enchant,gem,stat,consumable,talent"`
	// Slot is the gear slot an enchant or gem goes in, as simc names it (a ring's is "finger1" or "finger2").
	Slot string `json:"slot,omitempty" example:"finger1"`
	What string `json:"what" example:"Enchant Amani Mastery on finger1"`
	// Evidence says why: how many of the top players do it.
	Evidence    string         `json:"evidence" example:"14 of 20 top players"`
	Variant     *SimVariant    `json:"variant,omitempty"`
	Consumables SimConsumables `json:"consumables,omitempty"`
	Talent      *TalentShare   `json:"talent,omitempty"`
	// Expected is a stat suggestion's DPS gain if the gap closed, from the sim's stat weights; the others' gain is the
	// sim's own.
	Expected float64 `json:"expected,omitempty" example:"410"`
}
