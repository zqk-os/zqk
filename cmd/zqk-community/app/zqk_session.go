// Package app: zqk_session lifecycle for CLI invocations.
// Sessions can be reused across invocations; state is persisted under .zqk/state/session
// and optionally in ZQK_SESSION_ID. A file lock ensures consistency across processes/terminals.

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/lanceman/zqk/pkg/storage"
	idgen "github.com/lanceman/zqk/pkg/storage/id_generation"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqktime"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// sessionStateFile is the filename under .zqk/state/ holding the current session ID (one line).
const sessionStateFile = "session"

// sessionLockFile is the lock file for cross-process consistency when reading/writing session state.
const sessionLockFile = "session.lock"

// lastSessionTouchFile stores the last time we touched the session (RFC3339); used to throttle touches.
const lastSessionTouchFile = "last_session_touch"

// sessionTouchThrottleWindow limits how often we persist a session touch when many invocations share the same session.
const sessionTouchThrottleWindow = 5 * time.Second

// sessionFileLockTimeout bounds blocking flock acquisition for session state so one stuck peer
// cannot wedge other CLI processes indefinitely (advisory locks; fail fast on contention).
const sessionFileLockTimeout = 5 * time.Second

// defaultIdleTimeout is used when session.idle_timeout is not set or invalid.
const defaultIdleTimeout = 24 * time.Hour

const (
	zqkSessionKind   = objects.KindZqkSession
	zqkSessionPrefix = "ZQK"
	zqkStatusActive  = "active"
	zqkStatusDone    = "completed"
)

// sessionIDKey is the context key for the current CLI zqk_session ID.
type sessionIDKey struct{}

// StartZqkSession creates a zqk_session with status "active" and returns its ID.
// accountID ties the session to that user account (use pkgctx.SystemAccountID when no user). Title is optional.
// sp must be the app-provided storage (from context/cache); do not create storage here.
func StartZqkSession(ctx context.Context, projectRoot, title, accountID string, sp storage.ObjectStorageProvider) string {
	if projectRoot == EmptyValue || sp == nil {
		return ""
	}
	if accountID == EmptyValue {
		accountID = pkgctx.SystemAccountID
	}
	processDir := paths.ResolvePathFromCacheOrConstant(projectRoot, "process", paths.ProcessDir)
	dirName := objects.GetDirectoryFromKind(zqkSessionKind)
	if dirName == EmptyValue {
		return ""
	}
	sessionsDir := filepath.Join(processDir, dirName)
	if err := os.MkdirAll(sessionsDir, paths.DirPerm755); err != nil {
		return ""
	}
	gen := idgen.GetBatchIDGenerator(ctx, sessionsDir, zqkSessionKind, zqkSessionPrefix, 3, 1)

	sessionID, err := gen.GenerateNextID()
	if err != nil {
		return ""
	}
	registry := validation.GetNamespaceRegistry()
	namespaceID := registry.GetNamespaceForKind(zqkSessionKind)
	builder := bldr_instance_v1.NewZqkSessionInstanceBuilder(objects.DefaultSchemaVersion)
	builder.SetID(sessionID).SetStatus(zqkStatusActive).SetField(objects.FieldKeyNamespaceID, namespaceID)
	builder.SetAccountId(accountID).SetField(objects.FieldKeyCreatedBy, accountID).SetField(objects.FieldKeyUpdatedBy, accountID)
	if title != EmptyValue {
		builder.SetTitle(title)
	}
	instance, err := builder.Build()
	if err != nil {
		return ""
	}
	secCtx := &pkgctx.SecurityContext{AccountID: accountID}
	if err := sp.Create(ctx, secCtx, instance); err != nil {
		return ""
	}
	return sessionID
}

// EndZqkSession updates the zqk_session to the given status ("completed" or "error").
// accountID is the actor ending the session (use pkgctx.SystemAccountID when no user).
func EndZqkSession(ctx context.Context, projectRoot, sessionID, status, accountID string, sp storage.ObjectStorageProvider) {
	if projectRoot == EmptyValue || sessionID == EmptyValue || status == EmptyValue || sp == nil {
		return
	}
	if accountID == EmptyValue {
		accountID = pkgctx.SystemAccountID
	}
	secCtx := &pkgctx.SecurityContext{AccountID: accountID}
	now := zqktime.NowRFC3339UTC()
	updates := map[string]any{
		objects.FieldKeyStatus:    status,
		objects.FieldKeyUpdatedAt: now,
		objects.FieldKeyUpdatedBy: accountID,
	}
	_ = sp.Update(ctx, secCtx, sessionID, updates)
}

// TouchSession updates the session's updated_at and optional title; keeps status active for reuse.
// accountID is the actor touching the session (use pkgctx.SystemAccountID when no user).
func TouchSession(ctx context.Context, projectRoot, sessionID, title, accountID string, sp storage.ObjectStorageProvider) {
	if projectRoot == EmptyValue || sessionID == EmptyValue || sp == nil {
		return
	}
	if accountID == EmptyValue {
		accountID = pkgctx.SystemAccountID
	}
	secCtx := &pkgctx.SecurityContext{AccountID: accountID}
	now := zqktime.NowRFC3339UTC()
	updates := map[string]any{
		objects.FieldKeyUpdatedAt: now,
		objects.FieldKeyUpdatedBy: accountID,
	}
	if title != EmptyValue {
		updates[objects.FieldKeyTitle] = title
	}
	_ = sp.Update(ctx, secCtx, sessionID, updates)
}

// TouchSessionIfNotThrottled updates the session's updated_at and title only if the last touch
// (for this project, under the session lock) was more than sessionTouchThrottleWindow ago.
// This prevents a storm of WAL updates when many CLI invocations (e.g. a delete loop) reuse the same session.
func TouchSessionIfNotThrottled(ctx context.Context, projectRoot, sessionID, title, accountID string, sp storage.ObjectStorageProvider) {
	if projectRoot == EmptyValue || sessionID == EmptyValue || sp == nil {
		return
	}
	fl, err := storage.NewFileLock(sessionLockPath(projectRoot))
	if err != nil {
		TouchSession(ctx, projectRoot, sessionID, title, accountID, sp)
		return
	}
	defer func() { _ = fl.Close() }()
	var shouldTouch bool
	_ = fl.WithLock(func() error {
		touchPath := lastSessionTouchPath(projectRoot)
		data, err := os.ReadFile(touchPath)
		if err != nil {
			shouldTouch = true
		} else {
			lastStr := strings.TrimSpace(string(data))
			if lastStr == EmptyValue {
				shouldTouch = true
			} else if last, parseErr := time.Parse(time.RFC3339, lastStr); parseErr != nil {
				shouldTouch = true
			} else if time.Since(last) >= sessionTouchThrottleWindow {
				shouldTouch = true
			}
		}
		if shouldTouch {
			dir := filepath.Dir(touchPath)
			if mkErr := os.MkdirAll(dir, paths.DirPerm755); mkErr != nil {
				return nil
			}
			now := zqktime.NowRFC3339UTC()
			_ = os.WriteFile(touchPath, []byte(now+"\n"), paths.FilePerm600)
		}
		return nil
	})
	if shouldTouch {
		TouchSession(ctx, projectRoot, sessionID, title, accountID, sp)
	}
}

func sessionStateDir(projectRoot string) string {
	return paths.ResolvePathFromCacheOrConstant(projectRoot, "state", filepath.Join(paths.ProjectDataDir, paths.StateDir))
}

// sessionStatePath returns the path to the persisted session file under projectRoot.
func sessionStatePath(projectRoot string) string {
	return filepath.Join(sessionStateDir(projectRoot), sessionStateFile)
}

// sessionLockPath returns the path to the session state lock file (ensures consistency across processes/terminals).
func sessionLockPath(projectRoot string) string {
	return filepath.Join(sessionStateDir(projectRoot), sessionLockFile)
}

// lastSessionTouchPath returns the path to the last-session-touch timestamp file (throttle state).
func lastSessionTouchPath(projectRoot string) string {
	return filepath.Join(sessionStateDir(projectRoot), lastSessionTouchFile)
}

// readSessionFileUnderLock reads the session ID from .zqk/state/session while holding the session lock.
// Returns "" on any error. Caller must not hold other locks that could deadlock with session lock.
func readSessionFileUnderLock(projectRoot string) string {
	if projectRoot == EmptyValue {
		return ""
	}
	fl, err := storage.NewFileLock(sessionLockPath(projectRoot))
	if err != nil {
		return ""
	}
	defer func() { _ = fl.Close() }()
	var id string
	_ = fl.WithLockTimeout(sessionFileLockTimeout, func() error {
		data, err := os.ReadFile(sessionStatePath(projectRoot))
		if err != nil {
			return err
		}
		id = strings.TrimSpace(string(data))
		return nil
	})
	return id
}

// writeSessionFileUnderLock writes the session ID to .zqk/state/session while holding the session lock.
// Returns false on error. Ensures .zqk/state exists.
func writeSessionFileUnderLock(projectRoot, sessionID string) bool {
	if projectRoot == EmptyValue || sessionID == EmptyValue {
		return false
	}
	dir := sessionStateDir(projectRoot)
	if err := os.MkdirAll(dir, paths.DirPerm755); err != nil {
		return false
	}
	fl, err := storage.NewFileLock(sessionLockPath(projectRoot))
	if err != nil {
		return false
	}
	defer func() { _ = fl.Close() }()
	var writeErr error
	_ = fl.WithLockTimeout(sessionFileLockTimeout, func() error {
		writeErr = os.WriteFile(sessionStatePath(projectRoot), []byte(sessionID+"\n"), paths.FilePerm600)
		return writeErr
	})
	return writeErr == nil
}

// clearSessionFileUnderLock removes .zqk/state/session while holding the session lock.
// Returns false on error (e.g. lock failed); missing file is not an error.
func clearSessionFileUnderLock(projectRoot string) bool {
	if projectRoot == EmptyValue {
		return false
	}
	fl, err := storage.NewFileLock(sessionLockPath(projectRoot))
	if err != nil {
		return false
	}
	defer func() { _ = fl.Close() }()
	_ = fl.WithLockTimeout(sessionFileLockTimeout, func() error {
		_ = os.Remove(sessionStatePath(projectRoot))
		return nil
	})
	return true
}

// GetCurrentSessionID returns the session ID from ZQK_SESSION_ID (if set) or from .zqk/state/session (with lock).
// Env takes precedence so a shell that exported ZQK_SESSION_ID ties that process tree to the session.
func GetCurrentSessionID(projectRoot string) string {
	if id := strings.TrimSpace(os.Getenv(zqkenv.SessionID())); id != EmptyValue {
		return id
	}
	return readSessionFileUnderLock(projectRoot)
}

// ReadPersistedSessionID returns the current session ID from file only (with lock). Prefer GetCurrentSessionID for CLI.
func ReadPersistedSessionID(projectRoot string) string {
	return readSessionFileUnderLock(projectRoot)
}

// WritePersistedSessionID writes the session ID to .zqk/state/session under lock so other terminals can reuse it.
func WritePersistedSessionID(projectRoot, sessionID string) {
	writeSessionFileUnderLock(projectRoot, sessionID)
}

// ClearPersistedSession removes the persisted session file under lock (e.g. on logout).
func ClearPersistedSession(projectRoot string) {
	clearSessionFileUnderLock(projectRoot)
}

// GetSessionIdleTimeout returns session.idle_timeout from project config, or defaultIdleTimeout.
func GetSessionIdleTimeout(projectRoot string) time.Duration {
	for _, rel := range []string{
		filepath.Join(paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile),
		filepath.Join(paths.ProjectDataDir, paths.ProjectConfigFile),
	} {
		p := filepath.Join(projectRoot, rel)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var cfg map[string]any
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			continue
		}
		session, _ := cfg["session"].(map[string]any)
		if session == nil {
			continue
		}
		s, _ := session["idle_timeout"].(string)
		if s == EmptyValue {
			continue
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			continue
		}
		if d > 0 {
			return d
		}
	}
	return defaultIdleTimeout
}

// TryReuseSession loads the current session ID (env or file with lock); if it exists, is active, belongs to accountID,
// and is not idle-expired, touches it and returns (id, true). Otherwise returns ("", false).
// accountID ties reuse to that user (use pkgctx.SystemAccountID when no user); sessions for other accounts are not reused.
func TryReuseSession(ctx context.Context, projectRoot, title, accountID string, sp storage.ObjectStorageProvider) (sessionID string, reused bool) {
	if projectRoot == EmptyValue || sp == nil {
		return "", false
	}
	if accountID == EmptyValue {
		accountID = pkgctx.SystemAccountID
	}
	id := GetCurrentSessionID(projectRoot)
	if id == EmptyValue {
		return "", false
	}
	secCtx := &pkgctx.SecurityContext{AccountID: accountID}
	existing, err := sp.Read(ctx, secCtx, id)
	if err != nil {
		return "", false
	}
	if status, _ := existing[objects.FieldKeyStatus].(string); status != zqkStatusActive {
		return "", false
	}
	// Only reuse if the session belongs to the current account
	if sid, _ := existing[objects.FieldKeyAccountID].(string); sid != EmptyValue && sid != accountID {
		return "", false
	}
	// Idle timeout: if updated_at is too old, close session and clear state so next run gets a new session
	idleTimeout := GetSessionIdleTimeout(projectRoot)
	if idleTimeout > 0 {
		updatedAtStr, _ := existing[objects.FieldKeyUpdatedAt].(string)
		if updatedAtStr != EmptyValue {
			updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
			if err == nil && time.Since(updatedAt) > idleTimeout {
				EndZqkSession(ctx, projectRoot, id, zqkStatusDone, accountID, sp)
				clearSessionFileUnderLock(projectRoot)
				return "", false
			}
		}
	}
	// Do not touch here: root PersistentPostRunE touches once per command. Touching in both PreRun and PostRun produced duplicate WAL entries.
	return id, true
}

// GetZqkSessionIDFromContext returns the current CLI zqk_session ID from context, or "".
func GetZqkSessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(sessionIDKey{}).(string); ok {
		return id
	}
	return ""
}

// WithZqkSessionID attaches the session ID to the context.
func WithZqkSessionID(ctx context.Context, sessionID string) context.Context {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}
