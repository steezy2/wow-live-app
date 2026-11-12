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

// unitName is a line's unit name, "" for none ("nil": the environment, or an effect without a source).
func unitName(name string) string {
	if name == "nil" {
		return ""
	}
	return name
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

// specInfo is a specialization as Warcraft Logs names it, and its role: "healer" (ranked by healing, not damage),
// "tank" or "" for damage.
type specInfo struct {
	Class, Name, Role string
}

// label is the spec and class with spaces, as players say them: "Beast Mastery Hunter", "Blood Death Knight".
func (s specInfo) label() string {
	return spaced.ReplaceAllString(s.Name+" "+s.Class, "$1 $2")
}

// spaced finds a lower-case letter followed by an upper-case one, where Warcraft Logs' names drop a space.
var spaced = regexp.MustCompile(`([a-z])([A-Z])`)

// specs is each specialization ID's class and name.
var specs = map[int]specInfo{
	250: {"DeathKnight", "Blood", "tank"}, 251: {"DeathKnight", "Frost", ""}, 252: {"DeathKnight", "Unholy", ""},
	577: {"DemonHunter", "Havoc", ""}, 581: {"DemonHunter", "Vengeance", "tank"},
	1480: {"DemonHunter", "Devourer", ""},
	102:  {"Druid", "Balance", ""}, 103: {"Druid", "Feral", ""}, 104: {"Druid", "Guardian", "tank"},
	105:  {"Druid", "Restoration", "healer"},
	1467: {"Evoker", "Devastation", ""}, 1468: {"Evoker", "Preservation", "healer"},
	1473: {"Evoker", "Augmentation", ""},
	253:  {"Hunter", "BeastMastery", ""}, 254: {"Hunter", "Marksmanship", ""}, 255: {"Hunter", "Survival", ""},
	62: {"Mage", "Arcane", ""}, 63: {"Mage", "Fire", ""}, 64: {"Mage", "Frost", ""},
	268: {"Monk", "Brewmaster", "tank"}, 269: {"Monk", "Windwalker", ""}, 270: {"Monk", "Mistweaver", "healer"},
	65: {"Paladin", "Holy", "healer"}, 66: {"Paladin", "Protection", "tank"}, 70: {"Paladin", "Retribution", ""},
	256: {"Priest", "Discipline", "healer"}, 257: {"Priest", "Holy", "healer"}, 258: {"Priest", "Shadow", ""},
	259: {"Rogue", "Assassination", ""}, 260: {"Rogue", "Outlaw", ""}, 261: {"Rogue", "Subtlety", ""},
	262: {"Shaman", "Elemental", ""}, 263: {"Shaman", "Enhancement", ""}, 264: {"Shaman", "Restoration", "healer"},
	265: {"Warlock", "Affliction", ""}, 266: {"Warlock", "Demonology", ""},
	267: {"Warlock", "Destruction", ""},
	71:  {"Warrior", "Arms", ""}, 72: {"Warrior", "Fury", ""}, 73: {"Warrior", "Protection", "tank"},
}
