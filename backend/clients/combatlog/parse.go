// Package combatlog reads World of Warcraft's combat log (Logs/WoWCombatLog-*.txt): it follows the newest file as
// the game writes it and parses each line into a models.LogLine.
package combatlog

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"wow-live/backend/models"
)

// timeLayout is the line's local time before its UTC offset: "11/3/2026 19:14:54.254".
const timeLayout = "1/2/2006 15:04:05.000"

// ParseLine reads one line, e.g. `11/3/2026 19:14:54.254-7  UNIT_DIED,0000000000000000,nil,...`: its time (ms since
// the epoch, from the line's own UTC offset, so the PC's time zone doesn't matter), event name and fields.
func ParseLine(text string) (models.LogLine, error) {
	stamp, body, ok := strings.Cut(strings.TrimRight(text, "\r\n"), "  ")
	if !ok {
		return models.LogLine{}, fmt.Errorf("no event in line %.60q", text)
	}
	ms, err := parseTime(stamp)
	if err != nil {
		return models.LogLine{}, err
	}
	fields := splitFields(body)
	return models.LogLine{Time: ms, Event: fields[0], Fields: fields[1:]}, nil
}

// parseTime reads "11/3/2026 19:14:54.254-7" (the offset is hours from UTC, and may be missing).
func parseTime(stamp string) (int64, error) {
	dot := strings.LastIndexByte(stamp, '.')
	if dot < 0 {
		return 0, fmt.Errorf("reading the time %q: no milliseconds", stamp)
	}
	local, offset := stamp, ""
	if i := strings.IndexAny(stamp[dot:], "+-"); i >= 0 {
		local, offset = stamp[:dot+i], stamp[dot+i:]
	}
	zone, ok := zones[offset]
	if !ok {
		hours, err := strconv.ParseFloat(offset, 64)
		if err != nil {
			return 0, fmt.Errorf("reading the UTC offset in %q: %w", stamp, err)
		}
		zone = time.FixedZone("log", int(hours*3600))
	}
	if t, ok := localTime(local, zone); ok {
		return t.UnixMilli(), nil
	}
	t, err := time.ParseInLocation(timeLayout, local, zone)
	if err != nil {
		return 0, fmt.Errorf("reading the time %q: %w", stamp, err)
	}
	return t.UnixMilli(), nil
}

// zones are the UTC offsets as the log writes them ("-7", "+5.5", "" for none), every half hour from -12 to +14,
// built once and only read after, so a line doesn't make a zone of its own.
var zones = func() map[string]*time.Location {
	z := map[string]*time.Location{"": time.FixedZone("log", 0)}
	for half := -24; half <= 28; half++ {
		text := strconv.FormatFloat(float64(half)/2, 'f', -1, 64)
		if half >= 0 {
			text = "+" + text
		}
		z[text] = time.FixedZone("log", half*1800)
	}
	return z
}()

// localTime reads "11/3/2026 19:14:54.254" (timeLayout) in zone by hand, several times faster than time.Parse. It's
// false for anything but that exact form with in-range values, which parseTime then leaves to time.Parse to judge.
func localTime(local string, zone *time.Location) (time.Time, bool) {
	date, clock, _ := strings.Cut(local, " ")
	month, rest, _ := strings.Cut(date, "/")
	day, year, _ := strings.Cut(rest, "/")
	hour, rest, _ := strings.Cut(clock, ":")
	minute, rest, _ := strings.Cut(rest, ":")
	second, milli, _ := strings.Cut(rest, ".")
	mo, d, y := digits(month, 1, 2), digits(day, 1, 2), digits(year, 4, 4)
	h, mi, s, ms := digits(hour, 1, 2), digits(minute, 2, 2), digits(second, 2, 2), digits(milli, 3, 3)
	if mo < 1 || mo > 12 || d < 1 || y < 0 || h < 0 || h > 23 || mi < 0 || mi > 59 || s < 0 || s > 59 || ms < 0 ||
		d > 28 && d > time.Date(y, time.Month(mo)+1, 0, 0, 0, 0, 0, time.UTC).Day() {
		return time.Time{}, false
	}
	return time.Date(y, time.Month(mo), d, h, mi, s, ms*1e6, zone), true
}

// digits is s as a number when it's minLen to maxLen ASCII digits (no sign), else -1.
func digits(s string, minLen, maxLen int) int {
	if len(s) < minLen || len(s) > maxLen {
		return -1
	}
	n := 0
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// splitFields splits on commas outside quotes and brackets, and drops the quotes around a quoted field, so a name
// with a comma or a COMBATANT_INFO talent list stays one field.
func splitFields(body string) []string {
	fields := make([]string, 0, strings.Count(body, ",")+1)
	depth, quoted, start := 0, false, 0
	for i := 0; i < len(body); i++ {
		switch c := body[i]; {
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == '[' || c == '(':
			depth++
		case c == ']' || c == ')':
			depth--
		case c == ',' && depth == 0:
			fields = append(fields, unquote(body[start:i]))
			start = i + 1
		}
	}
	return append(fields, unquote(body[start:]))
}

// unquote drops the quotes around a field.
func unquote(field string) string {
	if len(field) >= 2 && field[0] == '"' && field[len(field)-1] == '"' {
		return field[1 : len(field)-1]
	}
	return field
}
