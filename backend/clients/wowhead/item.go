package wowhead

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"wow-live/backend/clients/cachefile"
)

// itemMaxAge is how long a downloaded item tooltip is reused: a weapon's slot never changes.
const itemMaxAge = 365 * 24 * time.Hour

// weaponSlot is a weapon tooltip's slot and type: `<td>Two-Hand</td><th><!--scstart2:10--><span class="q1">Staff`.
var weaponSlot = regexp.MustCompile(`<td>([^<]+)</td><th><!--scstart\d+:\d+--><span class="q1">([^<]+)<`)

// TwoHanded says whether the weapon with this item ID is two-handed, from Wowhead's tooltip (kept in CacheDir).
func (c Client) TwoHanded(ctx context.Context, id int) (bool, error) {
	file := filepath.Join(c.CacheDir, fmt.Sprintf("item-%d.json", id))
	url := fmt.Sprintf("%s/tooltip/item/%d?dataEnv=1&locale=0", cmp.Or(c.BaseURL, "https://nether.wowhead.com"), id)
	data, err := cachefile.Fresh(ctx, c.HTTP, url, file, itemMaxAge)
	if err != nil {
		return false, err
	}
	return ParseTwoHanded(data)
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
