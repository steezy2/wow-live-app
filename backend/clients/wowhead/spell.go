package wowhead

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

// spellMaxAge is how long a downloaded spell is reused: spells change with patches, not by the day.
const spellMaxAge = 7 * 24 * time.Hour

var (
	cooldown = regexp.MustCompile(`<!--baseCooldown:([^>]*?)-->`)
	lineEnd  = regexp.MustCompile(`<br\s*/?>`)
	htmlTag  = regexp.MustCompile(`<[^>]*>`)
)

// Spell is what Wowhead's tooltip shows for a spell ID (fetched when the player hovers it and kept in CacheDir).
func (c Client) Spell(ctx context.Context, id int) (models.SpellInfo, error) {
	file := filepath.Join(c.CacheDir, fmt.Sprintf("spell-%d.json", id))
	url := fmt.Sprintf("%s/tooltip/spell/%d?dataEnv=1&locale=0", cmp.Or(c.BaseURL, "https://nether.wowhead.com"), id)
	if err := cachefile.Fresh(ctx, c.HTTP, url, file, spellMaxAge); err != nil {
		return models.SpellInfo{}, err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return models.SpellInfo{}, fmt.Errorf("reading %s: %w", file, err)
	}
	info, err := ParseSpell(data)
	info.ID = id
	return info, err
}

// ParseSpell reads a tooltip answer: the name, the cooldown the tooltip states, and the description (the block after
// the header, as plain text; empty for a spell that has none).
func ParseSpell(data []byte) (models.SpellInfo, error) {
	var tooltip struct {
		Name    string `json:"name"`
		Tooltip string `json:"tooltip"`
	}
	if err := json.Unmarshal(data, &tooltip); err != nil || tooltip.Name == "" {
		return models.SpellInfo{}, fmt.Errorf("no spell in the tooltip answer (%v)", err)
	}
	info := models.SpellInfo{Name: tooltip.Name}
	if m := cooldown.FindStringSubmatch(tooltip.Tooltip); m != nil {
		info.Cooldown = strings.TrimSpace(m[1])
	}
	if blocks := strings.Split(tooltip.Tooltip, "</table><table>"); len(blocks) > 1 { // the first is the header
		text := lineEnd.ReplaceAllString(blocks[len(blocks)-1], "\n")
		info.Description = strings.TrimSpace(html.UnescapeString(htmlTag.ReplaceAllString(text, "")))
	}
	return info, nil
}
