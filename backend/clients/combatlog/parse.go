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
	local, offset := stamp, 0.0
	if i := strings.IndexAny(stamp[dot:], "+-"); i >= 0 {
		local = stamp[:dot+i]
		hours, err := strconv.ParseFloat(stamp[dot+i:], 64)
		if err != nil {
			return 0, fmt.Errorf("reading the UTC offset in %q: %w", stamp, err)
		}
		offset = hours
	}
	t, err := time.ParseInLocation(timeLayout, local, time.FixedZone("log", int(offset*3600)))
	if err != nil {
		return 0, fmt.Errorf("reading the time %q: %w", stamp, err)
	}
	return t.UnixMilli(), nil
}

// splitFields splits on commas outside quotes and brackets, and drops the quotes around a quoted field, so a name
// with a comma or a COMBATANT_INFO talent list stays one field.
func splitFields(body string) []string {
	var fields []string
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
