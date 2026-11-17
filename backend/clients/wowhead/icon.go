package wowhead

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

// iconMaxAge is how long a downloaded icon is reused: they change with the game's art, rarely.
const iconMaxAge = 30 * 24 * time.Hour

// iconName is what a Wowhead icon's name looks like; one that isn't is never put in a URL or a file name.
var iconName = regexp.MustCompile(`^[a-z0-9_]{1,80}$`)

// Icon is the image of a spell icon by its Wowhead name ("spell_nature_starfall"), downloaded once and kept in
// CacheDir. The page can't load images from Wowhead itself, so the app serves them.
func (c Client) Icon(ctx context.Context, name string) ([]byte, error) {
	if !iconName.MatchString(name) {
		return nil, fmt.Errorf("%q: %w", name, models.ErrNoSuchIcon)
	}
	file := filepath.Join(c.CacheDir, "icons", name+".jpg")
	url := fmt.Sprintf("%s/images/wow/icons/medium/%s.jpg", cmp.Or(c.IconURL, "https://wow.zamimg.com"), name)
	if err := cachefile.Fresh(ctx, c.HTTP, url, file, iconMaxAge); errors.Is(err, cachefile.ErrNotFound) {
		return nil, fmt.Errorf("%q: %w", name, models.ErrNoSuchIcon)
	} else if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}
	return data, nil
}
