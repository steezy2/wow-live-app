package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"wow-live/backend/models"
)

type fakeSpells struct {
	asked []int
	err   error
}

func (f *fakeSpells) Spell(_ context.Context, id int) (models.SpellInfo, error) {
	f.asked = append(f.asked, id)
	if f.err != nil {
		return models.SpellInfo{}, f.err
	}
	return models.SpellInfo{ID: id, Name: "Icebound Fortitude", Cooldown: "2 min cooldown"}, nil
}

// TestSpellsInfoAsksForTheIDTheAbilityWasCastBy: an ability's spell ID comes from the lines read (the cast's when
// another spell shares the name), and an ability the log hasn't shown is unknown without asking the source.
func TestSpellsInfoAsksForTheIDTheAbilityWasCastBy(t *testing.T) {
	keys := newTracker(t, nil)
	const none = `0000000000000000,nil,0x80000000,0x80000000`
	feed(t, keys,
		at(1, `SPELL_AURA_APPLIED,`+testPlayer+`,`+testPlayer+`,99999,"Icebound Fortitude",0x10,BUFF`),
		at(2, `SPELL_CAST_SUCCESS,`+testPlayer+`,`+none+`,48792,"Icebound Fortitude",0x10`))
	source := &fakeSpells{}
	spells := &Spells{Keys: keys, Source: source}
	info, err := spells.Info(context.Background(), "Icebound Fortitude")
	if err != nil || info.ID != 48792 || info.Cooldown != "2 min cooldown" {
		t.Errorf("info = %+v (%v); want spell 48792 from the source", info, err)
	}
	if _, err := spells.Info(context.Background(), "Nothing"); !errors.Is(err, ErrUnknownSpell) || len(source.asked) != 1 {
		t.Errorf("an unseen ability: err = %v, asked %v; want ErrUnknownSpell and no second ask", err, source.asked)
	}
}

// TestSpellsInfoLeavesAFailedLookupAloneForAWhile: a source that fails is asked once, then the failure is answered
// without asking again until spellRetry has passed.
func TestSpellsInfoLeavesAFailedLookupAloneForAWhile(t *testing.T) {
	keys := newTracker(t, nil)
	feed(t, keys, at(2, `SPELL_CAST_SUCCESS,`+testPlayer+`,0000000000000000,nil,0x80000000,0x80000000,48792,"Icebound Fortitude",0x10`))
	source := &fakeSpells{err: errors.New("down")}
	spells := &Spells{Keys: keys, Source: source}
	for i, want := range []error{source.err, ErrSpellDown} {
		if _, err := spells.Info(context.Background(), "Icebound Fortitude"); !errors.Is(err, want) {
			t.Errorf("ask %d: err = %v; want %v", i+1, err, want)
		}
	}
	if len(source.asked) != 1 {
		t.Errorf("asked %v; want one ask", source.asked)
	}
	spells.failed[48792] = time.Now().Add(-2 * spellRetry)
	if _, err := spells.Info(context.Background(), "Icebound Fortitude"); !errors.Is(err, source.err) || len(source.asked) != 2 {
		t.Errorf("after the wait: err = %v, asked %v; want asked again", err, source.asked)
	}
}
