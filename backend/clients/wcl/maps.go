package wcl

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"

	"wow-live/backend/clients/cachefile"
)

// MapImages are the maps Warcraft Logs' replay draws (the game's map art, one image per uiMap, its edges the map's
// UiMapAssignment bounds), kept in CacheDir once downloaded. They're public files, so no client keys are needed.
type MapImages struct {
	CacheDir string
	Base     string // https://assets.rpglogs.com when empty
}

// Image is a uiMap's image as PNG: downloaded the first time, then from the cache.
func (m MapImages) Image(ctx context.Context, uiMapID int) ([]byte, error) {
	path := filepath.Join(m.CacheDir, fmt.Sprintf("%d.png", uiMapID))
	url := fmt.Sprintf("%s/img/warcraft/maps/%d-map.png", cmp.Or(m.Base, "https://assets.rpglogs.com"), uiMapID)
	data, err := cachefile.Fresh(ctx, nil, url, path, cachefile.Forever)
	if err != nil {
		return nil, fmt.Errorf("Warcraft Logs map: %w", err)
	}
	return data, nil
}
