package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

const (
	// maxItemLookups bounds one Items.Infos call: a character's worn and carried gear, or a Droptimizer's rows, take
	// a few of them (the page asks in chunks of SimChoices.MaxItemLookups).
	maxItemLookups = 120
	// itemLookupsAtOnce is how many of them go to the source side by side.
	itemLookupsAtOnce = 6
)

// ItemSource gives an item's name, icon and quality by its ID (clients/wowhead).
type ItemSource interface {
	Item(ctx context.Context, id int) (models.SimItemInfo, error)
}

// Items names and draws the items of a pasted character or a sim's result.
type Items struct {
	Source ItemSource
}

// Infos is what the source knows of each of ids, in their order; an item it can't look up is left out, so the page
// draws it by its export name. More than maxItemLookups IDs is ErrBadSimOptions.
func (it Items) Infos(ctx context.Context, ids []int) ([]models.SimItemInfo, error) {
	if len(ids) > maxItemLookups {
		return nil, fmt.Errorf("%w: ask for at most %d items", ErrBadSimOptions, maxItemLookups)
	}
	found := make([]*models.SimItemInfo, len(ids))
	var wg sync.WaitGroup
	var logged sync.Once
	sem := make(chan struct{}, itemLookupsAtOnce)
	for i, id := range ids {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			info, err := it.Source.Item(ctx, id)
			if err == nil {
				found[i] = &info
			} else if !errors.Is(err, cachefile.ErrNotFound) && ctx.Err() == nil {
				logged.Do(func() { log.Printf("sim items: item %d: %v", id, err) })
			}
		})
	}
	wg.Wait()
	items := []models.SimItemInfo{}
	for _, info := range found {
		if info != nil {
			items = append(items, *info)
		}
	}
	return items, nil
}
