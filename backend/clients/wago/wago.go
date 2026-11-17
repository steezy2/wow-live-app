// Package wago reads the game's data tables from wago.tools.
package wago

import (
	"bytes"
	"cmp"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
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
	// Branch is the game branch whose newest build is read ("wow" is the live game); wago.tools' newest build of any
	// branch, often a test realm's, when empty.
	Branch string
}

// Rows calls each with every row of the game table, holding the columns asked for in their order. The row slice is
// reused between calls. A column the table doesn't have is an error.
func (c Client) Rows(ctx context.Context, table string, columns []string, each func(row []string) error) error {
	file, url := table+".csv", cmp.Or(c.Base, "https://wago.tools")+"/db2/"+table+"/csv"
	if c.Branch != "" {
		file, url = table+"."+c.Branch+".csv", url+"?branch="+c.Branch
	}
	path := filepath.Join(c.CacheDir, file)
	data, err := cachefile.Fresh(ctx, nil, url, path, 7*24*time.Hour)
	if err != nil {
		return fmt.Errorf("wago.tools %s: %w", table, err)
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.ReuseRecord = true
	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	at := make([]int, len(columns))
	for i, name := range columns {
		if at[i] = slices.Index(header, name); at[i] < 0 {
			return fmt.Errorf("reading %s: no %s column", path, name)
		}
	}
	row := make([]string, len(columns))
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		for i, j := range at {
			row[i] = record[j]
		}
		if err := each(row); err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
	}
}

// MapBounds is each of the uiMaps' edges, from the game's UiMapAssignment table (its first row per map). A map the
// table doesn't have is left out.
func (c Client) MapBounds(ctx context.Context, uiMapIDs []int) (map[int]models.RaidMap, error) {
	// Region_0 to Region_2 is the corner with the smallest x, y and z; Region_3 to Region_5 the largest.
	columns := []string{"UiMapID", "Region_0", "Region_1", "Region_3", "Region_4"}
	bounds := map[int]models.RaidMap{}
	err := c.Rows(ctx, "UiMapAssignment", columns, func(row []string) error {
		id, err := strconv.Atoi(row[0])
		if _, seen := bounds[id]; err != nil || seen || !slices.Contains(uiMapIDs, id) {
			return nil
		}
		var edge [4]float64
		for i, s := range row[1:] {
			if edge[i], err = strconv.ParseFloat(s, 64); err != nil {
				return fmt.Errorf("map %d's %s: %w", id, columns[i+1], err)
			}
		}
		bounds[id] = models.RaidMap{UIMapID: id, MinX: edge[0], MinY: edge[1], MaxX: edge[2], MaxY: edge[3]}
		return nil
	})
	return bounds, err
}
