package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"wow-live/backend/models"
)

type memoryStore map[string]string

func (m memoryStore) Get(name string) (string, error) { return m[name], nil }
func (m memoryStore) Set(name, secret string) error   { m[name] = secret; return nil }
func (m memoryStore) Delete(name string) error        { delete(m, name); return nil }

type fakeChecker struct {
	err  error
	used models.Credentials
}

func (f *fakeChecker) Test(context.Context, models.Credentials) (models.RateLimit, error) {
	return models.RateLimit{LimitPerHour: 1000, PointsSpentThisHour: 60}, f.err
}
func (f *fakeChecker) SetCredentials(creds models.Credentials) { f.used = creds }

func TestWCLKeysAreSavedOnlyWhenTheyWorkOrAnyway(t *testing.T) {
	creds := models.Credentials{ID: "abcd-1234", Secret: "s3cret"}
	for _, tc := range []struct {
		name   string
		err    error
		force  bool
		saved  bool
		status models.KeyStatus
	}{
		{"working keys", nil, false, true, models.KeyStatus{Set: true, IDEnd: "1234", Works: true, PointsLeft: 940,
			LimitPerHour: 1000}},
		{"turned down", models.ErrBadCredentials, false, false, models.KeyStatus{}},
		{"turned down, saved anyway", models.ErrBadCredentials, true, true,
			models.KeyStatus{Set: true, IDEnd: "1234", Problem: "wrong client ID or secret", PointsLeft: 940,
				LimitPerHour: 1000}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, client := memoryStore{}, &fakeChecker{err: tc.err}
			keys := &WCLKeys{Store: store, Client: client, Now: func() time.Time { return time.UnixMilli(5) }}
			status, err := keys.Save(context.Background(), creds, tc.force)
			if tc.saved {
				tc.status.TestedMS = 5
			}
			if (store[wclKeysName] != "") != tc.saved || (client.used == creds) != tc.saved || status != tc.status ||
				(err == nil) != tc.saved || err != nil && !errors.Is(err, models.ErrBadCredentials) {
				t.Fatalf("status %+v, err %v, stored %q, client %+v", status, err, store[wclKeysName], client.used)
			}
		})
	}
}
