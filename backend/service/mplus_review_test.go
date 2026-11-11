package service

import (
	"slices"
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

func TestKeySuggestions(t *testing.T) {
	rules := models.KeySuggestionRules{Most: 5, SlowPullPct: 30, GapSeconds: 60}
	pullsLane := func(lengths ...int64) []models.RaidLane {
		var marks []models.RaidMark
		var at int64
		for _, l := range lengths {
			marks = append(marks, models.RaidMark{ClockMS: at, EndMS: at + l})
			at += l + 10_000
		}
		return []models.RaidLane{{Kind: "pulls", Marks: marks}}
	}
	best := &models.Key{Level: 14, Lanes: pullsLane(60_000, 60_000)}
	tests := []struct {
		name string
		key  models.Key
		best *models.Key
		max  int
		want []string
	}{
		{"a death with a defensive ready", models.Key{Deaths: []models.KeyDeath{{Player: "Tank", Ability: "Smash",
			ClockMS: 75_000, Ready: []string{"Shield Wall"}}}}, nil, 5, []string{"Tank died to Smash at 1:15 with Shield Wall ready"}},
		{"a covered death is not a line", models.Key{Deaths: []models.KeyDeath{{Player: "Tank", Defensive: "Shield Wall",
			Ready: []string{"Last Stand"}}}}, nil, 5, []string{}},
		{"the slowest pull against the best run", models.Key{Lanes: pullsLane(65_000, 120_000)}, best, 5,
			[]string{"Pull 2 took 2:00 against 1:00 in your best +14"}},
		{"a slightly slower pull is not", models.Key{Lanes: pullsLane(70_000, 70_000)}, best, 5, []string{}},
		{"no best run, no pull line", models.Key{Lanes: pullsLane(300_000)}, nil, 5, []string{}},
		{"must-stops that went off", models.Key{Stops: []models.KeyStop{{ClockMS: 5_000, Mob: "Mob", Spell: "Bolt",
			Outcome: "went off"}}}, nil, 5, []string{"1 must-stop cast went off, the first Bolt from Mob at 0:05"}},
		{"a long wait", models.Key{BetweenPulls: []int64{20_000, 90_000}}, nil, 5, []string{"1:30 waiting before pull 3"}},
		{"the list is cut at Most", models.Key{BetweenPulls: []int64{90_000}, Pulls: []models.KeyPull{{Late: true}}}, nil, 1,
			[]string{"1:30 waiting before pull 2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := rules
			r.Most = tt.max
			if got := keySuggestions(tt.key, tt.best, r); !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
