// Package app: thin wrappers over pkg/zqksession for CLI session lifecycle.
// TRACK: REQ-COMMS-RUNTIME-SESSION-001 — session identity lives in pkg/zqksession.
package app

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqksession"
)

// Compatibility aliases for call sites that still use the pre-extraction names.
const (
	zqkStatusActive = zqksession.StatusActive
	zqkStatusDone   = zqksession.StatusCompleted
	zqkStatusError  = zqksession.StatusError
)

// StartZqkSession creates a zqk_session with status "active" and returns its ID.
func StartZqkSession(ctx context.Context, projectRoot, title, accountID string, sp storage.ObjectStorageProvider) string {
	return zqksession.StartCLISession(ctx, projectRoot, title, accountID, sp)
}

// EndZqkSession updates the zqk_session to the given status ("completed" or "error").
func EndZqkSession(ctx context.Context, projectRoot, sessionID, status, accountID string, sp storage.ObjectStorageProvider) {
	zqksession.End(ctx, projectRoot, sessionID, status, accountID, sp)
}

// TouchSession updates the session's updated_at and optional title.
func TouchSession(ctx context.Context, projectRoot, sessionID, title, accountID string, sp storage.ObjectStorageProvider) {
	zqksession.Touch(ctx, projectRoot, sessionID, title, accountID, sp)
}

// TouchSessionIfNotThrottled updates the session only when outside the throttle window.
func TouchSessionIfNotThrottled(ctx context.Context, projectRoot, sessionID, title, accountID string, sp storage.ObjectStorageProvider) {
	zqksession.TouchIfNotThrottled(ctx, projectRoot, sessionID, title, accountID, sp)
}

// GetCurrentSessionID returns the session ID from env or persisted state.
func GetCurrentSessionID(projectRoot string) string {
	return zqksession.GetCurrentID(projectRoot)
}

// ReadPersistedSessionID returns the current session ID from file only.
func ReadPersistedSessionID(projectRoot string) string {
	return zqksession.ReadPersistedID(projectRoot)
}

// WritePersistedSessionID writes the session ID for cross-terminal reuse.
func WritePersistedSessionID(ctx context.Context, projectRoot, sessionID, accountID string, sp storage.ObjectStorageProvider) bool {
	return zqksession.WritePersistedID(ctx, projectRoot, sessionID, accountID, sp)
}

// ClearPersistedSession removes the persisted session file.
func ClearPersistedSession(projectRoot string) {
	zqksession.ClearPersisted(projectRoot)
}

// GetSessionIdleTimeout returns session.idle_timeout from project config.
func GetSessionIdleTimeout(projectRoot string) time.Duration {
	return zqksession.GetIdleTimeout(projectRoot)
}

// TryReuseSession reuses an active, non-idle session for accountID when present.
func TryReuseSession(ctx context.Context, projectRoot, title, accountID string, sp storage.ObjectStorageProvider) (sessionID string, reused bool) {
	return zqksession.TryReuse(ctx, projectRoot, title, accountID, sp)
}

// GetZqkSessionIDFromContext returns the current CLI zqk_session ID from context, or "".
func GetZqkSessionIDFromContext(ctx context.Context) string {
	return zqksession.GetIDFromContext(ctx)
}

// WithZqkSessionID attaches the session ID to the context.
func WithZqkSessionID(ctx context.Context, sessionID string) context.Context {
	return zqksession.WithID(ctx, sessionID)
}
