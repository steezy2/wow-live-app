package service

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// maxCustomBytes caps a request's custom simc text.
const maxCustomBytes = 16 << 10

// customOptions are the simc options a custom line may set, lowercase; one ending in "." is a prefix
// (override.bloodlust=0). The ones the app sets itself (fight style, length, targets, accuracy, threads) are not here,
// and neither is anything that reads or writes a file. Add a key here to allow it.
var customOptions = []string{
	"optimal_raid", "fixed_time", "vary_combat_length", "override.", "apl_variable.",
	"bloodlust_percent", "bloodlust_time", "allow_potions", "allow_food", "allow_flasks", "allow_augmentations",
	"seed", "deterministic", "skill", "position", "world_lag", "world_lag_stddev",
}

// customRefused are words no custom key or option value may hold: simc's file, report, network and profileset
// options, and the ones the app owns.
var customRefused = []string{"input", "output", "html", "json", "xml", "csv", "save", "load", "profileset", "copy",
	"proxy", "armory", "apikey", "threads", "iterations", "target_error", "max_time", "fight_style", "desired_targets"}

var (
	aplKey      = regexp.MustCompile(`^actions(\.[a-z0-9_]+)?$`)
	keyTail     = regexp.MustCompile(`^[a-z0-9_]+$`)
	customValue = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)
)

// customLines is the owner's custom simc text as the lines simc gets, each normalized to key=value (or key+=value):
// an action list (actions=, actions+=, actions.<list>= and +=) and customOptions; blank and # lines are dropped.
// Anything else, or text over maxCustomBytes, is an error wrapping ErrBadSimOptions that names the line.
func customLines(text string) ([]string, error) {
	if len(text) > maxCustomBytes {
		return nil, fmt.Errorf("%w: custom lines are over %d KB", ErrBadSimOptions, maxCustomBytes>>10)
	}
	var lines []string
	for n, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out, err := customLine(line)
		if err != nil {
			return nil, fmt.Errorf("%w: custom line %d (%q): %w", ErrBadSimOptions, n+1, line, err)
		}
		lines = append(lines, out)
	}
	return lines, nil
}

// customLine is one trimmed custom line normalized, or why it is refused. simc splits a line at spaces into several
// options, so neither kind of value may hold one.
func customLine(line string) (string, error) {
	k, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", errors.New("not key=value")
	}
	key, value, op := strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(value), "="
	if strings.HasSuffix(key, "+") {
		key, op = strings.TrimSpace(strings.TrimSuffix(key, "+")), "+="
	}
	if w := refusedWord(key); w != "" {
		return "", fmt.Errorf("%s is not allowed", w)
	}
	if aplKey.MatchString(key) {
		if strings.ContainsFunc(value, func(r rune) bool { return r <= ' ' || r > '~' || r == '"' || r == '$' }) {
			return "", errors.New("an action list holds no spaces, quotes, $ or unprintable characters")
		}
		return key + op + value, nil
	}
	if !allowedOption(key) {
		return "", fmt.Errorf("%s is not an option custom lines may set", key)
	}
	if op != "=" {
		return "", errors.New("only an action list takes +=")
	}
	if w := refusedWord(strings.ToLower(value)); w != "" {
		return "", fmt.Errorf("%s is not allowed", w)
	}
	if !customValue.MatchString(value) || strings.Contains(value, "..") {
		return "", errors.New("a value is letters, digits, _ . : or -")
	}
	return key + "=" + value, nil
}

// refusedWord is the first customRefused word s holds, or "".
func refusedWord(s string) string {
	i := slices.IndexFunc(customRefused, func(w string) bool { return strings.Contains(s, w) })
	if i < 0 {
		return ""
	}
	return customRefused[i]
}

// allowedOption is whether key is in customOptions, or a prefix entry followed by a plain name.
func allowedOption(key string) bool {
	return slices.ContainsFunc(customOptions, func(o string) bool {
		if strings.HasSuffix(o, ".") {
			tail, found := strings.CutPrefix(key, o)
			return found && keyTail.MatchString(tail)
		}
		return key == o
	})
}

// customInput is the accepted custom lines as simc input: one line each, nothing for none.
func customInput(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
