package service

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"wow-live/backend/models"
)

func gearItem(slot string, id int) models.SimItem { return models.SimItem{Slot: slot, ID: id} }

// twoHanders says the weapons with these IDs are two-handed; asking about any other than 31 and 33 fails the test.
func twoHanders(t *testing.T, ids ...int) func(int) (bool, error) {
	return func(id int) (bool, error) {
		if id != 31 && id != 33 {
			t.Errorf("asked whether item %d is two-handed", id)
		}
		return slices.Contains(ids, id), nil
	}
}

// gearProfile wears a head, two rings, two trinkets and a main hand (31; indexes 0 to 5), and an off-hand (6) unless
// twoHanded. Its bags hold a head (0), a ring (1), a second copy of the first worn ring (2), a trinket (3), a main
// hand (33; 4), an off-hand (5) and a neck (6).
func gearProfile(twoHanded bool) models.SimProfile {
	p := models.SimProfile{
		Equipped: []models.SimItem{gearItem("head", 1), gearItem("finger1", 11), gearItem("finger2", 12),
			gearItem("trinket1", 21), gearItem("trinket2", 22), gearItem("main_hand", 31)},
		Bags: []models.SimItem{gearItem("head", 2), gearItem("finger1", 13), gearItem("finger1", 11),
			gearItem("trinket1", 23), gearItem("main_hand", 33), gearItem("off_hand", 34), gearItem("neck", 41)},
	}
	if !twoHanded {
		p.Equipped = append(p.Equipped, gearItem("off_hand", 32))
	}
	return p
}

func TestTopGearCombosFollowTheSlotRules(t *testing.T) {
	tests := []struct {
		name          string
		twoHanders    []int // 31 is worn without an off-hand
		equipped, bag []int
		want          int // 0: refused
	}{
		{"nothing picked", nil, nil, nil, 0},
		{"only the equipped gear", nil, []int{0, 1, 2, 3, 4, 5, 6}, nil, 0},
		{"a bag head beside the equipped one", nil, []int{0}, []int{0}, 1},
		{"an unticked equipped head is not a candidate", nil, nil, []int{0, 6}, 1},
		{"a bag head and neck, the equipped head ticked", nil, []int{0}, []int{0, 6}, 2},
		{"a bag ring beside both equipped", nil, []int{1, 2}, []int{1}, 2},
		{"the equipped rings make up the pair", nil, nil, []int{1}, 2},
		{"the same ring can't be worn twice", nil, []int{1, 2}, []int{2}, 1},
		{"a ring and a trinket", nil, []int{1, 2, 3, 4}, []int{1, 3}, 8},
		{"a bag main hand beside a one-hander", nil, []int{5, 6}, []int{4}, 1},
		{"main hands and off-hands", nil, []int{5, 6}, []int{4, 5}, 3},
		{"a bag two-hander beside a worn one-hander and off-hand empties the off-hand", []int{33}, []int{5, 6},
			[]int{4, 5}, 2},
		{"a two-hander and a bag main hand", []int{31}, []int{5}, []int{4}, 1},
		{"a two-hander, a bag main hand and an off-hand", []int{31}, []int{5}, []int{4, 5}, 1},
		{"an index outside the export", nil, nil, []int{7}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			combos, err := topGearCombos(gearProfile(slices.Contains(tt.twoHanders, 31)), tt.equipped, tt.bag,
				twoHanders(t, tt.twoHanders...))
			if tt.want == 0 {
				if !errors.Is(err, ErrBadSimOptions) || combos != nil {
					t.Errorf("got %d combos, err %v; want ErrBadSimOptions", len(combos), err)
				}
				return
			}
			if err != nil || len(combos) != tt.want {
				t.Errorf("got %d combos, err %v; want %d", len(combos), err, tt.want)
			}
		})
	}
}

func TestTopGearWeaponsEmptyOrFillTheOffHand(t *testing.T) {
	combos, err := topGearCombos(gearProfile(false), []int{5, 6}, []int{4, 5}, twoHanders(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range combos {
		for _, s := range c.swaps {
			if s.Slot == "off_hand" && s.In == nil {
				t.Errorf("%s empties the off-hand beside a one-hander: %+v", c.name, c.swaps)
			}
		}
	}
	// a picked off-hand makes the bag main hand a one-hander beside a worn two-hander
	// a one-handed bag main hand beside a worn two-hander takes the picked off-hand
	combos, err = topGearCombos(gearProfile(true), []int{5}, []int{4, 5}, twoHanders(t, 31))
	if err != nil || len(combos) != 1 || len(combos[0].swaps) != 2 || combos[0].swaps[0].In.ID != 33 ||
		combos[0].swaps[1].Slot != "off_hand" || combos[0].swaps[1].Out != nil || combos[0].swaps[1].In.ID != 34 {
		t.Errorf("combos = %+v, %v", combos, err)
	}
	if got := gearProfilesets(combos); got != "profileset.combo1+=main_hand=,id=33\nprofileset.combo1+=off_hand=,id=34\n" {
		t.Errorf("profilesets = %q", got)
	}
	// a two-handed bag main hand takes the worn off-hand off
	combos, err = topGearCombos(gearProfile(false), nil, []int{4}, twoHanders(t, 33))
	want := "profileset.combo1+=main_hand=,id=33\nprofileset.combo1+=off_hand=\n"
	if got := gearProfilesets(combos); err != nil || got != want {
		t.Errorf("profilesets = %q, %v", got, err)
	}
	if _, err := topGearCombos(gearProfile(false), nil, []int{4}, func(int) (bool, error) {
		return false, errors.New("offline")
	}); err == nil {
		t.Error("a weapon whose type can't be read should fail the sim, not guess")
	}
}

func TestTopGearRefusesOverTheCap(t *testing.T) {
	p := models.SimProfile{Equipped: []models.SimItem{gearItem("head", 1), gearItem("neck", 2)}}
	var picked []int
	for i := range 10 {
		p.Bags = append(p.Bags, gearItem("head", 100+i), gearItem("neck", 200+i))
		picked = append(picked, 2*i, 2*i+1)
	}
	if combos, err := topGearCombos(p, nil, picked, twoHanders(t)); err != nil || len(combos) != maxCombos {
		t.Fatalf("10 heads by 10 necks: %d combos, %v; want %d", len(combos), err, maxCombos)
	}
	p.Bags = append(p.Bags, gearItem("head", 110))
	if combos, err := topGearCombos(p, nil, append(picked, len(p.Bags)-1), twoHanders(t)); !errors.Is(err,
		ErrBadSimOptions) ||
		combos != nil {
		t.Errorf("11 heads by 10 necks: %d combos, %v; want refused, not cut", len(combos), err)
	}
}

func TestSimsRunTopGear(t *testing.T) {
	s := newTestSims(t, "ok")
	result, err := filepath.Abs("testdata/sim-topgear-result.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WOWLIVE_FAKE_RESULT", result)
	args := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("WOWLIVE_FAKE_ARGS", args)
	req := simRequest(t)
	// all equipped ticked, plus the bags' head and ring: the head 2 ways times the rings 3 ways, less the equipped gear
	req.Kind, req.Equipped, req.Bags = "topgear", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, []int{0, 1}
	started, err := s.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	sim := waitSim(t, s, started.ID)
	if sim.State != models.SimDone {
		t.Fatalf("sim = %+v", sim)
	}

	input, err := os.ReadFile(args + ".simc")
	if err != nil {
		t.Fatal(err)
	}
	ring := "finger2=,id=251194,bonus_id=12843/13440/6652/13668/12699,content_tuning=1279,enchant_id=7967,gem_id=240892\n"
	for _, want := range []string{
		"\nmain_hand=,id=271092,", // the equipped gear is the baseline
		"\nprofileset.combo1+=head=,id=249979,bonus_id=13338/13440/6652/13575/12806/13534,enchant_id=8017,gem_id=240967\n",
		"\nprofileset.combo2+=" + ring,
		"\nprofileset.combo4+=finger1=,id=251194,", // beside the equipped finger2, the bag ring takes finger1
	} {
		if !strings.Contains(string(input), want) {
			t.Errorf("the input file lacks %q:\n%s", want, input)
		}
	}
	if n := strings.Count(string(input), "profileset."); n != 7 {
		t.Errorf("%d profileset lines, want 7 (5 combos, 2 of them changing 2 slots)", n)
	}
	if got, _ := os.ReadFile(args); !strings.Contains(string(got), "target_error=0.2\n") {
		t.Errorf("profilesets should run at target_error=0.2: %q", got)
	}

	combos := sim.Result.Combos
	if sim.Result.DPS != 100000 || len(combos) != 5 {
		t.Fatalf("result = %+v", sim.Result)
	}
	best := combos[0]
	if best.DPS != 103000 || best.Gain != 3000 || best.DPSError != 64 || len(best.Swaps) != 2 ||
		best.Swaps[0].Slot != "finger2" || best.Swaps[0].Out.ID != 240949 || best.Swaps[0].In.ID != 251194 ||
		best.Swaps[1].Slot != "head" || best.Swaps[1].In.ID != 249979 {
		t.Errorf("best = %+v", best)
	}
	if last := combos[len(combos)-1]; last.DPS != 98000 || last.Gain != -2000 {
		t.Errorf("worst = %+v", last)
	}
}
