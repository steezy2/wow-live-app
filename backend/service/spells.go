package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"wow-live/backend/models"
)

// ErrUnknownSpell is returned by Spells.Info for an ability the combat log hasn't shown yet.
var ErrUnknownSpell = errors.New("no spell by that name in the combat log")

// ErrSpellDown is returned by Spells.Info for a spell whose lookup failed less than spellRetry ago.
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
	mu     sync.Mutex
	failed map[int]time.Time // spell ID -> when its lookup last failed
}

// Info is what the ability called name does and its cooldown.
func (s *Spells) Info(ctx context.Context, name string) (models.SpellInfo, error) {
	id := s.Keys.SpellID(name)
	if id == 0 {
		return models.SpellInfo{}, ErrUnknownSpell
	}
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
		return models.SpellInfo{}, fmt.Errorf("reading spell %d (%s): %w", id, name, err)
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
