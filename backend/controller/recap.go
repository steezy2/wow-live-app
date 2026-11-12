package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"wow-live/backend/models"
)

// Recaps builds the Recap tab (service.Recaps): the periods to pick from, and a period's recap.
type Recaps interface {
	Periods(ctx context.Context) []models.RecapPeriod
	Recap(from, to int64) models.Recap
}

// RecapSpan is the period a recap covers, in ms since the epoch (to excluded).
type RecapSpan struct {
	From int64 `form:"from" binding:"min=0"`
	To   int64 `form:"to" binding:"required,gtfield=From"`
}

// RecapPeriods godoc
//
//	@Summary		The periods a recap covers
//	@Description	This month, last month, this Mythic+ season (from Raider.IO; left out when it can't be reached),
//	@Description	this year and all time, each with its span to pass to /api/recap.
//	@Tags			recap
//	@Produce		json
//	@Success		200	{array}	models.RecapPeriod
//	@Router			/api/recap/periods [get]
func (m MPlus) RecapPeriods(c *gin.Context) {
	c.JSON(http.StatusOK, m.Recaps.Periods(c.Request.Context()))
}

// Recap godoc
//
//	@Summary		A period's keys and raid pulls as a story
//	@Description	The keys run and timed, the highest timed level, the time in keys and the deaths; each dungeon's
//	@Description	runs; your character (whose game wrote most of the logs): specs, median DPS or HPS per dungeon and
//	@Description	what killed you; who you ran with most; a few facts, each opening its key; and the raid by boss.
//	@Description	Read from the saved keys and pulls only: no Warcraft Logs points.
//	@Tags			recap
//	@Produce		json
//	@Param			from	query		int	true	"the period's start, ms since the epoch"
//	@Param			to		query		int	true	"the period's end, ms since the epoch (excluded); after from"
//	@Success		200		{object}	models.Recap
//	@Failure		400		{object}	ErrorResponse
//	@Router			/api/recap [get]
func (m MPlus) Recap(c *gin.Context) {
	var span RecapSpan
	if err := c.ShouldBindQuery(&span); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send from and to (ms since the epoch), to after from"})
		return
	}
	c.JSON(http.StatusOK, m.Recaps.Recap(span.From, span.To))
}
