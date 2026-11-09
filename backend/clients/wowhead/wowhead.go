// Package wowhead reads Wowhead's Mythic+ dungeon overviews for the facts they tag: the important interrupts,
// dispels and purges table and the boss tips. Only spell IDs, mob names and tags are kept, never the guide's text
// (CLAUDE.md: store facts and links). Pages are cached in CacheDir.
package wowhead

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wow-live/backend/models"
)

// maxAge is how long a downloaded guide is reused.
const maxAge = 24 * time.Hour

// guidePaths are the overview URLs, by dungeon slug: new dungeons' and returning ones'.
var guidePaths = []string{"/guide/midnight/%s-dungeon-overview-location-rewards", "/guide/midnight/%s-dungeon-overview-mythic-plus"}

// Client downloads Wowhead pages. BaseURL defaults to Wowhead's; HTTP may be nil.
type Client struct {
	HTTP     *http.Client
	CacheDir string
	BaseURL  string
}

// Guide is the dungeon overview for a Raider.IO dungeon slug ("den-of-nalorakk").
func (c Client) Guide(ctx context.Context, slug string) (models.DungeonGuide, error) {
	var errs []string
	for i, path := range guidePaths {
		page, err := c.page(ctx, fmt.Sprintf(path, slug), fmt.Sprintf("%s-%d.html", slug, i))
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		guide := ParseGuide(page)
		guide.URL = c.base() + fmt.Sprintf(path, slug)
		return guide, nil
	}
	return models.DungeonGuide{}, fmt.Errorf("no Wowhead overview for %s: %s", slug, strings.Join(errs, "; "))
}

var (
	table = regexp.MustCompile(`(?s)\[b\]Important Interrupts\[/b\].*?\[/tr\]\s*\[tr\](.*?)\[/tr\]`)
	cell  = regexp.MustCompile(`(?s)\[td[^\]]*\](.*?)\[/td\]`)
	entry = regexp.MustCompile(`(?s)\[spell=(\d+)\].*?\[i\]([^\[(]+)`)
	tip   = regexp.MustCompile(`\[spell=(\d+)\] - \[b\]([^\[]+)\[/b\]`)
)

// ParseGuide reads a guide page's markup: the row after "Important Interrupts" (one cell each for interrupts, dispels
// and purges: spell ID and the mob casting it) and every "[spell=ID] - [b]Tag[/b]" tip.
func ParseGuide(page string) models.DungeonGuide {
	page = strings.NewReplacer(`\/`, "/", `\r\n`, "\n", `\"`, `"`).Replace(page)
	g := models.DungeonGuide{Interrupts: map[int]string{}, Dispels: map[int]string{}, Purges: map[int]string{},
		Tips: map[int]string{}}
	if row := table.FindStringSubmatch(page); row != nil {
		for n, c := range cell.FindAllStringSubmatch(row[1], 3) {
			for _, e := range entry.FindAllStringSubmatch(c[1], -1) {
				id, _ := strconv.Atoi(e[1])
				[]map[int]string{g.Interrupts, g.Dispels, g.Purges}[n][id] = strings.TrimSpace(e[2])
			}
		}
	}
	for _, t := range tip.FindAllStringSubmatch(page, -1) {
		id, _ := strconv.Atoi(t[1])
		g.Tips[id] = strings.TrimSpace(t[2])
	}
	return g
}
