package wowhead

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"wow-live/backend/models"
)

// itemMaxAge is how long a downloaded item is reused: an item's name, icon, quality and weapon slot never change.
const itemMaxAge = 30 * 24 * time.Hour

// weaponSlot is a weapon tooltip's slot and type: `<td>Two-Hand</td><th><!--scstart2:10--><span class="q1">Staff`.
var weaponSlot = regexp.MustCompile(`<td>([^<]+)</td><th><!--scstart\d+:\d+--><span class="q1">([^<]+)<`)

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

// TwoHanded says whether the weapon with this item ID is two-handed, from Wowhead's tooltip (kept in CacheDir).
func (c Client) TwoHanded(ctx context.Context, id int) (bool, error) {
	data, err := c.tooltip(ctx, "item", id, itemMaxAge)
	if err != nil {
		return false, err
	}
	return ParseTwoHanded(data)
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

// ParseTwoHanded reads an item tooltip answer: a "Two-Hand" weapon, or a "Ranged" one other than a wand (bows, guns
// and crossbows take both hands), is two-handed. A tooltip with no weapon slot is an error.
func ParseTwoHanded(data []byte) (bool, error) {
	var tooltip struct {
		Tooltip string `json:"tooltip"`
	}
	if err := json.Unmarshal(data, &tooltip); err != nil {
		return false, fmt.Errorf("reading the item tooltip: %w", err)
	}
	m := weaponSlot.FindStringSubmatch(tooltip.Tooltip)
	if m == nil {
		return false, errors.New("the item tooltip names no weapon slot")
	}
	return m[1] == "Two-Hand" || m[1] == "Ranged" && m[2] != "Wand", nil
}
