package models

// SavedFight is what a saved Key and a saved RaidPull have in common, so one piece of code can save, link and replace
// either. T is the type itself.
type SavedFight[T any] interface {
	// FightStart is when it started (ms since the epoch), which names its saved files.
	FightStart() int64
	// FromWCL is whether it was imported from Warcraft Logs rather than read from the game's log.
	FromWCL() bool
	// LinkedReport is the Warcraft Logs fight it is linked to, nil if none.
	LinkedReport() *ReportLink
	// LinkedTo is a copy of it linked to report.
	LinkedTo(report *ReportLink) T
}

// FightStart implements SavedFight.
func (k Key) FightStart() int64 { return k.Start }

// FromWCL implements SavedFight.
func (k Key) FromWCL() bool { return k.Imported }

// LinkedReport implements SavedFight.
func (k Key) LinkedReport() *ReportLink { return k.Report }

// LinkedTo implements SavedFight.
func (k Key) LinkedTo(report *ReportLink) Key { k.Report = report; return k }

// FightStart implements SavedFight.
func (p RaidPull) FightStart() int64 { return p.Start }

// FromWCL implements SavedFight.
func (p RaidPull) FromWCL() bool { return p.Imported }

// LinkedReport implements SavedFight.
func (p RaidPull) LinkedReport() *ReportLink { return p.Report }

// LinkedTo implements SavedFight.
func (p RaidPull) LinkedTo(report *ReportLink) RaidPull { p.Report = report; return p }

// FightStore keeps finished fights (keys or raid pulls) and their tracks, each under its start time. Track fails
// with fs.ErrNotExist for a fight saved without one; List is newest first; Delete removes one with its track.
type FightStore[T any] interface {
	Save(fight T) error
	SaveTrack(start int64, track KeyTrack) error
	List() ([]T, error)
	Track(start int64) (KeyTrack, error)
	Delete(start int64) error
}
