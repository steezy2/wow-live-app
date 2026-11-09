package models

import (
	"context"
)

// KeySource gives the running Mythic+ key read at now (ms since the epoch), or the last one that ended (false before
// any key), and the saved keys: listed newest first, read or deleted by start time (0 deletes them all); a key's
// track for the map by its start time, a dungeon's map (by map ID): its floors and where parties have walked, and the
// ready check before the next key (false outside a Mythic dungeon between keys).
type KeySource interface {
	Latest(now int64) (KeyView, bool)
	Saved() []KeySummary
	SavedKey(start int64) (KeyView, error)
	DeleteSaved(start int64) error
	Track(start int64) (KeyTrack, error)
	DungeonMap(mapID int) (DungeonMap, error)
	Ready() (ReadyCheck, bool)
}

// MPlusBaselines gives what a key review compares against from Warcraft Logs: the season's dungeons by name
// (encounter IDs), a spec's top runs in a dungeon at a keystone level by "dps" or "hps", a ranked run's party
// damage taken, and the points left this hour.
type MPlusBaselines interface {
	MPlusEncounters(ctx context.Context) (map[string]int, error)
	MPlusRankings(ctx context.Context, encounter, level int, class, spec, metric string) ([]MPlusRanking, error)
	RunDamageTaken(ctx context.Context, code string, fightID int) (int64, error)
	PointsLeft(ctx context.Context) (float64, error)
}

// TileSource gives one map tile of a floor (its tile folder) as WebP.
type TileSource interface {
	Tile(ctx context.Context, tiles string, x, y int) ([]byte, error)
}

// ReviewStore keeps key reviews by the key's start time; Review fails with fs.ErrNotExist when there's none.
type ReviewStore interface {
	SaveReview(review KeyReview) error
	Review(start int64) (KeyReview, error)
}

// RaidSource gives the raid as of now (ms since the epoch), the raid nights' pulls (newest night first) and one pull
// by its start time.
type RaidSource interface {
	RaidLive(now int64) RaidLive
	RaidNights() []RaidNight
	RaidPull(start int64) (RaidPull, error)
}

// PullStore keeps finished raid pulls, each under its start time; List is every saved pull, newest first.
type PullStore interface {
	SavePull(pull RaidPull) error
	ListPulls() ([]RaidPull, error)
}

// KeyStore keeps finished Mythic+ keys and their tracks, each under its start time. Track fails with
// fs.ErrNotExist for a key saved without one.
type KeyStore interface {
	Save(key Key) error
	SaveTrack(start int64, track KeyTrack) error
	// List is every saved key, newest first.
	List() ([]Key, error)
	Track(start int64) (KeyTrack, error)
	Delete(start int64) error
	Clear() error
}
