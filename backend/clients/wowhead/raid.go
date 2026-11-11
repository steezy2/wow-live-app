package wowhead

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

// raidPath is a raid's overview, which links each boss's cheat sheet, by the raid's slug ("venomous-abyss").
const raidPath = "/guide/midnight/raids/the-%s-overview-location-rewards-bosses"

var (
	title     = regexp.MustCompile(`<title>(.*?) Raid Boss Guide`)
	cheat     = regexp.MustCompile(`(?s)\[tab name="Boss (?:Cheat Sheet|Strategy)"\](.*?)\[/tab\]`)
	item      = regexp.MustCompile(`(?s)\[li[^\]]*\](.*?)(?:\[/li\]|\[ul\])`)
	spellTag  = regexp.MustCompile(`\[spell=(\d+)\]`)
	markup    = regexp.MustCompile(`\[[^\]]*\]`)
	headSplit = " - "
)

// RaidGuides is each boss cheat sheet the raid's overview links, in its order.
func (c Client) RaidGuides(ctx context.Context, raid string) ([]models.RaidGuide, error) {
	overview, err := c.page(ctx, fmt.Sprintf(raidPath, raid), raid+".html")
	if err != nil {
		return nil, err
	}
	bossSlug := regexp.MustCompile(regexp.QuoteMeta(raid) + `-[a-z0-9-]+-boss-strategy-abilities`)
	var guides []models.RaidGuide
	var paths []string
	for _, slug := range bossSlug.FindAllString(overview, -1) { // links are JSON-escaped in the page's data
		if path := "/guide/midnight/raids/" + slug; !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	for _, path := range paths {
		page, err := c.page(ctx, path, filepath.Base(path)+".html")
		if err != nil {
			return nil, err
		}
		guide := ParseRaidGuide(page)
		guide.URL = c.base() + path
		guides = append(guides, guide)
	}
	if len(guides) == 0 {
		return nil, fmt.Errorf("no boss cheat sheets linked from the %s overview", raid)
	}
	return guides, nil
}

// ParseRaidGuide reads a boss cheat sheet: the boss's name from the page title and, from the cheat sheet tab, each
// list item headed by spells before " - ": those spells and its words without markup.
func ParseRaidGuide(page string) models.RaidGuide {
	page = strings.NewReplacer(`\/`, "/", `\r\n`, "\n", `\"`, `"`).Replace(page)
	var g models.RaidGuide
	if m := title.FindStringSubmatch(page); m != nil {
		g.Name = html.UnescapeString(m[1])
	}
	sheet := cheat.FindStringSubmatch(page)
	if sheet == nil {
		return g
	}
	for _, m := range item.FindAllStringSubmatch(sheet[1], -1) {
		text := m[1]
		head, _, ok := strings.Cut(text, headSplit)
		spells := spellTag.FindAllStringSubmatch(head, -1)
		if !ok || len(spells) == 0 {
			continue // a note naming a spell in passing ("split soaks to deal with [debuff]") isn't its mechanic
		}
		line := models.RaidGuideLine{Text: strings.TrimSpace(markup.ReplaceAllString(text, ""))}
		for _, s := range spells {
			id, _ := strconv.Atoi(s[1])
			line.Spells = append(line.Spells, id)
		}
		g.Lines = append(g.Lines, line)
	}
	return g
}

// itemAdd starts an item's data on an item-set page: g_items.add(271519, {...}).
var itemAdd = regexp.MustCompile(`g_items\.add\((\d+), `)

// ItemSet is a Wowhead item set (a class's tier set): its items and the specs that wear them.
func (c Client) ItemSet(ctx context.Context, id int) (models.ItemSet, error) {
	page, err := c.page(ctx, fmt.Sprintf("/item-set=%d", id), fmt.Sprintf("item-set-%d.html", id))
	if err != nil {
		return models.ItemSet{}, err
	}
	set, err := ParseItemSet(page)
	if err != nil {
		return set, fmt.Errorf("item set %d: %w", id, err)
	}
	return set, nil
}

// ParseItemSet reads an item-set page's items (each g_items.add call) and their specs.
func ParseItemSet(page string) (models.ItemSet, error) {
	var set models.ItemSet
	for _, m := range itemAdd.FindAllStringSubmatchIndex(page, -1) {
		var item struct {
			Equip struct {
				Specs []int `json:"specs"`
			} `json:"jsonequip"`
		}
		if err := json.NewDecoder(strings.NewReader(page[m[1]:])).Decode(&item); err != nil {
			return set, fmt.Errorf("reading item %s: %w", page[m[2]:m[3]], err)
		}
		id, _ := strconv.Atoi(page[m[2]:m[3]])
		set.Items = append(set.Items, id)
		for _, s := range item.Equip.Specs {
			if !slices.Contains(set.Specs, s) {
				set.Specs = append(set.Specs, s)
			}
		}
	}
	if len(set.Items) == 0 {
		return set, errors.New("no items on the page")
	}
	slices.Sort(set.Items)
	return set, nil
}

// page downloads a Wowhead page into the cache (reused for a day) and returns it.
func (c Client) page(ctx context.Context, path, file string) (string, error) {
	file = filepath.Join(c.CacheDir, file)
	if err := cachefile.Fresh(ctx, c.HTTP, c.base()+path, file, maxAge); err != nil {
		return "", err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", file, err)
	}
	return string(data), nil
}

func (c Client) base() string {
	return cmp.Or(c.BaseURL, "https://www.wowhead.com")
}
