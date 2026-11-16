package service

import (
	"cmp"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"wow-live/backend/models"
)

// Limits on a route: its pulls and the characters in its name.
const (
	maxPulls    = 60
	maxNameLen  = 60
	routeIDSize = 16 // characters in an id made here
)

// drawingLimits are the limits on a route's drawings, served with each route so the editor keeps to them.
var drawingLimits = models.DrawingLimits{Drawings: 200, NoteText: 200, LinePoints: 500}

var (
	// ErrNoSuchRoute is returned for a route that isn't there.
	ErrNoSuchRoute = errors.New("no such route")
	// ErrInvalidRoute is returned (wrapped, its text says why) for a route or request the owner's input made invalid.
	ErrInvalidRoute = errors.New("invalid route")
)

var pullColor = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)

// defaultPullColor is the color an imported pull gets in place of one that isn't 6 hex digits: Mythic Dungeon Tools'
// first default pull color.
const defaultPullColor = "3eff3e"

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRoute, fmt.Sprintf(format, args...))
}

// Routes keeps the owner's planned routes through the season's dungeons: the rules for importing, editing, starring
// and exporting them. Facts is by map ID.
type Routes struct {
	Store models.RouteStore
	Facts map[int]models.DungeonFacts
	Now   func() time.Time
	// Saves reads the routes Mythic Dungeon Tools saved (mdt.SavedRoutes on the log folder); nil means none.
	Saves func() ([]models.MDTPreset, error)
	// Decode and Encode turn a Mythic Dungeon Tools string into a route and back (mdt.DecodeRoute, mdt.EncodeRoute).
	Decode func(string) (models.MDTPreset, error)
	Encode func(models.MDTPreset) (string, error)
	// Track is a key's dungeon and track by its start (LogTracker.RouteTrack).
	Track func(start int64) (int, models.KeyTrack, error)
	mu    sync.Mutex // one change at a time, since starring changes several files
}

// List is the routes of one dungeon (every dungeon's for mapID 0), newest first, without their pulls' enemies or
// their drawings.
func (r *Routes) List(mapID int) ([]models.RouteView, error) {
	routes, err := r.Store.List()
	if err != nil {
		return nil, fmt.Errorf("listing routes: %w", err)
	}
	views := []models.RouteView{}
	for _, route := range routes {
		if mapID != 0 && route.MapID != mapID {
			continue
		}
		view := r.view(route)
		view.Drawings = nil
		for i := range view.Pulls {
			view.Pulls[i].Enemies = nil
		}
		views = append(views, view)
	}
	return views, nil
}

// Get is one route with its forces; ErrNoSuchRoute when there's none.
func (r *Routes) Get(id string) (models.RouteView, error) {
	route, err := r.get(id)
	if err != nil {
		return models.RouteView{}, err
	}
	return r.view(route), nil
}

// Import saves the route in a Mythic Dungeon Tools string; see ImportPreset.
func (r *Routes) Import(code string) (models.RouteView, error) {
	preset, err := r.Decode(code)
	if err != nil {
		return models.RouteView{}, fmt.Errorf("%w: %w", ErrInvalidRoute, err)
	}
	return r.ImportPreset(preset)
}

// ImportPreset saves a Mythic Dungeon Tools preset as a route and counts in Dropped the enemies it pulls that the
// dungeon's map doesn't have or an earlier pull has, and in DroppedDrawings the drawings importDrawings can't take
// (kept for the export); a color that isn't 6 hex digits becomes defaultPullColor. Its uid
// becomes the id, so importing it again replaces the route and keeps its star when it's of the same dungeon; a
// dungeon's first route is starred. A dungeon outside the season's reference, or more than maxPulls pulls, is
// refused.
func (r *Routes) ImportPreset(p models.MDTPreset) (models.RouteView, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	mapID, ok := r.mapOf(p.DungeonIdx)
	if !ok {
		return models.RouteView{}, invalidf("that route's dungeon isn't in this season")
	}
	known := r.known(mapID)
	route := models.Route{ID: p.UID, Name: clip(strings.TrimSpace(p.Name)), MapID: mapID, MDT: p.Raw,
		Pulls: make([]models.RoutePull, len(p.Pulls))}
	if !models.ValidRouteID(route.ID) {
		route.ID = rand.Text()[:routeIDSize]
	}
	if route.Name == "" {
		route.Name = "Imported route"
	}
	dropped, used := 0, map[models.EnemyRef]bool{}
	for i, pull := range p.Pulls {
		route.Pulls[i] = models.RoutePull{Color: pull.Color, Enemies: []models.EnemyRef{}}
		if !pullColor.MatchString(pull.Color) {
			route.Pulls[i].Color = defaultPullColor
		}
		for _, index := range slices.Sorted(maps.Keys(pull.Enemies)) {
			for _, clone := range slices.Sorted(slices.Values(pull.Enemies[index])) {
				ref := models.EnemyRef{Index: index, Clone: clone}
				if _, ok := known[ref]; !ok || used[ref] {
					dropped++
					continue
				}
				used[ref] = true
				route.Pulls[i].Enemies = append(route.Pulls[i].Enemies, ref)
			}
		}
	}
	if err := r.checkPulls(mapID, route.Pulls); err != nil {
		return models.RouteView{}, err
	}
	route.Drawings, route.MDTDrawings = r.importDrawings(mapID, p.Drawings)
	if err := r.save(&route, true); err != nil {
		return models.RouteView{}, err
	}
	view := r.view(route)
	view.Dropped, view.DroppedDrawings = dropped, len(route.MDTDrawings)
	return view, nil
}

// mdtPinColor is the color an imported note or marker gets: MDT's pins have none, and white is its brush's default.
const mdtPinColor = "ffffff"

// importDrawings is a preset's drawings on the dungeon's floors, and those it can't take: on a floor without an
// MDTPlane, past drawingLimits.Drawings, or failing drawingProblem. Each goes on the floor of its sublevel whose
// enemies' box on MDT's map is nearest its first point; a pin with text is a note, one without a marker.
func (r *Routes) importDrawings(mapID int, mdts []models.MDTDrawing) (drawings []models.Drawing,
	kept []models.MDTDrawing) {
	floors := r.Facts[mapID].Floors
	for _, m := range mdts {
		if len(m.Points) == 0 {
			continue
		}
		f := -1
		for i, g := range floors {
			if g.MDT != nil && g.MDT.Sublevel == m.Sublevel &&
				(f < 0 || boxDistance(g.MDT.Box, m.Points[0]) < boxDistance(floors[f].MDT.Box, m.Points[0])) {
				f = i
			}
		}
		if f < 0 || len(drawings) == drawingLimits.Drawings {
			kept = append(kept, m)
			continue
		}
		d := models.Drawing{ID: fmt.Sprint("mdt", len(drawings)+1), Map: floors[f].UIMapID, Kind: models.DrawLine,
			Color: m.Color}
		switch {
		case m.Note && strings.TrimSpace(m.Text) == "":
			d.Kind, d.Color = models.DrawMarker, mdtPinColor
		case m.Note:
			d.Kind, d.Color, d.Text = models.DrawNote, mdtPinColor, m.Text
		}
		plane := floors[f].MDT
		for _, p := range m.Points {
			x, y := ksgToLog(floors[f], plane.Scale*p[1]+plane.Y, plane.Scale*p[0]+plane.X)
			d.Points = append(d.Points, [2]float64{x, y})
		}
		if drawingProblem(d, floors) != "" {
			kept = append(kept, m)
			continue
		}
		drawings = append(drawings, d)
	}
	return drawings, kept
}

// boxDistance is how far a point is from a box (min x, min y, max x, max y): 0 inside it.
func boxDistance(b [4]float64, p [2]float64) float64 {
	return math.Hypot(max(b[0]-p[0], 0, p[0]-b[2]), max(b[1]-p[1], 0, p[1]-b[3]))
}

// exportDrawings is a route's drawings on MDT's map, then the ones its import couldn't take, as they were. A drawing
// on a floor without an MDTPlane stays in the app; a marker is a pin without text.
func (r *Routes) exportDrawings(route models.Route) []models.MDTDrawing {
	floors := r.Facts[route.MapID].Floors
	var out []models.MDTDrawing
	for _, d := range route.Drawings {
		f := slices.IndexFunc(floors, func(f models.DungeonFloor) bool { return f.UIMapID == d.Map && f.MDT != nil })
		if f < 0 {
			continue
		}
		plane := floors[f].MDT
		m := models.MDTDrawing{Sublevel: plane.Sublevel, Note: d.Kind != models.DrawLine, Text: d.Text}
		if !m.Note {
			m.Color = d.Color
		}
		for _, p := range d.Points {
			lat, lng := logToKsg(floors[f], p[0], p[1])
			m.Points = append(m.Points, [2]float64{(lng - plane.X) / plane.Scale, (lat - plane.Y) / plane.Scale})
		}
		out = append(out, m)
	}
	return append(out, route.MDTDrawings...)
}

// Dungeons are the season's dungeons that have enemies on the map, by name.
func (r *Routes) Dungeons() []models.RouteDungeon {
	out := []models.RouteDungeon{}
	for mapID, f := range r.Facts {
		if len(f.Enemies) > 0 {
			out = append(out, models.RouteDungeon{MapID: mapID, Name: f.Name, ForcesTotal: f.Forces.Total})
		}
	}
	slices.SortFunc(out, func(a, b models.RouteDungeon) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

// Saved lists the routes in Mythic Dungeon Tools' saved variables that can be imported: those of the season's dungeons
// that have a uid (MDT writes the file on /reload or logout).
func (r *Routes) Saved() ([]models.SavedRoute, error) {
	presets, err := r.savedPresets()
	if err != nil {
		return nil, err
	}
	out := []models.SavedRoute{}
	for _, p := range presets {
		mapID, _ := r.mapOf(p.DungeonIdx)
		_, err := r.Store.Get(p.UID)
		out = append(out, models.SavedRoute{UID: p.UID, Name: p.Name, MapID: mapID, Dungeon: r.Facts[mapID].Name,
			Pulls: len(p.Pulls), Imported: err == nil})
	}
	return out, nil
}

// ImportSaved saves the route of Mythic Dungeon Tools' saved variables with this uid; see ImportPreset.
func (r *Routes) ImportSaved(uid string) (models.RouteView, error) {
	presets, err := r.savedPresets()
	if err != nil {
		return models.RouteView{}, err
	}
	if i := slices.IndexFunc(presets, func(p models.MDTPreset) bool { return p.UID == uid }); i >= 0 {
		return r.ImportPreset(presets[i])
	}
	return models.RouteView{}, ErrNoSuchRoute
}

// savedPresets are the saved routes of the season's dungeons that have a uid, MDT's unnamed "Default" named.
func (r *Routes) savedPresets() ([]models.MDTPreset, error) {
	if r.Saves == nil {
		return nil, nil
	}
	all, err := r.Saves()
	if err != nil {
		return nil, fmt.Errorf("reading MDT's saved routes: %w", err)
	}
	var out []models.MDTPreset
	for _, p := range all {
		if _, ok := r.mapOf(p.DungeonIdx); ok && p.UID != "" {
			p.Name = cmp.Or(p.Name, "Default")
			out = append(out, p)
		}
	}
	return out, nil
}

// Create saves an empty route for a dungeon of the season.
func (r *Routes) Create(mapID int, name string) (models.RouteView, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.Facts[mapID]; !ok {
		return models.RouteView{}, invalidf("that dungeon isn't in this season")
	}
	name, err := checkName(name)
	if err != nil {
		return models.RouteView{}, err
	}
	route := models.Route{ID: rand.Text()[:routeIDSize], Name: name, MapID: mapID, Pulls: []models.RoutePull{}}
	if err := r.save(&route, true); err != nil {
		return models.RouteView{}, err
	}
	return r.view(route), nil
}

// Update replaces a route's name, pulls and drawings. Every enemy must be on the dungeon's map and in one pull at
// most, a route has at most maxPulls pulls, and its drawings keep to checkDrawings.
func (r *Routes) Update(id string, u models.RouteUpdate) (models.RouteView, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	route, err := r.get(id)
	if err != nil {
		return models.RouteView{}, err
	}
	name, err := checkName(u.Name)
	if err != nil {
		return models.RouteView{}, err
	}
	if err := r.checkPulls(route.MapID, u.Pulls); err != nil {
		return models.RouteView{}, err
	}
	if err := r.checkDrawings(route.MapID, u.Drawings); err != nil {
		return models.RouteView{}, err
	}
	route.Name, route.Pulls, route.Drawings = name, u.Pulls, u.Drawings
	if err := r.save(&route, false); err != nil {
		return models.RouteView{}, err
	}
	return r.view(route), nil
}

// Star makes a route the one its dungeon's live page and key comparison use, unstarring the dungeon's others.
func (r *Routes) Star(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	route, err := r.get(id)
	if err != nil {
		return err
	}
	return r.setStar(route.MapID, id)
}

// Delete removes a route; when it was starred, the dungeon's newest route left is starred.
func (r *Routes) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	route, err := r.get(id)
	if err != nil {
		return err
	}
	if err := r.Store.Delete(id); err != nil {
		return fmt.Errorf("deleting a route: %w", err)
	}
	if !route.Starred {
		return nil
	}
	others, err := r.ofDungeon(route.MapID)
	if err != nil || len(others) == 0 {
		return err
	}
	return r.setStar(route.MapID, others[0].ID)
}

// Export is the route as a string for Mythic Dungeon Tools' Import: the preset it came from with this route's name,
// pulls and drawings (exportDrawings').
func (r *Routes) Export(id string) (string, error) {
	route, err := r.get(id)
	if err != nil {
		return "", err
	}
	preset := models.MDTPreset{Name: route.Name, UID: route.ID, DungeonIdx: r.Facts[route.MapID].MDTIndex, Raw: route.MDT,
		Pulls: make([]models.MDTPull, len(route.Pulls)), Drawings: r.exportDrawings(route)}
	for i, pull := range route.Pulls {
		preset.Pulls[i] = models.MDTPull{Color: pull.Color, Enemies: map[int][]int{}}
		for _, e := range pull.Enemies {
			preset.Pulls[i].Enemies[e.Index] = append(preset.Pulls[i].Enemies[e.Index], e.Clone)
		}
	}
	code, err := r.Encode(preset)
	if err != nil {
		return "", fmt.Errorf("exporting a route: %w", err)
	}
	return code, nil
}

// get reads a route, mapping a missing file to ErrNoSuchRoute.
func (r *Routes) get(id string) (models.Route, error) {
	route, err := r.Store.Get(id)
	if errors.Is(err, fs.ErrNotExist) {
		return route, ErrNoSuchRoute
	}
	return route, err
}

// save stamps and writes a route. With first, a route that's new takes the star when its dungeon has no other route,
// and one that replaces another of its dungeon keeps the other's.
func (r *Routes) save(route *models.Route, first bool) error {
	if first {
		old, err := r.get(route.ID)
		switch {
		case err == nil && old.MapID == route.MapID:
			route.Starred = old.Starred
		case err == nil || errors.Is(err, ErrNoSuchRoute):
			others, err := r.ofDungeon(route.MapID)
			if err != nil {
				return err
			}
			route.Starred = len(others) == 0
		default:
			return err
		}
	}
	route.Updated = r.Now().UnixMilli()
	if err := r.Store.Save(*route); err != nil {
		return fmt.Errorf("saving a route: %w", err)
	}
	return nil
}

// ofDungeon is a dungeon's routes, newest first.
func (r *Routes) ofDungeon(mapID int) ([]models.Route, error) {
	routes, err := r.Store.List()
	if err != nil {
		return nil, fmt.Errorf("listing routes: %w", err)
	}
	return slices.DeleteFunc(routes, func(x models.Route) bool { return x.MapID != mapID }), nil
}

// setStar stars one route of a dungeon and unstars the rest.
func (r *Routes) setStar(mapID int, id string) error {
	routes, err := r.ofDungeon(mapID)
	if err != nil {
		return err
	}
	for _, route := range routes {
		if route.Starred == (route.ID == id) {
			continue
		}
		route.Starred = route.ID == id
		if err := r.Store.Save(route); err != nil {
			return fmt.Errorf("starring a route: %w", err)
		}
	}
	return nil
}

// mapOf is the map ID of the season's dungeon that Mythic Dungeon Tools knows as dungeonIdx.
func (r *Routes) mapOf(dungeonIdx int) (int, bool) {
	for mapID, f := range r.Facts {
		if f.MDTIndex == dungeonIdx {
			return mapID, true
		}
	}
	return 0, false
}

// known is a dungeon's enemies by reference.
func (r *Routes) known(mapID int) map[models.EnemyRef]models.RouteEnemy {
	enemies := r.Facts[mapID].Enemies
	m := make(map[models.EnemyRef]models.RouteEnemy, len(enemies))
	for _, e := range enemies {
		m[models.EnemyRef{Index: e.Index, Clone: e.Clone}] = e
	}
	return m
}

func (r *Routes) checkPulls(mapID int, pulls []models.RoutePull) error {
	if len(pulls) > maxPulls {
		return invalidf("%d pulls; at most %d", len(pulls), maxPulls)
	}
	known := r.known(mapID)
	used := map[models.EnemyRef]bool{}
	for i, pull := range pulls {
		if !pullColor.MatchString(pull.Color) {
			return invalidf("pull %d: color must be 6 hex digits", i+1)
		}
		for _, ref := range pull.Enemies {
			switch _, ok := known[ref]; {
			case !ok:
				return invalidf("pull %d: no enemy %d clone %d on this map", i+1, ref.Index, ref.Clone)
			case used[ref]:
				return invalidf("pull %d: enemy %d clone %d is in another pull", i+1, ref.Index, ref.Clone)
			}
			used[ref] = true
		}
	}
	return nil
}

// checkDrawings refuses more drawings than drawingLimits allows, two with one id, or one drawingProblem finds wrong:
// a bad id (ValidRouteID's rule), a kind other than a line (2 points or more), a marker or a note (1 point each), a
// color that isn't 6 hex digits, text on anything but a note (which needs some), a floor the dungeon doesn't have or
// a point that isn't on that floor.
func (r *Routes) checkDrawings(mapID int, drawings []models.Drawing) error {
	if len(drawings) > drawingLimits.Drawings {
		return invalidf("%d drawings; at most %d", len(drawings), drawingLimits.Drawings)
	}
	floors := r.Facts[mapID].Floors
	ids := map[string]bool{}
	for i, d := range drawings {
		if ids[d.ID] {
			return invalidf("drawing %d: its id is another's", i+1)
		}
		if why := drawingProblem(d, floors); why != "" {
			return invalidf("drawing %d: %s", i+1, why)
		}
		ids[d.ID] = true
	}
	return nil
}

// drawingProblem is what's wrong with a drawing on its own, "" when nothing is.
func drawingProblem(d models.Drawing, floors []models.DungeonFloor) string {
	points := len(d.Points)
	f := slices.IndexFunc(floors, func(f models.DungeonFloor) bool { return f.UIMapID == d.Map })
	switch {
	case !models.ValidRouteID(d.ID):
		return "a missing or bad id"
	case d.Kind != models.DrawLine && d.Kind != models.DrawMarker && d.Kind != models.DrawNote:
		return fmt.Sprintf("no kind %q", d.Kind)
	case !pullColor.MatchString(d.Color):
		return "color must be 6 hex digits"
	case d.Kind == models.DrawLine && (points < 2 || points > drawingLimits.LinePoints):
		return fmt.Sprintf("a line has 2 to %d points", drawingLimits.LinePoints)
	case d.Kind != models.DrawLine && points != 1:
		return fmt.Sprintf("a %s has one point", d.Kind)
	case d.Kind == models.DrawNote &&
		(strings.TrimSpace(d.Text) == "" || utf8.RuneCountInString(d.Text) > drawingLimits.NoteText):
		return fmt.Sprintf("a note has 1 to %d characters", drawingLimits.NoteText)
	case d.Kind != models.DrawNote && d.Text != "":
		return "only a note has text"
	case f < 0:
		return fmt.Sprintf("no floor %d in this dungeon", d.Map)
	}
	for _, p := range d.Points {
		if !onFloor(floors[f].RaidMap, p) {
			return "a point off the floor"
		}
	}
	return ""
}

// onFloor is whether a point in log yards is inside a floor's bounds (so never NaN or infinite), whose x is the log's
// -y and y the log's x, and whose min can be above its max (Keystone.guru's often are).
func onFloor(f models.RaidMap, p [2]float64) bool {
	between := func(v, a, b float64) bool { return v >= min(a, b) && v <= max(a, b) }
	return between(-p[1], f.MinX, f.MaxX) && between(p[0], f.MinY, f.MaxY)
}

// view is a route with its pulls' forces.
func (r *Routes) view(route models.Route) models.RouteView {
	known, total := r.known(route.MapID), r.Facts[route.MapID].Forces.Total
	pct := func(n int) float64 { return forcesPct(n, total) }
	v := models.RouteView{ID: route.ID, Name: route.Name, MapID: route.MapID, Starred: route.Starred,
		Drawings: route.Drawings, Limits: drawingLimits, Updated: route.Updated,
		Pulls: make([]models.RoutePullView, len(route.Pulls))}
	running := 0
	for i, pull := range route.Pulls {
		forces := 0
		for _, ref := range pull.Enemies {
			forces += known[ref].Count
		}
		running += forces
		v.Pulls[i] = models.RoutePullView{RoutePull: pull, Forces: forces, Total: running, Pct: pct(forces),
			TotalPct: pct(running)}
	}
	v.TotalPct = pct(running)
	return v
}

// forcesPct is n forces as a percent of a dungeon's total (0 without one).
func forcesPct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

// checkName is a route's name without its outer spaces, refused when that leaves none or more than maxNameLen
// characters.
func checkName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n == 0 || n > maxNameLen {
		return "", invalidf("the name must be 1 to %d characters", maxNameLen)
	}
	return name, nil
}

// clip cuts a name to maxNameLen characters.
func clip(name string) string {
	if runes := []rune(name); len(runes) > maxNameLen {
		return string(runes[:maxNameLen])
	}
	return name
}
