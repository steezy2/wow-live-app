package mdt

import (
	"reflect"
	"testing"

	"wow-live/backend/models"
)

// fixture is a cut-down Den of Nalorakk file: the header, one boss and one trash mob with spells.
const fixture = `MDT.mapInfo[dungeonIndex] = {
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
        ["sublevel"] = 1,
      },
    },
  },
};`

func TestParseReadsForcesMobsAndSpellFlags(t *testing.T) {
	got := Parse(fixture)
	want := models.MDTDungeon{Name: "Den of Nalorakk", MapID: 586, Total: 729,
		Mobs: map[int]models.MDTMob{241812: {Name: "The Hoardmonger", Boss: true}, 241814: {Name: "Earthwhisper Tender", Count: 7}},
		Spells: map[int]models.MDTSpell{1241214: {Interruptible: true}, 1297696: {Interruptible: true, Magic: true},
			1238760: {}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse = %+v\nwant %+v", got, want)
	}
}
