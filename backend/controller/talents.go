package controller

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"wow-live/backend/models"
)

// TalentTree godoc
//
//	@Summary	A spec's talent tree: its class, spec and hero talents with their places
//	@Tags		mplus
//	@Produce	json
//	@Param		specID	path		int	true	"the specialization ID (62 Arcane Mage)"
//	@Success	200		{object}	models.TalentTree
//	@Failure	400		{object}	ErrorResponse
//	@Failure	404		{object}	ErrorResponse
//	@Router		/api/talents/{specID} [get]
func (m MPlus) TalentTree(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("specID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "the spec ID is a number"})
		return
	}
	tree, ok := m.Talents[id]
	if !ok {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "no talent tree for that spec"})
		return
	}
	c.JSON(http.StatusOK, tree)
}

// Icons gives a spell icon's image by its Wowhead name (clients/wowhead).
type Icons interface {
	Icon(ctx context.Context, name string) ([]byte, error)
}

// Icon godoc
//
//	@Summary	A spell icon's image (Wowhead's medium JPEG), kept after the first request
//	@Tags		mplus
//	@Produce	image/jpeg
//	@Param		name	path	string	true	"the icon's name, from a talent entry (spell_nature_starfall)"
//	@Success	200
//	@Failure	404	{object}	ErrorResponse
//	@Failure	502	{object}	ErrorResponse	"Wowhead failed"
//	@Router		/api/icons/{name} [get]
func (m MPlus) Icon(c *gin.Context) {
	img, err := m.Icons.Icon(c.Request.Context(), c.Param("name"))
	switch {
	case err == nil:
		c.Header("Cache-Control", "max-age=604800")
		c.Data(http.StatusOK, "image/jpeg", img)
	case errors.Is(err, models.ErrNoSuchIcon):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "no such icon"})
	default:
		log.Printf("icon: %v", err)
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: "Wowhead failed; try again later"})
	}
}
