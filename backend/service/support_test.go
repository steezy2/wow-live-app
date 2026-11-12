package service

import (
	"fmt"
	"reflect"
	"testing"

	"wow-live/backend/models"
)

// TestSupportDamageMovesToTheEvoker: an Augmentation Evoker's support line moves its share of a party member's hit on
// the boss to the Evoker, on the meter, the ability rows and priority damage; one from an Evoker outside the party
// changes nothing.
func TestSupportDamageMovesToTheEvoker(t *testing.T) {
	const boss = "Creature-0-3882-2859-27547-245333-00006BE4EA"
	for _, tc := range []struct {
		name      string
		supporter string
		between   []string         // lines between the hit and its support line
		want      map[string]int64 // damage, also priority damage, by player
		abilities []models.KeyAbility
	}{
		{"an Evoker in the party", "Player-76-0000BBBB", nil, map[string]int64{"Testmage": 800, "Testevoker": 200},
			[]models.KeyAbility{{Player: "Testmage", Ability: "Arcane Barrage", Damage: 800, Hits: 1, School: 0x40},
				{Player: "Testevoker", Ability: "Ebon Might", Damage: 200, Hits: 1, School: 0xc}}},
		{"an Evoker in another party", "Player-76-0000CCCC", nil, map[string]int64{"Testmage": 1000},
			[]models.KeyAbility{{Player: "Testmage", Ability: "Arcane Barrage", Damage: 1000, Hits: 1, School: 0x40}}},
		{"not after its hit", "Player-76-0000BBBB", []string{at(3, `DAMAGE_SHIELD,`+testPlayer+`,`+mobUnit(3)+
			`,17,"Thorns",0x8,`+advancedOf(boss, "0000000000000000")+`,5,5,-1,8,0,0,0,nil,nil,nil`)},
			map[string]int64{"Testmage": 1000},
			[]models.KeyAbility{{Player: "Testmage", Ability: "Arcane Barrage", Damage: 1000, Hits: 1, School: 0x40}}},
	} {
		keys := newTracker(t, nil)
		lines := append(partyOfTwo(at(0, `CHALLENGE_MODE_START,"Den of Nalorakk",2825,586,10,[9]`)),
			at(1, `SPELL_AURA_APPLIED,Player-76-0000BBBB,"Testevoker-Sargeras-US",0x512,0x80000000,`+
				`Player-76-0000BBBB,"Testevoker-Sargeras-US",0x512,0x80000000,395296,"Ebon Might",0xc,BUFF`),
			at(2, `ENCOUNTER_START,3285,"Nalorakk",8,5,2825`),
			at(3, `SPELL_DAMAGE,`+testPlayer+`,`+mobUnit(3)+`,44425,"Arcane Barrage",0x40,`+
				advancedOf(boss, "0000000000000000")+`,1000,1000,-1,64,0,0,0,nil,nil,nil,ST`))
		lines = append(append(lines, tc.between...),
			at(3, `SPELL_DAMAGE_SUPPORT,`+testPlayer+`,`+mobUnit(3)+`,395152,"Ebon Might",0xc,`+
				advancedOf(boss, "0000000000000000")+`,200,200,-1,12,0,0,0,nil,nil,nil,`+tc.supporter),
			at(9, `CHALLENGE_MODE_END,2825,1,10,9000,100.0,3000.0`))
		feed(t, keys, lines...)
		got, _ := keys.Latest(0)
		rows := map[string]int64{}
		for _, r := range got.Overall.Rows {
			if r.Damage != r.PriorityDamage {
				t.Errorf("%s: %s's damage %d, priority %d; want all into the boss", tc.name, r.Player, r.Damage,
					r.PriorityDamage)
			}
			if r.Damage != 0 {
				rows[r.Player] = r.Damage
			}
		}
		if !reflect.DeepEqual(rows, tc.want) || !reflect.DeepEqual(got.Abilities, tc.abilities) {
			t.Errorf("%s: damage %v, abilities %+v; want %v, %+v", tc.name, rows, got.Abilities, tc.want, tc.abilities)
		}
	}
}

// TestSupportHealingMovesToTheEvoker: the Evoker's share of a party member's heal (overheal left out) and of their
// shield's absorb moves to the Evoker, named after the buff.
func TestSupportHealingMovesToTheEvoker(t *testing.T) {
	const evoker = "Player-76-0000BBBB"
	heal := func(event string, amount, overheal int, spell, tail string) string {
		return fmt.Sprintf(`%s,%s,%s,%s,%s,%d,%d,%d,0,nil%s`, event, testPlayer, testPlayer, spell,
			advancedOf("Player-76-0000AAAA", "0000000000000000"), amount, amount, overheal, tail)
	}
	absorb := func(event string, amount int, spell, tail string) string {
		return fmt.Sprintf(`%s,%s,%s,1238725,"Feast of Misery",0x20,%s,%s,%d,%d,nil%s`, event, mobUnit(3),
			testPlayer, testPlayer, spell, amount, amount, tail)
	}
	keys := newTracker(t, nil)
	feed(t, keys, append(partyOfTwo(at(0, `CHALLENGE_MODE_START,"Den of Nalorakk",2825,586,10,[9]`)),
		at(0.8, `SPELL_AURA_APPLIED,`+evoker+`,"Testevoker-Sargeras-US",0x512,0x80000000,`+evoker+
			`,"Testevoker-Sargeras-US",0x512,0x80000000,395296,"Ebon Might",0xc,BUFF`),
		at(1, heal("SPELL_HEAL", 1000, 100, `2061,"Flash Heal",0x2`, "")),
		at(1, heal("SPELL_HEAL_SUPPORT", 300, 100, `413786,"Fate Mirror",0x40`, ","+evoker)),
		at(2, absorb("SPELL_ABSORBED", 500, `17,"Power Word: Shield",0x2`, "")),
		at(2, absorb("SPELL_ABSORBED_SUPPORT", 50, `413984,"Shifting Sands",0x40`, ","+evoker)),
		at(9, `CHALLENGE_MODE_END,2825,1,10,9000,100.0,3000.0`))...)
	got, _ := keys.Latest(0)
	healing := map[string]int64{}
	for _, r := range got.Overall.Rows {
		healing[r.Player] = r.Healing
	}
	byAbility := map[string]int64{}
	for _, a := range got.Abilities {
		byAbility[a.Player+" "+a.Ability] = a.Healing
	}
	want := map[string]int64{"Testmage Flash Heal": 700, "Testevoker Fate Mirror": 200,
		"Testmage Power Word: Shield": 450, "Testevoker Shifting Sands": 50}
	if healing["Testmage"] != 1150 || healing["Testevoker"] != 250 || !reflect.DeepEqual(byAbility, want) {
		t.Errorf("healing %v, abilities %v; want Testmage 1150, Testevoker 250, %v", healing, byAbility, want)
	}
}
