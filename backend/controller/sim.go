package controller

import (
	"errors"
	"io/fs"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"wow-live/backend/models"
	"wow-live/backend/service"
)

// simBodyBytes bounds a sim request's body: the export it carries is at most 64 KB before JSON escaping.
const simBodyBytes = 128 << 10

// SimRuns runs character sims with the downloaded SimulationCraft engine (service.Sims). Start's errors wrap
// service.ErrBadSimOptions, service.ErrBadSimC, service.ErrNoEngine or service.ErrSimBusy; Get and Remove's wrap
// fs.ErrNotExist for an ID that isn't a sim.
type SimRuns interface {
	Choices() models.SimChoices
	EngineStatus() models.SimEngineStatus
	InstallEngine() error
	Start(req models.SimRequest) (models.Sim, error)
	Get(id string) (models.Sim, error)
	List() ([]models.Sim, error)
	Remove(id string) error
}

// SimAPI serves the sim page; only the desktop app does, since a browser can't download an engine to the PC.
type SimAPI struct {
	Runs SimRuns
}

// SimExport is a pasted SimulationCraft addon export.
type SimExport struct {
	Export string `json:"export" binding:"required"`
}

// Choices godoc
//
//	@Summary	The options a sim can run with
//	@Tags		sim
//	@Produce	json
//	@Success	200	{object}	models.SimChoices
//	@Router		/api/sim/choices [get]
func (a SimAPI) Choices(c *gin.Context) {
	c.JSON(http.StatusOK, a.Runs.Choices())
}

// Engine godoc
//
//	@Summary		Whether the SimulationCraft engine is installed
//	@Description	Its version, game patch and build time, whether a download runs or failed, and whether the build is
//	@Description	old enough that a patch has probably made it wrong.
//	@Tags			sim
//	@Produce		json
//	@Success		200	{object}	models.SimEngineStatus
//	@Router			/api/sim/engine [get]
func (a SimAPI) Engine(c *gin.Context) {
	c.JSON(http.StatusOK, a.Runs.EngineStatus())
}

// InstallEngine godoc
//
//	@Summary		Download or update the SimulationCraft engine
//	@Description	Downloads the newest Windows nightly (about 115 MB) from simulationcraft.org, plain HTTP with no
//	@Description	checksum, in the background and keeps simc.exe from it; poll the engine's status.
//	@Tags			sim
//	@Produce		json
//	@Success		202	{object}	models.SimEngineStatus
//	@Failure		409	{object}	ErrorResponse	"a sim or a download is running"
//	@Router			/api/sim/engine [post]
func (a SimAPI) InstallEngine(c *gin.Context) {
	if simFailed(c, a.Runs.InstallEngine()) {
		return
	}
	c.JSON(http.StatusAccepted, a.Runs.EngineStatus())
}

// ParseProfile godoc
//
//	@Summary		Read a pasted SimulationCraft addon export
//	@Description	The character, talents, equipped gear and carried gear in the export, so the page can show them.
//	@Description	Nothing is run or kept.
//	@Tags			sim
//	@Accept			json
//	@Produce		json
//	@Param			body	body		SimExport	true	"the text of /simc"
//	@Success		200		{object}	models.SimProfile
//	@Failure		400		{object}	ErrorResponse
//	@Router			/api/sim/profile [post]
func (a SimAPI) ParseProfile(c *gin.Context) {
	var body SimExport
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, simBodyBytes)
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send the export text as {\"export\": \"...\"}"})
		return
	}
	profile, err := service.ParseSimC(body.Export)
	if simFailed(c, err) {
		return
	}
	c.JSON(http.StatusOK, profile)
}

// Start godoc
//
//	@Summary		Start a sim
//	@Description	Runs SimulationCraft on this PC in the background, one at a time: the pasted export (read again,
//	@Description	never passed to simc as it is) with the options. Poll the sim by its ID.
//	@Tags			sim
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.SimRequest	true	"the kind, the export and the options"
//	@Success		202		{object}	models.Sim
//	@Failure		400		{object}	ErrorResponse	"options outside the choices, or not an export"
//	@Failure		409		{object}	ErrorResponse	"a sim or a download is running, or no engine is installed"
//	@Router			/api/sim [post]
func (a SimAPI) Start(c *gin.Context) {
	var req models.SimRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, simBodyBytes)
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send the kind, the export text and the options"})
		return
	}
	sim, err := a.Runs.Start(req)
	if simFailed(c, err) {
		return
	}
	c.JSON(http.StatusAccepted, sim)
}

// List godoc
//
//	@Summary		The running sim and the saved ones
//	@Description	Newest first, the last 20, without their results.
//	@Tags			sim
//	@Produce		json
//	@Success		200	{array}		models.Sim
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/sim [get]
func (a SimAPI) List(c *gin.Context) {
	sims, err := a.Runs.List()
	if simFailed(c, err) {
		return
	}
	c.JSON(http.StatusOK, sims)
}

// Get godoc
//
//	@Summary		One sim: its progress while it runs, then its result
//	@Tags			sim
//	@Produce		json
//	@Param			id	path		string	true	"the sim's ID"
//	@Success		200	{object}	models.Sim
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/sim/{id} [get]
func (a SimAPI) Get(c *gin.Context) {
	sim, err := a.Runs.Get(c.Param("id"))
	if simFailed(c, err) {
		return
	}
	c.JSON(http.StatusOK, sim)
}

// Remove godoc
//
//	@Summary		Stop a running sim, or delete a saved one
//	@Tags			sim
//	@Param			id	path	string	true	"the sim's ID"
//	@Success		204
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/sim/{id} [delete]
func (a SimAPI) Remove(c *gin.Context) {
	if simFailed(c, a.Runs.Remove(c.Param("id"))) {
		return
	}
	c.Status(http.StatusNoContent)
}

// simFailed answers a sim's error, if any: 400 for options or an export that don't fit, 409 for a busy or missing
// engine, 404 for an unknown sim, else 500.
func simFailed(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, service.ErrBadSimOptions), errors.Is(err, service.ErrBadSimC):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrNoEngine), errors.Is(err, service.ErrSimBusy):
		c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	case errors.Is(err, fs.ErrNotExist):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "no such sim"})
	default:
		log.Printf("sim: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "the sim failed: see the app's log"})
	}
	return true
}
