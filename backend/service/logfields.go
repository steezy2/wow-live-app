package service

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	// advancedFields is the length of the actor block advanced logging adds (GUID, owner, health, max health,
	// attack and spell power, armor, absorb, two stat fields, power type, power, max power, cost, x, y, map,
	// facing, level).
	advancedFields = 19
	// friendlyFlag and hostileFlag are the unit-flag bits for "friendly" and "hostile to the logger".
	friendlyFlag = 0x10
	hostileFlag  = 0x40
	// mineFlag is the unit-flag bit for the logger's own character and pets.
	mineFlag = 0x1
	// physicalSchool is the spell school of melee swings, which the log gives no school for.
	physicalSchool = 0x1
)

// hexInt is a hex field ("0x514") as a number, 0 when it isn't one.
func hexInt(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimPrefix(s, "0x"), 16, 64)
	return v
}

// hasFlag is whether unit flags ("0x514") have a bit set.
func hasFlag(flags string, bit int64) bool {
	return hexInt(flags)&bit != 0
}

// splitName splits "Testmage-Illidan-US" into the name and realm Warcraft Logs shows.
func splitName(full string) (string, string) {
	parts := strings.SplitN(full, "-", 3)
	if len(parts) < 2 {
		return full, ""
	}
	return parts[0], parts[1]
}

func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// specInfo is a specialization as Warcraft Logs names it, and whether it heals (ranked by healing, not damage).
type specInfo struct {
	Class, Name string
	Healer      bool
}

// label is the spec and class with spaces, as players say them: "Beast Mastery Hunter", "Blood Death Knight".
func (s specInfo) label() string {
	return spaced.ReplaceAllString(s.Name+" "+s.Class, "$1 $2")
}

// spaced finds a lower-case letter followed by an upper-case one, where Warcraft Logs' names drop a space.
var spaced = regexp.MustCompile(`([a-z])([A-Z])`)

// specs is each specialization ID's class and name.
var specs = map[int]specInfo{
	250: {"DeathKnight", "Blood", false}, 251: {"DeathKnight", "Frost", false}, 252: {"DeathKnight", "Unholy", false},
	577: {"DemonHunter", "Havoc", false}, 581: {"DemonHunter", "Vengeance", false},
	1480: {"DemonHunter", "Devourer", false},
	102:  {"Druid", "Balance", false}, 103: {"Druid", "Feral", false}, 104: {"Druid", "Guardian", false},
	105:  {"Druid", "Restoration", true},
	1467: {"Evoker", "Devastation", false}, 1468: {"Evoker", "Preservation", true},
	1473: {"Evoker", "Augmentation", false},
	253:  {"Hunter", "BeastMastery", false}, 254: {"Hunter", "Marksmanship", false}, 255: {"Hunter", "Survival", false},
	62: {"Mage", "Arcane", false}, 63: {"Mage", "Fire", false}, 64: {"Mage", "Frost", false},
	268: {"Monk", "Brewmaster", false}, 269: {"Monk", "Windwalker", false}, 270: {"Monk", "Mistweaver", true},
	65: {"Paladin", "Holy", true}, 66: {"Paladin", "Protection", false}, 70: {"Paladin", "Retribution", false},
	256: {"Priest", "Discipline", true}, 257: {"Priest", "Holy", true}, 258: {"Priest", "Shadow", false},
	259: {"Rogue", "Assassination", false}, 260: {"Rogue", "Outlaw", false}, 261: {"Rogue", "Subtlety", false},
	262: {"Shaman", "Elemental", false}, 263: {"Shaman", "Enhancement", false}, 264: {"Shaman", "Restoration", true},
	265: {"Warlock", "Affliction", false}, 266: {"Warlock", "Demonology", false},
	267: {"Warlock", "Destruction", false},
	71:  {"Warrior", "Arms", false}, 72: {"Warrior", "Fury", false}, 73: {"Warrior", "Protection", false},
}
