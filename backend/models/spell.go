package models

// SpellInfo is what a spell's hover box shows: its name, the cooldown it has ("2 min cooldown", empty for none) and
// what it does.
type SpellInfo struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Cooldown    string `json:"cooldown,omitempty"`
	Description string `json:"description"`
}
