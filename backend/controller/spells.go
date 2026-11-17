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

// Spells explains an ability by its name or a spell by its ID (service.Spells).
type Spells interface {
	Info(ctx context.Context, name string) (models.SpellInfo, error)
	ReferenceSpell(ctx context.Context, id int) (models.SpellInfo, error)
}

// SpellQuery is the ability to explain, as the combat log names it, or a spell ID.
type SpellQuery struct {
	Name string `form:"name" binding:"required_without=ID,max=100"`
	ID   int    `form:"id" binding:"omitempty,min=1"`
}

// Spell godoc
//
//	@Summary		What an ability does
//	@Description	The ability's description and cooldown, from Wowhead's tooltip (cached a week) for the spell ID
//	@Description	given, else for the one the combat log gave its name. 404 for an ability the log hasn't shown since
//	@Description	the app started or a spell ID no dungeon mob casts, 502 when Wowhead can't be reached.
//	@Tags			spells
//	@Produce		json
//	@Param			name	query		string	false	"the ability's name as the combat log shows it (needed without id)"
//	@Param			id		query		int		false	"a spell ID (a mob's spell from /api/mplus/dungeons/{mapID}/mobs, or a talent's from /api/talents/{specID})"
//	@Success		200		{object}	models.SpellInfo
//	@Failure		400		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		502		{object}	ErrorResponse
//	@Router			/api/spells [get]
func (m MPlus) Spell(c *gin.Context) {
	var q SpellQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "send the ability's name, at most 100 characters, or a spell ID"})
		return
	}
	var info models.SpellInfo
	var err error
	if q.ID > 0 {
		info, err = m.Spells.ReferenceSpell(c.Request.Context(), q.ID)
	} else {
		info, err = m.Spells.Info(c.Request.Context(), q.Name)
	}
	switch {
	case errors.Is(err, service.ErrUnknownSpell):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrSpellDown):
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
	case err != nil:
		log.Print(err)
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: "Wowhead's tooltip can't be read right now"})
	default:
		c.JSON(http.StatusOK, info)
	}
}
