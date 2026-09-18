// Package zqksession manages persisted ZQK session lifecycle state.
package zqksession

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	enumzqksession "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/zqk_session"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/zqk-os/zqk/pkg/storage"
	idgen "github.com/zqk-os/zqk/pkg/storage/id_generation"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	EmptyValue = ""

	SessionTypeCLI         = "cli"
	SessionTypeAgentWorker = "agent_worker"
	StatusActive           = "active"
	StatusCompleted        = "completed"
	StatusError            = "error"
	ExecutorTypeAgentX     = "agentx"

	sessionStateFile     = "session"
	sessionLockFile      = "session.lock"
	lastSessionTouchFile = "last_session_touch"
	zqkSessionPrefix     = "ZQK"
	sessionConfigKey     = "session"
	idleTimeoutConfigKey = "idle_timeout"
)

const (
	sessionTouchThrottleWindow = 5 * time.Second
	sessionFileLockTimeout     = 5 * time.Second
	defaultIdleTimeout         = 24 * time.Hour
)

// sessionStorageSecCtx returns a SecurityContext authorized for zqk_session storage I/O.
// TRACK: BLI-1785905136581480000-1f317f44 — remove when authenticated account contexts
// always carry the permissions needed for session lifecycle writes.
func sessionStorageSecCtx(ctx context.Context) *pkgctx.SecurityContext {
	if sec := pkgctx.GetSecurityContext(ctx); sec != nil {
		if err := storage.CheckPermission(sec, "write", objects.KindZqkSession); err == nil {
			return sec
		}
	}
	return pkgctx.NewSystemSecurityContext()
}

// StartCLISession creates an active CLI session and returns its ID.
func StartCLISession(
	ctx context.Context,
	projectRoot, title, accountID string,
	sp storage.ObjectStorageProvider,
) string {
	sessionID, err := createSession(
		ctx,
		projectRoot,
		title,
		accountID,
		enumzqksession.SessionTypeCli,
		sp,
		nil,
	)
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(logger).Warn("Failed to create zqk_session object").WithError(err).Log()
		return EmptyValue
	}
	return sessionID
}

func createSession(
	ctx context.Context,
	projectRoot, title, accountID string,
	sessionType enumzqksession.SessionType,
	sp storage.ObjectStorageProvider,
	configure func(*bldr_instance_v1.ZqkSessionInstanceBuilder),
) (string, error) {
	if projectRoot == EmptyValue {
		return EmptyValue, fmt.Errorf("project root is required")
	}
	if sp == nil {
		return EmptyValue, fmt.Errorf("storage provider is required")
	}
	if accountID == EmptyValue {
		accountID = pkgctx.SystemAccountID
	}

	processDir := paths.ResolvePathFromCacheOrConstant(projectRoot, "process", paths.ProcessDir)
	dirName := objects.GetDirectoryFromKind(objects.KindZqkSession)
	if dirName == EmptyValue {
		return EmptyValue, fmt.Errorf("directory is not registered for kind %s", objects.KindZqkSession)
	}
	sessionsDir := filepath.Join(processDir, dirName)
	if err := fileutil.MkdirAll(sessionsDir, paths.DirPerm755); err != nil {
		return EmptyValue, fmt.Errorf("create session directory: %w", err)
	}

	casProvider := storage.GetCASIDProvider(sp, objects.KindZqkSession)
	gen := idgen.GetBatchIDGeneratorWithCAS(ctx, sessionsDir, objects.KindZqkSession, zqkSessionPrefix, 3, 1, casProvider)

	namespaceID := validation.GetNamespaceRegistry().GetNamespaceForKind(objects.KindZqkSession)
	const maxCreateAttempts = 10
	var lastErr error
	for attempt := 0; attempt < maxCreateAttempts; attempt++ {
		sessionID, err := gen.GenerateNextID()
		if err != nil {
			return EmptyValue, fmt.Errorf("generate session ID: %w", err)
		}

		builder := bldr_instance_v1.NewZqkSessionInstanceBuilder(objects.DefaultSchemaVersion)
		builder.SetID(sessionID).SetStatus(StatusActive).SetField(objects.FieldKeyNamespaceID, namespaceID)
		builder.SetAccountId(accountID)
		builder.SetField(objects.FieldKeyCreatedBy, accountID).SetField(objects.FieldKeyUpdatedBy, accountID)
		builder.SetSessionType(sessionType)
		if title != EmptyValue {
			builder.SetTitle(title)
		}
		if configure != nil {
			configure(builder)
		}

		instance, err := builder.Build()
		if err != nil {
			return EmptyValue, fmt.Errorf("build zqk_session: %w", err)
		}
		createCtx := pkgctx.WithPromoteOnCreate(ctx)
		if err := sp.Create(createCtx, sessionStorageSecCtx(ctx), instance); err != nil {
			lastErr = err
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			return EmptyValue, fmt.Errorf("create zqk_session: %w", err)
		}
		return sessionID, nil
	}
	return EmptyValue, fmt.Errorf("create zqk_session: %w", lastErr)
}

// End updates a session to the supplied lifecycle status.
func End(
	ctx context.Context,
	projectRoot, sessionID, status, accountID string,
	sp storage.ObjectStorageProvider,
) {
	if projectRoot == EmptyValue || sessionID == EmptyValue || status == EmptyValue || sp == nil {
		return
	}
	if accountID == EmptyValue {
		accountID = pkgctx.SystemAccountID
	}
	updates := map[string]any{
		objects.FieldKeyStatus:    status,
		objects.FieldKeyUpdatedAt: zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy: accountID,
	}
	_ = sp.Update(ctx, sessionStorageSecCtx(ctx), sessionID, updates)
}

// Touch refreshes a session's updated timestamp and optional title.
func Touch(
	ctx context.Context,
	projectRoot, sessionID, title, accountID string,
	sp storage.ObjectStorageProvider,
) {
	if projectRoot == EmptyValue || sessionID == EmptyValue || sp == nil {
		return
	}
	if accountID == EmptyValue {
		accountID = pkgctx.SystemAccountID
	}
	updates := map[string]any{
		objects.FieldKeyUpdatedAt: zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy: accountID,
	}
	if title != EmptyValue {
		updates[objects.FieldKeyTitle] = title
	}
	_ = sp.Update(ctx, sessionStorageSecCtx(ctx), sessionID, updates)
}

// TouchIfNotThrottled touches a session at most once per throttle window.
func TouchIfNotThrottled(
	ctx context.Context,
	projectRoot, sessionID, title, accountID string,
	sp storage.ObjectStorageProvider,
) {
	if projectRoot == EmptyValue || sessionID == EmptyValue || sp == nil {
		return
	}
	fl, err := storage.NewFileLock(sessionLockPath(projectRoot))
	if err != nil {
		Touch(ctx, projectRoot, sessionID, title, accountID, sp)
		return
	}
	defer func() { _ = fl.Close() }()

	var shouldTouch bool
	_ = fl.WithLock(func() error {
		touchPath := lastSessionTouchPath(projectRoot)
		data, err := fileutil.ReadFile(touchPath)
		if err != nil {
			shouldTouch = true
		} else {
			lastValue := strings.TrimSpace(string(data))
			if lastValue == EmptyValue {
				shouldTouch = true
			} else if last, parseErr := time.Parse(time.RFC3339, lastValue); parseErr != nil {
				shouldTouch = true
			} else if time.Since(last) >= sessionTouchThrottleWindow {
				shouldTouch = true
			}
		}
		if shouldTouch {
			if err := fileutil.MkdirAll(filepath.Dir(touchPath), paths.DirPerm755); err == nil {
				_ = fileutil.WriteFile(touchPath, []byte(zqktime.NowRFC3339UTC()+"\n"), paths.FilePerm600)
			}
		}
		return nil
	})
	if shouldTouch {
		Touch(ctx, projectRoot, sessionID, title, accountID, sp)
	}
}

func sessionStateDir(projectRoot string) string {
	return paths.ResolvePathFromCacheOrConstant(
		projectRoot,
		"state",
		filepath.Join(paths.ProjectDataDir, paths.StateDir),
	)
}

func sessionStatePath(projectRoot string) string {
	return filepath.Join(sessionStateDir(projectRoot), sessionStateFile)
}

func sessionLockPath(projectRoot string) string {
	return filepath.Join(sessionStateDir(projectRoot), sessionLockFile)
}

func lastSessionTouchPath(projectRoot string) string {
	return filepath.Join(sessionStateDir(projectRoot), lastSessionTouchFile)
}

func readSessionFileUnderLock(projectRoot string) string {
	if projectRoot == EmptyValue {
		return EmptyValue
	}
	fl, err := storage.NewFileLock(sessionLockPath(projectRoot))
	if err != nil {
		return EmptyValue
	}
	defer func() { _ = fl.Close() }()

	var sessionID string
	_ = fl.WithLockTimeout(sessionFileLockTimeout, func() error {
		data, err := fileutil.ReadFile(sessionStatePath(projectRoot))
		if err != nil {
			return err
		}
		sessionID = strings.TrimSpace(string(data))
		return nil
	})
	return sessionID
}

func writeSessionFileUnderLock(projectRoot, sessionID string) bool {
	if projectRoot == EmptyValue || sessionID == EmptyValue {
		return false
	}
	dir := sessionStateDir(projectRoot)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return false
	}
	fl, err := storage.NewFileLock(sessionLockPath(projectRoot))
	if err != nil {
		return false
	}
	defer func() { _ = fl.Close() }()

	var writeErr error
	_ = fl.WithLockTimeout(sessionFileLockTimeout, func() error {
		writeErr = fileutil.WriteFile(sessionStatePath(projectRoot), []byte(sessionID+"\n"), paths.FilePerm600)
		return writeErr
	})
	return writeErr == nil
}

func clearSessionFileUnderLock(projectRoot string) {
	if projectRoot == EmptyValue {
		return
	}
	fl, err := storage.NewFileLock(sessionLockPath(projectRoot))
	if err != nil {
		return
	}
	defer func() { _ = fl.Close() }()
	_ = fl.WithLockTimeout(sessionFileLockTimeout, func() error {
		_ = fileutil.Remove(sessionStatePath(projectRoot))
		return nil
	})
}

// GetCurrentID returns the environment session ID, or the persisted session ID.
func GetCurrentID(projectRoot string) string {
	if sessionID := strings.TrimSpace(zqkenv.SessionID().Get()); sessionID != EmptyValue {
		return sessionID
	}
	return readSessionFileUnderLock(projectRoot)
}

// ReadPersistedID returns only the persisted session ID.
func ReadPersistedID(projectRoot string) string {
	return readSessionFileUnderLock(projectRoot)
}

// WritePersistedID writes the CLI reuse session ID unless that would clobber another account's active session.
func WritePersistedID(
	ctx context.Context,
	projectRoot, sessionID, accountID string,
	sp storage.ObjectStorageProvider,
) bool {
	if projectRoot == EmptyValue || sessionID == EmptyValue || sp == nil {
		return false
	}
	existingID := readSessionFileUnderLock(projectRoot)
	if existingID != EmptyValue && existingID != sessionID {
		existing, err := sp.Read(ctx, sessionStorageSecCtx(ctx), existingID)
		if err == nil {
			if status, _ := existing[objects.FieldKeyStatus].(string); status == StatusActive {
				if ownerID, _ := existing[objects.FieldKeyAccountID].(string); ownerID != EmptyValue && ownerID != accountID {
					return false
				}
			}
		}
	}
	return writeSessionFileUnderLock(projectRoot, sessionID)
}

// ClearPersisted removes the CLI reuse session state.
func ClearPersisted(projectRoot string) {
	clearSessionFileUnderLock(projectRoot)
}

// GetIdleTimeout reads the configured session idle timeout.
func GetIdleTimeout(projectRoot string) time.Duration {
	configPaths := []string{
		filepath.Join(paths.ProjectDataDir, paths.ConfigDir, paths.ProjectConfigFile),
		filepath.Join(paths.ProjectDataDir, paths.ProjectConfigFile),
	}
	for _, rel := range configPaths {
		data, err := fileutil.ReadFile(filepath.Join(projectRoot, rel))
		if err != nil {
			continue
		}
		var cfg map[string]any
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			continue
		}
		session, _ := cfg[sessionConfigKey].(map[string]any)
		idleTimeout, _ := session[idleTimeoutConfigKey].(string)
		if idleTimeout == EmptyValue {
			continue
		}
		duration, err := time.ParseDuration(idleTimeout)
		if err == nil && duration > 0 {
			return duration
		}
	}
	return defaultIdleTimeout
}

// TryReuse returns an active, account-owned CLI session when it has not expired.
func TryReuse(
	ctx context.Context,
	projectRoot, title, accountID string,
	sp storage.ObjectStorageProvider,
) (string, bool) {
	if projectRoot == EmptyValue || sp == nil {
		return EmptyValue, false
	}
	if accountID == EmptyValue {
		accountID = pkgctx.SystemAccountID
	}
	sessionID := GetCurrentID(projectRoot)
	if sessionID == EmptyValue {
		return EmptyValue, false
	}
	existing, err := sp.Read(ctx, sessionStorageSecCtx(ctx), sessionID)
	if err != nil {
		return EmptyValue, false
	}
	if status, _ := existing[objects.FieldKeyStatus].(string); status != StatusActive {
		return EmptyValue, false
	}
	if ownerID, _ := existing[objects.FieldKeyAccountID].(string); ownerID != EmptyValue && ownerID != accountID {
		return EmptyValue, false
	}
	if idleTimeout := GetIdleTimeout(projectRoot); idleTimeout > 0 {
		updatedAtValue, _ := existing[objects.FieldKeyUpdatedAt].(string)
		if updatedAtValue != EmptyValue {
			updatedAt, parseErr := time.Parse(time.RFC3339, updatedAtValue)
			if parseErr == nil && time.Since(updatedAt) > idleTimeout {
				End(ctx, projectRoot, sessionID, StatusCompleted, accountID, sp)
				clearSessionFileUnderLock(projectRoot)
				return EmptyValue, false
			}
		}
	}
	return sessionID, true
}
