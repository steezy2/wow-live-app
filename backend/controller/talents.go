package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
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
