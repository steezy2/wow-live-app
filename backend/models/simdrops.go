package models

// SimDrops is reference/sim-drops.json (written by cmd/simref): the gear the Droptimizer sims, each item the current
// season's raid bosses and Mythic+ dungeons drop with the item levels it drops at.
type SimDrops struct {
	Season  string            `json:"season"`
	Sources map[string]string `json:"sources"`
	// Specs is every playable spec by its ID, named as simc names it.
	Specs    map[int]SimSpec `json:"specs"`
	Raids    []DropSource    `json:"raids"`
	Dungeons []DropSource    `json:"dungeons"`
	// HeroicDungeon is the item level a Heroic dungeon drops, and Keys the Mythic+ levels by key level.
	HeroicDungeon int         `json:"heroicDungeon"`
	Keys          []KeyLevels `json:"keys"`
}

// SimSpec is a spec as simc names it: "death_knight" and "frost".
type SimSpec struct {
	Class string `json:"class"`
	Spec  string `json:"spec"`
}

// DropSource is a raid or dungeon: its Encounter Journal instance, its map (the combat log's instance ID) and its
// bosses in journal order.
type DropSource struct {
	InstanceID int        `json:"instanceID"`
	MapID      int        `json:"mapID"`
	Name       string     `json:"name"`
	Bosses     []DropBoss `json:"bosses"`
}

// DropBoss is one boss's loot, by item ID, and for a raid boss the item level each difficulty drops.
type DropBoss struct {
	Name   string            `json:"name"`
	Levels *DifficultyLevels `json:"levels,omitempty"`
	Items  []DropItem        `json:"items"`
}

// DropItem is a piece of gear a boss drops: its slot ("head", "finger", "trinket", "one_hand", "two_hand",
// "main_hand", "off_hand", ...), its armor type (cloth, leather, mail or plate; empty for the rest) and the spec IDs
// that can loot it (empty when every spec can).
type DropItem struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Slot  string `json:"slot"`
	Armor string `json:"armor,omitempty"`
	Specs []int  `json:"specs,omitempty"`
}

// DifficultyLevels is the item level a raid boss drops on each difficulty.
type DifficultyLevels struct {
	LFR    int `json:"lfr" toml:"lfr"`
	Normal int `json:"normal" toml:"normal"`
	Heroic int `json:"heroic" toml:"heroic"`
	Mythic int `json:"mythic" toml:"mythic"`
}

// KeyLevels is what a Mythic+ key of Level (0 is Mythic 0) drops at its end and in the Great Vault.
type KeyLevels struct {
	Level int `json:"level" toml:"level"`
	Loot  int `json:"loot" toml:"loot"`
	Vault int `json:"vault" toml:"vault"`
}

// SimDropLevels is reference/sim-drops.toml, kept by hand: the season it is for, the DisplaySeasonID its raid's
// journal rows carry, and the item levels of each raid boss group, Heroic dungeons and Mythic+ keys.
type SimDropLevels struct {
	Season        string          `toml:"season"`
	DisplaySeason int             `toml:"display_season"`
	Raid          []RaidDropGroup `toml:"raid"`
	HeroicDungeon int             `toml:"heroic_dungeon"`
	Keys          []KeyLevels     `toml:"keys"`
}

// RaidDropGroup is raid bosses (by their journal names) that drop the same item levels.
type RaidDropGroup struct {
	Bosses []string         `toml:"bosses"`
	Levels DifficultyLevels `toml:"levels"`
}
