// Package wago reads the game's data tables from wago.tools.
package wago

import (
	"cmp"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

// Client reads wago.tools' tables, keeping each in CacheDir for a week.
type Client struct {
	CacheDir string
	Base     string // https://wago.tools when empty
}

// MapBounds is each of the uiMaps' edges, from the game's UiMapAssignment table (its first row per map). A map the
// table doesn't have is left out.
func (c Client) MapBounds(ctx context.Context, uiMapIDs []int) (map[int]models.RaidMap, error) {
	path := filepath.Join(c.CacheDir, "UiMapAssignment.csv")
	url := cmp.Or(c.Base, "https://wago.tools") + "/db2/UiMapAssignment/csv"
	if err := cachefile.Fresh(ctx, nil, url, path, 7*24*time.Hour); err != nil {
		return nil, fmt.Errorf("wago.tools UiMapAssignment: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil || len(rows) == 0 {
		return nil, fmt.Errorf("reading %s: %w", path, cmp.Or(err, fmt.Errorf("no rows")))
	}
	col := map[string]int{}
	for i, name := range rows[0] {
		col[name] = i
	}
	for _, name := range []string{"UiMapID", "Region_0", "Region_1", "Region_3", "Region_4"} {
		if _, ok := col[name]; !ok {
			return nil, fmt.Errorf("reading %s: no %s column", path, name)
		}
	}
	bounds := map[int]models.RaidMap{}
	for _, row := range rows[1:] {
		num := func(name string) float64 {
			v, _ := strconv.ParseFloat(row[col[name]], 64)
			return v
		}
		id := int(num("UiMapID"))
		if _, seen := bounds[id]; seen || !slices.Contains(uiMapIDs, id) {
			continue
		}
		// Region_0 to Region_2 is the corner with the smallest x, y and z; Region_3 to Region_5 the largest.
		bounds[id] = models.RaidMap{UIMapID: id, MinX: num("Region_0"), MinY: num("Region_1"), MaxX: num("Region_3"),
			MaxY: num("Region_4")}
	}
	return bounds, nil
}
