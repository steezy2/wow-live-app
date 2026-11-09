package service

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"wow-live/backend/models"
)

var (
	// stackLimit finds the stacks a cheat sheet warns about ("Avoid getting to 11 stacks").
	stackLimit = regexp.MustCompile(`(\d+) stacks`)
	// burst finds the unit a cheat sheet says to burst or focus ("Burst down Ula'tek's Heart as it ...").
	burst = regexp.MustCompile(`(?:Burst down|Focus down|Focus) ([A-Z][^,.]*?)(?: as | who |,|\.|$)`)
	// dodgeVerbs start a line about something to get out of.
	dodgeVerbs = []string{"dodge", "avoid", "run away", "move out", "watch"}
)

// RaidBossFacts sorts a boss cheat sheet's lines into the mechanic kinds the raid overview checks, by their words:
// a soak; stacks with a number (the limit); a dispel; an interrupt; a line starting with a dodge verb or saying
// dodge; stacks without a number; and a unit to burst. Each line's spells go to its first kind; the words stay here.
func RaidBossFacts(g models.RaidGuide) models.RaidBoss {
	b := models.RaidBoss{Name: g.Name, Guide: g.URL, Avoidable: []int{}, Soak: []int{}, Dispel: []int{},
		MustStop: []int{}, Stacks: map[int]int{}, Priority: []string{}}
	for _, line := range g.Lines {
		lower := strings.ToLower(line.Text)
		_, rest, ok := strings.Cut(lower, " - ")
		if !ok {
			rest = lower
		}
		limit := stackLimit.FindStringSubmatch(lower)
		switch {
		case strings.Contains(lower, "soak"):
			b.Soak = appendNew(b.Soak, line.Spells...)
		case limit != nil:
			n, _ := strconv.Atoi(limit[1])
			for _, id := range line.Spells {
				b.Stacks[id] = n
			}
		case strings.Contains(lower, "dispel"):
			b.Dispel = appendNew(b.Dispel, line.Spells...)
		case strings.Contains(lower, "interrupt"):
			b.MustStop = appendNew(b.MustStop, line.Spells...)
		case strings.Contains(lower, "dodge") || slices.ContainsFunc(dodgeVerbs, func(v string) bool {
			return strings.HasPrefix(strings.TrimSpace(rest), v)
		}):
			b.Avoidable = appendNew(b.Avoidable, line.Spells...)
		case strings.Contains(lower, "stacks"):
			for _, id := range line.Spells {
				if _, seen := b.Stacks[id]; !seen {
					b.Stacks[id] = 0
				}
			}
		}
		for _, m := range burst.FindAllStringSubmatch(line.Text, -1) {
			b.Priority = appendNew(b.Priority, strings.TrimSpace(m[1]))
		}
	}
	return b
}

// appendNew appends the values s doesn't have yet.
func appendNew[T comparable](s []T, values ...T) []T {
	for _, v := range values {
		if !slices.Contains(s, v) {
			s = append(s, v)
		}
	}
	return s
}
