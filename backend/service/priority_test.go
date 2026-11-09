package service

import (
	"fmt"
	"strings"
	"testing"

	"wow-live/backend/models"
)

func TestMinibossesAndWhenTheyDied(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mapID  int   // 586's dungeon file names mobs 1 and 2 minibosses; 585's names none (the health rule)
		health []int // max health of mobs 1, 2, ...
		deaths []int // the order mobs die in
		want   []models.KeyMiniboss
		late   bool
	}{
		{"the miniboss dies first", 585, []int{900, 300, 300}, []int{1, 2, 3},
			[]models.KeyMiniboss{{Name: "Mob 1", Order: 1, Of: 3}}, false},
		{"the miniboss dies last", 585, []int{900, 300, 300}, []int{2, 3, 1},
			[]models.KeyMiniboss{{Name: "Mob 1", Order: 3, Of: 3}}, true},
		{"two minibosses, one late", 585, []int{900, 900, 300, 300}, []int{1, 3, 4, 2},
			[]models.KeyMiniboss{{Name: "Mob 1", Order: 1, Of: 3}, {Name: "Mob 2", Order: 3, Of: 3}}, true},
		{"a pack of equals has none", 585, []int{300, 300, 300}, []int{1, 2, 3}, nil, false},
		// Only minibosses in the pack: no regular mob to die before, so never late.
		{"a pack of minibosses", 586, []int{900, 900}, []int{2, 1},
			[]models.KeyMiniboss{{Name: "Mob 1", Order: 1, Of: 1}, {Name: "Mob 2", Order: 1, Of: 1}}, false},
	} {
		keys := newTracker(t, nil)
		lines := partyOfTwo(at(0, fmt.Sprintf(`CHALLENGE_MODE_START,"Voidscar Arena",2923,%d,10,[9]`, tc.mapID)))
		for mob, hp := range tc.health {
			guid := fmt.Sprintf("Creature-0-3882-2859-27547-24533%d-00006BE4EA", mob+1)
			block := strings.Replace(advancedOf(guid, "0000000000000000"), ",9,9,", fmt.Sprintf(",%d,%d,", hp, hp), 1)
			lines = append(lines, at(1, fmt.Sprintf(
				`SPELL_DAMAGE,%s,%s,44425,"Arcane Barrage",0x40,%s,1000,1000,-1,64,0,0,0,nil,nil,nil,ST`,
				testPlayer, mobUnit(mob+1), block)))
		}
		for i, mob := range tc.deaths {
			lines = append(lines, at(float64(12+i), "UNIT_DIED,0000000000000000,nil,0x80000000,0x80000000,"+mobUnit(mob)+",0"))
		}
		fed := feed(t, keys, lines...)
		got, _ := keys.Latest(fed[len(fed)-1].Time + 1000)
		if tc.want == nil {
			if len(got.Pulls) != 0 {
				t.Errorf("%s: pulls = %+v, want none", tc.name, got.Pulls)
			}
			continue
		}
		if len(got.Pulls) != 1 || got.Pulls[0].Late != tc.late || len(got.Pulls[0].Minibosses) != len(tc.want) {
			t.Fatalf("%s: pulls = %+v; want %d miniboss(es), late %v", tc.name, got.Pulls, len(tc.want), tc.late)
		}
		for i, m := range got.Pulls[0].Minibosses {
			if m.Name != tc.want[i].Name || m.Order != tc.want[i].Order || m.Of != tc.want[i].Of || m.DiedMS == 0 {
				t.Errorf("%s: miniboss %d = %+v, want %+v", tc.name, i, m, tc.want[i])
			}
		}
	}
}

func TestDamageIntoABossIsPriorityButNotIntoItsAdds(t *testing.T) {
	keys := newTracker(t, nil)
	hit := func(mob int) string {
		guid := fmt.Sprintf("Creature-0-3882-2859-27547-24533%d-00006BE4EA", mob)
		return at(3, fmt.Sprintf(`SPELL_DAMAGE,%s,%s,44425,"Arcane Barrage",0x40,%s,1000,1000,-1,64,0,0,0,nil,nil,nil,ST`,
			testPlayer, mobUnit(mob), advancedOf(guid, "0000000000000000")))
	}
	lines := append(partyOfTwo(at(0, `CHALLENGE_MODE_START,"Den of Nalorakk",2825,586,10,[9]`)),
		at(2, `ENCOUNTER_START,3285,"Nalorakk",8,5,2825`), hit(3), hit(4))
	fed := feed(t, keys, lines...)
	got, _ := keys.Latest(fed[len(fed)-1].Time + 1000)
	if r := got.Overall.Rows[0]; r.Damage != 2000 || r.PriorityDamage != 1000 {
		t.Errorf("row = %+v; want 1000 of 2000 into priority: the boss, not its add", r)
	}
}
