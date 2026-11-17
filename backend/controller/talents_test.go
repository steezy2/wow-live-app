package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"wow-live/backend/models"
)

func TestTalentTreeServesASpecsTree(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/talents/:specID", MPlus{Talents: map[int]models.TalentTree{62: {SpecID: 62, Spec: "Arcane"}}}.TalentTree)
	for path, want := range map[string]int{"/talents/62": 200, "/talents/63": 404, "/talents/abc": 400} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != want {
			t.Errorf("GET %s = %d, want %d", path, w.Code, want)
		}
	}
}
