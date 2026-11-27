package wowhead

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"wow-live/backend/models"
)

var (
	// mainStat is a tooltip's main stat line, `<!--stat5-->+106 Intellect`, and ratingStat a secondary one,
	// `<!--rtg36-->50 Haste`.
	mainStat   = regexp.MustCompile(`<!--stat(\d+)-->\+([\d,]+) `)
	ratingStat = regexp.MustCompile(`<!--rtg(\d+)-->([\d,]+) `)
	// effectSpell is a Use: or Equip: effect's spell link with its name as a slug, `href="/spell=1292291/gebbos-bag"`,
	// and effectText the whole of those paragraphs.
	effectSpell = regexp.MustCompile(`href="/spell=(\d+)(?:/([a-z0-9-]+))?`)
	effectText  = regexp.MustCompile(`(?s)<!--useText:0:\d+-->(.*?)<!--useText:1-->`)
)

// intellectStats are the game's stat IDs that give a healer intellect: Intellect and the primary stats that follow the
// spec (any of the three, Agility or Intellect, Strength or Intellect).
var intellectStats = map[int]bool{5: true, 71: true, 73: true, 74: true}

// ItemStats is what Wowhead's tooltip says the item gives at level (0: as its bonus IDs make it) with bonus IDs
// (kept in CacheDir like Item).
func (c Client) ItemStats(ctx context.Context, id, level int, bonus []int) (models.ItemStats, error) {
	query := ""
	if level > 0 { // else the bonus IDs set it
		query = fmt.Sprintf("&ilvl=%d", level)
	}
	if len(bonus) > 0 {
		ids := make([]string, len(bonus))
		for i, b := range bonus {
			ids[i] = strconv.Itoa(b)
		}
		query += "&bonus=" + strings.Join(ids, ":")
	}
	data, err := c.tooltipAt(ctx, "item", id, query, itemMaxAge)
	if err != nil {
		return models.ItemStats{}, err
	}
	return ParseItemStats(data)
}

// ParseItemStats reads a tooltip answer's intellect, secondary ratings and Use:/Equip: effects (their spell IDs, the
// spells' names as slugs and the plain text). A stat another spec would get (Agility on a healer's mail) counts only
// when it can be intellect.
func ParseItemStats(data []byte) (models.ItemStats, error) {
	var tooltip struct {
		Name    string `json:"name"`
		Tooltip string `json:"tooltip"`
	}
	if err := json.Unmarshal(data, &tooltip); err != nil || tooltip.Name == "" {
		return models.ItemStats{}, fmt.Errorf("no item in the tooltip answer (%v)", err)
	}
	var s models.ItemStats
	for _, m := range mainStat.FindAllStringSubmatch(tooltip.Tooltip, -1) {
		if id, _ := strconv.Atoi(m[1]); intellectStats[id] {
			s.Stats.Main += number(m[2])
		}
	}
	for _, m := range ratingStat.FindAllStringSubmatch(tooltip.Tooltip, -1) {
		if r := s.Stats.Rating(m[1]); r != nil {
			*r += number(m[2])
		}
	}
	var text []string
	for _, m := range effectText.FindAllStringSubmatch(tooltip.Tooltip, -1) {
		for _, sp := range effectSpell.FindAllStringSubmatch(m[1], -1) {
			id, _ := strconv.Atoi(sp[1])
			s.Spells = append(s.Spells, id)
			if sp[2] != "" {
				s.Slugs = append(s.Slugs, sp[2])
			}
		}
		text = append(text, html.UnescapeString(tag.ReplaceAllString(m[1], "")))
	}
	s.Effect = strings.Join(strings.Fields(strings.Join(text, " ")), " ")
	return s, nil
}

// number reads a tooltip's "1,885".
func number(s string) int {
	n, _ := strconv.Atoi(strings.ReplaceAll(s, ",", ""))
	return n
}
