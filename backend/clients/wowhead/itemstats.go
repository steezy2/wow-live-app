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
	// effectBuff is a stat buff in an effect's text, "Increases your Haste by 1,234" (not "by 5%") or "grants 1,234
	// Haste", and effectNumber any number in it.
	effectBuff   = regexp.MustCompile(`(` + buffStats + `) by ([\d,]+)(?:[^\d,%]|$)|([\d,]+) (` + buffStats + `)`)
	effectNumber = regexp.MustCompile(`\d[\d,]*`)
)

// buffStats are the stats an effect's buff can name, and buffNames their names in RankedStats.Rating.
const buffStats = `Critical Strike|Haste|Mastery|Versatility|Intellect`

var buffNames = map[string]string{"Critical Strike": "Crit", "Haste": "Haste", "Mastery": "Mastery",
	"Versatility": "Vers"}

// ratingIDs are the game's rating IDs in a tooltip's `<!--rtg36-->`, by their names in RankedStats.Rating.
var ratingIDs = map[string]string{"32": "Crit", "36": "Haste", "49": "Mastery", "40": "Vers"}

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
// spells' names as slugs, the stat buff and the first number their text states). A stat another spec would get
// (Agility on a healer's mail) counts only when it can be intellect.
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
		if r := s.Stats.Rating(ratingIDs[m[1]]); r != nil {
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
	effect := strings.Join(strings.Fields(strings.Join(text, " ")), " ")
	s.Buff, s.Amount = buffOf(effect), number(effectNumber.FindString(effect))
	return s, nil
}

// buffOf is the stat buff an effect's text states, the stat and the amount either way round; none when it states none.
func buffOf(text string) models.RankedStats {
	var s models.RankedStats
	m := effectBuff.FindStringSubmatch(text)
	if m == nil {
		return s
	}
	stat, amount := m[1], m[2]
	if stat == "" {
		stat, amount = m[4], m[3]
	}
	if stat == "Intellect" {
		s.Main = number(amount)
	} else {
		*s.Rating(buffNames[stat]) = number(amount)
	}
	return s
}

// number reads a tooltip's "1,885".
func number(s string) int {
	n, _ := strconv.Atoi(strings.ReplaceAll(s, ",", ""))
	return n
}
