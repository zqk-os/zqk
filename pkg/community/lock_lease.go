package community

import (
	"errors"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/strutil"
)

const (
	errLockBlankResourceKeyMsg = "resource key cannot be blank"
	errLockBlankClientIDMsg    = "client ID cannot be blank"
)

var (
	// ErrBlankResourceKey indicates empty or whitespace resource key.
	ErrBlankResourceKey = errors.New(errLockBlankResourceKeyMsg)

	// ErrBlankLockClientID indicates empty or whitespace client ID.
	ErrBlankLockClientID = errors.New(errLockBlankClientIDMsg)

	// ErrInvalidLeaseTTL indicates non-positive lease duration.
	ErrInvalidLeaseTTL = errors.New("lease TTL must be positive")

	// ErrLockHeldByAnother indicates contention from an active, unexpired lease.
	ErrLockHeldByAnother = errors.New("resource lock already held by another client")

	// ErrLockNotFound indicates the requested lock is absent or expired.
	ErrLockNotFound = errors.New("lock not found or already released")

	// ErrLockNotOwned indicates caller does not hold the active lease token.
	ErrLockNotOwned = errors.New("client does not own the active lock")
)

// EphemeralLockRecord models a single time-bounded lease reservation.
type EphemeralLockRecord struct {
	ResourceKey  string
	ClientID     string
	FencingToken uint64
	AcquiredAt   time.Time
	ExpiresAt    time.Time
}

// EphemeralLockManager orchestrates atomic distributed lock reservations and auto-reclaims.
type EphemeralLockManager struct {
	mu           sync.RWMutex
	fencingToken uint64
	locks        map[string]EphemeralLockRecord
}

// NewEphemeralLockManager constructs an initialized lock manager instance.
func NewEphemeralLockManager() *EphemeralLockManager {
	return &EphemeralLockManager{
		locks: make(map[string]EphemeralLockRecord),
	}
}

// validateLockIdentifiers ensures resourceKey and clientID are non-blank.
func validateLockIdentifiers(resourceKey, clientID string) error {
	if err := strutil.ErrorOnBlank(resourceKey, errLockBlankResourceKeyMsg); err != nil {
		return ErrBlankResourceKey
	}
	if err := strutil.ErrorOnBlank(clientID, errLockBlankClientIDMsg); err != nil {
		return ErrBlankLockClientID
	}
	return nil
}

// validateLockArgs provides declarative validation for key, client, and TTL.
func validateLockArgs(resourceKey, clientID string, ttl time.Duration) error {
	if err := validateLockIdentifiers(resourceKey, clientID); err != nil {
		return err
	}
	if ttl <= 0 {
		return ErrInvalidLeaseTTL
	}
	return nil
}

// verifyLockOwnership ensures the lock exists, is active, and is owned by clientID.
func verifyLockOwnership(existing EphemeralLockRecord, exists bool, clientID string, now time.Time) error {
	if !exists || now.After(existing.ExpiresAt) {
		return ErrLockNotFound
	}
	if existing.ClientID != clientID {
		return ErrLockNotOwned
	}
	return nil
}

func (m *EphemeralLockManager) mutateLock(resourceKey string, fn func(existing EphemeralLockRecord, exists bool) (EphemeralLockRecord, bool, error)) (EphemeralLockRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.locks[resourceKey]
	updated, deleteEntry, err := fn(existing, exists)
	if err != nil {
		return EphemeralLockRecord{}, err
	}
	if deleteEntry {
		delete(m.locks, resourceKey)
		return EphemeralLockRecord{}, nil
	}
	m.locks[resourceKey] = updated
	return updated, nil
}

// Acquire attempts to acquire exclusive ownership of a resource key.
func (m *EphemeralLockManager) Acquire(resourceKey, clientID string, ttl time.Duration, now time.Time) (EphemeralLockRecord, error) {
	if err := validateLockArgs(resourceKey, clientID, ttl); err != nil {
		return EphemeralLockRecord{}, err
	}

	return m.mutateLock(resourceKey, func(existing EphemeralLockRecord, exists bool) (EphemeralLockRecord, bool, error) {
		if exists && now.Before(existing.ExpiresAt) && existing.ClientID != clientID {
			return EphemeralLockRecord{}, false, ErrLockHeldByAnother
		}

		m.fencingToken++
		rec := EphemeralLockRecord{
			ResourceKey:  resourceKey,
			ClientID:     clientID,
			FencingToken: m.fencingToken,
			AcquiredAt:   now,
			ExpiresAt:    now.Add(ttl),
		}
		return rec, false, nil
	})
}

// ExtendLease refreshes the expiration window of a previously acquired lock.
func (m *EphemeralLockManager) ExtendLease(resourceKey, clientID string, ttl time.Duration, now time.Time) (EphemeralLockRecord, error) {
	if err := validateLockArgs(resourceKey, clientID, ttl); err != nil {
		return EphemeralLockRecord{}, err
	}

	return m.mutateLock(resourceKey, func(existing EphemeralLockRecord, exists bool) (EphemeralLockRecord, bool, error) {
		if err := verifyLockOwnership(existing, exists, clientID, now); err != nil {
			return EphemeralLockRecord{}, false, err
		}

		existing.ExpiresAt = now.Add(ttl)
		return existing, false, nil
	})
}

// Release yields ownership of the lock if owned and unexpired.
func (m *EphemeralLockManager) Release(resourceKey, clientID string, now time.Time) error {
	if err := validateLockIdentifiers(resourceKey, clientID); err != nil {
		return err
	}

	_, err := m.mutateLock(resourceKey, func(existing EphemeralLockRecord, exists bool) (EphemeralLockRecord, bool, error) {
		if err := verifyLockOwnership(existing, exists, clientID, now); err != nil {
			return EphemeralLockRecord{}, false, err
		}
		return EphemeralLockRecord{}, true, nil
	})
	return err
}

// Inspect retrieves active unexpired lock information if held.
func (m *EphemeralLockManager) Inspect(resourceKey string, now time.Time) (EphemeralLockRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	existing, exists := m.locks[resourceKey]
	if !exists || now.After(existing.ExpiresAt) {
		return EphemeralLockRecord{}, false
	}
	return existing, true
}
