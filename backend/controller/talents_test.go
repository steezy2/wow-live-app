package controller

import (
	"context"
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

type fakeIcons struct{}

func (fakeIcons) Icon(_ context.Context, name string) ([]byte, error) {
	if name == "spell_a" {
		return []byte("jpeg"), nil
	}
	return nil, models.ErrNoSuchIcon
}

func TestIconServesAnImageAndRefusesAnUnknownOne(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/icons/:name", MPlus{Icons: fakeIcons{}}.Icon)
	for path, want := range map[string]int{"/icons/spell_a": 200, "/icons/other": 404} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != want || (want == 200 && w.Header().Get("Content-Type") != "image/jpeg") {
			t.Errorf("GET %s = %d %s, want %d", path, w.Code, w.Header().Get("Content-Type"), want)
		}
	}
}
