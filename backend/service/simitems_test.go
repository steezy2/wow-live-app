package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

// itemSource knows the items whose ID is under 100.
type itemSource struct{}

func (itemSource) Item(_ context.Context, id int) (models.SimItemInfo, error) {
	if id >= 100 {
		return models.SimItemInfo{}, fmt.Errorf("item %d: %w", id, cachefile.ErrNotFound)
	}
	return models.SimItemInfo{ID: id, Name: "Item"}, nil
}

type spellSource struct{}

func (spellSource) Spell(_ context.Context, id int) (models.SpellInfo, error) {
	if id >= 100 {
		return models.SpellInfo{}, fmt.Errorf("spell %d: %w", id, cachefile.ErrNotFound)
	}
	return models.SpellInfo{ID: id, Name: "Spell", Icon: "spell_x"}, nil
}

func TestSpellInfosLeaveOutWhatTheSourceDoesNotKnow(t *testing.T) {
	items := Items{Spells: spellSource{}}
	got, err := items.SpellInfos(context.Background(), []int{1, 500, 2})
	if err != nil || len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Errorf("infos = %+v, %v; want spells 1 and 2", got, err)
	}
	if _, err := items.SpellInfos(context.Background(), make([]int, maxItemLookups+1)); !errors.Is(err, ErrBadSimOptions) {
		t.Errorf("too many IDs: err = %v, want ErrBadSimOptions", err)
	}
}

func TestItemsInfosLeaveOutWhatTheSourceDoesNotKnow(t *testing.T) {
	items := Items{Source: itemSource{}}
	got, err := items.Infos(context.Background(), []int{1, 500, 2})
	if err != nil || len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Errorf("infos = %+v, %v; want items 1 and 2", got, err)
	}
	if _, err := items.Infos(context.Background(), make([]int, maxItemLookups+1)); !errors.Is(err, ErrBadSimOptions) {
		t.Errorf("too many IDs: err = %v, want ErrBadSimOptions", err)
	}
}
