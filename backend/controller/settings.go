package controller

import (
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
)

// minOpacity is the overlay background's least opacity in %: 0 leaves only the text over the game.
const minOpacity = 0

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

// LogFolder reads, lists, archives and deletes the game's combat logs (app.Site); the errors wrap service.ErrBusy
// while another of these runs, and service.ErrBadLog for a log the list doesn't offer.
type LogFolder interface {
	Learn(all bool) (read, saved int, err error)
	LogFiles() ([]models.LogFile, error)
	ArchiveLogs(names []string) ([]models.LogFile, error)
	DeleteArchivedLogs(names []string) ([]models.LogFile, error)
}

// SettingsAPI serves the desktop app's settings and Warcraft Logs keys, and the log folder's old logs.
type SettingsAPI struct {
	Settings Settings
	Keys     WCLKeys
	Logs     LogFolder // set by app.Start
}

// LogNames names the logs to archive or delete, as LogFiles lists them.
type LogNames struct {
	Names []string `json:"names" binding:"required,min=1"`
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
//	@Summary		List the combat logs and the archived ones
//	@Description	The log folder's combat logs, then the ones archived (zipped into its WoW Live archive folder), newest
//	@Description	first: size, last write, saved keys and pulls from each, whether it's read, whether the game may still
//	@Description	write it (it can't be archived) and whether to tick it at first (older than 14 days and read).
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
//	@Description	still reads it. Answers the new list; a log that couldn't be archived carries its error.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			logs	body		LogNames	true	"names from the list, not archived and not being written"
//	@Success		200		{array}		models.LogFile
//	@Failure		400		{object}	ErrorResponse
//	@Failure		409		{object}	ErrorResponse	"already reading, archiving or deleting logs"
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/settings/logs/archive [post]
func (s SettingsAPI) ArchiveLogs(c *gin.Context) {
	s.onLogs(c, "archiving logs", s.Logs.ArchiveLogs)
}

// DeleteArchivedLogs godoc
//
//	@Summary		Delete archived combat logs for good
//	@Description	Removes the named archived logs' zips. Saved keys and pulls stay. Answers the new list; a log that
//	@Description	couldn't be deleted carries its error.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			logs	body		LogNames	true	"names of archived logs from the list"
//	@Success		200		{array}		models.LogFile
//	@Failure		400		{object}	ErrorResponse
//	@Failure		409		{object}	ErrorResponse	"already reading, archiving or deleting logs"
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/settings/logs/archive [delete]
func (s SettingsAPI) DeleteArchivedLogs(c *gin.Context) {
	s.onLogs(c, "deleting archived logs", s.Logs.DeleteArchivedLogs)
}

// onLogs reads the names in the body and answers what do did to them.
func (s SettingsAPI) onLogs(c *gin.Context, doing string, do func([]string) ([]models.LogFile, error)) {
	var body LogNames
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send the names of the logs"})
		return
	}
	files, err := do(body.Names)
	if logFolderFailed(c, doing, err) {
		return
	}
	c.JSON(http.StatusOK, files)
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
//	@Description	start.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			settings	body		models.Settings	true	"the settings"
//	@Success		200			{object}	models.SettingsView
//	@Failure		400			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/api/settings [put]
func (s SettingsAPI) Put(c *gin.Context) {
	var next models.Settings
	err := c.ShouldBindJSON(&next)
	next.LogsDir = filepath.Clean(next.LogsDir)
	if o := next.Overlay; err != nil || !filepath.IsAbs(next.LogsDir) || !slices.Contains(regions, next.Region) ||
		!slices.Contains(overlayShows, o.Show) || o.Opacity < minOpacity || o.Opacity > 100 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("send a full log folder path, a region (%s), when "+
			"overlay shows (%s) and its opacity (%d to 100)", strings.Join(regions, ", "),
			strings.Join(overlayShows, ", "), minOpacity)})
		return
	}
	if err := s.Settings.Save(next); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "couldn't save the settings"})
		return
	}
	c.JSON(http.StatusOK, s.view())
}

// view is the settings with the choices to pick from.
func (s SettingsAPI) view() models.SettingsView {
	view := s.Settings.Get()
	view.Regions, view.OverlayShows, view.MinOpacity = regions, overlayShows, minOpacity
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
