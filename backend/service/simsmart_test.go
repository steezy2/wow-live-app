package service

import (
	"fmt"
	"testing"
)

func TestSmartStagesRaceOnlyBigSimsAtCoarserErrorsThanTheFinalOne(t *testing.T) {
	tests := []struct {
		n     int
		final float64
		want  []float64
	}{
		{smartMin - 1, 0.2, nil},
		{smartMin, 0.2, []float64{1, 0.5}},
		{500, 0.5, []float64{1}},
		{500, 1, nil},
	}
	for _, tt := range tests {
		if got := smartStages(tt.n, tt.final); fmt.Sprint(got) != fmt.Sprint(tt.want) {
			t.Errorf("smartStages(%d, %v) = %v, want %v", tt.n, tt.final, got, tt.want)
		}
	}
}

func TestSurvivorsDropWhatCannotReachTheBest(t *testing.T) {
	var combos []gearCombo
	var sets []simSetJSON
	for i := range 100 {
		name := fmt.Sprintf("c%d", i)
		combos = append(combos, gearCombo{name: name})
		sets = append(sets, simSetJSON{Name: name, Mean: 1000 - 10*float64(i), MeanStdDev: 1})
	}
	combos = append(combos, gearCombo{name: "missing"}) // simc gave no result: it stays
	got := survivors(combos, sets)
	// the 25th best is 760 (757 with its 3 errors): c0 to c24 are above it, and c25 (750 + 3) is still below
	last := got[len(got)-1].name
	if len(got) != smartKeep+1 || got[0].name != "c0" || got[smartKeep-1].name != "c24" || last != "missing" {
		t.Errorf("survivors = %d, from %s to %s, want the best %d and the one without a result", len(got), got[0].name,
			got[len(got)-1].name, smartKeep)
	}
	// results too noisy to tell apart rule nothing out
	for i := range sets {
		sets[i].MeanStdDev = 1000
	}
	if got := survivors(combos, sets); len(got) != len(combos) {
		t.Errorf("noisy survivors = %d, want all %d", len(got), len(combos))
	}
	if got := survivors(combos[:smartKeep], sets[:smartKeep]); len(got) != smartKeep {
		t.Errorf("a sim of no more than smartKeep combinations keeps them all, got %d", len(got))
	}
}

func TestSmartRunsCostLessThanRunningEverythingAtFullPrecision(t *testing.T) {
	stages := smartStages(1000, 0.2)
	if got := smartRuns(1000, stages, 0.2); got >= 400 || got < 100 {
		t.Errorf("smartRuns(1000) = %v, want well under the 1000 full runs and over its last stage's", got)
	}
}
