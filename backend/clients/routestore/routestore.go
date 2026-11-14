// Package routestore keeps planned routes as JSON files in a folder (logs/cache/routes/), one per route, named by its id.
package routestore

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

// Store is a folder of routes.
type Store struct {
	Dir string
}

// Save writes a route, replacing an earlier copy of it.
func (s Store) Save(route models.Route) error {
	path, err := s.path(route.ID)
	if err != nil {
		return err
	}
	return cachefile.WriteJSON(path, route)
}

// Get reads a route; the error wraps os.ErrNotExist when there's none.
func (s Store) Get(id string) (models.Route, error) {
	var r models.Route
	path, err := s.path(id)
	if err != nil {
		return r, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return r, fmt.Errorf("reading a route: %w", err)
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("reading a route: %w", err)
	}
	return r, nil
}

// List is every route, newest first; a missing folder is no routes.
func (s Store) List() ([]models.Route, error) {
	routes, err := cachefile.ReadAll[models.Route](s.Dir)
	slices.SortFunc(routes, func(a, b models.Route) int { return cmp.Compare(b.Updated, a.Updated) })
	return routes, err
}

// Delete removes a route; one that isn't there is already gone.
func (s Store) Delete(id string) error {
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting a route: %w", err)
	}
	return nil
}

// path is a route's file; ids are checked here so nothing else can build a path outside the folder.
func (s Store) path(id string) (string, error) {
	if !models.ValidRouteID(id) {
		return "", fmt.Errorf("route id %q: letters, digits and brackets only, 1 to 32", id)
	}
	return filepath.Join(s.Dir, id+".json"), nil
}
