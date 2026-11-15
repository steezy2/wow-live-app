package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"wow-live/backend/clients/wago"
	"wow-live/backend/models"
)

// journal is the game tables simref reads, cut to the columns it uses.
type journal struct {
	instances  map[int]instance
	encounters map[int]encounter
	loot       []loot
	items      map[int]item  // the looted items only
	overrides  map[int][]int // ItemSpecOverride: the specs an item is for, when the game names them
	itemSpecs  []itemSpec
	specs      map[int]spec // the playable specs
}

type instance struct {
	name  string
	mapID int
}

type encounter struct {
	name            string
	instance, order int
}

// loot is a JournalEncounterItem row: an item a boss drops, tagged with a season when the journal shows it in one.
type loot struct{ encounter, item, season int }

// item is a looted item's Item and ItemSparse rows; name is empty when ItemSparse has no row for it.
type item struct {
	id, class, subclass, inventory, allowableClasses int
	name                                             string
	stats                                            []int // ItemSparse's StatModifier_bonusStat_*
}

// itemSpec is an ItemSpec row: a spec that loots items of a kind with these stats.
type itemSpec struct{ kind, primary, secondary, spec int }

type spec struct {
	class int
	models.SimSpec
}

// readJournal reads the tables, keeping only the looted items' rows of the item tables.
func readJournal(ctx context.Context, tables wago.Client) (journal, error) {
	j := journal{instances: map[int]instance{}, encounters: map[int]encounter{}, items: map[int]item{},
		overrides: map[int][]int{}, specs: map[int]spec{}}
	classes := map[int]string{}
	statColumns := []string{"Display_lang", "ID", "AllowableClass"}
	for i := range 10 {
		statColumns = append(statColumns, fmt.Sprintf("StatModifier_bonusStat_%d", i))
	}
	steps := []struct {
		table   string
		columns []string // the text columns first, then the numbers
		text    int
		each    func(text []string, n []int)
	}{
		{"JournalInstance", []string{"Name_lang", "ID", "MapID"}, 1, func(t []string, n []int) {
			j.instances[n[0]] = instance{name: t[0], mapID: n[1]}
		}},
		{"JournalEncounter", []string{"Name_lang", "ID", "JournalInstanceID", "OrderIndex"}, 1, func(t []string, n []int) {
			j.encounters[n[0]] = encounter{name: t[0], instance: n[1], order: n[2]}
		}},
		{"JournalEncounterItem", []string{"JournalEncounterID", "ItemID", "DisplaySeasonID"}, 0, func(_ []string, n []int) {
			j.loot = append(j.loot, loot{encounter: n[0], item: n[1], season: n[2]})
			j.items[n[1]] = item{id: n[1]}
		}},
		{"Item", []string{"ID", "ClassID", "SubclassID", "InventoryType"}, 0, func(_ []string, n []int) {
			if _, looted := j.items[n[0]]; looted {
				j.items[n[0]] = item{id: n[0], class: n[1], subclass: n[2], inventory: n[3]}
			}
		}},
		{"ItemSparse", statColumns, 1, func(t []string, n []int) {
			if it, looted := j.items[n[0]]; looted {
				it.name, it.allowableClasses, it.stats = t[0], n[1], append([]int{}, n[2:]...)
				j.items[n[0]] = it
			}
		}},
		{"ItemSpecOverride", []string{"ItemID", "SpecID"}, 0, func(_ []string, n []int) {
			if _, looted := j.items[n[0]]; looted {
				j.overrides[n[0]] = append(j.overrides[n[0]], n[1])
			}
		}},
		{"ItemSpec", []string{"ItemType", "PrimaryStat", "SecondaryStat", "SpecializationID"}, 0, func(_ []string, n []int) {
			j.itemSpecs = append(j.itemSpecs, itemSpec{kind: n[0], primary: n[1], secondary: n[2], spec: n[3]})
		}},
		{"ChrClasses", []string{"Name_lang", "ID"}, 1, func(t []string, n []int) { classes[n[0]] = simcName(t[0]) }},
		// A playable spec is one ItemSpec gives loot to, but OrderIndex 4, a class's "Initial" spec before one is
		// picked; the rest are pets' and test classes'.
		{"ChrSpecialization", []string{"Name_lang", "ID", "ClassID", "OrderIndex"}, 1, func(t []string, n []int) {
			looter := slices.ContainsFunc(j.itemSpecs, func(s itemSpec) bool { return s.spec == n[0] })
			if class, ok := classes[n[1]]; ok && looter && n[2] < 4 {
				j.specs[n[0]] = spec{class: n[1], SimSpec: models.SimSpec{Class: class, Spec: simcName(t[0])}}
			}
		}},
	}
	for _, s := range steps {
		err := tables.Rows(ctx, s.table, s.columns, func(row []string) error {
			n := make([]int, len(row)-s.text)
			for i, v := range row[s.text:] {
				var err error
				if n[i], err = strconv.Atoi(v); err != nil {
					return fmt.Errorf("%s %q: %w", s.columns[s.text+i], v, err)
				}
			}
			s.each(row[:s.text], n)
			return nil
		})
		if err != nil {
			return j, err
		}
	}
	return j, nil
}

// simcName is a class or spec as simc writes it: "Death Knight" is "death_knight".
func simcName(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "_")
}
