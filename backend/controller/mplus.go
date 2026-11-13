package controller

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"wow-live/backend/models"
	"wow-live/backend/service"
)

// MPlus serves Mythic+ keys read from the combat log. Now is the clock the running key is read at (the wall clock
// live, the replay's clock when replaying a log).
type MPlus struct {
	Keys    models.KeySource
	Tiles   Tiles
	Reviews Reviews // nil without Warcraft Logs credentials
	Changes Changes
	Recaps  Recaps
	Planner Planner
	Now     func() time.Time
}

// Planner keeps the owner's planned routes (service.Routes).
type Planner interface {
	List(mapID int) ([]models.RouteView, error)
	Get(id string) (models.RouteView, error)
	Import(code string) (models.RouteView, error)
	ImportSaved(uid string) (models.RouteView, error)
	Saved() ([]models.SavedRoute, error)
	Create(mapID int, name string) (models.RouteView, error)
	Update(id, name string, pulls []models.RoutePull) (models.RouteView, error)
	Star(id string) error
	Delete(id string) error
	Export(id string) (string, error)
	Compare(mapID int, id string, track models.KeyTrack) (models.RouteCompare, error)
	Live(mapID int, track models.KeyTrack) (*models.RouteLive, error)
}

// Changes gives the keys' new versions as they change (service.Changes).
type Changes interface {
	Subscribe() (<-chan int64, func())
}

// keepAlive is how often an idle event stream gets a comment line, so nothing between closes it.
const keepAlive = 15 * time.Second

// Tiles gives a dungeon floor's map tiles (by the combat log's map IDs).
type Tiles interface {
	Tile(ctx context.Context, mapID, uiMapID, x, y int) ([]byte, error)
}

// Reviews gives finished keys' reviews: Cached only reads one, Review makes one if needed (spending points).
type Reviews interface {
	Cached(start int64) (models.KeyReview, error)
	Review(ctx context.Context, start int64) (models.KeyReview, error)
}

// Live godoc
//
//	@Summary		The running Mythic+ key, or the last one
//	@Description	The key being played (its clock read now, death penalty included) or, between keys, the last one
//	@Description	that ended. The page reads it again on each notice from /api/mplus/events. A running key carries
//	@Description	its dungeon's starred route followed so far (route), when the dungeon has one.
//	@Tags			mplus
//	@Produce		json
//	@Success		200	{object}	models.KeyView
//	@Success		204	"No key in the combat log yet"
//	@Router			/api/mplus/live [get]
func (m MPlus) Live(c *gin.Context) {
	key, ok := m.Keys.Latest(m.Now().UnixMilli())
	if !ok {
		c.Status(http.StatusNoContent)
		return
	}
	if key.Status == "running" {
		_, track, err := m.Keys.RouteTrack(key.Start)
		if err == nil {
			key.Route, err = m.Planner.Live(key.MapID, track)
		}
		if err != nil {
			log.Printf("live route: %v", err) // the live key is still worth showing without it
		}
	}
	c.JSON(http.StatusOK, key)
}

// KeyRoute godoc
//
//	@Summary		A key next to a route of its dungeon: which planned pulls were done, skipped or added
//	@Description	Each dead mob is matched to the nearest map enemy of its NPC on its floor within 20 yards, in
//	@Description	death order. A pull is done when all of it died or 80% of its forces. noMobs is a key saved before
//	@Description	mob positions were kept: only the route can be shown until its log is read again.
//	@Tags			mplus
//	@Produce		json
//	@Param			start	path		int		true	"the key's start (ms since the epoch): the running key's, the last one's or a saved one's"
//	@Param			id		query		string	false	"the route's id; the dungeon's starred route when left out"
//	@Success		200		{object}	models.RouteCompare
//	@Failure		400		{object}	ErrorResponse	"a bad start or id, or a route of another dungeon"
//	@Failure		404		{object}	ErrorResponse	"no such key, or no such route (the dungeon has none)"
//	@Router			/api/mplus/keys/{start}/route [get]
func (m MPlus) KeyRoute(c *gin.Context) {
	start, ok := startParam(c)
	if !ok {
		return
	}
	id := c.Query("id")
	if id != "" && !checkRouteID(c, id) {
		return
	}
	mapID, track, err := m.Keys.RouteTrack(start)
	if err != nil {
		respondError(c, err)
		return
	}
	compare, err := m.Planner.Compare(mapID, id, track)
	respondRoute(c, http.StatusOK, compare, err)
}

// Abandon godoc
//
//	@Summary		End the running key as abandoned
//	@Description	For a party that left a key: the game logs no end for it, and writes the line that says they left
//	@Description	with its next batch, which can be minutes later. The key ends at its last line and is saved and
//	@Description	reviewed like any other; if the log goes on in it for 2 more minutes, it runs again.
//	@Tags			mplus
//	@Success		204
//	@Failure		409	{object}	ErrorResponse	"no key is running"
//	@Router			/api/mplus/live/abandon [post]
func (m MPlus) Abandon(c *gin.Context) {
	if !m.Keys.Abandon() {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "no key is running"})
		return
	}
	c.Status(http.StatusNoContent)
}

// Events godoc
//
//	@Summary		Notices that the keys changed
//	@Description	Server-sent events: a "change" event with the keys' new version whenever the running key or the
//	@Description	saved keys change, at most one every 250 ms, and a comment line every 15 s while nothing does. The
//	@Description	page then reads the endpoints it shows again.
//	@Tags			mplus
//	@Produce		text/event-stream
//	@Success		200	{object}	ChangeEvent
//	@Router			/api/mplus/events [get]
func (m MPlus) Events(c *gin.Context) {
	versions, stop := m.Changes.Subscribe()
	defer stop()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Status(http.StatusOK)
	c.Writer.Flush() // the headers now, so the page sees the stream open before the first change
	c.Stream(func(io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case v := <-versions:
			c.SSEvent("change", ChangeEvent{Version: v})
		case <-time.After(keepAlive):
			_, err := c.Writer.WriteString(": keep-alive\n\n")
			return err == nil
		}
		return true
	})
}

// ChangeEvent is the data of one "change" event.
type ChangeEvent struct {
	Version int64 `json:"version"`
}

// Saved godoc
//
//	@Summary	The saved Mythic+ keys, newest first
//	@Tags		mplus
//	@Produce	json
//	@Param		character	query		string	false	"only this character's keys (an id from /api/characters)"
//	@Success	200			{array}		models.KeySummary
//	@Failure	400			{object}	ErrorResponse
//	@Router		/api/mplus/keys [get]
func (m MPlus) Saved(c *gin.Context) {
	if character, ok := characterParam(c); ok {
		c.JSON(http.StatusOK, m.Keys.Saved(character))
	}
}

// SavedKey godoc
//
//	@Summary	One saved Mythic+ key
//	@Tags		mplus
//	@Produce	json
//	@Param		start	path		int	true	"the key's start (ms since the epoch)"
//	@Success	200		{object}	models.KeyView
//	@Failure	400		{object}	ErrorResponse
//	@Failure	404		{object}	ErrorResponse
//	@Router		/api/mplus/keys/{start} [get]
func (m MPlus) SavedKey(c *gin.Context) {
	start, ok := startParam(c)
	if !ok {
		return
	}
	key, err := m.Keys.SavedKey(start)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, key)
}

// Track godoc
//
//	@Summary	Where a key's party went and its mobs died, for the map
//	@Tags		mplus
//	@Produce	json
//	@Param		start	path		int	true	"the key's start (ms since the epoch): the running key's, the last one's or a saved one's"
//	@Success	200		{object}	models.KeyTrack
//	@Failure	400		{object}	ErrorResponse
//	@Failure	404		{object}	ErrorResponse
//	@Router		/api/mplus/keys/{start}/track [get]
func (m MPlus) Track(c *gin.Context) {
	start, ok := startParam(c)
	if !ok {
		return
	}
	track, err := m.Keys.Track(start)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, track)
}

// DungeonMap godoc
//
//	@Summary	A dungeon's map: its floors with Keystone.guru tiles, and where parties have walked in its saved runs
//	@Tags		mplus
//	@Produce	json
//	@Param		mapID	path		int	true	"the dungeon's map ID (challenge mode ID)"
//	@Success	200		{object}	models.DungeonMap
//	@Failure	400		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/mplus/dungeons/{mapID}/map [get]
func (m MPlus) DungeonMap(c *gin.Context) {
	ids, ok := intParams(c, "mapID")
	if !ok {
		return
	}
	dungeon, err := m.Keys.DungeonMap(ids[0])
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, dungeon)
}

// Tile godoc
//
//	@Summary	One map tile of a dungeon floor (384 by 256 WebP, Keystone.guru's zoom 2: 4 by 4 tiles a floor)
//	@Tags		mplus
//	@Produce	image/webp
//	@Param		mapID	path	int	true	"the dungeon's map ID (challenge mode ID)"
//	@Param		uiMapID	path	int	true	"the floor's map ID in the combat log"
//	@Param		x		path	int	true	"tile column, 0 to 3"
//	@Param		y		path	int	true	"tile row, 0 to 3"
//	@Success	200
//	@Failure	400	{object}	ErrorResponse
//	@Failure	404	{object}	ErrorResponse
//	@Failure	502	{object}	ErrorResponse	"Keystone.guru failed"
//	@Router		/api/mplus/dungeons/{mapID}/floors/{uiMapID}/tiles/{x}/{y} [get]
func (m MPlus) Tile(c *gin.Context) {
	ids, ok := intParams(c, "mapID", "uiMapID", "x", "y")
	if !ok {
		return
	}
	tile, err := m.Tiles.Tile(c.Request.Context(), ids[0], ids[1], ids[2], ids[3])
	switch {
	case err == nil:
		c.Header("Cache-Control", "max-age=604800")
		c.Data(http.StatusOK, "image/webp", tile)
	case errors.Is(err, service.ErrNoSuchKey):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "no map tile there"})
	default:
		log.Printf("map tile: %v", err)
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: "Keystone.guru failed; try again later"})
	}
}

// boolQuery reads a boolean query parameter (false when absent), answering 400 when it isn't one.
func boolQuery(c *gin.Context, name string) (bool, bool) {
	v, err := strconv.ParseBool(c.DefaultQuery(name, "false"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: name + " must be true or false"})
	}
	return v, err == nil
}

// intParams reads whole-number path parameters, answering 400 when one isn't.
func intParams(c *gin.Context, names ...string) ([]int, bool) {
	ids := make([]int, len(names))
	for i, name := range names {
		v, err := strconv.Atoi(c.Param(name))
		if err != nil || v < 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: name + " must be a whole number"})
			return nil, false
		}
		ids[i] = v
	}
	return ids, true
}

// Review godoc
//
//	@Summary	A finished key's review, if one was made
//	@Tags		mplus
//	@Produce	json
//	@Param		start	path		int	true	"the key's start (ms since the epoch)"
//	@Success	200		{object}	models.KeyReview
//	@Failure	400		{object}	ErrorResponse
//	@Failure	404		{object}	ErrorResponse
//	@Failure	503		{object}	ErrorResponse	"no Warcraft Logs credentials"
//	@Router		/api/mplus/keys/{start}/review [get]
func (m MPlus) Review(c *gin.Context) {
	m.review(c, func(start int64) (models.KeyReview, error) { return m.Reviews.Cached(start) })
}

// MakeReview godoc
//
//	@Summary	Review a finished key against Warcraft Logs' top runs and the players' earlier runs (spends points)
//	@Tags		mplus
//	@Produce	json
//	@Param		start	path		int	true	"the key's start (ms since the epoch)"
//	@Success	200		{object}	models.KeyReview
//	@Failure	400		{object}	ErrorResponse
//	@Failure	404		{object}	ErrorResponse
//	@Failure	409		{object}	ErrorResponse	"the key is still running"
//	@Failure	429		{object}	ErrorResponse	"fewer than 100 Warcraft Logs points left this hour"
//	@Failure	502		{object}	ErrorResponse	"Warcraft Logs failed"
//	@Failure	503		{object}	ErrorResponse	"no Warcraft Logs credentials"
//	@Router		/api/mplus/keys/{start}/review [post]
func (m MPlus) MakeReview(c *gin.Context) {
	m.review(c, func(start int64) (models.KeyReview, error) { return m.Reviews.Review(c.Request.Context(), start) })
}

// review answers with a review, mapping the reviewer's errors to statuses (502 for Warcraft Logs failing, without
// its text).
func (m MPlus) review(c *gin.Context, get func(int64) (models.KeyReview, error)) {
	start, ok := startParam(c)
	if !ok {
		return
	}
	if m.Reviews == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "no Warcraft Logs credentials in .env"})
		return
	}
	review, err := get(start)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, review)
	case errors.Is(err, service.ErrNoSuchKey):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrKeyRunning):
		c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrNoPoints):
		c.JSON(http.StatusTooManyRequests, ErrorResponse{Error: err.Error()})
	case errors.Is(err, models.ErrNoCredentials), errors.Is(err, models.ErrBadCredentials):
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "no working Warcraft Logs keys: add them in Settings"})
	default:
		log.Printf("key review: %v", err)
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: "Warcraft Logs failed; try again later"})
	}
}

// DeleteSaved godoc
//
//	@Summary	Delete one saved Mythic+ key
//	@Tags		mplus
//	@Param		start	path	int	true	"the key's start (ms since the epoch)"
//	@Success	204
//	@Failure	400	{object}	ErrorResponse
//	@Failure	404	{object}	ErrorResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/mplus/keys/{start} [delete]
func (m MPlus) DeleteSaved(c *gin.Context) {
	if start, ok := startParam(c); ok {
		m.deleteSaved(c, start)
	}
}

// ClearSaved godoc
//
//	@Summary	Delete every saved Mythic+ key
//	@Tags		mplus
//	@Success	204
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/mplus/keys [delete]
func (m MPlus) ClearSaved(c *gin.Context) {
	m.deleteSaved(c, 0)
}

func (m MPlus) deleteSaved(c *gin.Context, start int64) {
	if err := m.Keys.DeleteSaved(start); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// startParam reads the :start path parameter (a key's or a raid pull's), answering 400 when it isn't a positive
// number.
func startParam(c *gin.Context) (int64, bool) {
	start, err := strconv.ParseInt(c.Param("start"), 10, 64)
	if err != nil || start <= 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "start must be a start time in ms"})
		return 0, false
	}
	return start, true
}

// respondError answers 404 for a key that isn't there, else 500 without the internal error text.
func respondError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrNoSuchKey) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}
	log.Printf("saved keys: %v", err)
	c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "couldn't read or change the saved keys"})
}

// RouteRequest is the body of a new route: a Mythic Dungeon Tools export string, the uid of a route in its saved
// variables, or a dungeon and a name for an empty one.
type RouteRequest struct {
	Code   string `json:"code"`
	MDTUID string `json:"mdtUID"`
	MapID  int    `json:"mapID"`
	Name   string `json:"name"`
}

// RouteUpdate is the body that replaces a route's name and pulls.
type RouteUpdate struct {
	Name  string             `json:"name"`
	Pulls []models.RoutePull `json:"pulls"`
}

// RouteCode is a route as a Mythic Dungeon Tools export string.
type RouteCode struct {
	Code string `json:"code"`
}

// MDTSaves godoc
//
//	@Summary		The routes saved in Mythic Dungeon Tools, to import
//	@Description	Mythic Dungeon Tools writes them on /reload or logout. An empty list when it has none.
//	@Tags			mplus
//	@Produce		json
//	@Success		200	{array}		models.SavedRoute
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/mplus/routes/mdt-saves [get]
func (m MPlus) MDTSaves(c *gin.Context) {
	saves, err := m.Planner.Saved()
	respondRoute(c, http.StatusOK, saves, err)
}

// Routes godoc
//
//	@Summary	The planned routes, newest first, without their pulls' enemies
//	@Tags		mplus
//	@Produce	json
//	@Param		mapID	query		int	false	"only this dungeon's routes (its map ID)"
//	@Success	200		{array}		models.RouteView
//	@Failure	400		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/mplus/routes [get]
func (m MPlus) Routes(c *gin.Context) {
	mapID, err := strconv.Atoi(c.DefaultQuery("mapID", "0"))
	if err != nil || mapID < 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "mapID must be a whole number"})
		return
	}
	routes, err := m.Planner.List(mapID)
	respondRoute(c, http.StatusOK, routes, err)
}

// Route godoc
//
//	@Summary	One planned route with its pulls' forces
//	@Tags		mplus
//	@Produce	json
//	@Param		id	path		string	true	"the route's id (letters, digits and brackets, up to 32)"
//	@Success	200	{object}	models.RouteView
//	@Failure	400	{object}	ErrorResponse
//	@Failure	404	{object}	ErrorResponse
//	@Router		/api/mplus/routes/{id} [get]
func (m MPlus) Route(c *gin.Context) {
	if id, ok := routeID(c); ok {
		route, err := m.Planner.Get(id)
		respondRoute(c, http.StatusOK, route, err)
	}
}

// AddRoute godoc
//
//	@Summary		Add a route: import a Mythic Dungeon Tools export string, or make an empty one
//	@Description	The string's uid becomes the route's id, so importing it again replaces that route. The reply's
//	@Description	dropped counts the enemies the string pulls that the dungeon's map doesn't have.
//	@Tags			mplus
//	@Accept			json
//	@Produce		json
//	@Param			route	body		RouteRequest	true	"code, mdtUID, or mapID and name"
//	@Success		201		{object}	models.RouteView
//	@Failure		400		{object}	ErrorResponse	"a bad string (the text says which step failed) or dungeon"
//	@Router			/api/mplus/routes [post]
func (m MPlus) AddRoute(c *gin.Context) {
	var req RouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send a code, or a mapID and a name"})
		return
	}
	var route models.RouteView
	var err error
	switch {
	case req.Code != "":
		route, err = m.Planner.Import(req.Code)
	case req.MDTUID != "":
		route, err = m.Planner.ImportSaved(req.MDTUID)
	default:
		route, err = m.Planner.Create(req.MapID, req.Name)
	}
	respondRoute(c, http.StatusCreated, route, err)
}

// UpdateRoute godoc
//
//	@Summary		Replace a route's name and pulls
//	@Description	Every enemy must be on the dungeon's map and in one pull at most; a route has at most 60 pulls and
//	@Description	a name of up to 60 characters.
//	@Tags			mplus
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string		true	"the route's id"
//	@Param			route	body		RouteUpdate	true	"name and pulls"
//	@Success		200		{object}	models.RouteView
//	@Failure		400		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Router			/api/mplus/routes/{id} [put]
func (m MPlus) UpdateRoute(c *gin.Context) {
	id, ok := routeID(c)
	if !ok {
		return
	}
	var req RouteUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "send a name and pulls"})
		return
	}
	route, err := m.Planner.Update(id, req.Name, req.Pulls)
	respondRoute(c, http.StatusOK, route, err)
}

// StarRoute godoc
//
//	@Summary	Make a route its dungeon's starred one (the live page and key comparison use it)
//	@Tags		mplus
//	@Param		id	path	string	true	"the route's id"
//	@Success	204
//	@Failure	400	{object}	ErrorResponse
//	@Failure	404	{object}	ErrorResponse
//	@Router		/api/mplus/routes/{id}/star [post]
func (m MPlus) StarRoute(c *gin.Context) {
	if id, ok := routeID(c); ok {
		respondRoute(c, http.StatusNoContent, nil, m.Planner.Star(id))
	}
}

// DeleteRoute godoc
//
//	@Summary	Delete a route; if it was starred, its dungeon's newest route is
//	@Tags		mplus
//	@Param		id	path	string	true	"the route's id"
//	@Success	204
//	@Failure	400	{object}	ErrorResponse
//	@Failure	404	{object}	ErrorResponse
//	@Router		/api/mplus/routes/{id} [delete]
func (m MPlus) DeleteRoute(c *gin.Context) {
	if id, ok := routeID(c); ok {
		respondRoute(c, http.StatusNoContent, nil, m.Planner.Delete(id))
	}
}

// ExportRoute godoc
//
//	@Summary	A route as a string for Mythic Dungeon Tools' Import
//	@Tags		mplus
//	@Produce	json
//	@Param		id	path		string	true	"the route's id"
//	@Success	200	{object}	RouteCode
//	@Failure	400	{object}	ErrorResponse
//	@Failure	404	{object}	ErrorResponse
//	@Router		/api/mplus/routes/{id}/export [get]
func (m MPlus) ExportRoute(c *gin.Context) {
	if id, ok := routeID(c); ok {
		code, err := m.Planner.Export(id)
		respondRoute(c, http.StatusOK, RouteCode{Code: code}, err)
	}
}

// routeID reads the :id path parameter, answering 400 when it can't name a route (checkRouteID).
func routeID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	return id, checkRouteID(c, id)
}

// checkRouteID is whether id can name a route, answering 400 when it can't, before anything builds a file path from it.
func checkRouteID(c *gin.Context, id string) bool {
	ok := models.ValidRouteID(id)
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "id must be 1 to 32 letters, digits or brackets"})
	}
	return ok
}

// respondRoute answers body with status, or the status of the planner's error: 404 for a route that isn't there, 400
// with the reason for one the request made invalid, else 500 without the internal error text.
func respondRoute(c *gin.Context, status int, body any, err error) {
	switch {
	case err == nil && body == nil:
		c.Status(status)
	case err == nil:
		c.JSON(status, body)
	case errors.Is(err, service.ErrNoSuchRoute):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	case errors.Is(err, service.ErrInvalidRoute):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	default:
		log.Printf("routes: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "couldn't read or change the routes"})
	}
}
