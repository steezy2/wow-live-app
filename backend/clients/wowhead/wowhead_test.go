package wowhead

import (
	"reflect"
	"testing"

	"wow-live/backend/models"
)

// page is Den of Nalorakk's overview markup as Wowhead embeds it (escaped), cut to one entry per cell and two tips.
const page = `[tr][td width=250px][b]Important Interrupts[\/b][\/td][td width=250px][b]Important Dispels[\/b][\/td][td width=250px][b]Important Purges[\/b][\/td][\/tr]\r\n[tr][td][ul][li][spell=1239352]\r\n[i]Keen-Eyed Striker (T1)[\/i][\/li]\r\n[\/ul]\r\n[\/td]\r\n\r\n[td][ul][li][symbol=wow-atlas-icons_64x64_curse] [spell=1238801]\r\n[i]Spirit of Hunger (T1)[\/i][\/li]\r\n[\/ul][\/td]\r\n\r\n[td][ul][li][symbol=wow-atlas-icons_64x64_magic] [spell=1297696]*\r\n[i]Earthwhisper Tender (T1)[\/i][\/li]\r\n[\/ul]\r\n[\/td][\/tr]\r\n[tr][td colspan=3][b]Legend[\/b]: T# (Trash Before # Boss)[\/td][\/tr]\r\n[ul][li][spell=1234021] - [b]Dodge[\/b]\r\n[i]Dodgeable cone attack.[\/i][\/li]\r\n[li][spell=1234233] - [b]Soak[\/b]\r\n`

func TestParseGuideReadsTheTableAndTips(t *testing.T) {
	got := ParseGuide(page)
	want := models.DungeonGuide{Interrupts: map[int]string{1239352: "Keen-Eyed Striker"},
		Dispels: map[int]string{1238801: "Spirit of Hunger"}, Purges: map[int]string{1297696: "Earthwhisper Tender"},
		Tips: map[int]string{1234021: "Dodge", 1234233: "Soak"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseGuide = %+v\nwant %+v", got, want)
	}
}
