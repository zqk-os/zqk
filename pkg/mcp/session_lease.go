package mcp

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// DefaultSessionLeaseTTL is how long a session may sit without refreshed last_activity
// before SessionLeaseExpired reports true. Independent of transport IdleTimeout (process
// lifetime); this is the object-level lease for health / mesh observers.
const DefaultSessionLeaseTTL = 30 * time.Minute

// DefaultSessionActivityPersistMinInterval throttles CAS/CLI writes for last_activity
// so high-frequency MCP requests do not flood object update.
const DefaultSessionActivityPersistMinInterval = 30 * time.Second

// sessionActivityPersistTimeout bounds the background object-update used for lease refresh.
const sessionActivityPersistTimeout = 5 * time.Second

// SessionLeaseExpired reports whether lastActivity (RFC3339 UTC) is older than ttl
// relative to now. Empty lastActivity or unparseable timestamps are treated as expired
// when ttl > 0 (unknown activity is not a healthy lease).
func SessionLeaseExpired(lastActivity string, ttl time.Duration, now time.Time) bool {
	if ttl <= 0 {
		return false
	}
	if lastActivity == emptyValue {
		return true
	}
	t, err := time.Parse(time.RFC3339, lastActivity)
	if err != nil {
		return true
	}
	return now.Sub(t) > ttl
}

// TouchSessionLastActivity refreshes in-memory session activity and, when the persist
// throttle allows, asynchronously updates mcp_session.last_activity via CLI.
// Best-effort: never blocks the MCP request path on storage.
//
// TRACK: [REDACTED-ID] — Phase B session lease/heartbeat.
func TouchSessionLastActivity(s *Server) {
	if s == nil {
		return
	}
	sessionID := s.GetCurrentSessionID()
	if sessionID == emptyValue {
		return
	}

	now := time.Now().UTC()
	persist := false
	_ = concurrency.RunInLockWithLogger(
		&s.sessionActivityMu, LockNameMcpServerSessionActivity, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.sessionLastActivityAt = now
			if s.sessionLastPersistAt.IsZero() || now.Sub(s.sessionLastPersistAt) >= DefaultSessionActivityPersistMinInterval {
				s.sessionLastPersistAt = now
				persist = true
			}
			return nil
		},
	)
	if !persist {
		return
	}

	goroutinelabels.NewGoroutine("mcp", "session last_activity refresh").
		StartSimple(func() {
			ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), sessionActivityPersistTimeout)
			defer cancel()

			args := map[string]any{
				"_command_path":       GetCommandPath("object update"),
				objects.FieldKeyID:    sessionID,
				objects.FieldKeyField: objects.FieldKeyLastActivity + "=" + zqktime.NowRFC3339UTC(),
			}
			_, err := s.executeCLICommandWithContext(ctx, args)
			if err != nil && s.getTraceWriter() != nil {
				logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
				logging.Fluent(logger).Warn("Failed to refresh MCP session last_activity").
					String("session_id", sessionID).
					WithError(err).
					EmitComponent("mcp_server").
					Log()
			}
		})
}

// GetSessionLastActivityAt returns the in-memory last activity time (zero if never touched).
func (s *Server) GetSessionLastActivityAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	var at time.Time
	_ = concurrency.RunInLockWithLogger(
		&s.sessionActivityMu, LockNameMcpServerSessionActivity, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			at = s.sessionLastActivityAt
			return nil
		},
	)
	return at
}

// CurrentSessionLeaseExpired reports whether the in-memory last activity exceeds ttl.
// If activity was never touched, returns true when a session ID is set (stale until first touch).
func (s *Server) CurrentSessionLeaseExpired(ttl time.Duration) bool {
	if s == nil || s.GetCurrentSessionID() == emptyValue {
		return false
	}
	at := s.GetSessionLastActivityAt()
	if at.IsZero() {
		return true
	}
	return SessionLeaseExpired(at.UTC().Format(time.RFC3339), ttl, time.Now().UTC())
}
