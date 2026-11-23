package service

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"wow-live/backend/models"
)

const (
	// maxProfileBytes is the longest export ParseSimC reads; a real one is about 10 KB.
	maxProfileBytes = 64 << 10
	// maxBagItems is the most carried items ParseSimC keeps; a bigger export is refused.
	maxBagItems = 200
	// maxItemValue bounds every number in an item, far above any real ID.
	maxItemValue = 1 << 31
)

// ErrBadSimC is an export ParseSimC can't use; its text says why.
var ErrBadSimC = errors.New("not a SimulationCraft export")

var (
	simClasses = []string{"deathknight", "demonhunter", "druid", "evoker", "hunter", "mage", "monk", "paladin",
		"priest", "rogue", "shaman", "warlock", "warrior"}
	// simSlots are the gear slots simc knows.
	simSlots = []string{"head", "neck", "shoulder", "back", "chest", "shirt", "tabard", "wrist", "hands", "waist",
		"legs", "feet", "finger1", "finger2", "trinket1", "trinket2", "main_hand", "off_hand"}
	// simItemKeys are the numbers of an item that ParseSimC keeps; any other key (an input= or output= too) is dropped.
	simItemKeys = []string{"bonus_id", "gem_id", "enchant_id", "content_tuning", "crafted_stats", "crafting_quality",
		"redirected_base_stats", "ilevel"}
	// simWord is what a header field may hold: names, realms, races, specs and talents are only these characters.
	simWord        = regexp.MustCompile(`^[a-z0-9_'-]{1,40}$`)
	simName        = regexp.MustCompile(`^[\p{L}]{2,12}$`)
	simTalents     = regexp.MustCompile(`^[A-Za-z0-9+/_=-]{1,300}$`)
	simPairs       = regexp.MustCompile(`^[a-z_]+=\d+(/[a-z_]+=\d+)*$`)
	simOmnium      = regexp.MustCompile(`^\d+:\d+(/\d+:\d+)*$`)
	simItemComment = regexp.MustCompile(`^# (.{1,100}?) \((\d+)\)$`)
	simClassLine   = regexp.MustCompile(`^(\w+)="([^"]+)"$`)
	bagsHeading    = "### Gear from Bags"
)

// ParseSimC reads the SimulationCraft addon's export. The text is untrusted, and simc reads and writes any file a
// line such as input= or output= names, so nothing is kept but fields that matched a strict pattern; use SimInput,
// never the pasted text, to feed simc.
func ParseSimC(text string) (models.SimProfile, error) {
	if len(text) > maxProfileBytes {
		return models.SimProfile{}, fmt.Errorf("%w: over %d KB", ErrBadSimC, maxProfileBytes>>10)
	}
	var p models.SimProfile
	var comment string // the last "# Name (311)" line, which names the item line after it
	inBags := false
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "###"):
			inBags = line == bagsHeading
		case line == "":
		case strings.HasPrefix(line, "#"):
			if m := simItemComment.FindStringSubmatch(line); m != nil {
				comment = line
			} else if inBags {
				if item, ok := parseSimItem(strings.TrimSpace(strings.TrimPrefix(line, "#")), comment); ok {
					p.Bags = append(p.Bags, item)
					comment = ""
				}
			}
		default:
			if item, ok := parseSimItem(line, comment); ok {
				p.Equipped = append(p.Equipped, item)
			} else {
				setSimField(&p, line)
			}
			comment = ""
		}
		if len(p.Bags) > maxBagItems || len(p.Equipped) > len(simSlots) {
			return models.SimProfile{}, fmt.Errorf("%w: too many items", ErrBadSimC)
		}
	}
	switch {
	case p.Class == "":
		return models.SimProfile{}, fmt.Errorf(`%w: no character line (shaman="Name")`, ErrBadSimC)
	case p.Spec == "" || p.Talents == "":
		return models.SimProfile{}, fmt.Errorf("%w: no spec or talents", ErrBadSimC)
	case len(p.Equipped) == 0:
		return models.SimProfile{}, fmt.Errorf("%w: no equipped gear", ErrBadSimC)
	}
	p.ItemLevel = itemLevel(p.Equipped)
	return p, nil
}

// itemLevel is the equipped item level the game shows (SimProfile.ItemLevel).
func itemLevel(equipped []models.SimItem) int {
	sum, offHand, mainHand := 0, false, 0
	for _, item := range equipped {
		switch item.Slot {
		case "shirt", "tabard":
			continue
		case "off_hand":
			offHand = true
		case "main_hand":
			mainHand = item.Level
		}
		sum += item.Level
	}
	if !offHand {
		sum += mainHand
	}
	return sum / (len(simSlots) - 2)
}

// setSimField reads one header line (shaman="Name", level=90, spec=...) into p; a line that doesn't match its field's
// pattern is dropped.
func setSimField(p *models.SimProfile, line string) {
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return
	}
	if m := simClassLine.FindStringSubmatch(line); m != nil && slices.Contains(simClasses, m[1]) {
		if simName.MatchString(m[2]) {
			p.Class, p.Name = m[1], m[2]
		}
		return
	}
	word := func(dst *string) {
		if simWord.MatchString(value) {
			*dst = value
		}
	}
	switch key {
	case "level":
		if level, err := strconv.Atoi(value); err == nil && level > 0 && level < 200 {
			p.Level = level
		}
	case "race":
		word(&p.Race)
	case "region":
		word(&p.Region)
	case "server":
		word(&p.Server)
	case "role":
		word(&p.Role)
	case "spec":
		word(&p.Spec)
	case "talents":
		if simTalents.MatchString(value) {
			p.Talents = value
		}
	case "professions":
		if simPairs.MatchString(value) {
			p.Professions = value
		}
	case "omnium_talents":
		if simOmnium.MatchString(value) {
			p.Omnium = value
		}
	}
}

// parseSimItem reads "head=,id=271483,bonus_id=1/2/3,..." (the item's name, which the addon leaves empty and
// simc's own profiles fill, is dropped); comment is the "# Name (334)" line before it. It is false for a line that
// isn't a gear line.
func parseSimItem(line, comment string) (models.SimItem, bool) {
	slot, rest, _ := strings.Cut(line, "=")
	if !slices.Contains(simSlots, slot) {
		return models.SimItem{}, false
	}
	item := models.SimItem{Slot: slot, Group: gearKey(slot)}
	if m := simItemComment.FindStringSubmatch(comment); m != nil {
		item.Name = m[1]
		item.Level, _ = strconv.Atoi(m[2])
	}
	for field := range strings.SplitSeq(rest, ",") {
		key, value, _ := strings.Cut(field, "=")
		nums, ok := parseSimNumbers(value)
		switch {
		case !ok:
		case key == "id" && len(nums) == 1:
			item.ID = nums[0]
		case slices.Contains(simItemKeys, key):
			if item.Opts == nil {
				item.Opts = map[string][]int{}
			}
			item.Opts[key] = nums
		}
	}
	return item, item.ID > 0
}

// parseSimNumbers is "1/2/3" as numbers, false for anything else.
func parseSimNumbers(value string) ([]int, bool) {
	var nums []int
	for part := range strings.SplitSeq(value, "/") {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > maxItemValue {
			return nil, false
		}
		nums = append(nums, n)
	}
	return nums, true
}

// SimInput is p written as a simc input file: the character and what they wear. It is built only from the fields
// ParseSimC validated, so p must come from there.
func SimInput(p models.SimProfile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s=%q\nlevel=%d\nrace=%s\nregion=%s\nserver=%s\nspec=%s\ntalents=%s\n",
		p.Class, p.Name, p.Level, p.Race, p.Region, p.Server, p.Spec, p.Talents)
	for _, field := range [][2]string{{"role", p.Role}, {"professions", p.Professions}, {"omnium_talents", p.Omnium}} {
		if field[1] != "" {
			fmt.Fprintf(&b, "%s=%s\n", field[0], field[1])
		}
	}
	for _, item := range p.Equipped {
		b.WriteString(SimItemLine(item))
	}
	return b.String()
}

// SimItemLine is item as a simc line ("head=,id=271483,bonus_id=1/2"), with its numbers in key order.
func SimItemLine(item models.SimItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s=,id=%d", item.Slot, item.ID)
	for _, key := range slices.Sorted(maps.Keys(item.Opts)) {
		parts := make([]string, len(item.Opts[key]))
		for i, n := range item.Opts[key] {
			parts[i] = strconv.Itoa(n)
		}
		fmt.Fprintf(&b, ",%s=%s", key, strings.Join(parts, "/"))
	}
	b.WriteString("\n")
	return b.String()
}
