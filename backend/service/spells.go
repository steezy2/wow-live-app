package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"wow-live/backend/models"
)

// ErrUnknownSpell is returned for an ability the combat log hasn't shown yet, and for a spell ID no dungeon mob casts.
var ErrUnknownSpell = errors.New("no such spell: not in the combat log or any dungeon mob's spells")

// ErrSpellDown is returned by Spells.Info and ReferenceSpell for a spell whose lookup failed less than spellRetry ago.
var ErrSpellDown = errors.New("spell lookup failed lately")

// spellRetry is how long a failed lookup is left alone, so a source that is down isn't asked on every hover.
const spellRetry = time.Minute

// SpellSource gives what a spell does by its ID (clients/wowhead).
type SpellSource interface {
	Spell(ctx context.Context, id int) (models.SpellInfo, error)
}

// Spells explains the abilities the app lists: it finds an ability's spell ID in the lines the tracker has read and
// asks the source for what it does.
type Spells struct {
	Keys   *LogTracker
	Source SpellSource
	// Talents are the spec talent trees, whose spells can be asked for by ID.
	Talents map[int]models.TalentTree
	mu      sync.Mutex
	failed  map[int]time.Time // spell ID -> when its lookup last failed
	talent  map[int]bool      // the talent spell IDs, from Talents
}

// Info is what the ability called name does and its cooldown.
func (s *Spells) Info(ctx context.Context, name string) (models.SpellInfo, error) {
	id := s.Keys.SpellID(name)
	if id == 0 {
		return models.SpellInfo{}, ErrUnknownSpell
	}
	info, err := s.byID(ctx, id)
	if err != nil {
		return models.SpellInfo{}, fmt.Errorf("%s: %w", name, err)
	}
	return info, nil
}

// ReferenceSpell is what a spell of one of the season's dungeon mobs or of a talent does, by its ID; ErrUnknownSpell
// for any other ID, so the endpoint can't be made to fetch and cache every spell Wowhead has.
func (s *Spells) ReferenceSpell(ctx context.Context, id int) (models.SpellInfo, error) {
	if !s.Keys.MobCasts(id) && !s.isTalent(id) {
		return models.SpellInfo{}, ErrUnknownSpell
	}
	return s.byID(ctx, id)
}

// isTalent is whether a spell ID is one of a talent's.
func (s *Spells) isTalent(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.talent == nil {
		s.talent = map[int]bool{}
		for _, tree := range s.Talents {
			for _, n := range tree.Nodes {
				for _, e := range n.Entries {
					s.talent[e.SpellID] = true
				}
			}
		}
	}
	return s.talent[id]
}

func (s *Spells) byID(ctx context.Context, id int) (models.SpellInfo, error) {
	s.mu.Lock()
	at, bad := s.failed[id]
	s.mu.Unlock()
	if bad && time.Since(at) < spellRetry {
		return models.SpellInfo{}, ErrSpellDown
	}
	info, err := s.Source.Spell(ctx, id)
	if err != nil {
		s.mu.Lock()
		if s.failed == nil {
			s.failed = map[int]time.Time{}
		}
		s.failed[id] = time.Now()
		s.mu.Unlock()
		return models.SpellInfo{}, fmt.Errorf("reading spell %d: %w", id, err)
	}
	return info, nil
}

// SpellID is the spell ID the log gave an ability name: the one it was cast by, else the first seen; 0 if none was
// read.
func (k *LogTracker) SpellID(name string) int {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.spellIDs[name]
}
