package models

import (
	"context"
)

// KeySource gives the running Mythic+ key read at now (ms since the epoch), or the last one that ended (false before
// any key), and the saved keys: listed newest first, read or deleted by start time (0 deletes them all); a key's
// track for the map by its start time, a dungeon's map (by map ID): its floors and where parties have walked, and
// ending the running key as abandoned (false when none is running).
type KeySource interface {
	Latest(now int64) (KeyView, bool)
	Saved(character string) []KeySummary
	Played(preferred string) []Character
	SavedKey(start int64) (KeyView, error)
	DeleteSaved(start int64) error
	Track(start int64) (KeyTrack, error)
	// RouteTrack is a key's dungeon and what comparing it with a route reads of its track: mobs (the dead ones too
	// while it runs), deaths, bosses and pulls.
	RouteTrack(start int64) (int, KeyTrack, error)
	DungeonMap(mapID int) (DungeonMap, error)
	Abandon() bool
}

// MPlusBaselines gives what a key review and the recap compare against from Warcraft Logs: the season's dungeons by
// name (encounter IDs), a spec's top runs in a dungeon at a keystone level by "dps" or "hps", the times of a dungeon's
// top speed-ranked runs at a level, a ranked run's party
// damage taken, and the points left this hour.
type MPlusBaselines interface {
	MPlusEncounters(ctx context.Context) (map[string]int, error)
	MPlusRankings(ctx context.Context, encounter, level int, class, spec, metric string) ([]MPlusRanking, error)
	MPlusTopTimes(ctx context.Context, encounter, level int) ([]int64, error)
	Run(ctx context.Context, code string, fightID int) (RunSummary, error)
	PointsLeft(ctx context.Context) (float64, error)
}

// TileSource gives one map tile of a floor (its tile folder) as WebP.
type TileSource interface {
	Tile(ctx context.Context, tiles string, x, y int) ([]byte, error)
}

// MapImageSource gives a raid room's map image (by the combat log's uiMapID) as PNG.
type MapImageSource interface {
	Image(ctx context.Context, uiMapID int) ([]byte, error)
}

// ReviewStore keeps key reviews by the key's start time; Review fails with fs.ErrNotExist when there's none.
type ReviewStore interface {
	SaveReview(review KeyReview) error
	Review(start int64) (KeyReview, error)
}

// RaidSource gives the raid as of now (ms since the epoch), the raid nights' pulls (newest first), and one pull
// and its replay track by its start time.
type RaidSource interface {
	RaidLive(now int64) RaidLive
	RaidNights(character string) []RaidNight
	RaidPull(start int64) (RaidPull, error)
	RaidTrack(start int64) (KeyTrack, error)
}

// PullStore keeps finished raid pulls and their tracks, each under its start time; ListPulls is every saved pull,
// newest first, and DeletePull removes one with its track. Track fails with fs.ErrNotExist for a pull saved without one.
type PullStore interface {
	SavePull(pull RaidPull) error
	SaveTrack(start int64, track KeyTrack) error
	ListPulls() ([]RaidPull, error)
	Track(start int64) (KeyTrack, error)
	DeletePull(start int64) error
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
