package wowhead

import (
	"fmt"
	"testing"
)

func TestParseRaidGuideReadsTheCheatSheetLines(t *testing.T) {
	page := `<title>Ula&#039;tek Raid Boss Guide - Midnight</title>` +
		`[tab name=\"Boss Cheat Sheet\"]\r\nPhase 1:\r\n[ul][li][b][spell=1292403][\/b] - Dodge the waves.[\/li]\r\n` +
		`[li][b][spell=1287265][\/b] - Soak the group soaks.\r\n[ul][li icon=none]Heroic: Split soaks to deal with ` +
		`[spell=1300685] debuff[\/li][\/ul][\/li]\r\n[li][b]Eggs[\/b] - Pick up all small eggs.[\/li][\/ul][\/tab]` +
		`[tab name=\"Strategy\"][li][spell=1][\/li][\/tab]`
	g := ParseRaidGuide(page)
	var got []string
	for _, l := range g.Lines {
		got = append(got, fmt.Sprint(l.Spells, " ", l.Text))
	}
	if g.Name != "Ula'tek" || fmt.Sprint(got) != "[[1292403] - Dodge the waves. [1287265] - Soak the group soaks.]" {
		t.Fatalf("guide = %q %q; want the name and the two lines headed by a spell, not the Heroic note naming a "+
			"debuff in passing, nor the strategy tab", g.Name, got)
	}
}
