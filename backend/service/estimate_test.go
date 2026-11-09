package service

import (
	"testing"

	"wow-live/backend/models"
)

func TestAFirstRunIsEstimatedFromTopRunsAtTheKeysPace(t *testing.T) {
	tops := []int64{1_200_000, 1_500_000, 1_800_000}
	for _, tc := range []struct {
		name          string
		clockMS       int64
		forcesPct     float64
		final, lo, hi int64
	}{
		{"at the start, their times", 0, 0, 1_500_000, 1_200_000, 1_800_000},
		// Half the forces in 900 s against their 750 s: a pace of 1.2 on the half still to go.
		{"half done, 20% slower", 900_000, 50, 1_800_000, 1_620_000, 1_980_000},
	} {
		key := models.Key{Level: 12, TimerMS: 1_800_000, Forces: &models.KeyForces{Pct: tc.forcesPct}}
		p := fromTopRuns(key, tc.clockMS, tops)
		if p.FinalMS != tc.final || p.LowMS != tc.lo || p.HighMS != tc.hi || p.Runs != 3 {
			t.Errorf("%s: %+v; want %d (%d–%d) from 3 runs", tc.name, p, tc.final, tc.lo, tc.hi)
		}
	}
}

func TestTopRunsAreAskedForWhileNoFinishedRunOfTheDungeonIsSaved(t *testing.T) {
	for _, tc := range []struct {
		name    string
		earlier models.Key
		want    bool
	}{
		{"only an abandoned run", models.Key{MapID: 585, Status: "abandoned"}, true},
		{"a timed run", models.Key{MapID: 585, Status: "timed", FinalMS: 1_500_000}, false},
	} {
		keys, err := NewLogTracker(nil, nil, models.MPlusRules{}, nil, &memStore{keys: []models.Key{tc.earlier}})
		if err != nil {
			t.Fatal(err)
		}
		feed(t, keys, partyOfTwo(at(0, `CHALLENGE_MODE_START,"Voidscar Arena",2923,585,10,[9]`))...)
		if _, ok := keys.FirstRun(); ok != tc.want {
			t.Errorf("%s: first run %v, want %v", tc.name, ok, tc.want)
		}
	}
}
