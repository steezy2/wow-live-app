package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"wow-live/backend/models"
)

// wclKeysName is the saved Warcraft Logs keys' name in the store.
const wclKeysName = "warcraftlogs"

// ErrKeyStore means the keys couldn't be read, saved or removed in the store (Windows Credential Manager).
var ErrKeyStore = errors.New("Windows Credential Manager failed")

// KeyChecker is the Warcraft Logs client the keys are for: it tries keys, and takes the ones saved.
type KeyChecker interface {
	Test(ctx context.Context, creds models.Credentials) (models.RateLimit, error)
	SetCredentials(creds models.Credentials)
}

// WCLKeys keeps the desktop app's Warcraft Logs client ID and secret for key reviews: tested before they're saved
// (unless saved anyway, so Warcraft Logs being down doesn't block it), kept in Store, never shown again.
type WCLKeys struct {
	Store  models.SecretStore
	Client KeyChecker
	Now    func() time.Time
	mu     sync.Mutex
	status models.KeyStatus
}

// Load hands the saved keys, if any, to the client.
func (k *WCLKeys) Load() error {
	creds, err := k.saved()
	if err != nil || creds.ID == "" {
		return err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Client.SetCredentials(creds)
	k.status = models.KeyStatus{Set: true, IDEnd: idEnd(creds.ID)}
	return nil
}

// Status is whether keys are saved, their ID's last 4 characters and the last test.
func (k *WCLKeys) Status() models.KeyStatus {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.status
}

// Save tests creds and saves them if they work, or anyway with force. The error is the failed test's
// (models.ErrBadCredentials, or Warcraft Logs unreachable) when they weren't saved.
func (k *WCLKeys) Save(ctx context.Context, creds models.Credentials, force bool) (models.KeyStatus, error) {
	test, testErr := k.test(ctx, creds)
	if testErr != nil && !force {
		return k.Status(), testErr
	}
	data, err := json.Marshal(creds)
	if err != nil {
		return k.Status(), err
	}
	if err := k.Store.Set(wclKeysName, string(data)); err != nil {
		return k.Status(), fmt.Errorf("saving the Warcraft Logs keys: %w: %w", ErrKeyStore, err)
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Client.SetCredentials(creds)
	test.Set, test.IDEnd = true, idEnd(creds.ID)
	k.status = test
	return test, nil
}

// Retest tries the saved keys again.
func (k *WCLKeys) Retest(ctx context.Context) (models.KeyStatus, error) {
	creds, err := k.saved()
	if err != nil {
		return k.Status(), err
	}
	if creds.ID == "" {
		return k.Status(), models.ErrNoCredentials
	}
	test, testErr := k.test(ctx, creds)
	k.mu.Lock()
	defer k.mu.Unlock()
	test.Set, test.IDEnd = true, idEnd(creds.ID)
	k.status = test
	return test, testErr
}

// Remove deletes the saved keys; reviews stop until new ones are saved.
func (k *WCLKeys) Remove() error {
	if err := k.Store.Delete(wclKeysName); err != nil {
		return fmt.Errorf("removing the Warcraft Logs keys: %w: %w", ErrKeyStore, err)
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Client.SetCredentials(models.Credentials{})
	k.status = models.KeyStatus{}
	return nil
}

// test tries creds and says how it went, without the secret.
func (k *WCLKeys) test(ctx context.Context, creds models.Credentials) (models.KeyStatus, error) {
	budget, err := k.Client.Test(ctx, creds)
	status := models.KeyStatus{TestedMS: k.Now().UnixMilli(), Works: err == nil,
		PointsLeft: budget.LimitPerHour - budget.PointsSpentThisHour, LimitPerHour: budget.LimitPerHour}
	switch {
	case errors.Is(err, models.ErrBadCredentials):
		status.Problem = "wrong client ID or secret"
	case err != nil:
		status.Problem = "couldn't reach Warcraft Logs"
	}
	return status, err
}

// saved reads the stored keys; an empty ID when there are none.
func (k *WCLKeys) saved() (models.Credentials, error) {
	var creds models.Credentials
	data, err := k.Store.Get(wclKeysName)
	if err != nil {
		return creds, fmt.Errorf("reading the Warcraft Logs keys: %w: %w", ErrKeyStore, err)
	}
	if data == "" {
		return creds, nil
	}
	if err := json.Unmarshal([]byte(data), &creds); err != nil {
		return creds, fmt.Errorf("reading the saved Warcraft Logs keys: %w", err)
	}
	return creds, nil
}

// idEnd is a client ID's last 4 characters, enough to tell which keys are saved.
func idEnd(id string) string {
	return id[max(0, len(id)-4):]
}
