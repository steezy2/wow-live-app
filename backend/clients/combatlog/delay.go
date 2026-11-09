package combatlog

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"wow-live/backend/clients/cachefile"
)

// Delays collects how far behind the game each line was read (the read time minus the game's own time on the line,
// so the game's flush plus ours) and logs them to File (logs/live-delay.log), one line per key or pull.
type Delays struct {
	File string
	ms   []int64
	live bool
}

// Add notes the delay of each line read at read. Lines read before a read first comes back empty are the log's
// backlog, not live, so they don't count.
func (d *Delays) Add(lines []string, read time.Time) {
	if !d.live {
		d.live = len(lines) == 0
		return
	}
	for _, text := range lines {
		stamp, _, _ := strings.Cut(text, "  ")
		if at, err := parseTime(stamp); err == nil {
			d.ms = append(d.ms, read.UnixMilli()-at)
		}
	}
}

// Log appends the median and 95th percentile delay since the last Log for what (a key or a pull), and starts over.
// With nothing read live since, it writes nothing.
func (d *Delays) Log(what string) error {
	if len(d.ms) == 0 {
		return nil
	}
	slices.Sort(d.ms)
	line := fmt.Sprintf("%s %s: read %.2f s behind the game (median), %.2f s (95th percentile), %d lines",
		time.Now().Format("2006-01-02 15:04"), what, float64(d.ms[len(d.ms)/2])/1000,
		float64(d.ms[len(d.ms)*95/100])/1000, len(d.ms))
	d.ms = d.ms[:0]
	return cachefile.AppendLine(d.File, line)
}
