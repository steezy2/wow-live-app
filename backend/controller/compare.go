package controller

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"wow-live/backend/models"
	"wow-live/backend/service"
)

// Compares gives a player's review of a finished key or raid pull: Cached only reads one, Run makes one if needed
// (spending Warcraft Logs points).
type Compares interface {
	Cached(kind models.FightKind, start int64, player string) (models.PlayerCompare, error)
	Run(ctx context.Context, kind models.FightKind, start int64, player string) (models.PlayerCompare, error)
}

// PlayerCompare godoc
//
//	@Summary	A player's review of a finished key against Warcraft Logs' top logs and their own runs, if one was made
//	@Tags		mplus
//	@Produce	json
//	@Param		start	path		int		true	"the key's start (ms since the epoch)"
//	@Param		player	path		string	true	"the player's name in the key"
//	@Success	200		{object}	models.PlayerCompare
//	@Failure	400		{object}	ErrorResponse
//	@Failure	404		{object}	ErrorResponse
//	@Failure	503		{object}	ErrorResponse	"no Warcraft Logs credentials"
//	@Router		/api/mplus/keys/{start}/players/{player}/compare [get]
func (m MPlus) PlayerCompare(c *gin.Context) { compare(c, m.Compares, models.FightMPlus, false) }

// RunPlayerCompare godoc
//
//	@Summary	Review a player of a finished key against Warcraft Logs' top logs and their own runs (spends points)
//	@Tags		mplus
//	@Produce	json
//	@Param		start	path		int		true	"the key's start (ms since the epoch)"
//	@Param		player	path		string	true	"the player's name in the key"
//	@Success	200		{object}	models.PlayerCompare
//	@Failure	400		{object}	ErrorResponse
//	@Failure	404		{object}	ErrorResponse
//	@Failure	409		{object}	ErrorResponse	"the key is still running"
//	@Failure	422		{object}	ErrorResponse	"no top log could be read to compare with"
//	@Failure	429		{object}	ErrorResponse	"too few Warcraft Logs points left this hour"
//	@Failure	502		{object}	ErrorResponse	"Warcraft Logs failed"
//	@Failure	503		{object}	ErrorResponse	"no Warcraft Logs credentials"
//	@Router		/api/mplus/keys/{start}/players/{player}/compare [post]
func (m MPlus) RunPlayerCompare(c *gin.Context) { compare(c, m.Compares, models.FightMPlus, true) }

// PlayerCompare godoc
//
//	@Summary	A raider's review of a raid pull against Warcraft Logs' top kills and their own pulls, if one was made
//	@Tags		raid
//	@Produce	json
//	@Param		start	path		int		true	"the pull's start (ms since the epoch)"
//	@Param		player	path		string	true	"the raider's name in the pull"
//	@Success	200		{object}	models.PlayerCompare
//	@Failure	400		{object}	ErrorResponse
//	@Failure	404		{object}	ErrorResponse
//	@Failure	503		{object}	ErrorResponse	"no Warcraft Logs credentials"
//	@Router		/api/raid/pulls/{start}/players/{player}/compare [get]
func (r *Raid) PlayerCompare(c *gin.Context) { compare(c, r.Compares, models.FightRaid, false) }

// RunPlayerCompare godoc
//
//	@Summary	Review a raider of a raid pull against Warcraft Logs' top kills and their own pulls (spends points)
//	@Tags		raid
//	@Produce	json
//	@Param		start	path		int		true	"the pull's start (ms since the epoch)"
//	@Param		player	path		string	true	"the raider's name in the pull"
//	@Success	200		{object}	models.PlayerCompare
//	@Failure	400		{object}	ErrorResponse
//	@Failure	404		{object}	ErrorResponse
//	@Failure	422		{object}	ErrorResponse	"no top log could be read to compare with"
//	@Failure	429		{object}	ErrorResponse	"too few Warcraft Logs points left this hour"
//	@Failure	502		{object}	ErrorResponse	"Warcraft Logs failed"
//	@Failure	503		{object}	ErrorResponse	"no Warcraft Logs credentials"
//	@Router		/api/raid/pulls/{start}/players/{player}/compare [post]
func (r *Raid) RunPlayerCompare(c *gin.Context) { compare(c, r.Compares, models.FightRaid, true) }

// compare answers with a player's review (making one when run), mapping the comparer's errors to statuses (502 for
// Warcraft Logs failing, without its text).
func compare(c *gin.Context, compares Compares, kind models.FightKind, run bool) {
	start, ok := startParam(c)
	if !ok {
		return
	}
	if compares == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "no Warcraft Logs credentials in .env"})
		return
	}
	var review models.PlayerCompare
	var err error
	if run {
		review, err = compares.Run(c.Request.Context(), kind, start, c.Param("player"))
	} else {
		review, err = compares.Cached(kind, start, c.Param("player"))
	}
	switch {
	case err == nil:
		c.JSON(http.StatusOK, review)
	case errors.Is(err, service.ErrNoSuchKey), errors.Is(err, service.ErrNoSuchPull), errors.Is(err, service.ErrNoSuchPlayer):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrKeyRunning):
		c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrNoTopLogs):
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrNoPoints):
		c.JSON(http.StatusTooManyRequests, ErrorResponse{Error: err.Error()})
	case errors.Is(err, models.ErrNoCredentials), errors.Is(err, models.ErrBadCredentials):
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "no working Warcraft Logs keys: add them in Settings"})
	default:
		log.Printf("player review: %v", err)
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: "Warcraft Logs failed; try again later"})
	}
}
