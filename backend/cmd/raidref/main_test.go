package main

import (
	"reflect"
	"testing"

	"wow-live/backend/models"
)

func TestCurateReplacesTheCheatSheet(t *testing.T) {
	generated := models.RaidBoss{Name: "Ula'tek", NPCs: []int{1}, Avoidable: []int{9}, Soak: []int{8},
		Stacks: map[int]int{7: 0}, Priority: []string{"Ula'tek's Heart"}}
	got, err := curate(generated, models.CuratedBoss{Checked: "log", Avoidable: []int{2}, Stacks: map[string]int{"3": 8},
		Soaks: []models.CuratedSoak{{Spell: 4, Soakers: 5}, {Spell: 6}}})
	if err != nil {
		t.Fatal(err)
	}
	want := models.RaidBoss{Name: "Ula'tek", NPCs: []int{1}, Checked: "log", Avoidable: []int{2}, Soak: []int{4, 6},
		Soakers: map[int]int{4: 5}, Dispel: []int{}, MustStop: []int{}, Stacks: map[int]int{3: 8}, Priority: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("curate = %+v; want %+v", got, want)
	}
	if _, err := curate(generated, models.CuratedBoss{Stacks: map[string]int{"x": 1}}); err == nil {
		t.Fatal("a stacks key that isn't a spell ID passed")
	}
}
