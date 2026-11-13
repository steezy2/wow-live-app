package service

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"wow-live/backend/models"
)

var (
	// regions are the regions the app reads affix names for.
	regions = []string{"us", "eu", "kr", "tw"}
	// overlayShows are when the overlay shows: during keys, always, or never.
	overlayShows = []string{"keys", "always", "off"}
	// overlayColumns are the columns the overlay's cards can show (the page names them).
	overlayColumns = []string{"dps", "hps", "kicks", "pullDps", "taken", "avoidable", "priority", "dispels", "deaths"}
)

const (
	// minOpacity is the overlay background's least opacity in %: 0 leaves only the text over the game.
	minOpacity = 0
	// maxAutoDays is the most days each of the auto rule's fields takes.
	maxAutoDays = 365
)

// ErrBadSettings is a settings request that breaks the rules of ValidateSettings; its text says what to send.
var ErrBadSettings = fmt.Errorf("send a full log folder path, a region (%s), when overlay shows (%s), its opacity "+
	"(%d to 100), its rows (0 or more), its columns (%s) and the auto rule's days (0 to %d)",
	strings.Join(regions, ", "), strings.Join(overlayShows, ", "), minOpacity, strings.Join(overlayColumns, ", "),
	maxAutoDays)

// ValidateSettings is nil when s is within the choices SettingsChoices serves, else ErrBadSettings.
func ValidateSettings(s models.Settings) error {
	o, auto := s.Overlay, s.Auto
	if !filepath.IsAbs(s.LogsDir) || !slices.Contains(regions, s.Region) || !slices.Contains(overlayShows, o.Show) ||
		o.Opacity < minOpacity || o.Opacity > 100 || o.Rows < 0 ||
		slices.ContainsFunc(o.Columns, func(c string) bool { return !slices.Contains(overlayColumns, c) }) ||
		!autoDays(auto.ArchiveGameLogsDays) || !autoDays(auto.DeleteImportsDays) {
		return ErrBadSettings
	}
	return nil
}

// autoDays is whether days fits an auto rule's field: 0 (off) to maxAutoDays.
func autoDays(days int) bool {
	return days >= 0 && days <= maxAutoDays
}

// SettingsChoices is view with the choices the settings are picked from.
func SettingsChoices(view models.SettingsView) models.SettingsView {
	view.Regions, view.OverlayShows, view.MinOpacity = regions, overlayShows, minOpacity
	view.OverlayColumns, view.MaxAutoDays = overlayColumns, maxAutoDays
	return view
}
