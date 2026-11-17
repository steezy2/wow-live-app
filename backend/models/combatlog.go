package models

import (
	"slices"
	"strconv"
)

// Unit flag bits: the combat log's affiliation, reaction, control and type of a unit.
const (
	FlagMine          = 0x1 // the logger's own character and pets
	FlagParty         = 0x2
	FlagRaid          = 0x4
	FlagOutsider      = 0x8
	FlagFriendly      = 0x10
	FlagHostile       = 0x40 // hostile to the logger
	FlagPlayerControl = 0x100
	FlagNPCControl    = 0x200
	FlagPlayerType    = 0x400
	FlagNPCType       = 0x800
	FlagPet           = 0x1000
)

// MPlusDifficulty is the game's difficulty ID for Mythic+ in ZONE_CHANGE and ENCOUNTER lines.
const MPlusDifficulty = 8

// RaidDifficulty is a raid difficulty as the game's log, Warcraft Logs and RaidPull.Difficulty name it.
type RaidDifficulty struct {
	ID     int    // in ZONE_CHANGE and ENCOUNTER lines
	Name   string // as RaidPull.Difficulty
	WCL    int    // Warcraft Logs' number
	Ranked bool   // Warcraft Logs ranks its pulls
}

// RaidDifficulties are every raid difficulty, the one place their IDs, names and numbers meet.
var RaidDifficulties = []RaidDifficulty{
	{ID: 17, Name: "LFR", WCL: 1},
	{ID: 14, Name: "Normal", WCL: 3, Ranked: true},
	{ID: 15, Name: "Heroic", WCL: 4, Ranked: true},
	{ID: 16, Name: "Mythic", WCL: 5, Ranked: true},
}

// raidDifficultyWhere is the first raid difficulty for which match is true; its Name is empty when there is none.
func raidDifficultyWhere(match func(RaidDifficulty) bool) RaidDifficulty {
	if i := slices.IndexFunc(RaidDifficulties, match); i >= 0 {
		return RaidDifficulties[i]
	}
	return RaidDifficulty{}
}

// RaidDifficultyByLogID is the raid difficulty a log line's difficulty field ("16") names.
func RaidDifficultyByLogID(id string) RaidDifficulty {
	return raidDifficultyWhere(func(d RaidDifficulty) bool { return strconv.Itoa(d.ID) == id })
}

// RaidDifficultyByWCL is the raid difficulty with Warcraft Logs' number n.
func RaidDifficultyByWCL(n int) RaidDifficulty {
	return raidDifficultyWhere(func(d RaidDifficulty) bool { return d.WCL == n })
}

// RankedDifficultyWCL is Warcraft Logs' number for the raid difficulty called name, 0 when it ranks none by it.
func RankedDifficultyWCL(name string) int {
	return raidDifficultyWhere(func(d RaidDifficulty) bool { return d.Name == name && d.Ranked }).WCL
}
