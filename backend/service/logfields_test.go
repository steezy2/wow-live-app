package service

import "testing"

func TestSplitNameAndNPCOf(t *testing.T) {
	for _, tc := range []struct{ full, name, realm string }{
		{"Testmage-Illidan-US", "Testmage", "Illidan"}, {"Testmage-Illidan", "Testmage", "Illidan"}, {"Testmage", "Testmage", ""},
	} {
		if name, realm := splitName(tc.full); name != tc.name || realm != tc.realm {
			t.Errorf("splitName(%q) = %q, %q; want %q, %q", tc.full, name, realm, tc.name, tc.realm)
		}
	}
	for guid, want := range map[string]int{
		"Creature-0-3882-2859-27547-245336-00006BE4EA": 245336, "Vehicle-0-1-2-3-99-0000": 99,
		"Player-3676-0A1B2C3D": 0, "Creature-0-3882-2859-27547-245336": 0, "Creature-0-1-2": 0, "": 0,
	} {
		if got := npcOf(guid); got != want {
			t.Errorf("npcOf(%q) = %d; want %d", guid, got, want)
		}
	}
}
