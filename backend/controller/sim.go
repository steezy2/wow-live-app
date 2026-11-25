package controller

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"wow-live/backend/models"
	"wow-live/backend/service"
)

// simBodyBytes bounds a sim request's body: the export it carries is at most 64 KB before JSON escaping.
const simBodyBytes = 128 << 10

// bindSim reads a sim request's body into v under simBodyBytes, or answers 400 with msg and reports false.
func bindSim(c *gin.Context, v any, msg string) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, simBodyBytes)
	if err := c.ShouldBindJSON(v); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: msg})
		return false
	}
	return true
}

// SimRuns runs character sims with the downloaded SimulationCraft engine (service.Sims). Start's errors wrap
// service.ErrBadSimOptions, service.ErrBadSimC, service.ErrNoEngine or service.ErrSimBusy; Get and Remove's wrap
// fs.ErrNotExist for an ID that isn't a sim.
type SimRuns interface {
	Choices() models.SimChoices
	Reference() models.SimReference
	Plan(req models.SimRequest) (models.SimPlan, error)
	ParseProfile(export string) (models.SimProfile, error)
	EngineStatus() models.SimEngineStatus
	InstallEngine(path string) error
	Start(req models.SimRequest) (models.Sim, error)
	Get(id string) (models.Sim, error)
	List() ([]models.Sim, error)
	Remove(id string) error
}

// SimItems names and draws the items of a pasted character and the spells of a result (service.Items); the errors
// wrap service.ErrBadSimOptions for too many IDs.
type SimItems interface {
	Infos(ctx context.Context, ids []int) ([]models.SimItemInfo, error)
	SpellInfos(ctx context.Context, ids []int) ([]models.SpellInfo, error)
}

// SimAPI serves the sim page; only the desktop app does, since a browser can't download an engine to the PC.
type SimAPI struct {
	Runs  SimRuns
	Items SimItems
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

// SimEngineFile is a simc.exe the owner has: its absolute path on this PC.
type SimEngineFile struct {
	Path string `json:"path" example:"C:/Tools/simc/simc.exe"`
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
//	@Summary		Download or update the SimulationCraft engine, or use one the owner has
//	@Description	With no body, downloads the newest simc.exe from this app's own GitHub release over HTTPS, checked
//	@Description	against the sha256 in the release's engine.json (which catches a broken download, not a swapped file:
//	@Description	both come from the same release); the release is rebuilt weekly from SimulationCraft's official nightly.
//	@Description	With a path, copies that simc.exe instead. Runs in the background: poll the engine's status.
//	@Tags			sim
//	@Accept			json
//	@Produce		json
//	@Param			body	body		SimEngineFile	false	"the path of a simc.exe to use"
//	@Success		202		{object}	models.SimEngineStatus
//	@Failure		400		{object}	ErrorResponse	"a body that isn't {"path": ...}"
//	@Failure		409		{object}	ErrorResponse	"a sim or an install is running"
//	@Router			/api/sim/engine [post]
func (a SimAPI) InstallEngine(c *gin.Context) {
	var body SimEngineFile
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, simBodyBytes)
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send nothing, or the path as {\"path\": \"C:\\...\\simc.exe\"}"})
		return
	}
	if simFailed(c, a.Runs.InstallEngine(body.Path)) {
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
	if !bindSim(c, &body, "send the export text as {\"export\": \"...\"}") {
		return
	}
	profile, err := a.Runs.ParseProfile(body.Export)
	if simFailed(c, err) {
		return
	}
	c.JSON(http.StatusOK, profile)
}

// ItemInfo godoc
//
//	@Summary		Names, icons and qualities of items
//	@Description	Wowhead's name, icon (for GET /api/icons/{name}) and quality of each item ID, as many as it knows: an
//	@Description	item it can't look up is left out, so the page draws it by its export name.
//	@Tags			sim
//	@Produce		json
//	@Param			ids	query		string	true	"item IDs, comma separated (at most SimChoices.maxItemLookups)"
//	@Success		200	{array}		models.SimItemInfo
//	@Failure		400	{object}	ErrorResponse	"no IDs, too many or not numbers"
//	@Router			/api/sim/items [get]
func (a SimAPI) ItemInfo(c *gin.Context) {
	ids, ok := queryIDs(c, "item")
	if !ok {
		return
	}
	items, err := a.Items.Infos(c.Request.Context(), ids)
	if simFailed(c, err) {
		return
	}
	c.JSON(http.StatusOK, items)
}

// SpellInfo godoc
//
//	@Summary		Names, icons and texts of spells
//	@Description	Wowhead's name, icon (for GET /api/icons/{name}), cooldown and text of each spell ID, as many as it
//	@Description	knows: a spell it can't look up is left out. The report draws its abilities and buffs with them.
//	@Tags			sim
//	@Produce		json
//	@Param			ids	query		string	true	"spell IDs, comma separated (at most SimChoices.maxItemLookups)"
//	@Success		200	{array}		models.SpellInfo
//	@Failure		400	{object}	ErrorResponse	"no IDs, too many or not numbers"
//	@Router			/api/sim/spells [get]
func (a SimAPI) SpellInfo(c *gin.Context) {
	ids, ok := queryIDs(c, "spell")
	if !ok {
		return
	}
	spells, err := a.Items.SpellInfos(c.Request.Context(), ids)
	if simFailed(c, err) {
		return
	}
	c.JSON(http.StatusOK, spells)
}

// queryIDs is the comma-separated ids query as numbers; it answers 400 and returns false for none or a bad one (what
// names the kind of ID in the message).
func queryIDs(c *gin.Context, what string) ([]int, bool) {
	parts := strings.Split(c.Query("ids"), ",")
	ids := make([]int, len(parts))
	for i, p := range parts {
		id, err := strconv.Atoi(p)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send " + what + " IDs as ids=212401,212402"})
			return nil, false
		}
		ids[i] = id
	}
	return ids, true
}

// Reference godoc
//
//	@Summary		What a sim can be given besides its gear
//	@Description	The current expansion's flasks, food, potions, augment runes, weapon oils, enchants and gems, the
//	@Description	upgrade tracks of the current seasons and the Omnium Folio's runes, from Raidbots' public game data
//	@Description	(reference/sim-options.json). Names are the ones a sim request may send.
//	@Tags			sim
//	@Produce		json
//	@Success		200	{object}	models.SimReference
//	@Router			/api/sim/options [get]
func (a SimAPI) Reference(c *gin.Context) {
	c.JSON(http.StatusOK, a.Runs.Reference())
}

// Plan godoc
//
//	@Summary		What a sim request would run, and about how long it takes
//	@Description	Counts the runs without starting the sim, so the page can show the cost (and that it is over the limit)
//	@Description	before a run. Takes the same body as starting a sim. A suggest plan also gives the Warcraft Logs points
//	@Description	its target spends, what it leaves out (notes) and, without Warcraft Logs keys or a target, why it is
//	@Description	refused (error).
//	@Tags			sim
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.SimRequest	true	"a sim request of any kind"
//	@Success		200		{object}	models.SimPlan
//	@Failure		400		{object}	ErrorResponse	"bad options or consumables, or items and variants that do not fit the export"
//	@Failure		500		{object}	ErrorResponse	"a weapon's type couldn't be read from Wowhead"
//	@Router			/api/sim/plan [post]
func (a SimAPI) Plan(c *gin.Context) {
	var req models.SimRequest
	if !bindSim(c, &req, "send a sim request") {
		return
	}
	plan, err := a.Runs.Plan(req)
	if simFailed(c, err) {
		return
	}
	c.JSON(http.StatusOK, plan)
}

// Start godoc
//
//	@Summary		Start a sim
//	@Description	Runs SimulationCraft on this PC in the background, up to SimChoices.maxParallel at once: the pasted
//	@Description	export (read again, never passed to simc as it is) with the options. Poll the sim by its ID. A Top Gear
//	@Description	request also sends `equipped` and `bags`: the indexes of the items it may put on. A `suggest` sim reads
//	@Description	the top Warcraft Logs players of the spec in its `target` (one of SimChoices.suggestTargets: Mythic+
//	@Description	overall or a raid boss on a difficulty), spending points as a player review, and runs the stat
//	@Description	weights, one profileset per change they make that the character doesn't, Top Gear over the bags, the
//	@Description	Droptimizer and every shown change together; its result's `suggest` holds the changes the sim confirmed.
//	@Description	The changes are built from the reference and the export's own items, never taken from the request.
//	@Tags			sim
//	@Accept			json
//	@Produce		json
//	@Param			body	body		models.SimRequest	true	"the kind, the export, the options and consumables, for topgear the picked equipped and bag indexes and variants, for drops the difficulty, keyLevel and upgrade, for suggest the target, and for any kind the consumables picked (two or more of a kind are compared, except by stats and suggest)"
//	@Success		202		{object}	models.Sim
//	@Failure		400		{object}	ErrorResponse	"bad options, indexes, variants, consumables or Omnium picks, not an export, nothing to try, over maxCombos, or a suggest sim without Warcraft Logs keys or a target"
//	@Failure		409		{object}	ErrorResponse	"as many sims as can run at once are running, the engine is downloading, or none is installed"
//	@Failure		500		{object}	ErrorResponse	"a weapon's type couldn't be read from Wowhead"
//	@Router			/api/sim [post]
func (a SimAPI) Start(c *gin.Context) {
	var req models.SimRequest
	if !bindSim(c, &req, "send the kind, the export text and the options") {
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
//	@Summary		The running sims and the saved ones
//	@Description	The running sims, then the saved ones (kept AutoSettings.deleteSimsDays days), each newest first and
//	@Description	without their results.
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
//	@Summary		Stop a running sim, forget one that failed or was cancelled, or delete a saved one
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
