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

// Spells explains an ability by its name (service.Spells).
type Spells interface {
	Info(ctx context.Context, name string) (models.SpellInfo, error)
}

// SpellQuery is the ability to explain, as the combat log names it.
type SpellQuery struct {
	Name string `form:"name" binding:"required,max=100"`
}

// Spell godoc
//
//	@Summary		What an ability does
//	@Description	The ability's description and cooldown, from Wowhead's tooltip for the spell ID the combat log gave
//	@Description	its name (cached a week). 404 for an ability the log hasn't shown since the app started, 502 when
//	@Description	Wowhead can't be reached.
//	@Tags			spells
//	@Produce		json
//	@Param			name	query		string	true	"the ability's name as the combat log shows it"
//	@Success		200		{object}	models.SpellInfo
//	@Failure		400		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		502		{object}	ErrorResponse
//	@Router			/api/spells [get]
func (m MPlus) Spell(c *gin.Context) {
	var q SpellQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send the ability's name, at most 100 characters"})
		return
	}
	info, err := m.Spells.Info(c.Request.Context(), q.Name)
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
