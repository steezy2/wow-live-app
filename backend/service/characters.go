package service

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"wow-live/backend/models"
)

// loggedBy is whether a run whose log logger wrote (realms: each player's realm) is character's ("Name-Realm" or a
// bare name; "" is every character). A run saved before realms were kept matches any character of that name.
func loggedBy(logger string, realms map[string]string, character string) bool {
	if character == "" {
		return true
	}
	if logger == "" {
		return false
	}
	_, realm, _ := strings.Cut(character, "-")
	got := realms[logger]
	if realm == "" || got == "" {
		got = realm
	}
	return models.SameCharacter(logger+"-"+got, character)
}

// Played is the characters whose game wrote the saved keys' and raid pulls' logs, most runs first; preferred (Settings'
// "Name-Realm") is marked.
func (k *LogTracker) Played(preferred string) []models.Character {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return playedCharacters(k.history, k.pulls, preferred)
}

// playedCharacters is Played over keys and pulls (newest first, so a character's class is its latest run's). A
// character's runs saved before realms were kept count for each character of that name, as loggedBy matches them.
func playedCharacters(keys []models.Key, pulls []models.RaidPull, preferred string) []models.Character {
	byID := map[string]*models.Character{}
	note := func(logger string, realms map[string]string, rows []models.KeyMeterRow, key bool) {
		if logger == "" {
			return
		}
		realm := realms[logger]
		id := logger
		if realm != "" {
			id += "-" + realm
		}
		c := byID[id]
		if c == nil {
			c = &models.Character{ID: id, Name: logger, Realm: realm}
			byID[id] = c
		}
		if c.Class == "" {
			if i := slices.IndexFunc(rows, func(r models.KeyMeterRow) bool { return r.Player == logger }); i >= 0 {
				c.Class = rows[i].Class
			}
		}
		if key {
			c.Keys++
		} else {
			c.Pulls++
		}
	}
	for _, k := range keys {
		note(k.Logger, k.Realms, k.Overall.Rows, true)
	}
	for _, p := range pulls {
		note(p.Logger, p.Realms, p.Meter.Rows, false)
	}
	for id, old := range byID {
		if old.Realm != "" {
			continue
		}
		folded := false
		for _, c := range byID {
			if c.Realm != "" && c.Name == old.Name {
				c.Keys, c.Pulls, c.Class, folded = c.Keys+old.Keys, c.Pulls+old.Pulls, cmp.Or(c.Class, old.Class), true
			}
		}
		if folded {
			delete(byID, id)
		}
	}
	out := []models.Character{}
	for _, id := range slices.Sorted(maps.Keys(byID)) {
		c := *byID[id]
		c.Preferred = preferred != "" && loggedBy(c.Name, map[string]string{c.Name: c.Realm}, preferred)
		out = append(out, c)
	}
	slices.SortStableFunc(out, func(a, b models.Character) int { return cmp.Compare(b.Keys+b.Pulls, a.Keys+a.Pulls) })
	return out
}
