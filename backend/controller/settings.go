package controller

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"wow-live/backend/models"
	"wow-live/backend/service"
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

// Settings reads and saves the desktop app's settings (app.Settings); only the app serves them.
type Settings interface {
	Get() models.SettingsView
	Save(models.Settings) error
}

// WCLKeys keeps the desktop app's Warcraft Logs keys (service.WCLKeys).
type WCLKeys interface {
	Status() models.KeyStatus
	Save(ctx context.Context, creds models.Credentials, force bool) (models.KeyStatus, error)
	Retest(ctx context.Context) (models.KeyStatus, error)
	Remove() error
}

// LogFolder reads, lists, archives and deletes the game's combat logs and the Warcraft Logs imports, and measures
// the app's folders (app.Site); the errors wrap service.ErrBusy while another of these runs, and service.ErrBadLog
// for a log the list doesn't offer.
type LogFolder interface {
	Learn(all bool) (read, saved int, err error)
	LogFiles() ([]models.LogFile, error)
	ArchiveLogs(source string, names []string) ([]models.LogFile, error)
	DeleteLogs(source string, names []string) ([]models.LogFile, error)
	Storage() (models.Storage, error)
}

// Characters checks the character setting and guesses it from the saved keys (service.Characters); Resolve's errors
// wrap service.ErrBadCharacter for a character that doesn't fit, models.ErrNoCredentials without Warcraft Logs keys.
type Characters interface {
	Resolve(ctx context.Context, region, character string) (name, slug string, err error)
	Guess() string
	Parses(ctx context.Context, region, character string) (models.CharacterParses, error)
}

// Import imports a character's Warcraft Logs fights in the background (service.Importer); Start's and Find's errors
// wrap service.ErrBusy while one runs and as Characters.Resolve's do.
type Import interface {
	Start(ctx context.Context, region, character string, kinds []models.ImportKind) error
	Find(ctx context.Context, region, character string) error
	Stop()
	Status() models.ImportStatus
	Remove() error
}

// SettingsAPI serves the desktop app's settings and Warcraft Logs keys, and the log folder's old logs.
type SettingsAPI struct {
	Settings   Settings
	Keys       WCLKeys
	Logs       LogFolder  // set by app.Start
	Characters Characters // set by app.Start
	Import     Import     // set by app.Start
}

// LogNames names the logs to archive or delete, as LogFiles lists them, and their source (the game's, the default, or
// Warcraft Logs imports).
type LogNames struct {
	Names  []string `json:"names" binding:"required,min=1"`
	Source string   `json:"source" binding:"omitempty,oneof=game wcl" enums:"game,wcl"`
}

// ImportKinds are the kinds of fight to import, each once.
type ImportKinds struct {
	Kinds []models.ImportKind `json:"kinds" binding:"required,min=1,unique"`
}

// OldKeys is how many logs were read and how many keys are saved after reading the log folder.
type OldKeys struct {
	Read  int `json:"read"`
	Saved int `json:"saved"`
}

// ReadKeys godoc
//
//	@Summary		Save the keys and raid pulls already in the log folder
//	@Description	Reads the combat logs in the folder that are new or changed since the last read (all=true: every
//	@Description	one) and saves each finished key and raid pull, so the estimate and the best pulls have earlier runs
//	@Description	to go on. Reading 3 GB of logs takes about 10 s.
//	@Tags			settings
//	@Produce		json
//	@Param			all	query		bool	false	"read every log again, even ones already read"
//	@Success		200	{object}	OldKeys
//	@Failure		400	{object}	ErrorResponse
//	@Failure		409	{object}	ErrorResponse	"already reading, archiving or deleting logs"
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/settings/read-keys [post]
func (s SettingsAPI) ReadKeys(c *gin.Context) {
	all, ok := boolQuery(c, "all")
	if !ok {
		return
	}
	read, saved, err := s.Logs.Learn(all)
	if logFolderFailed(c, "reading old logs", err) {
		return
	}
	c.JSON(http.StatusOK, OldKeys{Read: read, Saved: saved})
}

// LogFiles godoc
//
//	@Summary		List the combat logs, the archived ones and the Warcraft Logs imports
//	@Description	The log folder's combat logs, then the ones archived (zipped into its WoW Live archive folder), then
//	@Description	the Warcraft Logs fights imported (source wcl, dated at the fight's end), each newest first: size,
//	@Description	last write, saved keys and pulls from each, whether it's read (an import: its key or pull is saved),
//	@Description	whether the game may still write it (it can't be archived) and whether to tick it at first (older
//	@Description	than 14 days and read).
//	@Tags			settings
//	@Produce		json
//	@Success		200	{array}		models.LogFile
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/settings/logs [get]
func (s SettingsAPI) LogFiles(c *gin.Context) {
	files, err := s.Logs.LogFiles()
	if logFolderFailed(c, "listing logs", err) {
		return
	}
	c.JSON(http.StatusOK, files)
}

// ArchiveLogs godoc
//
//	@Summary		Archive combat logs
//	@Description	Zips each named log into the log folder's WoW Live archive folder and removes it (reading the folder
//	@Description	first when one isn't read yet), so live reading and Read old logs skip it; Read everything again
//	@Description	still reads it. Answers the new list; a log that couldn't be archived carries its error. Only the
//	@Description	game's logs: Warcraft Logs imports are zips already.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			logs	body		LogNames	true	"names of game logs from the list, not archived and not being written"
//	@Success		200		{array}		models.LogFile
//	@Failure		400		{object}	ErrorResponse
//	@Failure		409		{object}	ErrorResponse	"already reading, archiving or deleting logs"
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/settings/logs/archive [post]
func (s SettingsAPI) ArchiveLogs(c *gin.Context) {
	s.onLogs(c, "archiving logs", s.Logs.ArchiveLogs)
}

// DeleteLogs godoc
//
//	@Summary		Delete archived combat logs or Warcraft Logs imports for good
//	@Description	Removes the named archived logs' zips (source game) or imported fights' zips (source wcl). Saved
//	@Description	keys and pulls stay; importing a fight again costs Warcraft Logs points. Answers the new list; a log
//	@Description	that couldn't be deleted carries its error.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			logs	body		LogNames	true	"names of archived logs or imports from the list"
//	@Success		200		{array}		models.LogFile
//	@Failure		400		{object}	ErrorResponse
//	@Failure		409		{object}	ErrorResponse	"already reading, archiving or deleting logs"
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/settings/logs/archive [delete]
func (s SettingsAPI) DeleteLogs(c *gin.Context) {
	s.onLogs(c, "deleting logs", s.Logs.DeleteLogs)
}

// onLogs reads the names and source in the body and answers what do did to them.
func (s SettingsAPI) onLogs(c *gin.Context, doing string, do func(string, []string) ([]models.LogFile, error)) {
	var body LogNames
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send the names of the logs and their source (game or wcl)"})
		return
	}
	files, err := do(cmp.Or(body.Source, models.LogSourceGame), body.Names)
	if logFolderFailed(c, doing, err) {
		return
	}
	c.JSON(http.StatusOK, files)
}

// Storage godoc
//
//	@Summary		How much disk the app's data and the game's logs take
//	@Description	The size of the saved keys, the saved raid pulls, the Warcraft Logs imports, the archived combat logs
//	@Description	and the game's combat logs, measured now.
//	@Tags			settings
//	@Produce		json
//	@Success		200	{object}	models.Storage
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/storage [get]
func (s SettingsAPI) Storage(c *gin.Context) {
	storage, err := s.Logs.Storage()
	if logFolderFailed(c, "measuring the folders", err) {
		return
	}
	c.JSON(http.StatusOK, storage)
}

// logFolderFailed answers a log folder job's error, if any: 409 while another runs, 400 for a log the list doesn't
// offer, else 500 (logged).
func logFolderFailed(c *gin.Context, doing string, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, service.ErrBusy):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "already reading, archiving or deleting logs: try again soon"})
	case errors.Is(err, service.ErrBadLog):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	default:
		log.Printf("%s: %v", doing, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "couldn't use the log folder"})
	}
	return true
}

// Get godoc
//
//	@Summary	The desktop app's settings
//	@Tags		settings
//	@Produce	json
//	@Success	200	{object}	models.SettingsView
//	@Router		/api/settings [get]
func (s SettingsAPI) Get(c *gin.Context) {
	c.JSON(http.StatusOK, s.view())
}

// Put godoc
//
//	@Summary		Save the desktop app's settings
//	@Description	The log folder and the overlay change right away; a new region's affix names show from the next
//	@Description	start. A character ("Name-Realm", empty for none) needs a name of 2 to 12 letters and a realm Warcraft
//	@Description	Logs lists in the region, which takes the saved Warcraft Logs keys. Each of the auto rule's days is 0
//	@Description	(off) to 365.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			settings	body		models.Settings	true	"the settings"
//	@Success		200			{object}	models.SettingsView
//	@Failure		400			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse	"A character needs saved Warcraft Logs keys"
//	@Failure		500			{object}	ErrorResponse
//	@Failure		502			{object}	ErrorResponse	"Warcraft Logs couldn't be reached to check the character"
//	@Router			/api/settings [put]
func (s SettingsAPI) Put(c *gin.Context) {
	var next models.Settings
	err := c.ShouldBindJSON(&next)
	next.LogsDir = filepath.Clean(next.LogsDir)
	if o := next.Overlay; err != nil || !filepath.IsAbs(next.LogsDir) || !slices.Contains(regions, next.Region) ||
		!slices.Contains(overlayShows, o.Show) || o.Opacity < minOpacity || o.Opacity > 100 || o.Rows < 0 ||
		slices.ContainsFunc(o.Columns, func(c string) bool { return !slices.Contains(overlayColumns, c) }) ||
		!autoDays(next.Auto.ArchiveGameLogsDays) || !autoDays(next.Auto.DeleteImportsDays) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("send a full log folder path, a region (%s), when "+
			"overlay shows (%s), its opacity (%d to 100), its rows (0 or more), its columns (%s) and the auto rule's "+
			"days (0 to %d)", strings.Join(regions, ", "), strings.Join(overlayShows, ", "), minOpacity,
			strings.Join(overlayColumns, ", "), maxAutoDays)})
		return
	}
	before := s.Settings.Get().Settings
	next.Character = strings.TrimSpace(next.Character)
	if next.Character != "" && (next.Character != before.Character || next.Region != before.Region) {
		if _, _, err := s.Characters.Resolve(c.Request.Context(), next.Region, next.Character); characterFailed(c, err) {
			return
		}
	}
	if err := s.Settings.Save(next); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "couldn't save the settings"})
		return
	}
	if s.Import != nil && next.Character != before.Character {
		s.Import.Stop() // its fights are the old character's
	}
	c.JSON(http.StatusOK, s.view())
}

// autoDays is whether days fits an auto rule's field: 0 (off) to maxAutoDays.
func autoDays(days int) bool {
	return days >= 0 && days <= maxAutoDays
}

// characterFailed answers a character check's error, if any.
func characterFailed(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, service.ErrBadCharacter):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send the character as Name-Realm: a name of 2 to 12 " +
			"letters and a realm in the region"})
	case errors.Is(err, models.ErrNoCredentials):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "save the Warcraft Logs keys first: they check the realm"})
	default:
		log.Printf("asking Warcraft Logs: %v", err)
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: "couldn't reach Warcraft Logs"})
	}
	return true
}

// view is the settings with the choices to pick from.
func (s SettingsAPI) view() models.SettingsView {
	view := s.Settings.Get()
	view.Regions, view.OverlayShows, view.MinOpacity = regions, overlayShows, minOpacity
	view.OverlayColumns, view.MaxAutoDays = overlayColumns, maxAutoDays
	view.CharacterGuess = s.Characters.Guess()
	return view
}

// KeysStatus godoc
//
//	@Summary	Whether Warcraft Logs keys are saved, and their last test (never the secret)
//	@Tags		settings
//	@Produce	json
//	@Success	200	{object}	models.KeyStatus
//	@Router		/api/settings/warcraftlogs [get]
func (s SettingsAPI) KeysStatus(c *gin.Context) {
	c.JSON(http.StatusOK, s.Keys.Status())
}

// SaveKeys godoc
//
//	@Summary		Test and save the Warcraft Logs client ID and secret
//	@Description	Gets a token and reads the points left this hour, then saves the keys in Windows Credential
//	@Description	Manager. With force=true they're saved even when the test fails.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			keys	body		models.Credentials	true	"the client ID and secret"
//	@Param			force	query		bool				false	"save even if the test fails"
//	@Success		200		{object}	models.KeyStatus
//	@Failure		400		{object}	ErrorResponse
//	@Failure		422		{object}	ErrorResponse	"Warcraft Logs turned the keys down"
//	@Failure		500		{object}	ErrorResponse	"Windows Credential Manager failed"
//	@Failure		502		{object}	ErrorResponse	"Warcraft Logs couldn't be reached"
//	@Router			/api/settings/warcraftlogs [put]
func (s SettingsAPI) SaveKeys(c *gin.Context) {
	var creds models.Credentials
	err := c.ShouldBindJSON(&creds)
	creds.ID, creds.Secret = strings.TrimSpace(creds.ID), strings.TrimSpace(creds.Secret)
	if err != nil || creds.ID == "" || creds.Secret == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send a client ID and a secret"})
		return
	}
	force, ok := boolQuery(c, "force")
	if !ok {
		return
	}
	keyAnswer(c)(s.Keys.Save(c.Request.Context(), creds, force))
}

// TestKeys godoc
//
//	@Summary	Test the saved Warcraft Logs keys again
//	@Tags		settings
//	@Produce	json
//	@Success	200	{object}	models.KeyStatus
//	@Failure	404	{object}	ErrorResponse	"No keys saved"
//	@Failure	422	{object}	ErrorResponse	"Warcraft Logs turned the keys down"
//	@Failure	502	{object}	ErrorResponse	"Warcraft Logs couldn't be reached"
//	@Router		/api/settings/warcraftlogs/test [post]
func (s SettingsAPI) TestKeys(c *gin.Context) {
	keyAnswer(c)(s.Keys.Retest(c.Request.Context()))
}

// RemoveKeys godoc
//
//	@Summary	Remove the saved Warcraft Logs keys
//	@Tags		settings
//	@Success	204
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/settings/warcraftlogs [delete]
func (s SettingsAPI) RemoveKeys(c *gin.Context) {
	if err := s.Keys.Remove(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "couldn't remove the keys"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ImportStatus godoc
//
//	@Summary	How the import of the character's Warcraft Logs reports stands
//	@Tags		settings
//	@Produce	json
//	@Success	200	{object}	models.ImportStatus
//	@Router		/api/settings/warcraftlogs/import [get]
func (s SettingsAPI) ImportStatus(c *gin.Context) {
	c.JSON(http.StatusOK, s.Import.Status())
}

// FindFights godoc
//
//	@Summary		List the fights an import of the character's Warcraft Logs reports would fetch
//	@Description	In the background, lists the reports the settings' character is in (about 2 points a report the
//	@Description	first time, then only new reports; the list is reused for 10 minutes) and counts the fights an
//	@Description	import would fetch by kind (Mythic+ keys, raid kills, raid wipes), with their minutes and an
//	@Description	estimate of their points: the status's plan once its state leaves listing. DELETE on the import
//	@Description	stops it. A POST, since listing spends points.
//	@Tags			settings
//	@Produce		json
//	@Success		202	{object}	models.ImportStatus
//	@Failure		400	{object}	ErrorResponse	"No character, or one that doesn't fit"
//	@Failure		404	{object}	ErrorResponse	"No Warcraft Logs keys saved"
//	@Failure		409	{object}	ErrorResponse	"An import or a search is running"
//	@Failure		502	{object}	ErrorResponse	"Warcraft Logs couldn't be reached"
//	@Router			/api/settings/warcraftlogs/import/plan [post]
func (s SettingsAPI) FindFights(c *gin.Context) {
	settings, ok := s.importCharacter(c)
	if !ok {
		return
	}
	if importFailed(c, s.Import.Find(c.Request.Context(), settings.Region, settings.Character)) {
		return
	}
	c.JSON(http.StatusAccepted, s.Import.Status())
}

// CharacterParses godoc
//
//	@Summary		The character's Warcraft Logs parses this season
//	@Description	The settings' character's best and median parse in each Mythic+ dungeon of the season and each boss
//	@Description	of the current raid it has a ranked run or kill in, with its Warcraft Logs page (found false when
//	@Description	Warcraft Logs doesn't know the character). Read once a day; a read costs a few points.
//	@Tags			settings
//	@Produce		json
//	@Success		200	{object}	models.CharacterParses
//	@Failure		400	{object}	ErrorResponse	"No character, or one that doesn't fit"
//	@Failure		404	{object}	ErrorResponse	"No Warcraft Logs keys saved"
//	@Failure		502	{object}	ErrorResponse	"Warcraft Logs couldn't be reached"
//	@Router			/api/settings/warcraftlogs/parses [get]
func (s SettingsAPI) CharacterParses(c *gin.Context) {
	settings, ok := s.importCharacter(c)
	if !ok {
		return
	}
	parses, err := s.Characters.Parses(c.Request.Context(), settings.Region, settings.Character)
	if characterFailed(c, err) {
		return
	}
	c.JSON(http.StatusOK, parses)
}

// StartImport godoc
//
//	@Summary		Import the character's Warcraft Logs reports of this season
//	@Description	Lists the reports the settings' character is in, then in the background fetches each Mythic+ key and
//	@Description	raid boss pull of the kinds sent, of the current season and raid, not saved yet (a key or pull read
//	@Description	from a local log is skipped) and saves it, tagged with its report. It waits for the hour's points
//	@Description	when they run low, and goes on after a restart.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			kinds	body		ImportKinds	true	"the kinds of fight to import"
//	@Success		202		{object}	models.ImportStatus
//	@Failure		400		{object}	ErrorResponse	"No character, one that doesn't fit, or no or unknown kinds"
//	@Failure		404		{object}	ErrorResponse	"No Warcraft Logs keys saved"
//	@Failure		409		{object}	ErrorResponse	"An import is running"
//	@Failure		502		{object}	ErrorResponse	"Warcraft Logs couldn't be reached"
//	@Router			/api/settings/warcraftlogs/import [post]
func (s SettingsAPI) StartImport(c *gin.Context) {
	var body ImportKinds
	if err := c.ShouldBindJSON(&body); err != nil || slices.ContainsFunc(body.Kinds, func(k models.ImportKind) bool {
		return !slices.Contains(models.ImportKinds, k)
	}) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send the kinds of fight to import, each once: " +
			fmt.Sprint(models.ImportKinds)})
		return
	}
	settings, ok := s.importCharacter(c)
	if !ok {
		return
	}
	if importFailed(c, s.Import.Start(c.Request.Context(), settings.Region, settings.Character, body.Kinds)) {
		return
	}
	c.JSON(http.StatusAccepted, s.Import.Status())
}

// importCharacter is the settings, when they have a character to import for; it answers 400 when they don't.
func (s SettingsAPI) importCharacter(c *gin.Context) (models.Settings, bool) {
	settings := s.Settings.Get().Settings
	if settings.Character == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "save your character (Name-Realm) first"})
		return settings, false
	}
	return settings, true
}

// importFailed answers an import's or plan's error, if any: 409 while one runs, else as characterFailed.
func importFailed(c *gin.Context, err error) bool {
	if errors.Is(err, service.ErrBusy) {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "an import or a search is already running"})
		return true
	}
	return characterFailed(c, err)
}

// StopImport godoc
//
//	@Summary		Stop the import of Warcraft Logs reports
//	@Description	What it fetched stays, so the next import goes on from there.
//	@Tags			settings
//	@Produce		json
//	@Success		200	{object}	models.ImportStatus
//	@Router			/api/settings/warcraftlogs/import [delete]
func (s SettingsAPI) StopImport(c *gin.Context) {
	s.Import.Stop()
	c.JSON(http.StatusOK, s.Import.Status())
}

// RemoveImported godoc
//
//	@Summary		Remove every key and raid pull imported from Warcraft Logs
//	@Description	Stops the import and deletes what it fetched; importing them again spends the points again.
//	@Tags			settings
//	@Produce		json
//	@Success		204
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/settings/warcraftlogs/imported [delete]
func (s SettingsAPI) RemoveImported(c *gin.Context) {
	if err := s.Import.Remove(); err != nil {
		log.Printf("removing imported keys and pulls: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "couldn't remove the imported keys and pulls"})
		return
	}
	c.Status(http.StatusNoContent)
}

// keyAnswer writes a keys call's result: the status, or why the test failed (never the error's own text, which
// could carry the request).
func keyAnswer(c *gin.Context) func(models.KeyStatus, error) {
	return func(status models.KeyStatus, err error) {
		switch {
		case err == nil:
			c.JSON(http.StatusOK, status)
		case errors.Is(err, service.ErrKeyStore):
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Windows Credential Manager couldn't keep the keys"})
		case errors.Is(err, models.ErrNoCredentials):
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "no Warcraft Logs keys saved"})
		case errors.Is(err, models.ErrBadCredentials):
			c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: "Warcraft Logs turned down this client ID " +
				"and secret; check them, or save anyway"})
		default:
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: "couldn't reach Warcraft Logs; save anyway to keep " +
				"the keys"})
		}
	}
}
