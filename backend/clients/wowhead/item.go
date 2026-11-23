package wowhead

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"wow-live/backend/models"
)

// itemMaxAge is how long a downloaded item is reused: an item's name, icon and quality never change.
const itemMaxAge = 30 * 24 * time.Hour

// Item is what Wowhead's tooltip knows of an item ID (fetched when the Sim tab draws it and kept in CacheDir).
func (c Client) Item(ctx context.Context, id int) (models.SimItemInfo, error) {
	data, err := c.tooltip(ctx, "item", id, itemMaxAge)
	if err != nil {
		return models.SimItemInfo{}, err
	}
	info, err := ParseItem(data)
	info.ID = id
	return info, err
}

// ParseItem reads a tooltip answer's name, icon and quality.
func ParseItem(data []byte) (models.SimItemInfo, error) {
	var tooltip struct {
		Name    string `json:"name"`
		Icon    string `json:"icon"`
		Quality int    `json:"quality"`
	}
	if err := json.Unmarshal(data, &tooltip); err != nil {
		return models.SimItemInfo{}, fmt.Errorf("reading the item tooltip: %w", err)
	}
	if tooltip.Name == "" {
		return models.SimItemInfo{}, errors.New("no item in the tooltip answer")
	}
	return models.SimItemInfo{Name: tooltip.Name, Icon: tooltip.Icon, Quality: tooltip.Quality}, nil
}
