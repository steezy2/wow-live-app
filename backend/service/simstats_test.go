package service

import (
	"strings"
	"testing"
)

func TestStatWeightsAreAgainstTheMainStat(t *testing.T) {
	got := statWeights(map[string]float64{"Int": 40, "SP": 38, "Crit": 20, "Haste": 10, "Mastery": 0, "Vers": 8})
	want := map[string]float64{"Int": 1, "Crit": 0.5, "Haste": 0.25, "Mastery": 0, "Vers": 0.2}
	if len(got) != len(want) || got[0].Stat != "Int" {
		t.Fatalf("weights = %+v, want the main stat first and the four secondary ones", got)
	}
	for _, w := range got {
		if w.Weight != want[w.Stat] {
			t.Errorf("%s weight = %v, want %v", w.Stat, w.Weight, want[w.Stat])
		}
	}
	if got := statWeights(map[string]float64{"Crit": 5}); got != nil {
		t.Errorf("no main stat: %+v, want none", got)
	}
}

func TestPawnStringNamesTheStatsAsPawnDoes(t *testing.T) {
	got := pawnString("Mage-Frost", statWeights(map[string]float64{"Int": 40, "Crit": 20, "Vers": 8}))
	want := `( Pawn: v1: "Mage-Frost": Intellect=1.00, CritRating=0.50, Versatility=0.20 )`
	if !strings.Contains(got, want) {
		t.Errorf("pawn = %s, want %s", got, want)
	}
}
