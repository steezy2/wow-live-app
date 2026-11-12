package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CharacterQuery picks the runs of one of the owner's characters ("Name-Realm" or a bare name, as /api/characters
// gives them; empty for every character).
type CharacterQuery struct {
	Character string `form:"character" binding:"max=64"`
}

// characterParam is the request's character query, or false after answering 400.
func characterParam(c *gin.Context) (string, bool) {
	var q CharacterQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "character is a name and realm of at most 64 characters"})
		return "", false
	}
	return q.Character, true
}

// Characters godoc
//
//	@Summary		The owner's characters
//	@Description	The characters whose game wrote the saved keys' and raid pulls' logs, most runs first, with their
//	@Description	class and realm; the one matching preferred (Settings' character) is marked.
//	@Tags			mplus
//	@Produce		json
//	@Param			preferred	query	string	false	"Settings' character, Name-Realm"
//	@Success		200	{array}	models.Character
//	@Router			/api/characters [get]
func (m MPlus) Characters(c *gin.Context) {
	c.JSON(http.StatusOK, m.Keys.Played(c.Query("preferred")))
}
