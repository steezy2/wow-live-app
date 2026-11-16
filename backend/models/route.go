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

// The kinds of drawing on a route's map.
const (
	DrawLine   = "line"
	DrawMarker = "marker"
	DrawNote   = "note"
)

// Drawing is something drawn on a route's floor Map (its UIMapID): a freehand line through its points, or a marker or
// a note (its Text) at its one point. Points are in log yards as (x, y) like RouteEnemy's; Color is 6 hex digits.
type Drawing struct {
	ID     string       `json:"id"`
	Map    int          `json:"map"`
	Kind   string       `json:"kind" enums:"line,marker,note"`
	Color  string       `json:"color"`
	Points [][2]float64 `json:"points"`
	Text   string       `json:"text,omitempty"`
}

// DrawingLimits are the most drawings a route has, characters a note has and points a line has.
type DrawingLimits struct {
	Drawings   int `json:"drawings"`
	NoteText   int `json:"noteText"`
	LinePoints int `json:"linePoints"`
}

// RouteUpdate is what replaces a route's name, pulls and drawings.
type RouteUpdate struct {
	Name     string      `json:"name"`
	Pulls    []RoutePull `json:"pulls"`
	Drawings []Drawing   `json:"drawings"`
}

// Route is a planned route through a dungeon, as kept in its file. MDT is the Mythic Dungeon Tools preset it came
// from (CBOR, MDTPreset.Raw), MDTDrawings the preset's drawings the import couldn't take and MDTImported the ones it
// took as MDT wrote them, by drawing id, all kept so an export still carries what the app doesn't edit; empty for a
// route made in the app.
type Route struct {
	ID          string                `json:"id"` // the preset's uid when it has one
	Name        string                `json:"name"`
	MapID       int                   `json:"mapID"`
	Starred     bool                  `json:"starred"` // the dungeon's route the live page and key comparison use
	Pulls       []RoutePull           `json:"pulls"`
	Drawings    []Drawing             `json:"drawings,omitempty"` // absent in a route saved before drawings
	MDT         []byte                `json:"mdt,omitempty"`
	MDTDrawings []MDTDrawing          `json:"mdtDrawings,omitempty"`
	MDTImported map[string]MDTDrawing `json:"mdtImported,omitempty"`
	Updated     int64                 `json:"updated"` // ms since the epoch
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

// RouteView is what the routes endpoints return: the route without its MDT preset, its pulls' forces, the whole
// route's forces percent, its drawings and the limits on them.
type RouteView struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	MapID    int             `json:"mapID"`
	Starred  bool            `json:"starred"`
	Pulls    []RoutePullView `json:"pulls"`
	Drawings []Drawing       `json:"drawings,omitempty"` // left out of the route list
	Limits   DrawingLimits   `json:"limits"`
	TotalPct float64         `json:"totalPct"`
	Updated  int64           `json:"updated"`
	// an import's enemies that Mythic Dungeon Tools knows and this map doesn't
	Dropped int `json:"dropped,omitempty"`
	// DroppedDrawings counts an import's drawings the app can't show (kept for the export).
	DroppedDrawings int `json:"droppedDrawings,omitempty"`
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
	// Last is the highest route pull with a kill (-1 for none); Missed the unfinished pulls before it, and Behind how
	// far the key's forces trail the route's plan at Last (nil unless by a worthwhile margin).
	Last   int          `json:"last"`
	Missed []RouteMiss  `json:"missed"`
	Behind *RouteBehind `json:"behind,omitempty"`
}

// The states of a route pull on the map: done, missed (unfinished though a later pull was reached), next (one of the
// first unfinished pulls ahead, since the log shows nothing inside a fight) and later.
const (
	PullDone   = "done"
	PullMissed = "missed"
	PullNext   = "next"
	PullLater  = "later"
)

// RouteMiss is a missed route pull (an index): the enemies of it not killed, their forces and the forces as a percent
// of the dungeon's total.
type RouteMiss struct {
	Pull    int        `json:"pull"`
	Enemies []EnemyRef `json:"enemies"`
	Forces  int        `json:"forces"`
	Pct     float64    `json:"pct"`
}

// RouteBehind is the forces percent the route planned up to the last pull reached next to the percent the key has.
type RouteBehind struct {
	PlannedPct float64 `json:"plannedPct"`
	ActualPct  float64 `json:"actualPct"`
}

// RoutePullProgress is how much of one route pull a key killed: its enemies killed of those planned, whether it's
// done (all killed, or enough of its forces), and the key pull (an index in the track's Pulls, -1 for none) that
// killed most of it with that pull's start on the key's clock.
type RoutePullProgress struct {
	Killed  int    `json:"killed"`
	Of      int    `json:"of"`
	Done    bool   `json:"done"`
	KeyPull int    `json:"keyPull"`
	ClockMS int64  `json:"clockMS"`
	State   string `json:"state"` // PullDone, PullMissed, PullNext or PullLater
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
	Missed    int        `json:"missed"` // pulls missed so far
	Behind    bool       `json:"behind"` // forces trail the plan
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
