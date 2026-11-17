package service

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func fixtureSimC(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/simc-export.txt")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseSimCExport(t *testing.T) {
	p, err := ParseSimC(fixtureSimC(t))
	if err != nil {
		t.Fatal(err)
	}
	if p.Class != "shaman" || p.Name != "Testshaman" || p.Spec != "elemental" || p.Level != 90 ||
		p.Server != "testrealm" || p.Region != "us" || p.Omnium == "" || p.Professions != "enchanting=26/jewelcrafting=1" {
		t.Errorf("header = %+v", p)
	}
	if strings.Contains(p.Talents, "#") || !strings.HasPrefix(p.Talents, "CYQALMl7") {
		t.Errorf("talents = %q, want the active ones, not a saved loadout's", p.Talents)
	}
	if len(p.Equipped) != 11 || len(p.Bags) != 4 {
		t.Fatalf("equipped %d, bags %d, want 11 and 4", len(p.Equipped), len(p.Bags))
	}
	head := p.Equipped[0]
	if head.Slot != "head" || head.ID != 271483 || head.Name != "Serpent Crown of the Ophidian Oracle" ||
		head.Level != 334 || len(head.Opts["bonus_id"]) != 6 || head.Opts["enchant_id"][0] != 8017 {
		t.Errorf("head = %+v", head)
	}
	if ring := p.Bags[1]; ring.Slot != "finger1" || ring.Name != "Lightwarden's Bind" || ring.Level != 311 {
		t.Errorf("a carried ring = %+v", ring)
	}
}

func TestParseSimCDropsWhatSimcCouldAbuse(t *testing.T) {
	text := fixtureSimC(t) + `
input=C:\Windows\win.ini
output=C:\Users\x\Documents\overwritten.txt
json2=C:\x.json
head=,id=1,input=C:\x,output=C:\y,bonus_id=1/../../2
level=9999
`
	text = strings.Replace(text, `server=testrealm`, `server=C:\evil`, 1)
	p, err := ParseSimC(text)
	if err != nil {
		t.Fatal(err)
	}
	if p.Server != "" || p.Level != 90 {
		t.Errorf("server %q, level %d: a bad value should leave the field empty", p.Server, p.Level)
	}
	got := SimInput(p)
	for _, bad := range []string{"input", "output", "json2", "win.ini", "evil", ".."} {
		if strings.Contains(got, bad) {
			t.Errorf("the simc input holds %q:\n%s", bad, got)
		}
	}
}

func TestParseSimCRefuses(t *testing.T) {
	good := fixtureSimC(t)
	tests := map[string]string{
		"not an export": "hello",
		"no spec":       strings.Replace(good, "spec=elemental", "", 1),
		"bad name":      strings.Replace(good, `"Testshaman"`, `"x\ninput=y"`, 1),
		"no gear":       "shaman=\"Testshaman\"\nspec=elemental\ntalents=ABC\n",
		"too big":       good + strings.Repeat("#\n", maxProfileBytes),
		"too many carried": strings.Replace(good, bagsHeading,
			bagsHeading+"\n"+strings.Repeat("# head=,id=1\n", maxBagItems+1), 1),
	}
	for name, text := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseSimC(text); !errors.Is(err, ErrBadSimC) {
				t.Errorf("err = %v, want ErrBadSimC", err)
			}
		})
	}
}

func TestSimInputRoundTrips(t *testing.T) {
	p, err := ParseSimC(fixtureSimC(t))
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseSimC(SimInput(p))
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Equipped) != len(p.Equipped) || again.Talents != p.Talents || again.Equipped[0].ID != p.Equipped[0].ID ||
		SimInput(again) != SimInput(p) {
		t.Errorf("reading the generated input changed the profile:\n%s", SimInput(p))
	}
	const head = "head=,id=271483,bonus_id=13692/13440/6652/13695/13698/12854,enchant_id=8017,gem_id=240967"
	if !strings.Contains(SimInput(p), head) {
		t.Errorf("the head line is wrong:\n%s", SimInput(p))
	}
}
