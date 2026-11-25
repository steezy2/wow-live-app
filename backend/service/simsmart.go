package service

import (
	"cmp"
	"fmt"
	"slices"
)

const (
	// smartMin is how many combinations a sim needs before it races them in stages: fewer run once, at the precision
	// asked for.
	smartMin = 50
	// smartKeep is how many of the best combinations a stage always keeps (a Top Gear sim shows the top of its list, a
	// Droptimizer the best upgrades), however close the rest are.
	smartKeep = 25
	// smartZ is how many standard errors apart two means must be for the worse one to be dropped.
	smartZ = 3.0
	// profilesetThreads is how many threads each profileset runs on (simc's profileset_work_threads), so simc runs
	// threads/profilesetThreads of them at once: 24 profilesets took 17.5 s with simc's default and 12.6 s with 4 (15
	// took 13.1 s) on a 16-thread PC.
	profilesetThreads = 4
)

// smartErrors are the target errors (in % of the DPS) of the early stages of a raced sim, coarsest first; a stage no
// coarser than the sim's own precision is not run.
var smartErrors = []float64{1, 0.5}

// profilesetSetup is a profileset's setup cost in units of 1/e² (e its target error in %): on a 16-thread PC, 30
// profilesets took 73, 133 and 512 ms each at 1, 0.5 and 0.2%, about 53 ms of setup and 20 ms/e².
const profilesetSetup = 2.65

// smartStages are the target errors of the early stages of a sim of n combinations whose last stage runs at finalError:
// none for a small sim.
func smartStages(n int, finalError float64) []float64 {
	if n < smartMin {
		return nil
	}
	return slices.DeleteFunc(slices.Clone(smartErrors), func(e float64) bool { return e <= finalError })
}

// smartCosts are about how many full-precision runs each stage of a raced sim of n combinations costs, its early
// stages' then its last: a stage costs its combinations times a profileset's cost at its error (stageCost), and an
// early stage keeps about a third of them.
func smartCosts(n int, stages []float64, finalError float64) []float64 {
	left := float64(n)
	costs := make([]float64, 0, len(stages)+1)
	for _, e := range stages {
		costs = append(costs, left*stageCost(e, finalError))
		left = min(left, max(smartKeep*3, left*0.35))
	}
	return append(costs, left)
}

// stageCost is a profileset's cost at targetError in profilesets at finalError.
func stageCost(targetError, finalError float64) float64 {
	return (profilesetSetup + 1/(targetError*targetError)) / (profilesetSetup + 1/(finalError*finalError))
}

// smartRuns is about how many full-precision runs a raced sim of n combinations costs (smartCosts).
func smartRuns(n int, stages []float64, finalError float64) float64 {
	return sum(smartCosts(n, stages, finalError))
}

// sum is the sum of xs.
func sum(xs []float64) float64 {
	total := 0.0
	for _, x := range xs {
		total += x
	}
	return total
}

// survivors are the combinations that a stage's results cannot rule out of the best smartKeep: those whose DPS, plus
// smartZ standard errors, reaches the smartKeep-th best DPS minus as many. A combination with no result stays.
func survivors(combos []gearCombo, sets []simSetJSON) []gearCombo {
	lowers := make([]float64, len(sets))
	for i, s := range sets {
		lowers[i] = s.Mean - smartZ*s.MeanStdDev
	}
	slices.SortFunc(lowers, func(a, b float64) int { return cmp.Compare(b, a) })
	if len(lowers) <= smartKeep {
		return combos
	}
	cut := lowers[smartKeep-1]
	out := make([]gearCombo, 0, len(combos))
	for _, c := range combos {
		i := slices.IndexFunc(sets, func(s simSetJSON) bool { return s.Name == c.name })
		if i < 0 || sets[i].Mean+smartZ*sets[i].MeanStdDev >= cut {
			out = append(out, c)
		}
	}
	return out
}

// readSets is the profilesets' DPS in simc's json2 file at path.
func readSets(path string) ([]simSetJSON, error) {
	out, err := readSimJSON(path)
	return out.Sim.Profilesets.Results, err
}

// stageText says what a raced sim's stage is doing: "Stage 2 of 3: 140 combinations at 0.5%".
func stageText(stage, stages, combos int, targetError float64) string {
	return fmt.Sprintf("Stage %d of %d: %d combinations at %g%%", stage, stages, combos, targetError)
}
