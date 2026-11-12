package service

import (
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
		want      map[string]int64 // damage, also priority damage, by player
		abilities []models.KeyAbility
	}{
		{"an Evoker in the party", "Player-76-0000BBBB", map[string]int64{"Testmage": 800, "Testevoker": 200},
			[]models.KeyAbility{{Player: "Testmage", Ability: "Arcane Barrage", Damage: 800, Hits: 1, School: 0x40},
				{Player: "Testevoker", Ability: "Ebon Might", Damage: 200, Hits: 1, School: 0xc}}},
		{"an Evoker in another party", "Player-76-0000CCCC", map[string]int64{"Testmage": 1000},
			[]models.KeyAbility{{Player: "Testmage", Ability: "Arcane Barrage", Damage: 1000, Hits: 1, School: 0x40}}},
	} {
		keys := newTracker(t, nil)
		lines := append(partyOfTwo(at(0, `CHALLENGE_MODE_START,"Den of Nalorakk",2825,586,10,[9]`)),
			at(1, `SPELL_AURA_APPLIED,Player-76-0000BBBB,"Testevoker-Sargeras-US",0x512,0x80000000,`+
				`Player-76-0000BBBB,"Testevoker-Sargeras-US",0x512,0x80000000,395296,"Ebon Might",0xc,BUFF`),
			at(2, `ENCOUNTER_START,3285,"Nalorakk",8,5,2825`),
			at(3, `SPELL_DAMAGE,`+testPlayer+`,`+mobUnit(3)+`,44425,"Arcane Barrage",0x40,`+
				advancedOf(boss, "0000000000000000")+`,1000,1000,-1,64,0,0,0,nil,nil,nil,ST`),
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
