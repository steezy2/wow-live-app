package service

import (
	"context"
	"errors"
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
	// overlayRanks are what the overlay's cards can rank by.
	overlayRanks = []string{"dps", "hps"}
	// overlayPresets are the layouts the Settings page offers; each passes validateSettings.
	overlayPresets = []models.OverlayPreset{
		{Name: "Compact", OneCard: true, RaidOneCard: true, Rows: 5, Columns: []string{"dps", "hps"}},
		{Name: "Standard", Columns: []string{"dps", "hps", "kicks"}},
		{Name: "Full", SplitCards: true, Columns: slices.DeleteFunc(slices.Clone(overlayColumns),
			func(c string) bool { return c == "pullDps" })},
	}
)

const (
	// minOpacity is the overlay background's least opacity in %: 0 leaves only the text over the game.
	minOpacity = 0
	// minRecapSeconds and maxRecapSeconds bound how long a death's recap shows over the overlay (0 turns it off).
	minRecapSeconds = 3
	maxRecapSeconds = 60
	// maxAutoDays is the most days each of the auto rule's fields takes.
	maxAutoDays = 365
)

// ErrBadSettings is a settings request that breaks the rules of validateSettings; its text says what to send.
var ErrBadSettings = fmt.Errorf("send a full log folder path, a region (%s), when overlay shows (%s), its opacity "+
	"(%d to 100), its rows and the raid cards' (0 or more), its columns (%s), what it ranks by (%s), "+
	"its death recap's seconds (0 for off, else %d to %d) and the auto rule's days (0 to %d)",
	strings.Join(regions, ", "), strings.Join(overlayShows, ", "), minOpacity, strings.Join(overlayColumns, ", "),
	strings.Join(overlayRanks, ", "), minRecapSeconds, maxRecapSeconds, maxAutoDays)

// ErrSavingSettings is returned (wrapped) by SaveSettings when the settings couldn't be written.
var ErrSavingSettings = errors.New("couldn't save the settings")

// SettingsStore is the desktop app's saved settings (app.Settings).
type SettingsStore interface {
	Get() models.SettingsView
	Save(models.Settings) error
}

// CharacterResolver checks a character against Warcraft Logs (Characters.Resolve).
type CharacterResolver interface {
	Resolve(ctx context.Context, region, character string) (name, slug string, err error)
}

// SaveSettings cleans and validates next, checks its character when it is a new one (the errors are Resolve's) and
// saves it in store; it says whether the character changed, which makes an import's fights the old character's. The
// error is ErrBadSettings for settings that break the rules and wraps ErrSavingSettings when the save failed.
func SaveSettings(ctx context.Context, store SettingsStore, characters CharacterResolver,
	next models.Settings) (characterChanged bool, err error) {
	next = cleanSettings(next)
	if err := validateSettings(next); err != nil {
		return false, err
	}
	before := store.Get().Settings
	if characterNeedsCheck(before, next) {
		if _, _, err := characters.Resolve(ctx, next.Region, next.Character); err != nil {
			return false, err
		}
	}
	if err := store.Save(next); err != nil {
		return false, fmt.Errorf("%w: %w", ErrSavingSettings, err)
	}
	return next.Character != before.Character, nil
}

// cleanSettings is s with its log folder path cleaned and its character trimmed, as every save stores them.
func cleanSettings(s models.Settings) models.Settings {
	s.LogsDir = filepath.Clean(s.LogsDir)
	s.Character = strings.TrimSpace(s.Character)
	return s
}

// characterNeedsCheck is whether saving next over before sets a character Warcraft Logs must still confirm: a new
// one, or the same one in another region.
func characterNeedsCheck(before, next models.Settings) bool {
	return next.Character != "" && (next.Character != before.Character || next.Region != before.Region)
}

// validateSettings is nil when s is within the choices SettingsChoices serves, else ErrBadSettings.
func validateSettings(s models.Settings) error {
	o, auto := s.Overlay, s.Auto
	if !filepath.IsAbs(s.LogsDir) || !slices.Contains(regions, s.Region) || !slices.Contains(overlayShows, o.Show) ||
		o.Opacity < minOpacity || o.Opacity > 100 || min(o.Rows, o.RaidDpsRows, o.RaidHpsRows) < 0 ||
		!slices.Contains(overlayRanks, o.RankBy) ||
		o.DeathRecapSeconds != 0 && (o.DeathRecapSeconds < minRecapSeconds || o.DeathRecapSeconds > maxRecapSeconds) ||
		slices.ContainsFunc(o.Columns, func(c string) bool { return !slices.Contains(overlayColumns, c) }) ||
		len(slices.Compact(slices.Sorted(slices.Values(o.Columns)))) != len(o.Columns) ||
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
	view.OverlayRanks, view.OverlayPresets = overlayRanks, overlayPresets
	view.MinRecapSeconds, view.MaxRecapSeconds = minRecapSeconds, maxRecapSeconds
	return view
}
