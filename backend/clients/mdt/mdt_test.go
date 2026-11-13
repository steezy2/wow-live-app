package mdt

import (
	"reflect"
	"testing"

	"wow-live/backend/models"
)

// fixture is a cut-down Den of Nalorakk file: the header, one boss and one trash mob with spells.
const fixture = `local dungeonIndex = 161
MDT.mapInfo[dungeonIndex] = {
  englishName = "Den of Nalorakk",
  mapID = 586
};
MDT.dungeonTotalCount[dungeonIndex] = { normal = 729 }
MDT.dungeonEnemies[dungeonIndex] = {
  [1] = {
    ["name"] = "The Hoardmonger",
    ["id"] = 241812,
    ["count"] = 0,
    ["isBoss"] = true,
    ["clones"] = {
    },
  },
  [2] = {
    ["name"] = "Earthwhisper Tender",
    ["id"] = 241814,
    ["count"] = 7,
    ["spells"] = {
      [1241214] = {
        ["interruptible"] = true,
      },
      [1297696] = {
        ["interruptible"] = true,
        ["magic"] = true,
      },
      [1238760] = {
      },
    },
    ["clones"] = {
      [1] = {
        ["x"] = 126.96,
        ["y"] = -471.25,
        ["g"] = 22,
        ["sublevel"] = 1,
      },
      [3] = {
        ["x"] = 111.6,
        ["y"] = -351.89,
        ["sublevel"] = 2,
        ["patrol"] = {
          [1] = {
            ["x"] = 500,
            ["y"] = -10,
          },
        },
      },
    },
  },
};`

func TestParseReadsForcesMobsAndSpellFlags(t *testing.T) {
	got := Parse(fixture)
	got.Clones = nil // TestParseKeepsClones
	want := models.MDTDungeon{Name: "Den of Nalorakk", Index: 161, MapID: 586, Total: 729,
		Mobs: map[int]models.MDTMob{241812: {Name: "The Hoardmonger", Boss: true}, 241814: {Name: "Earthwhisper Tender", Count: 7}},
		Spells: map[int]models.MDTSpell{1241214: {Interruptible: true}, 1297696: {Interruptible: true, Magic: true},
			1238760: {}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse = %+v\nwant %+v", got, want)
	}
}

func TestParseKeepsClones(t *testing.T) {
	// Clone 3's patrol point (500, -10) is not where it stands.
	want := []models.MDTClone{{Index: 2, Clone: 1, NPC: 241814, X: 126.96, Y: -471.25, Group: 22, Sublevel: 1},
		{Index: 2, Clone: 3, NPC: 241814, X: 111.6, Y: -351.89, Sublevel: 2}}
	if got := Parse(fixture).Clones; !reflect.DeepEqual(got, want) {
		t.Fatalf("Clones = %+v\nwant %+v", got, want)
	}
}
