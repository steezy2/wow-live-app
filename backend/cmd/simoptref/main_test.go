package main

import (
	"slices"
	"testing"
)

func TestConsumablesOfKeepsTheNewestExpansion(t *testing.T) {
	got := consumablesOf([]rawConsumable{{Value: "new_2", Expansion: 11, Quality: 2}, {Value: "old", Expansion: 10},
		{Value: "new_1", Expansion: 11, Quality: 1}})
	if len(got) != 2 || got[0].Value != "new_2" || got[1].Value != "new_1" || got[0].Quality != 2 {
		t.Errorf("consumablesOf = %+v, want the two expansion-11 entries in order", got)
	}
}

func TestSlotsOf(t *testing.T) {
	tests := []struct {
		name         string
		class, types int
		want         []string
	}{
		{"a ring enchant", armorClass, 1 << 11, []string{"finger1", "finger2"}},
		{"a chest enchant fits chests and robes", armorClass, 1<<5 | 1<<20, []string{"chest"}},
		{"a weapon enchant", weaponClass, 0, []string{"main_hand", "off_hand"}},
		{"a profession's tool", 19, 1 << 29, nil},
		{"no slot of its own", armorClass, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := slotsOf(tt.class, tt.types); !slices.Equal(got, tt.want) {
				t.Errorf("slotsOf = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnchantsAndGemsSplitTheNewestExpansion(t *testing.T) {
	ring := &struct {
		ItemClass int `json:"itemClass"`
		InvTypes  int `json:"invTypeMask"`
	}{armorClass, 1 << 11}
	raw := []rawEnchant{
		{ID: 1, ItemID: 10, ItemName: "Ring Thing", Expansion: 11, Quality: 2, Requirements: ring},
		{ID: 2, ItemID: 11, ItemName: "A Gem", Expansion: 11, Slot: "socket"},
		{ID: 3, ItemID: 12, ItemName: "Old Ring Thing", Expansion: 10, Requirements: ring},
		{ID: 4, ItemName: "A spell enchant with no item", Expansion: 11, Requirements: ring},
	}
	enchants, gems := enchantsAndGems(raw)
	if len(enchants) != 1 || enchants[0].ID != 1 || len(gems) != 1 || gems[0].ID != 11 {
		t.Errorf("enchants %+v, gems %+v; want enchant 1 and gem item 11 only", enchants, gems)
	}
}

func TestUpgradeTracksLeaveOutOldSeasons(t *testing.T) {
	up := func(group, level, ilvl int) rawBonus {
		b := rawBonus{}
		b.Upgrade = &struct {
			Group     int    `json:"group"`
			Level     int    `json:"level"`
			ItemLevel int    `json:"itemLevel"`
			Name      string `json:"name"`
		}{group, level, ilvl, "Myth"}
		return b
	}
	got := upgradeTracks(map[string]rawBonus{"12850": up(618, 2, 321), "12849": up(618, 1, 318), "7195": up(163, 1, 36),
		"1": {}})
	if len(got) != 1 || got[0].Group != 618 || len(got[0].Levels) != 2 || got[0].Levels[0].BonusID != 12849 {
		t.Errorf("upgradeTracks = %+v, want only track 618 with its levels in order", got)
	}
}

func TestWowheadIcon(t *testing.T) {
	for in, want := range map[string]string{
		"inv_12_flask_red__": "inv_12_flask_red--", "inv_12_flask_white_": "inv_12_flask_white-",
		"inv_12_flask_black": "inv_12_flask_black", "": ""} {
		if got := wowheadIcon(in); got != want {
			t.Errorf("wowheadIcon(%q) = %q, want %q", in, got, want)
		}
	}
}
