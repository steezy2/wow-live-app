package models

import "regexp"

var routeID = regexp.MustCompile(`^[A-Za-z0-9()]{1,32}$`)

// ValidRouteID is whether id can name a route: Mythic Dungeon Tools' uids look like this, and nothing in it can
// reach outside the routes folder.
func ValidRouteID(id string) bool { return routeID.MatchString(id) }

// EnemyRef names one enemy of a dungeon by its Mythic Dungeon Tools enemy and clone index (RouteEnemy's).
type EnemyRef struct {
	Index int `json:"index"`
	Clone int `json:"clone"`
}

// RoutePull is one pull of a route: its color ("ff3eff") and the enemies in it.
type RoutePull struct {
	Color   string     `json:"color"`
	Enemies []EnemyRef `json:"enemies,omitempty"` // left out of the route list
}

// Route is a planned route through a dungeon, as kept in its file. MDT is the Mythic Dungeon Tools preset it came
// from (CBOR), kept so an export still carries what the app doesn't edit; empty for a route made in the app.
type Route struct {
	ID      string      `json:"id"` // the preset's uid when it has one
	Name    string      `json:"name"`
	MapID   int         `json:"mapID"`
	Starred bool        `json:"starred"` // the dungeon's route the live page and key comparison use
	Pulls   []RoutePull `json:"pulls"`
	MDT     []byte      `json:"mdt,omitempty"`
	Updated int64       `json:"updated"` // ms since the epoch
}

// RoutePullView is a route's pull with its forces: what its enemies give, the running total to it, and both as a
// percent of the dungeon's total.
type RoutePullView struct {
	RoutePull
	Forces   int     `json:"forces"`
	Total    int     `json:"total"`
	Pct      float64 `json:"pct"`
	TotalPct float64 `json:"totalPct"`
}

// RouteView is what the routes endpoints return: the route without its MDT preset, its pulls' forces, and the whole
// route's forces percent.
type RouteView struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	MapID    int             `json:"mapID"`
	Starred  bool            `json:"starred"`
	Pulls    []RoutePullView `json:"pulls"`
	TotalPct float64         `json:"totalPct"`
	Updated  int64           `json:"updated"`
	Dropped  int             `json:"dropped,omitempty"` // an import's enemies that Mythic Dungeon Tools knows and this map doesn't
}

// RouteProgress is how a key went along a route: each route pull's progress in the route's order, the route pull the
// last kill on the route was in and the first one not done (both -1 when none), what the key killed off the route by
// key pull, the route pulls with nothing killed (for a running key, those not reached yet too), and every map enemy
// matched to a kill, for ticking them on the map.
type RouteProgress struct {
	Pulls   []RoutePullProgress `json:"pulls"`
	Current int                 `json:"current"`
	Next    int                 `json:"next"`
	Off     []RouteOff          `json:"off"`
	Skipped []int               `json:"skipped"`
	Kills   []EnemyRef          `json:"kills"`
}

// RoutePullProgress is how much of one route pull a key killed: its enemies killed of those planned, whether it's
// done (all killed, or enough of its forces), and the key pull (an index in the track's Pulls, -1 for none) that
// killed most of it with that pull's start on the key's clock.
type RoutePullProgress struct {
	Killed  int   `json:"killed"`
	Of      int   `json:"of"`
	Done    bool  `json:"done"`
	KeyPull int   `json:"keyPull"`
	ClockMS int64 `json:"clockMS"`
}

// RouteOff is what one key pull (an index in the track's Pulls, -1 before the first) killed off the route: the map's
// enemies in no route pull, the kills worth forces that no map enemy was in reach of (OffMap), and the forces of both,
// also as a percent of the dungeon's total. ClockMS is the key pull's start.
type RouteOff struct {
	KeyPull int        `json:"keyPull"`
	ClockMS int64      `json:"clockMS"`
	Enemies []EnemyRef `json:"enemies"`
	OffMap  int        `json:"offMap"`
	Forces  int        `json:"forces"`
	Pct     float64    `json:"pct"`
}

// RouteCompare is a route (its pulls with their enemies) next to how a key went along it. NoMobs is a key saved before
// mob positions were kept (Nov 11): only the route can be shown, until its log is read again.
type RouteCompare struct {
	Route RouteView `json:"route"`
	RouteProgress
	NoMobs bool `json:"noMobs"`
}

// RouteStep is a route's pull with its number from 1, as the live page names it.
type RouteStep struct {
	Number int `json:"number"`
	RoutePullView
}

// RouteLive is the dungeon's starred route followed by the running key: the pull the last kill on the route was in
// (nil before any), the first pull not done (nil once all are), and the forces percent the whole route plans.
type RouteLive struct {
	Name      string     `json:"name"`
	Current   *RouteStep `json:"current"`
	Next      *RouteStep `json:"next"`
	ForcesPct float64    `json:"forcesPct"`
}

// SavedRoute is a route in Mythic Dungeon Tools' saved variables, listed to import: Imported when a route with its
// uid is already kept.
type SavedRoute struct {
	UID      string `json:"uid"`
	Name     string `json:"name"`
	MapID    int    `json:"mapID"`
	Dungeon  string `json:"dungeon"`
	Pulls    int    `json:"pulls"`
	Imported bool   `json:"imported"`
}

// RouteDungeon is a dungeon of the season a route can be planned in.
type RouteDungeon struct {
	MapID       int    `json:"mapID"`
	Name        string `json:"name"`
	ForcesTotal int    `json:"forcesTotal"`
}

// RouteStore keeps routes under their ids. Get fails with fs.ErrNotExist for a route that isn't there.
type RouteStore interface {
	Save(route Route) error
	Get(id string) (Route, error)
	List() ([]Route, error)
	Delete(id string) error
}
