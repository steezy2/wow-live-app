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

// RouteStore keeps routes under their ids. Get fails with fs.ErrNotExist for a route that isn't there.
type RouteStore interface {
	Save(route Route) error
	Get(id string) (Route, error)
	List() ([]Route, error)
	Delete(id string) error
}
