package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
)

// WriteSystemObjectAndRegisterHash writes a system object to disk and registers its hash
// Git's approach: Calculate hash from content BEFORE writing, then trust file.Sync() succeeded
// This eliminates race conditions from reading back immediately after writing
// If fileStorage is provided and the kind uses CAS, routes through CAS instead of direct file write
//
// Do not use for stream-backed kinds (audit_event, scheduler_job, etc.) when stream storage is
// enabled—that would create YAML under docs/process. Use storage.Create or writeObjectToStorage
// so writes go to the stream. See stream_config.go and docs/architecture/STREAM_STORAGE.md §7.
func WriteSystemObjectAndRegisterHash(filePath string, data []byte, kind, kindDir, objectID string, fileStorage *FileObjectStorage) error {
	if len(data) == 0 {
		return errfmt.Errorf(ErrMsgEmptyContent, filePath)
	}
	if StreamStorageEnabledForKind(kind) {
		return errfmt.Errorf(ErrMsgStreamBacked, kind, paths.ProcessDir)
	}

	// If fileStorage is provided and this kind uses CAS, route through CAS
	if fileStorage != nil && fileStorage.usesContentAddressableStorage(kind) {
		cas, err := fileStorage.getContentAddressableStorage(kind)
		if err != nil {
			// CAS is required for kinds that use it - do not fall back to direct write
			// This ensures consistency (e.g., audit events must always use CAS)
			return errfmt.Errorf(ErrMsgNoCAS, kind, err)
		}

		// Extract bucket directory from filePath if it's different from kindDir
		// This supports bucketed storage (e.g., audit/2026-01/)
		var bucketDir string
		if filePath != emptyValue {
			fileDir := filepath.Dir(filePath)
			// If fileDir is different from kindDir, it's in a bucket
			if fileDir != kindDir && strings.HasPrefix(fileDir, kindDir) {
				bucketDir = fileDir
			}
		}

		// Use CAS for creation, passing bucket directory if applicable
		if bucketDir != emptyValue {
			if err := cas.Create(objectID, data, bucketDir); err != nil {
				return errfmt.Newf(ErrMsgCreateCAS).Wrap(err)
			}
		} else {
			if err := cas.Create(objectID, data); err != nil {
				return errfmt.Newf(ErrMsgCreateCAS).Wrap(err)
			}
		}
		// Update reverse reference index (best effort - system objects may reference other objects)
		var obj map[string]any
		if err := yaml.Unmarshal(data, &obj); err == nil {
			updateReverseReferenceIndexOnCreate(objectID, obj)
		}
		return nil
	}

	// Fall back to direct file write (for non-CAS kinds or if CAS unavailable)
	// Calculate hash from the data we're about to write (Git's approach)
	// This is the hash of what we're writing, and file.Sync() guarantees it's on disk
	hash := CalculateSHA256Hash(data)

	// Ensure directory exists
	dirPath := filepath.Dir(filePath)
	if err := os.MkdirAll(dirPath, paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}

	// Write file to disk using file handle so we can sync
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, paths.FilePerm644)
	if err != nil {
		return errfmt.Newf(ErrMsgOpenFileWrite).Wrap(err)
	}
	defer file.Close()

	// Write data
	if _, err := file.Write(data); err != nil {
		return errfmt.Newf(ErrMsgWriteFile).Wrap(err)
	}

	// Sync file to ensure write is persisted to disk
	// Git trusts this sync - no read-back verification needed
	if err := file.Sync(); err != nil {
		// Log warning but don't fail - sync is best effort
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditSyncAfterWriteFailedWarn).Path(filePath).WithError(err).Log()
	}

	// Update hash registry (use cached registry when fileStorage is provided to cap goroutine growth)
	ctx := pkgctx.NewSystemContext()
	var hashRegistry *HashRegistry
	if fileStorage != nil {
		hashRegistry = fileStorage.newHashRegistry(ctx, kind, kindDir)
	} else {
		hashRegistry = NewHashRegistry(ctx, kind, kindDir)
	}
	if hashRegistry == nil {
		return errfmt.Errorf(ErrMsgGetHashReg, kind)
	}
	if err := hashRegistry.Load(); err != nil {
		// Hash registry doesn't exist yet - will be created on first save
	}
	filename := filepath.Base(filePath)
	hashRegistry.SetHash(filename, hash)

	// Use retry logic to ensure hash registry update succeeds
	if err := saveHashRegistryWithRetryForSystemObject(hashRegistry, objectID, filename, hash); err != nil {
		return errfmt.Newf(ErrMsgUpdateHashReg).Wrap(err)
	}

	// Update reverse reference index (best effort - system objects may reference other objects)
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err == nil {
		updateReverseReferenceIndexOnCreate(objectID, obj)
	}

	return nil
}

// saveHashRegistryWithRetryForSystemObject saves the hash registry with retry logic for system-created objects
// CRITICAL: System-created objects (audit events, change journal entries) must have their hashes registered
// to prevent hash mismatches. This function ensures the hash is persisted even under concurrent access.
//
//nolint:unparam // expectedHash kept for API consistency with saveHashRegistryWithRetry
func saveHashRegistryWithRetryForSystemObject(registry *HashRegistry, _, _, expectedHash string) error {
	const maxAttempts = 3
	const initialDelay = 50 * time.Millisecond
	const maxDelay = 500 * time.Millisecond
	const backoffFactor = 2.0

	var lastErr error
	delay := initialDelay

	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := registry.Save()
		if err == nil {
			// Success - Save() already does file.Sync(), so we trust it succeeded
			// Git doesn't verify by reading back - it trusts the write succeeded
			// Reading back immediately can hit file system caching issues (especially on macOS)
			// The file.Sync() in Save() is the real guarantee, not a read-back verification
			return nil
		}
		lastErr = err

		// Last attempt, don't wait
		if attempt == maxAttempts-1 {
			break
		}

		// Wait before retry with exponential backoff
		time.Sleep(delay)
		delay = time.Duration(float64(delay) * backoffFactor)
		if delay > maxDelay {
			delay = maxDelay
		}
	}

	// All retries exhausted
	return errfmt.Errorf(ErrMsgSaveHashReg, maxAttempts, lastErr)
}

// verifyHashRegistrySaveForSystemObject verifies that the hash registry was actually saved to disk
// and that the specific hash for the filename is present and correct
//
//nolint:unused,deadcode // Testing/debugging helper - reserved for future use
//nolint:unparam // objectID parameter is kept for API consistency
func verifyHashRegistrySaveForSystemObject(registry *HashRegistry, _, filename, expectedHash string) error {
	// Delay to ensure file system has flushed the write
	time.Sleep(100 * time.Millisecond)

	// Reload the registry to verify it was written
	// Use system context for background hash registry operations
	verifyRegistry := NewHashRegistry(pkgctx.NewSystemContext(), registry.kind, registry.dir)
	if err := verifyRegistry.Load(); err != nil {
		return errfmt.Newf(ErrMsgReloadHashReg).Wrap(err)
	}

	// Verify file exists
	if verifyRegistry.filePath == emptyValue {
		return errfmt.Errorf(ErrMsgHashRegPathEmpty)
	}

	if _, err := os.Stat(verifyRegistry.filePath); err != nil {
		return errfmt.Newf(ErrMsgHashRegNoExist).Wrap(err)
	}

	// Verify the specific hash is in the reloaded registry and matches
	savedHash := verifyRegistry.GetHash(filename)
	if savedHash == emptyValue {
		return errfmt.Errorf(ErrMsgHashNotFound, filename)
	}

	if savedHash != expectedHash {
		return errfmt.Errorf(ErrMsgHashMismatch, filename, expectedHash, savedHash)
	}

	return nil
}

// Context key for CLI authorization
type cliContextKey struct{}

// IsCLIOperation checks if the context indicates this is a CLI operation
func IsCLIOperation(ctx context.Context, secCtx *pkgctx.SecurityContext) bool {
	if ctx != nil && ctx.Value(cliContextKey{}) != nil {
		return true
	}
	if secCtx != nil {
		for _, p := range secCtx.Permissions {
			if p == "bypass_policy" {
				return true
			}
		}
	}
	return false
}

// WithCLIOperation marks a context as a CLI operation.
// If ctx is nil, returns context.Background() with the flag set (avoids panic from context.WithValue(nil, ...)).
func WithCLIOperation(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, cliContextKey{}, true)
}

// createDeleteAuditEvent creates an audit event for object deletion
// fileStorage is optional - if provided and CAS is enabled, routes through CAS
// When ctx has WithDeferAuditEvents, the event is enqueued and flushed later (e.g. by BulkDeleteOptimized).
func createDeleteAuditEvent(ctx context.Context, projectRoot, id, kind, filePath string, cascade bool, secCtx *pkgctx.SecurityContext, dependents []string, fileStorage *FileObjectStorage) error {
	// CRITICAL: Use fileStorage's project root if provided and projectRoot is empty
	// This ensures deterministic project root resolution - no auto-discovery fallback
	if projectRoot == emptyValue && fileStorage != nil {
		projectRoot = fileStorage.GetProjectRoot()
	}

	if projectRoot == emptyValue {
		// Can't create audit event without project root
		// Don't fall back to auto-discovery - this causes non-deterministic behavior
		return nil // Best effort - don't fail deletion
	}

	// Get relative file path
	relPath, err := filepath.Rel(projectRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	// Build operation description
	operation := fmt.Sprintf(DescDeletedObjectFmt, id)
	if cascade && len(dependents) > 0 {
		operation = fmt.Sprintf(DescDeletedCascadeFmt, id, len(dependents))
	} else if len(dependents) > 0 {
		operation = fmt.Sprintf(DescDeletedDependentFmt, id, len(dependents))
	}

	// Determine severity based on cascade and dependencies
	severity := SeverityMedium
	if cascade && len(dependents) > 0 {
		severity = SeverityHigh // Cascade deletes are high severity
	}

	// Build metadata
	metadata := map[string]any{
		"cascade":              cascade,
		"dependents":           dependents,
		objects.FieldKeySource: AuditMetadataSourceCLI,
		"project_root":         projectRoot,
	}

	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	createdBy := pkgctx.SystemAccountID
	if secCtx != nil {
		createdBy = secCtx.AccountID
	}
	options := &AuditEventOptions{
		EventType:  EventTypeObjectDeletion,
		Operation:  operation,
		Severity:   severity,
		TargetKind: kind,
		TargetID:   id,
		TargetPath: relPath,
		Metadata:   metadata,
		CreatedBy:  createdBy,
	}

	if fileStorage != nil {
		ctx = fileStorage.augmentCtxForAuditDuringWriteBehindApply(ctx)
	}

	if IsDeferAuditEvents(ctx) {
		EnqueuePendingAuditEvent(PendingAuditItem{ProjectRoot: projectRoot, SecCtx: secCtx, FileStorage: fileStorage, Options: options})
		return nil
	}

	// Test-only: skip creating delete audit event when env is set (see setup_test.go; PLAN-212)
	if os.Getenv(zqkenv.SkipDeleteAudit()) == "1" {
		return nil
	}

	runCtx := ctx
	if runCtx == nil {
		runCtx = pkgctx.NewSystemContext()
	}
	var storageProvider ObjectStorageProvider = fileStorage
	return CreateAuditEventWithBuilder(runCtx, projectRoot, secCtx, storageProvider, options)
}

// CreateCacheRefreshAuditEvent creates an audit event for cache refresh operations
// This is a privileged operation that should be audited with full context
// Exported so it can be called from CLI layer
func CreateCacheRefreshAuditEvent(projectRoot string, secCtx *pkgctx.SecurityContext, commandArgs []string, systemState map[string]any) error {
	if projectRoot == emptyValue {
		// Can't create audit event without project root
		return nil // Best effort - don't fail operation
	}

	// Build operation description
	operation := DescCacheRefreshReq
	if len(commandArgs) > 0 {
		operation = fmt.Sprintf(DescCacheRefreshFmt, strings.Join(commandArgs, " "))
	}

	// Build comprehensive metadata with system state
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	commandStr := cliCmd + CommandRefreshCache
	if len(commandArgs) > 0 {
		commandStr = cliCmd + " " + strings.Join(commandArgs, " ")
	}

	metadata := map[string]any{
		AuditMetadataKeySource:      AuditMetadataSourceCLI,
		AuditMetadataKeyProjectRoot: projectRoot,
		AuditMetadataKeyCommand:     commandStr,
		AuditMetadataKeyArgs:        commandArgs,
		AuditMetadataKeyRoles:       secCtx.Roles,
		AuditMetadataKeyPermissions: secCtx.Permissions,
	}

	// Add system state information
	for k, v := range systemState {
		metadata[k] = v
	}

	// Use instance builder helper to create audit event
	// Use system context for background audit event creation
	ctx := pkgctx.NewSystemContext()
	options := &AuditEventOptions{
		EventType:  EventTypeSystemConfigChange, // Cache refresh is a system configuration change
		Operation:  operation,
		TargetKind: "cache",      // Target is the cache system
		Severity:   SeverityHigh, // High severity - affects validation rules
		Metadata:   metadata,
	}

	// Use cached storage provider to avoid creating a new FileObjectStorage per event (each would add
	// a write-behind worker and hash registries, causing goroutine and memory explosion under load).
	provider, err := GetGlobalStorageProviderCache().GetOrCreate(ctx, projectRoot)
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditCacheRefreshProviderFailedWarn).WithError(err).Log()
		return nil // Best effort
	}
	return CreateAuditEventWithBuilder(ctx, projectRoot, secCtx, provider, options)
}

// CreateLintBypassAuditEvent creates an audit event when lint checks are bypassed with --no-verify
// This helps track when lint warnings increase and who is not taking time to resolve lint checks
// Exported so it can be called from git hooks or wrapper scripts
func CreateLintBypassAuditEvent(projectRoot, gitUser, gitEmail, commitMessage string, stagedFiles []string) error {
	if projectRoot == emptyValue {
		// Can't create audit event without project root
		return nil // Best effort - don't fail operation
	}

	// Use git user info as actor
	actor := gitUser
	if actor == emptyValue {
		actor = ValueUnknown
	}
	if gitEmail != emptyValue {
		actor = fmt.Sprintf("%s <%s>", actor, gitEmail)
	}

	// Build operation description
	operation := LogMsgLintBypassed
	if commitMessage != emptyValue {
		// Truncate commit message if too long
		msgPreview := commitMessage
		if len(msgPreview) > 100 {
			msgPreview = msgPreview[:100] + "..."
		}
		operation = fmt.Sprintf(LogFmtLintBypassed, msgPreview)
	}

	// Count staged Go files
	goFileCount := 0
	for _, file := range stagedFiles {
		if strings.HasSuffix(file, ".go") {
			goFileCount++
		}
	}

	// Build metadata
	metadata := map[string]any{
		AuditMetadataKeySource:        AuditMetadataSourceGitHook,
		AuditMetadataKeyProjectRoot:   projectRoot,
		AuditMetadataKeyGitUser:       gitUser,
		AuditMetadataKeyGitEmail:      gitEmail,
		AuditMetadataKeyCommitMessage: commitMessage,
		AuditMetadataKeyStagedFiles:   stagedFiles,
		AuditMetadataKeyGoFileCount:   goFileCount,
		AuditMetadataKeyTotalFiles:    len(stagedFiles),
	}

	// Use instance builder helper to create audit event
	// Use system context for background audit event creation
	ctx := pkgctx.NewSystemContext()
	options := &AuditEventOptions{
		EventType:  EventTypeCodeQualityBypass,
		Operation:  operation,
		TargetKind: "commit",
		Severity:   SeverityMedium, // Medium severity - indicates potential code quality issues
		Metadata:   metadata,
		CreatedBy:  actor,
	}

	// Create security context for the actor
	secCtx := pkgctx.NewSystemSecurityContext()
	secCtx.AccountID = actor

	// Use cached storage provider to avoid creating a new FileObjectStorage per event.
	provider, err := GetGlobalStorageProviderCache().GetOrCreate(ctx, projectRoot)
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditLintBypassProviderFailedWarn).WithError(err).Log()
		return nil // Best effort
	}
	return CreateAuditEventWithBuilder(ctx, projectRoot, secCtx, provider, options)
}

// createCreateAuditEvent creates an audit event for object creation.
// ctx is passed through so bulk-create callers can defer CAS flush (e.g. scan-tests job generation).
// fileStorage is optional - if provided and CAS is enabled, routes through CAS
//
//nolint:unparam // Always returns nil error - audit events are best-effort
func createCreateAuditEvent(ctx context.Context, projectRoot, id, kind, filePath string, secCtx *pkgctx.SecurityContext, fileStorage *FileObjectStorage) error {
	// CRITICAL: Use fileStorage's project root if provided and projectRoot is empty
	// This ensures deterministic project root resolution - no auto-discovery fallback
	if projectRoot == emptyValue && fileStorage != nil {
		projectRoot = fileStorage.GetProjectRoot()
	}

	if projectRoot == emptyValue {
		// Can't create audit event without project root
		// Don't fall back to auto-discovery - this causes non-deterministic behavior
		return nil // Best effort - don't fail creation
	}

	// Prevent infinite recursion: don't create audit events for audit events
	// This prevents writeObjectToCAS -> createCreateAuditEvent -> CreateAuditEventWithBuilder -> Create -> writeObjectToCAS loop
	// Also check context to prevent cycles even if kind is different
	if kind == objects.KindAuditEvent || IsCreatingAuditEvent() {
		return nil // Best effort - skip audit events for audit events or during audit event creation
	}

	// Get relative file path
	relPath, err := filepath.Rel(projectRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	// Build operation description
	operation := fmt.Sprintf(LogFmtCreatedObject, id)

	// Build metadata
	metadata := map[string]any{
		AuditMetadataKeySource:      AuditMetadataSourceCLI,
		AuditMetadataKeyProjectRoot: projectRoot,
	}

	// Pass caller ctx so bulk-create can defer CAS flush for audit_event
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	createdBy := pkgctx.SystemAccountID
	if secCtx != nil {
		createdBy = secCtx.AccountID
	}
	options := &AuditEventOptions{
		EventType:  EventTypeObjectCreation,
		Operation:  operation,
		Severity:   SeverityLow, // Object creation is typically low severity
		TargetKind: kind,
		TargetID:   id,
		TargetPath: relPath,
		Metadata:   metadata,
		CreatedBy:  createdBy,
	}

	ctx = fileStorage.augmentCtxForAuditDuringWriteBehindApply(ctx)

	if IsDeferAuditEvents(ctx) {
		if secCtx == nil {
			secCtx = pkgctx.NewSystemSecurityContext()
		}
		EnqueuePendingAuditEvent(PendingAuditItem{ProjectRoot: projectRoot, SecCtx: secCtx, FileStorage: fileStorage, Options: options})
		return nil
	}

	// Use the provided fileStorage if available (file backend)
	var storageProvider ObjectStorageProvider = fileStorage
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	return CreateAuditEventWithBuilder(ctx, projectRoot, secCtx, storageProvider, options)
}

// createUpdateAuditEvent creates an audit event for object updates
// fileStorage is optional - if provided and CAS is enabled, routes through CAS
// When ctx has WithDeferAuditEvents, the event is enqueued and flushed later.
//
//nolint:unparam // Always returns nil error - audit events are best-effort
func createUpdateAuditEvent(ctx context.Context, projectRoot, id, kind, filePath string, secCtx *pkgctx.SecurityContext, changedFields []string, fileStorage *FileObjectStorage) error {
	// CRITICAL: Use fileStorage's project root if provided and projectRoot is empty
	if projectRoot == emptyValue && fileStorage != nil {
		projectRoot = fileStorage.GetProjectRoot()
	}

	if projectRoot == emptyValue {
		return nil // Best effort - don't fail update
	}

	relPath, err := filepath.Rel(projectRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	operation := fmt.Sprintf(LogFmtUpdatedObject, id)
	when.When(func() bool { return len(changedFields) > 0 && len(changedFields) <= 3 }).Then(func() {
		operation = fmt.Sprintf(LogFmtUpdatedObjectDetail, id, strings.Join(changedFields, ", "))
	}).OrElseWhen(func() bool { return len(changedFields) > 3 }).Then(func() {
		operation = fmt.Sprintf(LogFmtUpdatedObjectMore, id, strings.Join(changedFields[:3], ", "), len(changedFields)-3)
	}).Run()

	metadata := map[string]any{
		AuditMetadataKeySource:        AuditMetadataSourceCLI,
		AuditMetadataKeyProjectRoot:   projectRoot,
		AuditMetadataKeyChangedFields: changedFields,
	}

	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	createdBy := pkgctx.SystemAccountID
	if secCtx != nil {
		createdBy = secCtx.AccountID
	}
	options := &AuditEventOptions{
		EventType:  EventTypeObjectUpdate,
		Operation:  operation,
		Severity:   SeverityLow,
		TargetKind: kind,
		TargetID:   id,
		TargetPath: relPath,
		Metadata:   metadata,
		CreatedBy:  createdBy,
	}

	if fileStorage != nil {
		ctx = fileStorage.augmentCtxForAuditDuringWriteBehindApply(ctx)
	}

	if IsDeferAuditEvents(ctx) {
		EnqueuePendingAuditEvent(PendingAuditItem{ProjectRoot: projectRoot, SecCtx: secCtx, FileStorage: fileStorage, Options: options})
		return nil
	}

	runCtx := ctx
	if runCtx == nil {
		runCtx = pkgctx.NewSystemContext()
	}
	var storageProvider ObjectStorageProvider = fileStorage
	return CreateAuditEventWithBuilder(runCtx, projectRoot, secCtx, storageProvider, options)
}

// hasAdminRole checks if the security context has admin role
//
//nolint:unused,deadcode // Helper function - reserved for future use
func hasAdminRole(secCtx *pkgctx.SecurityContext) bool {
	if secCtx == nil {
		return false
	}
	return slices.Contains(secCtx.Roles, "admin")
}
