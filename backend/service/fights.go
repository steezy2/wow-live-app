package service

import (
	"cmp"
	"fmt"
	"slices"
	"sync"

	"wow-live/backend/models"
)

// savedFight is a finished key or raid pull and its track, waiting to be saved.
type savedFight[T any] struct {
	fight T
	track models.KeyTrack
}

// upsert makes fight the newest of list, taking the place of the copy a restart read before it (a report linked
// since stays).
func upsert[T models.SavedFight[T]](list *[]T, fight T) {
	i := slices.IndexFunc(*list, func(h T) bool {
		return h.FightStart() == fight.FightStart() && h.FromWCL() == fight.FromWCL()
	})
	if i < 0 {
		*list = append([]T{fight}, *list...)
		return
	}
	(*list)[i] = fight.LinkedTo(cmp.Or(fight.LinkedReport(), (*list)[i].LinkedReport()))
}

// withUnsaved is list with the queued fights it doesn't have yet (read before they ended) added in front.
func withUnsaved[T models.SavedFight[T]](list []T, queue []savedFight[T]) []T {
	for _, q := range queue {
		if !slices.ContainsFunc(list, func(h T) bool {
			return h.FightStart() == q.fight.FightStart() && h.FromWCL() == q.fight.FromWCL()
		}) {
			list = append([]T{q.fight}, list...)
		}
	}
	return list
}

// saveFights saves the queued fights and their tracks in store, keeping the failed ones queued for the next call
// (saveQueued), and says which it saved (by start time). what names a fight in the error. list is the tracker's
// fights, which keepLocal settles.
func saveFights[T models.SavedFight[T]](mu *sync.RWMutex, queue *[]savedFight[T], list *[]T, store models.FightStore[T],
	same func(a, b T) bool, what func(T) string) ([]int64, error) {
	var saved []int64
	err := saveQueued(mu, queue, func(q savedFight[T]) error {
		skip, err := keepLocal(mu, list, q.fight, same, store.Delete)
		if skip || err != nil {
			return err
		}
		err = store.Save(q.fight)
		if err == nil {
			err = store.SaveTrack(q.fight.FightStart(), q.track)
		}
		if err != nil {
			return fmt.Errorf("saving the %s (retried after the next read): %w", what(q.fight), err)
		}
		saved = append(saved, q.fight.FightStart())
		return nil
	})
	return saved, err
}

// keepLocal settles a fight read both from a local log and from Warcraft Logs, before it is saved: the local copy
// wins. An imported fight with a local twin in list is taken off it and not saved (true); a local one's imported
// twins are deleted (del) and taken off. mu is the tracker's lock.
func keepLocal[T models.SavedFight[T]](mu *sync.RWMutex, list *[]T, fight T, same func(a, b T) bool,
	del func(int64) error) (bool, error) {
	mu.Lock()
	defer mu.Unlock()
	twin := func(other T) bool { return same(fight, other) }
	if fight.FromWCL() {
		if !slices.ContainsFunc(*list, twin) {
			return false, nil
		}
		*list = slices.DeleteFunc(*list, func(other T) bool { return other.FightStart() == fight.FightStart() && other.FromWCL() })
		return true, nil
	}
	for _, other := range *list {
		if twin(other) {
			if err := del(other.FightStart()); err != nil {
				return false, fmt.Errorf("deleting an imported copy: %w", err)
			}
		}
	}
	*list = slices.DeleteFunc(*list, twin)
	return false, nil
}

// unlinked is the fights of list that links has a link for (by start time) and that aren't imported or linked yet,
// as linked to it.
func unlinked[T models.SavedFight[T]](list []T, links map[int64]models.ReportLink) []T {
	var out []T
	for _, fight := range list {
		if link, ok := links[fight.FightStart()]; ok && !fight.FromWCL() && fight.LinkedReport() == nil {
			out = append(out, fight.LinkedTo(&link))
		}
	}
	return out
}

// applyLinks gives the local fights of list the reports of the linked ones with the same start, and returns the
// starts of linked ones no longer in list.
func applyLinks[T models.SavedFight[T]](list []T, linked []T) (gone []int64) {
	for _, l := range linked {
		if i := slices.IndexFunc(list, func(f T) bool { return f.FightStart() == l.FightStart() && !f.FromWCL() }); i >= 0 {
			list[i] = list[i].LinkedTo(l.LinkedReport())
		} else {
			gone = append(gone, l.FightStart())
		}
	}
	return gone
}

// importedStarts are the start times of the fights of list imported from Warcraft Logs.
func importedStarts[T models.SavedFight[T]](list []T) []int64 {
	var starts []int64
	for _, fight := range list {
		if fight.FromWCL() {
			starts = append(starts, fight.FightStart())
		}
	}
	return starts
}

// dropImported takes the imported fights with these start times off list.
func dropImported[T models.SavedFight[T]](list *[]T, starts []int64) {
	*list = slices.DeleteFunc(*list, func(f T) bool { return f.FromWCL() && slices.Contains(starts, f.FightStart()) })
}

// deleteAll deletes starts in order with del, and says which it deleted before one failed.
func deleteAll(starts []int64, del func(int64) error) ([]int64, error) {
	for i, start := range starts {
		if err := del(start); err != nil {
			return starts[:i], err
		}
	}
	return starts, nil
}
