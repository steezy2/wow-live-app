package controller

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
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

// SettingsAPI serves the desktop app's settings and Warcraft Logs keys, and reads the keys already in the log folder
// (app.Site.Learn).
type SettingsAPI struct {
	Settings    Settings
	Keys        WCLKeys
	ReadOldKeys func() (int, error)
}

// OldKeys is how many keys are saved after reading the log folder.
type OldKeys struct {
	Saved int `json:"saved"`
}

// ReadKeys godoc
//
//	@Summary		Save the keys already in the log folder
//	@Description	Reads every combat log in the folder and saves each finished key, so the estimate has earlier runs
//	@Description	to go on. Takes about a minute for a few hundred MB of logs.
//	@Tags			settings
//	@Produce		json
//	@Success		200	{object}	OldKeys
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/settings/read-keys [post]
func (s SettingsAPI) ReadKeys(c *gin.Context) {
	saved, err := s.ReadOldKeys()
	if err != nil {
		log.Printf("reading old keys: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "couldn't read the log folder"})
		return
	}
	c.JSON(http.StatusOK, OldKeys{Saved: saved})
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
	if o := next.Overlay; err != nil || next.LogsDir == "" || !slices.Contains(regions, next.Region) ||
		!slices.Contains(overlayShows, o.Show) || o.Opacity < minOpacity || o.Opacity > 100 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("send a log folder, a region (%s), when the "+
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
	keyAnswer(c)(s.Keys.Save(c.Request.Context(), creds, c.Query("force") == "true"))
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
