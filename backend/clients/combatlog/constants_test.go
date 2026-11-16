package combatlog

import "testing"

func TestRaidDifficultyLookups(t *testing.T) {
	if got := RaidDifficultyByLogID("16"); got.Name != "Mythic" || got.WCL != 5 {
		t.Errorf("by log ID 16 = %+v, want Mythic (WCL 5)", got)
	}
	if got := RaidDifficultyByLogID("8"); got.Name != "" {
		t.Errorf("by log ID 8 = %+v, want none (8 is Mythic+)", got)
	}
	if got := RaidDifficultyByWCL(1); got.ID != 17 {
		t.Errorf("by WCL 1 = %+v, want LFR (log ID 17)", got)
	}
	for name, want := range map[string]int{"Mythic": 5, "Heroic": 4, "Normal": 3, "LFR": 0, "Other": 0} {
		if got := RankedDifficultyWCL(name); got != want {
			t.Errorf("ranked %s = %d, want %d", name, got, want)
		}
	}
}
