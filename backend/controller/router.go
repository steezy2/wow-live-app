// Package controller is the HTTP API (Gin) the site's pages call: it reads the request, asks the service, and
// writes JSON. The page files themselves are served from frontend/dist.
package controller

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Router is the site for hosts (LocalHosts, or the app's own): the API under /api (the raid overview's when raid
// isn't nil), the settings and Warcraft Logs keys when settings isn't nil (the desktop app), its spec at
// /swagger/index.html, and the built page from pages (frontend/dist, on disk or built into the app) for every other
// path, so the page's own routes load index.html. Only requests addressed to this PC by name are answered, so another
// website can't read the API by pointing its domain at 127.0.0.1 (DNS rebinding), and requests sent by another
// website's page are refused, so it can't spend points or delete keys through a plain form post.
func Router(mplus MPlus, raid *Raid, settings *SettingsAPI, pages fs.FS, hosts ...string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		if !slices.Contains(hosts, c.Request.Host) {
			c.AbortWithStatusJSON(http.StatusMisdirectedRequest, ErrorResponse{Error: "use http://localhost"})
			return
		}
		if origin := c.GetHeader("Origin"); origin != "" && origin != "http://"+c.Request.Host {
			c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{Error: "only this site's pages can call the API"})
		}
	})
	api := r.Group("/api")
	api.GET("/mplus/live", mplus.Live)
	api.POST("/mplus/live/abandon", mplus.Abandon)
	api.GET("/mplus/events", mplus.Events)
	api.GET("/mplus/keys", mplus.Saved)
	api.GET("/characters", mplus.Characters)
	api.DELETE("/mplus/keys", mplus.ClearSaved)
	api.GET("/mplus/keys/:start", mplus.SavedKey)
	api.DELETE("/mplus/keys/:start", mplus.DeleteSaved)
	api.GET("/mplus/keys/:start/track", mplus.Track)
	api.GET("/mplus/keys/:start/review", mplus.Review)
	api.GET("/mplus/keys/:start/route", mplus.KeyRoute)
	api.GET("/spells", mplus.Spell)
	api.GET("/recap/periods", mplus.RecapPeriods)
	api.GET("/recap", mplus.Recap)
	api.GET("/recap/top-times", mplus.RecapTopTimes)
	api.POST("/mplus/keys/:start/review", mplus.MakeReview)
	api.GET("/mplus/routes", mplus.Routes)
	api.POST("/mplus/routes", mplus.AddRoute)
	api.GET("/mplus/route-dungeons", mplus.RouteDungeons)
	api.GET("/mplus/routes/mdt-saves", mplus.MDTSaves)
	api.GET("/mplus/routes/:id", mplus.Route)
	api.PUT("/mplus/routes/:id", mplus.UpdateRoute)
	api.DELETE("/mplus/routes/:id", mplus.DeleteRoute)
	api.POST("/mplus/routes/:id/star", mplus.StarRoute)
	api.GET("/mplus/routes/:id/export", mplus.ExportRoute)
	api.GET("/mplus/keys/:start/players/:player/compare", mplus.PlayerCompare)
	api.POST("/mplus/keys/:start/players/:player/compare", mplus.RunPlayerCompare)
	api.GET("/mplus/dungeons/:mapID/map", mplus.DungeonMap)
	api.GET("/mplus/dungeons/:mapID/floors/:uiMapID/tiles/:x/:y", mplus.Tile)
	if raid != nil {
		api.GET("/raid/live", raid.Live)
		api.GET("/raid/nights", raid.Nights)
		api.GET("/raid/pulls/:start", raid.Pull)
		api.GET("/raid/pulls/:start/track", raid.Track)
		api.GET("/raid/pulls/:start/players/:player/compare", raid.PlayerCompare)
		api.POST("/raid/pulls/:start/players/:player/compare", raid.RunPlayerCompare)
		api.GET("/raid/maps/:uiMapID", raid.Map)
	}
	if settings != nil {
		api.GET("/settings", settings.Get)
		api.PUT("/settings", settings.Put)
		api.GET("/settings/warcraftlogs", settings.KeysStatus)
		api.PUT("/settings/warcraftlogs", settings.SaveKeys)
		api.POST("/settings/warcraftlogs/test", settings.TestKeys)
		api.DELETE("/settings/warcraftlogs", settings.RemoveKeys)
		api.GET("/settings/warcraftlogs/import", settings.ImportStatus)
		api.GET("/settings/warcraftlogs/parses", settings.CharacterParses)
		api.POST("/settings/warcraftlogs/import/plan", settings.FindFights)
		api.POST("/settings/warcraftlogs/import", settings.StartImport)
		api.DELETE("/settings/warcraftlogs/import", settings.StopImport)
		api.DELETE("/settings/warcraftlogs/imported", settings.RemoveImported)
		api.POST("/settings/read-keys", settings.ReadKeys)
		api.GET("/settings/logs", settings.LogFiles)
		api.POST("/settings/logs/archive", settings.ArchiveLogs)
		api.DELETE("/settings/logs/archive", settings.DeleteLogs)
		api.GET("/storage", settings.Storage)
	}
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "no such endpoint"})
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+c.Request.URL.Path), "/")
		if info, err := fs.Stat(pages, name); err != nil || info.IsDir() {
			name = "index.html"
		}
		c.Header("Content-Security-Policy", pagePolicy)
		http.ServeFileFS(c.Writer, c.Request, pages, name)
	})
	return r
}

// pagePolicy is the built page's Content-Security-Policy: its own scripts, styles (React sets inline style
// attributes), images (map tiles; canvas exports as data: and blob:) and API only, and no framing. Swagger UI, which
// runs inline scripts, doesn't get it.
const pagePolicy = "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; " +
	"connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

// LocalHosts are the names a page on this PC reaches port by.
func LocalHosts(port int) []string {
	return []string{fmt.Sprintf("localhost:%d", port), fmt.Sprintf("127.0.0.1:%d", port)}
}

// ErrorResponse is the body of every API error.
type ErrorResponse struct {
	Error string `json:"error"`
}
