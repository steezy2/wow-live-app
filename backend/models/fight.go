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
	// Slim is a copy without the lists only its own page reads (what the tracker keeps in memory); the store's Load
	// gives the whole fight back. A slim fight is never saved.
	Slim() T
	// IsSlim is whether it is a Slim copy.
	IsSlim() bool
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

// Slim implements SavedFight: a key without its abilities and damage taken by ability (about a third of it).
func (k Key) Slim() Key {
	k.hadAbilities, k.slim = k.HasAbilities(), true
	k.Abilities, k.Taken = nil, nil
	return k
}

// IsSlim implements SavedFight.
func (k Key) IsSlim() bool { return k.slim }

// HasAbilities is whether the key saved its abilities (keys saved before Nov 10 did not), slim or not.
func (k Key) HasAbilities() bool { return k.hadAbilities || len(k.Abilities) > 0 }

// Slim implements SavedFight: a pull without its abilities, damage taken by ability and damage by target (about two
// thirds of it), keeping the damage taken by ability in total (TakenByAbility).
func (p RaidPull) Slim() RaidPull {
	p.hadAbilities, p.takenBy, p.slim = p.HasAbilities(), p.TakenByAbility(), true
	p.Abilities, p.Taken, p.Targets = nil, nil, nil
	return p
}

// IsSlim implements SavedFight.
func (p RaidPull) IsSlim() bool { return p.slim }

// HasAbilities is whether the pull saved its abilities (pulls saved before Nov 11 did not), slim or not.
func (p RaidPull) HasAbilities() bool { return p.hadAbilities || len(p.Abilities) > 0 }

// TakenByAbility is the raiders' damage taken in total by each ability, slim or not.
func (p RaidPull) TakenByAbility() map[string]int64 {
	if p.slim {
		return p.takenBy
	}
	total := map[string]int64{}
	for _, h := range p.Taken {
		total[h.Ability] += h.Damage
	}
	return total
}

// FightStore keeps finished fights (keys or raid pulls) and their tracks, each under its start time. Track fails
// with fs.ErrNotExist for a fight saved without one; List is newest first; Delete removes one with its track.
type FightStore[T any] interface {
	Save(fight T) error
	SaveTrack(start int64, track KeyTrack) error
	List() ([]T, error)
	Track(start int64) (KeyTrack, error)
	// Load is one whole fight; the error wraps fs.ErrNotExist for one not saved.
	Load(start int64) (T, error)
	Delete(start int64) error
}
