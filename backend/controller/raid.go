package controller

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"wow-live/backend/models"
	"wow-live/backend/service"
)

// Raid serves the raid overview: raid pulls read from the combat log. Now is the clock the overlay's timing is read
// at (the wall clock live, the replay's when replaying a log).
type Raid struct {
	Compares Compares // nil without Warcraft Logs credentials
	Pulls    models.RaidSource
	Maps     models.MapImageSource
	Now      func() time.Time
}

// Live godoc
//
//	@Summary		The raid now: a pull on, and the last pull
//	@Description	Whether a raid pull is on (its boss and start), the last pull that ended with its deaths,
//	@Description	mechanics, meters and cooldowns, compared with tonight's best and the best ever on the boss, and in
//	@Description	a raid between pulls the ready check (ready). The page reads it again on each notice from
//	@Description	/api/mplus/events.
//	@Tags			raid
//	@Produce		json
//	@Success		200	{object}	models.RaidLive
//	@Router			/api/raid/live [get]
func (r Raid) Live(c *gin.Context) {
	c.JSON(http.StatusOK, r.Pulls.RaidLive(r.Now().UnixMilli()))
}

// Nights godoc
//
//	@Summary	Every raid night's pulls
//	@Tags		raid
//	@Produce	json
//	@Param		character	query		string			false	"only this character's pulls (an id from /api/characters)"
//	@Success	200			{array}		models.RaidNight	"newest night first, each night's pulls newest first"
//	@Failure	400			{object}	ErrorResponse
//	@Router		/api/raid/nights [get]
func (r Raid) Nights(c *gin.Context) {
	if character, ok := characterParam(c); ok {
		c.JSON(http.StatusOK, r.Pulls.RaidNights(character))
	}
}

// Pull godoc
//
//	@Summary	One raid pull
//	@Tags		raid
//	@Produce	json
//	@Param		start	path		int	true	"the pull's start (ms since the epoch)"
//	@Success	200		{object}	models.RaidPull
//	@Failure	400		{object}	ErrorResponse
//	@Failure	404		{object}	ErrorResponse
//	@Router		/api/raid/pulls/{start} [get]
func (r Raid) Pull(c *gin.Context) {
	start, ok := startParam(c)
	if !ok {
		return
	}
	pull, err := r.Pulls.RaidPull(start)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, pull)
	case errors.Is(err, service.ErrNoSuchPull):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	default:
		log.Printf("raid pull: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "couldn't read the raid pull"})
	}
}

// Track godoc
//
//	@Summary		One raid pull's replay track
//	@Description	Each raider's position about once a second, the boss's units' and the adds to burst with their
//	@Description	health (hp) and which the health box shows (health), killed adds' last places, each raider's
//	@Description	damage, healing and damage taken so far about every 2 s, their casts, and what each ability did in
//	@Description	each 2 s stretch (abilities, abilityTimes), on the pull's clock.
//	@Tags			raid
//	@Produce		json
//	@Param			start	path		int	true	"the pull's start (ms since the epoch)"
//	@Success		200		{object}	models.KeyTrack
//	@Failure		400		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Router			/api/raid/pulls/{start}/track [get]
func (r Raid) Track(c *gin.Context) {
	start, ok := startParam(c)
	if !ok {
		return
	}
	track, err := r.Pulls.RaidTrack(start)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, track)
	case errors.Is(err, service.ErrNoSuchPull):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "no track for that pull"})
	default:
		log.Printf("raid track: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "couldn't read the pull's track"})
	}
}

// Map godoc
//
//	@Summary		A raid room's map image (the game's map art, as Warcraft Logs' replay draws it)
//	@Description	Placed by the pull's maps (models.RaidMap): its left edge is maxY, its top maxX.
//	@Tags			raid
//	@Produce		image/png
//	@Param			uiMapID	path	int	true	"the room's map ID in the combat log"
//	@Success		200
//	@Failure		400	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		502	{object}	ErrorResponse	"Warcraft Logs failed"
//	@Router			/api/raid/maps/{uiMapID} [get]
func (r Raid) Map(c *gin.Context) {
	ids, ok := intParams(c, "uiMapID")
	if !ok {
		return
	}
	image, err := r.Maps.Image(c.Request.Context(), ids[0])
	switch {
	case err == nil:
		c.Header("Cache-Control", "max-age=604800")
		c.Data(http.StatusOK, "image/png", image)
	case errors.Is(err, service.ErrNoSuchMap):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	default:
		log.Printf("raid map: %v", err)
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: "Warcraft Logs failed; try again later"})
	}
}
