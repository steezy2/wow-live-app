package wowhead

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestParseSpellReadsNameCooldownAndDescription(t *testing.T) {
	answer := `{"name":"Icebound Fortitude","tooltip":"<table><tr><td><b class=\"whtt-name\">Icebound Fortitude</b>` +
		`<table width=\"100%\"><tr><td>Instant</td><th><!--cooldownText-->2 min cooldown<!--cooldownText-->` +
		`<!--baseCooldown:2 min cooldown--></th></tr></table></td></tr></table><table><tr><td><div class=\"q\">` +
		`Your blood freezes.<br />Reduces damage by 30% &amp; more.</div></td></tr></table>"}`
	got, err := ParseSpell([]byte(answer))
	if err != nil || got.Name != "Icebound Fortitude" || got.Cooldown != "2 min cooldown" ||
		got.Description != "Your blood freezes.\nReduces damage by 30% & more." {
		t.Fatalf("spell = %+v (%v); want its name, cooldown and description as text", got, err)
	}
	bare, err := ParseSpell([]byte(`{"name":"Passive","tooltip":"<table><tr><td><b>Passive</b></td></tr></table>"}`))
	if err != nil || bare.Description != "" {
		t.Errorf("a spell with only a header = %+v (%v); want no description", bare, err)
	}
	if _, err := ParseSpell([]byte(`{"error":"nope"}`)); err == nil {
		t.Error("an answer without a spell read; want an error")
	}
}

func TestSpellDownloadsOnceAndKeepsIt(t *testing.T) {
	asked := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		if r.URL.Path != "/tooltip/spell/48792" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"name":"Icebound Fortitude","tooltip":"<table></table><table><tr><td>Reduces damage.</td></tr></table>"}`))
	}))
	defer server.Close()
	c := Client{CacheDir: t.TempDir(), BaseURL: server.URL}
	for range 2 {
		got, err := c.Spell(context.Background(), 48792)
		if err != nil || got.ID != 48792 || got.Description != "Reduces damage." {
			t.Fatalf("spell = %+v (%v); want its description", got, err)
		}
	}
	if asked != 1 {
		t.Errorf("asked %d times; want the second read from the cache", asked)
	}
	if _, err := c.Spell(context.Background(), 1); err == nil {
		t.Error("a spell Wowhead doesn't have read; want an error")
	}
}
