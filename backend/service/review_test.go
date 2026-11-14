package service

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"

	"wow-live/backend/models"
)

// reviewedKey is a +12 of 25:00 with an Arcane Mage and a Restoration Shaman: the mage died with Ice Block ready,
// took the most avoidable damage and kicked less while two must-stop casts went off; a fifth of their damage went into
// priority mobs, and they drank a potion 5 s before the one Bloodlust.
var reviewedKey = models.Key{Start: 2, MapID: 585, Level: 12, Status: "timed", FinalMS: 1_500_000,
	Players: []string{"Mage", "Shaman"}, Specs: map[string]int{"Mage": 62, "Shaman": 264},
	Overall: models.KeyMeter{Rows: []models.KeyMeterRow{
		{Player: "Mage", DPS: 100_000, Interrupts: 2, DamageTaken: 30_000_000, Avoidable: 4_000_000,
			Damage: 150_000_000, PriorityDamage: 30_000_000},
		{Player: "Shaman", HPS: 50_000, Healing: 75_000_000, Interrupts: 9, Dispels: 6, DamageTaken: 20_000_000, Avoidable: 1_000_000}}},
	Deaths: []models.KeyDeath{{Player: "Mage", ClockMS: 723_000, Ability: "Light Bolt", Ready: []string{"Ice Block"}}},
	Stops:  []models.KeyStop{{Outcome: "went off"}, {Outcome: "went off"}, {Outcome: "interrupted", By: "Shaman"}},
	AvoidableHits: []models.KeyHit{{Player: "Mage", Ability: "Burrow", Damage: 4_000_000},
		{Player: "Shaman", Ability: "Burrow", Damage: 1_000_000}},
	Activity: map[string]float64{"Mage": 82.5, "Shaman": 70}, Lusts: []int64{60_000},
	Casts: []models.KeyCast{{Player: "Mage", Kind: "combat potion", ClockMS: 55_000},
		{Player: "Mage", Ability: "Ring of Frost", Kind: "cc", ClockMS: 90_000}},
	BetweenPulls: []int64{20_000, 48_000, 12_000}}

func TestBuildReview(t *testing.T) {
	// The top Arcane runs' median is 105k; the shaman had no baseline but kicked 4 a run before.
	baselines := map[string]map[string][]models.MPlusRanking{"Mage": {
		"DPS": {{Amount: 50_000}, {Amount: 105_000}, {Amount: 200_000}}, "HPS": {{Amount: 4_000}, {Amount: 6_000}}}}
	earlier := []models.Key{{Overall: models.KeyMeter{Rows: []models.KeyMeterRow{{Player: "Shaman", Interrupts: 4}}}}}
	got := buildReview(reviewedKey, earlier, baselines, []float64{20_000, 25_000, 40_000})
	mage, shaman := got.Players[0], got.Players[1]
	// Everyone gets DPS then HPS with their totals, each against its own top runs.
	if m, s := mage.Metrics, shaman.Metrics; m[0].Name != "DPS" || m[0].Top != 105_000 || m[0].Total != 150_000_000 ||
		m[1].Name != "HPS" || m[1].Top != 5_000 || s[1].Top != 0 || s[0].Name != "DPS" || s[1].Name != "HPS" || s[1].Total != 75_000_000 ||
		s[1].TotalName != "healing" {
		t.Fatalf("metrics = %+v, %+v; want DPS and HPS first with totals, the mage's against 105k and 5k", m[:2], s[:2])
	}
	wantAdvice := []string{"Died to Light Bolt at 12:03 with Ice Block ready",
		"Took 4.0M avoidable damage, the most in the group, most of it from Burrow",
		"2 must-stop casts went off; you kicked 2"}
	if !reflect.DeepEqual(mage.Advice, wantAdvice) {
		t.Errorf("mage advice = %q\nwant %q", mage.Advice, wantAdvice)
	}
	if shaman.Advice == nil {
		t.Error("shaman advice is nil; want an empty list for the page")
	}
	for _, tc := range []struct{ player, want string }{
		{mage.Positive, "DPS 100k, 95% of top Arcane Mages at +12"},
		{shaman.Positive, "Better kicks than your usual here: 9 (usually 4)"},
	} {
		if tc.player != tc.want {
			t.Errorf("positive = %q, want %q", tc.player, tc.want)
		}
	}
	metrics := map[string]float64{}
	for _, m := range mage.Metrics {
		metrics[m.Name] = m.Value
	}
	if _, wasted := metrics["wasted %"]; wasted || metrics["priority %"] != 20 || metrics["active %"] != 82.5 ||
		metrics["combat potions"] != 1 || metrics["crowd control"] != 1 {
		t.Errorf("mage metrics = %v; want 20%% priority, 82.5%% active, a potion at lust, a CC, no wasted %%", metrics)
	}
	wantNotes := []string{"1 death added 0:15 to the clock", "Stopped 1 of 3 must-stop casts (33%)",
		"1:20 between pulls in all; the longest 0:48, before pull 3", "1 Bloodlust; 25:00 had room for 3"}
	if !reflect.DeepEqual(got.Notes, wantNotes) {
		t.Errorf("notes = %q, want %q", got.Notes, wantNotes)
	}
	// 50M taken in 1,500 s is 33,333 a second, against the similar runs' median of 25,000.
	if got.Party != "The party took 1.3× the damage per second of 3 top +12 runs of similar time" {
		t.Errorf("party = %q", got.Party)
	}
}

// TestAdviceNamesWhatElseToWorkOn: past the first rules, a player hears of a repeat killer, DPS under their usual, a
// Bloodlust without a potion, a missing flask, idle time and unused crowd control, four at most; the party of its
// late minibosses, debuffs left to run out and overlapped cooldowns.
func TestAdviceNamesWhatElseToWorkOn(t *testing.T) {
	key := models.Key{Level: 12, Status: "timed", FinalMS: 1_500_000, Players: []string{"Mage", "Shaman"},
		Specs:   map[string]int{"Mage": 62, "Shaman": 264},
		Overall: models.KeyMeter{Rows: []models.KeyMeterRow{{Player: "Mage", DPS: 80_000}, {Player: "Shaman", HPS: 50_000}}},
		Deaths:  []models.KeyDeath{{Player: "Mage", Ability: "Burrow"}, {Player: "Mage", Ability: "Burrow"}},
		Stops:   []models.KeyStop{{Outcome: "went off"}},
		Lusts:   []int64{60_000, 700_000, 1_300_000}, Activity: map[string]float64{"Mage": 70, "Shaman": 90},
		Casts: []models.KeyCast{{Player: "Mage", Kind: "combat potion", ClockMS: 61_000}},
		Buffs: []models.KeyBuffCheck{{Player: "Mage", Missing: []string{"flask", "Skyfury"}}},
		Pulls: []models.KeyPull{{Late: true}, {}}, Spikes: []models.KeySpike{{Overlap: true}},
		Debuffs: []models.KeyDispel{{}, {}, {}, {By: "Shaman"}}}
	earlier := []models.Key{{Overall: models.KeyMeter{Rows: []models.KeyMeterRow{{Player: "Mage", DPS: 100_000}}}}}
	got := buildReview(key, earlier, nil, nil)
	wantAdvice := []string{"Died to Burrow 2 times", "DPS 80k, 80% of your usual here (100k)",
		"Used 1 combat potion in a key with 3 Bloodlusts", "Started the key without flask"}
	if mage := got.Players[0].Advice; !reflect.DeepEqual(mage, wantAdvice) {
		t.Errorf("mage advice = %q\nwant %q", mage, wantAdvice)
	}
	key.Deaths, key.Buffs = nil, nil // room under the cap for the idle time
	const idle = "Active 70% of the time in pulls, the least in the group"
	if mage := buildReview(key, earlier, nil, nil).Players[0].Advice; !slices.Contains(mage, idle) {
		t.Errorf("mage advice = %q; want %q among it", mage, idle)
	}
	// The shaman kicked nothing either, but a healer isn't told to; their one line is the crowd control.
	wantShaman := []string{"Used no crowd control while 1 must-stop cast went off: a stun or knock stops a cast too"}
	if shaman := got.Players[1].Advice; !reflect.DeepEqual(shaman, wantShaman) {
		t.Errorf("shaman advice = %q\nwant %q", shaman, wantShaman)
	}
	wantNotes := []string{"2 deaths added 0:30 to the clock", "Stopped 0 of 1 must-stop casts (0%)",
		"Priority target died last of its pack in 1 of 2 pulls",
		"3 of 4 dispellable debuffs ran their full time", "Party cooldowns overlapped in 1 damage spike"}
	if !reflect.DeepEqual(got.Notes, wantNotes) {
		t.Errorf("notes = %q\nwant %q", got.Notes, wantNotes)
	}
}

func TestAnAbandonedKeyIsReviewedOverItsOwnTime(t *testing.T) {
	key := reviewedKey // left 8:20 in, before anyone died
	key.Status, key.FinalMS, key.End, key.Deaths = "abandoned", 0, key.Start+keyCountdownMS+500_000, nil
	baselines := map[string]map[string][]models.MPlusRanking{"Mage": {"DPS": {{Amount: 200_000}}}}
	got := buildReview(key, nil, baselines, []float64{25_000})
	// 50M taken in 500 s is 100,000 a second, against the top run's 25,000.
	wantParty := "The party took 4.0× the damage per second of 1 top +12 runs"
	wantBaseline := "Top runs of each spec at +12 from Warcraft Logs: whole keys, against the 8:20 of this one"
	wantAdvice := "DPS 100k, 50% of top Arcane Mages at +12 (200k)"
	if got.Party != wantParty || got.Baseline != wantBaseline || !slices.Contains(got.Players[0].Advice, wantAdvice) ||
		got.End != key.End {
		t.Errorf("review = %q, %q, %q, end %d\nwant %q, %q, %q, end %d", got.Party, got.Baseline,
			got.Players[0].Advice, got.End, wantParty, wantBaseline, wantAdvice, key.End)
	}
}

// fakeBaselines is Warcraft Logs for the reviewer and the recap: a fixed number of points left, counting its rankings
// asks, and the same top times for every dungeon and level.
type fakeBaselines struct {
	left     float64
	asks     int
	topTimes []int64
	topErr   error
}

func (f *fakeBaselines) MPlusEncounters(context.Context) (map[string]int, error) {
	return map[string]int{"Voidscar Arena": 12923}, nil
}
func (f *fakeBaselines) MPlusRankings(context.Context, int, int, string, string, string) ([]models.MPlusRanking, error) {
	f.asks++
	return []models.MPlusRanking{{Amount: 1, DurationMS: 1_500_000, Code: "x", FightID: 1}}, nil
}
func (f *fakeBaselines) MPlusTopTimes(context.Context, int, int) ([]int64, error) {
	return f.topTimes, f.topErr
}
func (f *fakeBaselines) Run(context.Context, string, int) (models.RunSummary, error) {
	return models.RunSummary{DamageTaken: 1, BossMS: map[string]int64{"Boss": 90_000}}, nil
}
func (f *fakeBaselines) PointsLeft(context.Context) (float64, error) { return f.left, nil }

type memReviews map[int64]models.KeyReview

func (m memReviews) SaveReview(r models.KeyReview) error { m[r.Start] = r; return nil }
func (m memReviews) Review(start int64) (models.KeyReview, error) {
	r, ok := m[start]
	if !ok {
		return r, fs.ErrNotExist
	}
	return r, nil
}

func TestReviewerGuardsPointsAndKeepsReviews(t *testing.T) {
	keys := newTracker(t, nil)
	key := reviewedKey
	key.Dungeon, key.End = "Voidscar Arena", 1_600_000
	keys.history = []models.Key{key}
	source := &fakeBaselines{left: 50}
	r := Reviewer{Keys: keys, Baselines: source, Store: memReviews{}}
	if _, err := r.Review(context.Background(), key.Start); !errors.Is(err, ErrNoPoints) {
		t.Fatalf("with 50 points left: %v, want ErrNoPoints", err)
	}
	source.left = 900
	if got, err := r.Review(context.Background(), key.Start); err != nil || source.asks != 4 ||
		got.TopBosses["Boss"] != 90_000 {
		t.Fatalf("review: %v after %d rankings asks, top bosses %v; want DPS and HPS per player, and Boss 1:30",
			err, source.asks, got.TopBosses)
	}
	if _, err := r.Review(context.Background(), key.Start); err != nil || source.asks != 4 {
		t.Fatalf("second review asked Warcraft Logs again (%d asks, %v); want the kept one", source.asks, err)
	}
	if _, err := r.Cached(99); !errors.Is(err, ErrNoSuchKey) {
		t.Fatalf("an unreviewed key: %v, want ErrNoSuchKey", err)
	}
	keys.history[0].End += 30_000 // closed for a silent log, reviewed, and then it went on
	if _, err := r.Cached(key.Start); !errors.Is(err, ErrNoSuchKey) {
		t.Fatalf("a review from before the key went on: %v, want ErrNoSuchKey", err)
	}
	if got, err := r.Review(context.Background(), key.Start); err != nil || source.asks != 8 || got.End != 1_630_000 {
		t.Fatalf("review after the key went on: end %d, %d asks, %v; want it made again", got.End, source.asks, err)
	}
}

func TestOlderSavedKeysGetEmptyLists(t *testing.T) {
	keys := newTracker(t, nil)
	keys.history = []models.Key{{Start: 5, Status: "timed"}} // saved before stops, spikes and the rest existed
	got, err := keys.SavedKey(5)
	if err != nil || got.Stops == nil || got.WastedKicks == nil || got.Debuffs == nil || got.Spikes == nil ||
		got.AvoidableHits == nil {
		t.Fatalf("saved key = %+v, %v; want empty lists, not null", got.Key, err)
	}
}

func TestSpecLabelAddsWarcraftLogsMissingSpaces(t *testing.T) {
	for id, want := range map[int]string{250: "Blood Death Knight", 253: "Beast Mastery Hunter", 62: "Arcane Mage"} {
		if got := specs[id].label(); got != want {
			t.Errorf("specs[%d].label() = %q, want %q", id, got, want)
		}
	}
}

func TestReviewHealthstonesOnlyWithWarlock(t *testing.T) {
	for _, tc := range []struct {
		class string
		want  bool
	}{{"Warlock", true}, {"Priest", false}} {
		key := models.Key{Players: []string{"Mage"}, Specs: map[string]int{"Mage": 62},
			Activity: map[string]float64{}, Overall: models.KeyMeter{Rows: []models.KeyMeterRow{{Player: "Mage"},
				{Player: "Other", Class: tc.class}}}}
		metrics := buildReview(key, nil, nil, nil).Players[0].Metrics
		got := slices.ContainsFunc(metrics, func(m models.ReviewMetric) bool { return m.Name == "healthstones" })
		if got != tc.want {
			t.Errorf("with a %s: healthstones shown %v, want %v", tc.class, got, tc.want)
		}
		if slices.ContainsFunc(metrics, func(m models.ReviewMetric) bool { return strings.HasPrefix(m.Name, "big hits") }) {
			t.Errorf("with a %s: a big hits column; it moved to the Big hits tab", tc.class)
		}
	}
}

func TestUnusedDefensive(t *testing.T) {
	for _, tc := range []struct {
		name    string
		members []models.KeySpikeMember
		want    string
	}{
		{"hardest hit used one", []models.KeySpikeMember{{Player: "A", Defensive: "Ice Block", Ready: []string{"Alter Time"}}}, ""},
		{"hardest hit had one ready", []models.KeySpikeMember{{Player: "A", Ready: []string{"Ice Block"}}}, "Ice Block"},
		{"only someone else had one ready", []models.KeySpikeMember{{Player: "A"}, {Player: "B", Ready: []string{"Barkskin"}}}, ""},
	} {
		if got := unusedDefensive(models.KeySpike{Members: tc.members}); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestReviewMetrics: combat potions count every potion over the key (a pre-pot too), health potions and healthstones
// are counts alone (no rank, never advice), and a Bloodlust passing without a potion near it is the only advice.
func TestReviewMetrics(t *testing.T) {
	casts := []models.KeyCast{{Player: "Mage", Kind: "combat potion", ClockMS: -3_000},
		{Player: "Mage", Kind: "combat potion", ClockMS: 900_000}, {Player: "Mage", Kind: "health potion"},
		{Player: "Mage", Kind: "healthstone"}, {Player: "Mage", Kind: "healthstone"}}
	for _, tc := range []struct {
		name       string
		lusts      []int64
		wantAdvice []string
	}{
		{"a pre-pot is near the Bloodlust", []int64{5_000}, []string{}},
		{"a Bloodlust without a potion near it", []int64{300_000},
			[]string{"Used 2 combat potions in a key with 1 Bloodlust"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := models.Key{Level: 10, Status: "timed", FinalMS: 1_500_000, Players: []string{"Mage"},
				Specs: map[string]int{"Mage": 62}, Activity: map[string]float64{"Mage": 90}, Casts: casts, Lusts: tc.lusts,
				Overall: models.KeyMeter{Rows: []models.KeyMeterRow{{Player: "Mage", DPS: 100_000},
					{Player: "Lock", Class: "Warlock"}}}}
			mage := buildReview(key, nil, nil, nil).Players[0]
			want := map[string]struct{ value, rank int }{"combat potions": {2, 1}, "health potions": {1, 0},
				"healthstones": {2, 0}}
			for _, m := range mage.Metrics {
				if w, ok := want[m.Name]; ok && (int(m.Value) != w.value || m.PartyRank != w.rank) {
					t.Errorf("%s = %v, rank %d; want %d, rank %d", m.Name, m.Value, m.PartyRank, w.value, w.rank)
				}
				delete(want, m.Name)
			}
			if len(want) > 0 {
				t.Errorf("metrics missing %v", want)
			}
			if !reflect.DeepEqual(mage.Advice, tc.wantAdvice) {
				t.Errorf("advice = %q, want %q", mage.Advice, tc.wantAdvice)
			}
			if strings.Contains(mage.Positive, "health") {
				t.Errorf("positive = %q; a reference count is never the callout", mage.Positive)
			}
		})
	}
}
