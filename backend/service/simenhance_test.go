package service

import (
	"errors"
	"testing"

	"wow-live/backend/models"
)

// enhancedFixture is a head and a ring that each have one socket holding a plain gem, a reference with two gems of
// one limit category (unique-equipped together) and a ring enchant, and a request that tries all of them.
func enhancedFixture() (models.SimReference, models.SimProfile, models.SimRequest) {
	ref := testRef()
	ref.Gems = append(ref.Gems, models.SimGem{ID: 300, Name: "Diamond A", Unique: 698},
		models.SimGem{ID: 301, Name: "Diamond B", Unique: 698})
	p := gearProfile(false)
	for _, i := range []int{0, 1} { // the head and the first ring
		p.Equipped[i].Opts = map[string][]int{"bonus_id": {9}, "gem_id": {240883}}
	}
	req := models.SimRequest{Equipped: []int{0, 1}, Enhancements: &models.SimEnhancements{
		Gems: []int{300, 301}, Enchants: map[string][]int{"finger": {7964}}}}
	return ref, p, req
}

func TestEnhancementCandidatesTryEachGemAndEnchantOnWhatTakesIt(t *testing.T) {
	ref, p, req := enhancedFixture()
	got, err := enhancementCandidates(ref, p, req)
	if err != nil || len(got) != 5 { // two gems on the head, two on the ring and the ring's enchant
		t.Fatalf("got %d candidates (%v), want 5", len(got), err)
	}
	if gems := got[0].item.Opts["gem_id"]; len(gems) != 1 || gems[0] != 300 {
		t.Errorf("head gems %v, want the first gem in its one socket", gems)
	}
	if p.Equipped[0].Opts["gem_id"][0] != 240883 {
		t.Error("the picked item itself changed")
	}
	for _, bad := range []models.SimEnhancements{{Gems: []int{1}}, {Enchants: map[string][]int{"finger": {1}}},
		{Enchants: map[string][]int{"head": {7964}}}} {
		req.Enhancements = &bad
		if _, err := enhancementCandidates(ref, p, req); !errors.Is(err, ErrBadSimOptions) {
			t.Errorf("%+v: err = %v, want ErrBadSimOptions for what the reference doesn't know or the slot can't take", bad, err)
		}
	}
}

func TestTopGearNeverPutsTwoUniqueGemsOfACategoryOnTheGear(t *testing.T) {
	ref, p, req := enhancedFixture()
	extra, err := enhancementCandidates(ref, p, req)
	if err != nil {
		t.Fatal(err)
	}
	combos, err := topGearCombos(ref, p, req.Equipped, nil, extra, false, twoHanders(t))
	// 3 head options x 4 ring options less the unchanged gear, less the 4 that carry a unique gem on both
	if err != nil || len(combos) != 7 {
		t.Errorf("got %d combinations (%v), want 7", len(combos), err)
	}
}
