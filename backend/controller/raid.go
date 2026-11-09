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
	Pulls models.RaidSource
	Now   func() time.Time
}

// Live godoc
//
//	@Summary		The raid now: a pull on, and the last pull
//	@Description	Whether a raid pull is on (its boss and start), and the last pull that ended with its deaths,
//	@Description	mechanics, meters and cooldowns, compared with tonight's best and the best ever on the boss. The
//	@Description	page reads it again on each notice from /api/mplus/events.
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
//	@Success	200	{array}	models.RaidNight	"newest night first, each night's pulls in order"
//	@Router		/api/raid/nights [get]
func (r Raid) Nights(c *gin.Context) {
	c.JSON(http.StatusOK, r.Pulls.RaidNights())
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
