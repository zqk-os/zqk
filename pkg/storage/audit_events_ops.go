// Extracted from audit_events.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

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
		SessionID:  zqkenv.SessionID().Get(),
	}

	if fileStorage != nil {
		ctx = fileStorage.augmentCtxForAuditDuringWriteBehindApply(ctx)
	}

	if IsDeferAuditEvents(ctx) {
		EnqueuePendingAuditEvent(PendingAuditItem{ProjectRoot: projectRoot, SecCtx: secCtx, FileStorage: fileStorage, Options: options})
		return nil
	}

	// Test-only: skip creating delete audit event when env is set (see setup_test.go; PRI-212)
	if zqkenv.SkipDeleteAudit().Get() == "1" {
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
	operation := LogMsgLintSkipped
	if commitMessage != emptyValue {
		// Truncate commit message if too long
		msgPreview := commitMessage
		if len(msgPreview) > 100 {
			msgPreview = msgPreview[:100] + "..."
		}
		operation = fmt.Sprintf(LogFmtLintSkipped, msgPreview)
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
		SessionID:  zqkenv.SessionID().Get(),
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
		SessionID:  zqkenv.SessionID().Get(),
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
