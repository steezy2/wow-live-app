package service

import (
	"testing"

	"wow-live/backend/models"
)

func TestUpsertReplacesTheSameFightAndKeepsItsLink(t *testing.T) {
	link := &models.ReportLink{Code: "abc", FightID: 3}
	list := []models.Key{{Start: 2}, {Start: 1, Report: link}, {Start: 1, Imported: true}}
	upsert(&list, models.Key{Start: 1, Level: 10})
	upsert(&list, models.Key{Start: 3})
	if len(list) != 4 || list[0].Start != 3 {
		t.Fatalf("list = %+v, want the new key first and four in all", list)
	}
	if got := list[2]; got.Level != 10 || got.Report != link {
		t.Errorf("replaced key = %+v, want the new copy with its link", got)
	}
	if list[3].Report != nil || !list[3].Imported {
		t.Errorf("the imported twin = %+v, want it untouched", list[3])
	}
}
