package models

// Character is one of the owner's characters: a player whose game wrote saved keys' or raid pulls' logs. ID is
// "Name-Realm", or the name alone when no saved run knows its realm (runs saved before Nov 12); Class is "" when no
// run says. Preferred marks the character Settings names.
type Character struct {
	ID        string `json:"id" example:"Steezy-MoonGuard"`
	Name      string `json:"name"`
	Realm     string `json:"realm"`
	Class     string `json:"class" example:"Mage"`
	Keys      int    `json:"keys"`
	Pulls     int    `json:"pulls"`
	Preferred bool   `json:"preferred,omitempty"`
}
