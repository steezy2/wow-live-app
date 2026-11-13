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
	rules := models.KeySuggestionRules{Most: 5, SlowPullPct: 30, SlowPullSeconds: 20, GapSeconds: 60}
	pullsOf := func(lengths ...int64) []models.RaidLane {
		var marks []models.RaidMark
		var at int64
		for _, l := range lengths {
			marks = append(marks, models.RaidMark{ClockMS: at, EndMS: at + l})
			at += l + 10_000
		}
		return []models.RaidLane{{Kind: "pulls", Marks: marks}}
	}
	best := &models.Key{Level: 14, Lanes: pullsOf(60_000, 60_000)}
	tests := []struct {
		name string
		key  models.Key
		best *models.Key
		max  int
		want []string
	}{
		{"deaths to one ability", models.Key{Deaths: []models.KeyDeath{{Ability: "Smash"}, {Ability: "Smash"},
			{Ability: "Bolt"}}}, nil, 5, []string{"2 deaths to Smash"}},
		{"a single death is not a line", models.Key{Deaths: []models.KeyDeath{{Ability: "Smash"}}}, nil, 5,
			[]string{}},
		{"the slowest pull against the best run", models.Key{Lanes: pullsOf(65_000, 120_000)}, best, 5,
			[]string{"Pull 2 took 2:00 against 1:00 in your best +14"}},
		{"a slightly slower pull is not", models.Key{Lanes: pullsOf(70_000, 70_000)}, best, 5, []string{}},
		{"another number of pulls is not compared", models.Key{Lanes: pullsOf(65_000, 120_000, 60_000)}, best, 5,
			[]string{}},
		{"no best run, no pull line", models.Key{Lanes: pullsOf(300_000)}, nil, 5, []string{}},
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

func TestLowPriorityDamage(t *testing.T) {
	row := func(name string, damage, priority int64) models.KeyMeterRow {
		return models.KeyMeterRow{Player: name, Damage: damage, PriorityDamage: priority}
	}
	pulls := []models.KeyPull{{Pull: 1}}
	specsOf := map[string]int{"Mage": 62, "Rogue": 259, "Priest": 256} // 256 is Discipline: a healer
	tests := []struct {
		name  string
		pulls []models.KeyPull
		rows  []models.KeyMeterRow
		want  string
	}{
		{"under 20%", pulls, []models.KeyMeterRow{row("Mage", 100, 8), row("Rogue", 100, 40)},
			"Mage put 8% of their damage into priority targets (party 24%)"},
		{"under half the average", pulls, []models.KeyMeterRow{row("Mage", 100, 25), row("Rogue", 100, 90)},
			"Mage put 25% of their damage into priority targets (party 58%)"},
		{"close to the average", pulls, []models.KeyMeterRow{row("Mage", 100, 40), row("Rogue", 100, 50)}, ""},
		{"a healer is skipped", pulls, []models.KeyMeterRow{row("Mage", 100, 50), row("Rogue", 100, 50),
			row("Priest", 100, 0)}, ""},
		{"no priority pulls", nil, []models.KeyMeterRow{row("Mage", 100, 1), row("Rogue", 100, 40)}, ""},
		{"no one hit a priority target", pulls, []models.KeyMeterRow{row("Mage", 100, 0), row("Rogue", 100, 0)}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := models.Key{Pulls: tt.pulls, Specs: specsOf, Overall: models.KeyMeter{Rows: tt.rows}}
			if got := lowPriorityText(key); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLateText(t *testing.T) {
	last := models.KeyMiniboss{Order: 3, Of: 3, AfterMS: 6_000}
	tests := []struct {
		name  string
		pulls []models.KeyPull
		want  string
	}{
		{"none late", []models.KeyPull{{Pull: 1}}, ""},
		{"one died last", []models.KeyPull{{Pull: 2}, {Pull: 5, Late: true, Minibosses: []models.KeyMiniboss{last}},
			{Pull: 6, Late: true, Minibosses: []models.KeyMiniboss{{Order: 3, Of: 3, AfterMS: 0}}}, {Pull: 7}},
			"Priority target died late in 2 of 4 pulls, last up to 6 s after the rest of its pack (pull 5)"},
		{"no gap known", []models.KeyPull{{Pull: 1, Late: true, Minibosses: []models.KeyMiniboss{{Order: 3, Of: 3}}},
			{Pull: 2}}, "Priority target died last of its pack in 1 of 2 pulls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lateText(tt.pulls); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
