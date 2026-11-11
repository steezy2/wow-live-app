package service

import (
	"testing"

	"wow-live/backend/models"
)

func TestKeyLanes(t *testing.T) {
	key := models.Key{
		Pulls:       []models.KeyPull{{ClockMS: 70_000, Late: true}},
		Bosses:      []models.KeyBoss{{Name: "Boss", StartMS: 300_000, EndMS: 360_000, Kill: true}},
		Deaths:      []models.KeyDeath{{Player: "Tank", ClockMS: 80_000, Ability: "Smash"}},
		Lusts:       []int64{100_000, 700_000},
		Casts:       []models.KeyCast{{Player: "Dps", Ability: "Potion", Kind: "combat potion", ClockMS: 101_000}},
		Spikes:      []models.KeySpike{{ClockMS: 90_000, Damage: 3_000_000}},
		Stops:       []models.KeyStop{{ClockMS: 50_000, Mob: "Mob", Spell: "Bolt", Outcome: "went off"}},
		WastedKicks: []models.KeyCast{{Player: "Dps", Ability: "Kick", ClockMS: 55_000}},
	}
	lanes := keyLanes(key, []models.PullSpan{{StartMS: 60_000, EndMS: 120_000}, {StartMS: 200_000, EndMS: 250_000}})
	bad := map[string][]bool{}
	for _, l := range lanes {
		for _, m := range l.Marks {
			bad[l.Name] = append(bad[l.Name], m.Bad)
		}
	}
	want := map[string][]bool{
		"Pulls":         {true, false}, // the first has a miniboss that died late
		"Bosses":        {false},
		"Deaths":        {true},
		"Bloodlust":     {false, true}, // only the first has a potion within 10 s
		"Potions":       {false},
		"Damage spikes": {true}, // no party cooldown
		"Must-stops":    {true},
		"Wasted kicks":  {true},
	}
	for name, flags := range want {
		if len(bad[name]) != len(flags) {
			t.Fatalf("%s: marks %v, want %v", name, bad[name], flags)
		}
		for i := range flags {
			if bad[name][i] != flags[i] {
				t.Errorf("%s mark %d bad = %v, want %v", name, i, bad[name][i], flags[i])
			}
		}
	}
	if lanes[0].Kind != "pulls" || lanes[1].Kind != "bosses" {
		t.Errorf("lanes start %s, %s; want pulls, bosses", lanes[0].Kind, lanes[1].Kind)
	}
}
