package interactive

import (
	"fmt"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// InteractiveSessionState stores the state of an interactive object creation session
type InteractiveSessionState struct {
	SessionID     string
	Kind          string
	LoopState     *LoopState
	Builder       any    // InstanceBuilder (using any to avoid import cycle)
	SchemaVersion string // Schema version for the builder
	CreatedAt     time.Time
	LastUpdatedAt time.Time
}

// InteractiveSessionManager manages sessions for interactive object creation
type InteractiveSessionManager struct {
	sessions map[string]*InteractiveSessionState
	mu       sync.RWMutex
	// Session timeout (sessions older than this are cleaned up)
	timeout time.Duration
	// Counter for generating unique session IDs
	sessionCounter int64
	counterMu      sync.Mutex
}

// NewInteractiveSessionManager creates a new session manager
func NewInteractiveSessionManager(timeout time.Duration) *InteractiveSessionManager {
	if timeout <= 0 {
		timeout = 30 * time.Minute // Default: 30 minutes
	}
	return &InteractiveSessionManager{
		sessions: make(map[string]*InteractiveSessionState),
		timeout:  timeout,
	}
}

// GenerateSessionID generates a unique session ID
func (ism *InteractiveSessionManager) GenerateSessionID() string {
	var counter int64
	_ = concurrency.RunInLockWithLogger(
		&ism.counterMu, LockNameSessionManagerGenerateId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			ism.sessionCounter++
			counter = ism.sessionCounter
			return nil
		},
	)
	return fmt.Sprintf("interactive-%d-%d", time.Now().UnixNano(), counter)
}

// CreateSession creates a new interactive session
func (ism *InteractiveSessionManager) CreateSession(kind string, loopState *LoopState) string {
	return ism.CreateSessionWithBuilder(kind, loopState, nil, "")
}

// CreateSessionWithBuilder creates a new interactive session with builder
func (ism *InteractiveSessionManager) CreateSessionWithBuilder(kind string, loopState *LoopState, builder any, schemaVersion string) string {
	sessionID := ism.GenerateSessionID()

	_ = concurrency.RunInLockWithLogger(
		&ism.mu, LockNameSessionManagerCreate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			ism.sessions[sessionID] = &InteractiveSessionState{
				SessionID:     sessionID,
				Kind:          kind,
				LoopState:     loopState,
				Builder:       builder,
				SchemaVersion: schemaVersion,
				CreatedAt:     time.Now(),
				LastUpdatedAt: time.Now(),
			}
			return nil
		},
	)
	return sessionID
}

// GetSession retrieves a session by ID
func (ism *InteractiveSessionManager) GetSession(sessionID string) (*InteractiveSessionState, bool) {
	var session *InteractiveSessionState
	var exists bool
	var timeout time.Duration
	_ = concurrency.RunInRLockWithLogger(
		&ism.mu, LockNameSessionManagerGet, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			session, ok = ism.sessions[sessionID]
			exists = ok
			timeout = ism.timeout
			return nil
		},
	)

	if !exists {
		return nil, false
	}

	// Check if session has expired
	if time.Since(session.LastUpdatedAt) > timeout {
		return nil, false
	}

	return session, true
}

// UpdateSession updates an existing session
func (ism *InteractiveSessionManager) UpdateSession(sessionID string, loopState *LoopState) error {
	var session *InteractiveSessionState
	var exists bool
	var timeout time.Duration
	err := concurrency.RunInLockWithLogger(
		&ism.mu, LockNameSessionManagerUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			session, ok = ism.sessions[sessionID]
			exists = ok
			timeout = ism.timeout
			if !exists {
				return errfmt.Errorf("session not found: %s", sessionID)
			}

			// Check if session has expired
			if time.Since(session.LastUpdatedAt) > timeout {
				delete(ism.sessions, sessionID)
				return errfmt.Errorf("session expired: %s", sessionID)
			}

			session.LoopState = loopState
			session.LastUpdatedAt = time.Now()
			return nil
		},
	)
	return err
}

// UpdateSessionWithBuilder updates an existing session with builder (if provided)
func (ism *InteractiveSessionManager) UpdateSessionWithBuilder(sessionID string, loopState *LoopState, builder any, schemaVersion string) error {
	return concurrency.RunInLockWithLogger(
		&ism.mu, LockNameSessionManagerUpdateBuilder, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			session, exists := ism.sessions[sessionID]
			if !exists {
				return errfmt.Errorf("session not found: %s", sessionID)
			}

			if time.Since(session.LastUpdatedAt) > ism.timeout {
				delete(ism.sessions, sessionID)
				return errfmt.Errorf("session expired: %s", sessionID)
			}

			session.LoopState = loopState
			if builder != nil {
				session.Builder = builder
			}
			if schemaVersion != emptyValue {
				session.SchemaVersion = schemaVersion
			}
			session.LastUpdatedAt = time.Now()
			return nil
		},
	)
}

// DeleteSession deletes a session
func (ism *InteractiveSessionManager) DeleteSession(sessionID string) {
	_ = concurrency.RunInLockWithLogger(
		&ism.mu, LockNameSessionManagerDelete, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			delete(ism.sessions, sessionID)
			return nil
		},
	)
}

// CleanupExpiredSessions removes expired sessions
func (ism *InteractiveSessionManager) CleanupExpiredSessions() int {
	var count int
	var timeout time.Duration
	_ = concurrency.RunInLockWithLogger(
		&ism.mu, LockNameSessionManagerCleanup, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			now := time.Now()
			timeout = ism.timeout
			count = 0

			for sessionID, session := range ism.sessions {
				if now.Sub(session.LastUpdatedAt) > timeout {
					delete(ism.sessions, sessionID)
					count++
				}
			}
			return nil
		},
	)

	return count
}

// Global session manager instance
var (
	globalSessionManager     *InteractiveSessionManager
	globalSessionManagerOnce sync.Once
)

// GetGlobalInteractiveSessionManager returns the global session manager
func GetGlobalInteractiveSessionManager() *InteractiveSessionManager {
	globalSessionManagerOnce.Do(func() {
		globalSessionManager = NewInteractiveSessionManager(30 * time.Minute)
	})
	return globalSessionManager
}
