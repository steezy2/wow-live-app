package service

import (
	"cmp"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"wow-live/backend/clients/mdt"
	"wow-live/backend/models"
)

// Limits on a route: its pulls and the characters in its name.
const (
	MaxPulls    = 60
	maxNameLen  = 60
	routeIDSize = 16 // characters in an id made here
)

var (
	// ErrNoSuchRoute is returned for a route that isn't there.
	ErrNoSuchRoute = errors.New("no such route")
	// ErrInvalidRoute is returned (wrapped, its text says why) for a route or request the owner's input made invalid.
	ErrInvalidRoute = errors.New("invalid route")
)

var pullColor = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)

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
	mu    sync.Mutex // one change at a time, since starring changes several files
}

// List is the routes of one dungeon (every dungeon's for mapID 0), newest first, without their pulls' enemies.
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
	preset, err := mdt.DecodeRoute(code)
	if err != nil {
		return models.RouteView{}, fmt.Errorf("%w: %w", ErrInvalidRoute, err)
	}
	return r.ImportPreset(preset)
}

// ImportPreset saves a Mythic Dungeon Tools preset as a route and counts in Dropped the enemies it pulls that the
// dungeon's map doesn't have. Its uid becomes the id, so importing it again replaces the route and keeps its star;
// a dungeon's first route is starred. A dungeon outside the season's reference is refused.
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
	dropped := 0
	for i, pull := range p.Pulls {
		route.Pulls[i] = models.RoutePull{Color: pull.Color, Enemies: []models.EnemyRef{}}
		for _, index := range slices.Sorted(maps.Keys(pull.Enemies)) {
			for _, clone := range slices.Sorted(slices.Values(pull.Enemies[index])) {
				if ref := (models.EnemyRef{Index: index, Clone: clone}); hasEnemy(known, ref) {
					route.Pulls[i].Enemies = append(route.Pulls[i].Enemies, ref)
				} else {
					dropped++
				}
			}
		}
	}
	if err := r.save(&route, true); err != nil {
		return models.RouteView{}, err
	}
	view := r.view(route)
	view.Dropped = dropped
	return view, nil
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
	if err := checkName(name); err != nil {
		return models.RouteView{}, err
	}
	route := models.Route{ID: rand.Text()[:routeIDSize], Name: name, MapID: mapID, Pulls: []models.RoutePull{}}
	if err := r.save(&route, true); err != nil {
		return models.RouteView{}, err
	}
	return r.view(route), nil
}

// Update replaces a route's name and pulls. Every enemy must be on the dungeon's map and in one pull at most, and a
// route has at most MaxPulls pulls.
func (r *Routes) Update(id, name string, pulls []models.RoutePull) (models.RouteView, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	route, err := r.get(id)
	if err != nil {
		return models.RouteView{}, err
	}
	if err := checkName(name); err != nil {
		return models.RouteView{}, err
	}
	if err := r.checkPulls(route.MapID, pulls); err != nil {
		return models.RouteView{}, err
	}
	route.Name, route.Pulls = name, pulls
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

// Export is the route as a string for Mythic Dungeon Tools' Import: the preset it came from with this route's name and
// pulls.
func (r *Routes) Export(id string) (string, error) {
	route, err := r.get(id)
	if err != nil {
		return "", err
	}
	preset := models.MDTPreset{Name: route.Name, UID: route.ID, DungeonIdx: r.Facts[route.MapID].MDTIndex, Raw: route.MDT,
		Pulls: make([]models.MDTPull, len(route.Pulls))}
	for i, pull := range route.Pulls {
		preset.Pulls[i] = models.MDTPull{Color: pull.Color, Enemies: map[int][]int{}}
		for _, e := range pull.Enemies {
			preset.Pulls[i].Enemies[e.Index] = append(preset.Pulls[i].Enemies[e.Index], e.Clone)
		}
	}
	code, err := mdt.EncodeRoute(preset)
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
// and one that replaces another keeps the other's.
func (r *Routes) save(route *models.Route, first bool) error {
	if first {
		old, err := r.get(route.ID)
		switch {
		case err == nil:
			route.Starred = old.Starred
		case errors.Is(err, ErrNoSuchRoute):
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
	if len(pulls) > MaxPulls {
		return invalidf("%d pulls; at most %d", len(pulls), MaxPulls)
	}
	known := r.known(mapID)
	used := map[models.EnemyRef]bool{}
	for i, pull := range pulls {
		if !pullColor.MatchString(pull.Color) {
			return invalidf("pull %d: color must be 6 hex digits", i+1)
		}
		for _, ref := range pull.Enemies {
			switch {
			case !hasEnemy(known, ref):
				return invalidf("pull %d: no enemy %d clone %d on this map", i+1, ref.Index, ref.Clone)
			case used[ref]:
				return invalidf("pull %d: enemy %d clone %d is in another pull", i+1, ref.Index, ref.Clone)
			}
			used[ref] = true
		}
	}
	return nil
}

// view is a route with its pulls' forces.
func (r *Routes) view(route models.Route) models.RouteView {
	known, total := r.known(route.MapID), r.Facts[route.MapID].Forces.Total
	pct := func(n int) float64 {
		if total == 0 {
			return 0
		}
		return float64(n) / float64(total) * 100
	}
	v := models.RouteView{ID: route.ID, Name: route.Name, MapID: route.MapID, Starred: route.Starred,
		Updated: route.Updated, Pulls: make([]models.RoutePullView, len(route.Pulls))}
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

func hasEnemy(known map[models.EnemyRef]models.RouteEnemy, ref models.EnemyRef) bool {
	_, ok := known[ref]
	return ok
}

func checkName(name string) error {
	if n := utf8.RuneCountInString(name); n == 0 || n > maxNameLen {
		return invalidf("the name must be 1 to %d characters", maxNameLen)
	}
	return nil
}

// clip cuts a name to maxNameLen characters.
func clip(name string) string {
	if runes := []rune(name); len(runes) > maxNameLen {
		return string(runes[:maxNameLen])
	}
	return name
}
