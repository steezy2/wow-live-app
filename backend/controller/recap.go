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
	Recap(from, to int64, character string) models.Recap
}

// RecapSpan is the period a recap covers, in ms since the epoch (to excluded), and whose runs.
type RecapSpan struct {
	From int64 `form:"from" binding:"min=0"`
	To   int64 `form:"to" binding:"required,gtfield=From"`
	CharacterQuery
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
//	@Summary		A period's keys and raid pulls as a story, Mythic+ and raid apart
//	@Description	Mythic+: the keys run and timed, the highest timed level, the time in keys, the party's deaths,
//	@Description	kicks, CC, dispels and priority damage per key (the party's and yours); each dungeon's runs and its
//	@Description	times at the highest level run; your specs, median DPS or HPS per dungeon and what killed you; who you
//	@Description	ran with most; a few facts, each opening its key. Raid: pulls, kills, time and deaths, and each boss.
//	@Description	One character's runs, or everyone's with the character who played most as "you". Read from the
//	@Description	saved keys and pulls only: no Warcraft Logs points.
//	@Tags			recap
//	@Produce		json
//	@Param			from	query		int	true	"the period's start, ms since the epoch"
//	@Param			to		query		int	true	"the period's end, ms since the epoch (excluded); after from"
//	@Param			character	query	string	false	"only this character's runs (an id from /api/characters)"
//	@Success		200		{object}	models.Recap
//	@Failure		400		{object}	ErrorResponse
//	@Router			/api/recap [get]
func (m MPlus) Recap(c *gin.Context) {
	var span RecapSpan
	if err := c.ShouldBindQuery(&span); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send from and to (ms since the epoch), to after from, and a character of at most 64 characters"})
		return
	}
	c.JSON(http.StatusOK, m.Recaps.Recap(span.From, span.To, span.Character))
}
