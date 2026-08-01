package storage

import (
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/when"

	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	idgen "github.com/lanceman/zqk/pkg/storage/id_generation"
	"gopkg.in/yaml.v3"
)

// IsTestOrTempProjectRoot returns true if projectRoot is likely a test or temp directory
// (e.g. os.TempDir() or a path containing "-test-"). Used to avoid logging at Warn when
// tests run against global singletons that are already bound to the real project root.
// Callers outside storage (e.g. check command) use this to pick test-scoped storage without
// write-behind/WAL so t.TempDir cleanup does not race with background workers.
func IsTestOrTempProjectRoot(projectRoot string) bool {
	if projectRoot == emptyValue {
		return false
	}
	clean := filepath.Clean(projectRoot)
	if strings.Contains(clean, "-test-") {
		return true
	}
	tmpDir := filepath.Clean(os.TempDir())
	if tmpDir != emptyValue && (clean == tmpDir || strings.HasPrefix(clean+string(filepath.Separator), tmpDir+string(filepath.Separator))) {
		return true
	}
	return false
}

// ============================================================================
// Permissions
// ============================================================================

// checkPermission checks if the security context has permission for the operation
// Uses shared CheckPermissionWithKindSpecialCases utility for consistency
func (f *FileObjectStorage) checkPermission(secCtx *pkgctx.SecurityContext, operation, kind string) error {
	return CheckPermissionWithKindSpecialCases(secCtx, operation, kind)
}

// ============================================================================
// Metadata
// ============================================================================

// ensureObjectMetadata ensures required metadata fields are set
// Uses shared EnsureObjectMetadata utility for consistency
func (f *FileObjectStorage) ensureObjectMetadata(obj map[string]any, secCtx *pkgctx.SecurityContext, isCreate bool) {
	adapter := &IDValidatorAdapter{IDValidator: f.idValidator}
	EnsureObjectMetadata(obj, secCtx, isCreate, adapter)
}

// kindDirectoryName resolves and validates the storage directory token for a kind.
// Guardrail: mappings must be simple directory names (e.g. "criteria"), never paths.
func (f *FileObjectStorage) kindDirectoryName(kind string) (string, error) {
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return "", errfmt.Errorf(ErrMsgUnknownKind, kind)
	}
	if err := validateKindDirectoryName(kind, dirName); err != nil {
		return "", err
	}
	return filepath.Clean(dirName), nil
}

func validateKindDirectoryName(kind, dirName string) error {
	clean := filepath.Clean(dirName)
	if clean == "." || clean == ".." || filepath.IsAbs(dirName) ||
		strings.Contains(clean, "/") || strings.Contains(clean, "\\") {
		return errfmt.Errorf(ErrMsgInvalidDirMapping, kind, dirName)
	}
	return nil
}

// ============================================================================
// Hash Operations
// ============================================================================

// calculateHash calculates SHA256 hash of file content
// This is the canonical hash calculation function - all hash calculations should use this
func (f *FileObjectStorage) calculateHash(content []byte) string {
	return CalculateSHA256Hash(content)
}

// CalculateSHA256Hash calculates SHA256 hash of content
// This is the shared utility function for all hash calculations to ensure consistency
// and prevent variance from duplicate implementations
func CalculateSHA256Hash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// VerifyContentHash verifies that content hashes to expectedHash.
// Used by all CAS read paths (FileObjectStorage discovery, ContentAddressableStorage.Read, Move).
// Returns an error only on mismatch (integrity failure).
func VerifyContentHash(content []byte, expectedHash string) error {
	actual := CalculateSHA256Hash(content)
	if actual != expectedHash {
		return errfmt.Errorf(ErrMsgHashMismatchVerify, expectedHash, actual)
	}
	return nil
}

// saveHashRegistryWithRetry saves the hash registry with retry logic
// Returns error if all retries fail - this is critical for object integrity
// filename and expectedHash are used to verify the hash was actually persisted
//
//nolint:unparam // expectedHash kept for API consistency and potential future verification
func (f *FileObjectStorage) saveHashRegistryWithRetry(registry *HashRegistry, _, _, expectedHash string) error {
	const maxAttempts = 3
	const initialDelay = 50 * time.Millisecond
	const maxDelay = 500 * time.Millisecond
	const backoffFactor = 2.0

	var lastErr error
	delay := initialDelay

	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := f.saveHashRegistry(registry)
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

// isHashRegistrySaveQueueFull reports whether err is due to the hash registry save queue being full (backpressure).
// Such failures are not user-actionable (creates/updates are rolled back and may replay from WAL); log at Warn, not Error.
func isHashRegistrySaveQueueFull(err error) bool {
	return err != nil && strings.Contains(err.Error(), ConstStreamSaveQueueIsFull)
}

// verifyHashRegistrySave verifies that the hash registry was actually saved to disk
// and that the specific hash for the filename is present and correct
// It also reloads the in-memory registry to ensure cache consistency
//
//nolint:unused // Testing/debugging helper - reserved for future use
//nolint:unparam // objectID parameter is kept for API consistency
func (f *FileObjectStorage) verifyHashRegistrySave(registry *HashRegistry, _, filename, expectedHash string) error {
	// Longer delay to ensure file system has flushed the write and directory metadata is updated
	// This helps with file system caching issues, especially on macOS which may have aggressive caching
	// The delay ensures that subsequent reads (like system check) will see the updated file
	// For system-created objects (audit_aggregation_metric, etc.), use longer delay to ensure consistency
	isSystemKind := registry.kind == objects.KindAuditAggregationMetric ||
		registry.kind == objects.KindAuditEvent ||
		registry.kind == objects.KindChangeJournalEntry ||
		registry.kind == objects.KindBaseMetric ||
		registry.kind == objects.KindCommandMetric ||
		registry.kind == objects.KindSchedulerHealthMetric ||
		registry.kind == objects.KindFileLockMetric
	delay := 100 * time.Millisecond
	if isSystemKind {
		delay = 300 * time.Millisecond // Longer delay for system objects to ensure file system consistency
	}
	time.Sleep(delay)

	// Reload the registry to verify it was written
	verifyRegistry := NewHashRegistry(registry.ctx, registry.kind, registry.dir)
	if err := verifyRegistry.Load(); err != nil {
		return errfmt.Newf(ErrMsgReloadHashReg).Wrap(err)
	}

	// Verify file exists and is readable
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

	// Reload the original registry instance to ensure in-memory cache is consistent
	// This ensures that if this registry instance is reused, it has the latest data
	if err := registry.Load(); err != nil {
		// Log but don't fail - verification already succeeded
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageObjectHelpersReloadRegistryAfterVerifyFailed).
			Kind(registry.kind).
			WithError(err).
			Log()
	}

	return nil
}

// ============================================================================
// File I/O Operations
// ============================================================================

// writeObjectFile writes an object to a YAML file
// Uses file locking (flock) to ensure exclusive access during write
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) writeObjectFile(ctx context.Context, filePath string, obj map[string]any) error {
	config := GetStorageConfig()
	return f.writeObjectFileWithPerm(ctx, filePath, obj, config.DefaultFilePerm)
}

// writeObjectFileWithPerm writes an object to a YAML file with specified permissions
// Uses file locking (flock) to ensure exclusive access during write
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) writeObjectFileWithPerm(ctx context.Context, filePath string, obj map[string]any, perm os.FileMode) error {
	data, err := f.yamlMarshalForPersistence(obj)
	if err != nil {
		return errfmt.Newf(ErrMsgMarshalYAML).Wrap(err)
	}
	return f.writeObjectFileWithPermAndData(ctx, filePath, data, perm)
}

// writeObjectFileWithPermAndData writes pre-marshaled YAML data to a file with specified permissions
// This allows callers to marshal once and use the same data for writing and hash calculation
// Uses I/O queue for deadlock prevention and load distribution
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) writeObjectFileWithPermAndData(ctx context.Context, filePath string, data []byte, perm os.FileMode) error {
	// Route through I/O queue (on-demand workers, deadlock prevention)
	return f.writeObjectFileViaQueue(ctx, filePath, data, perm)
}

// writeObjectFileViaQueue writes an object via I/O queue
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) writeObjectFileViaQueue(ctx context.Context, filePath string, data []byte, perm os.FileMode) error {
	// Ensure directory exists (must be done before enqueueing)
	dirPath := filepath.Dir(filePath)
	config := GetStorageConfig()
	if err := os.MkdirAll(dirPath, config.DefaultDirPerm); err != nil {
		return errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}
	// For keystore directory, use more restrictive permissions
	if strings.Contains(dirPath, "keystore") {
		if err := os.Chmod(dirPath, config.KeystoreDirPerm); err != nil {
			// Log but don't fail - best effort
		}
	}

	manager := GetGlobalIOQueueManager(ctx)

	// Create result channel
	resultChan := make(chan IOResult, 1)

	// Enqueue write operation
	op := &IOOperation{
		Type:     IOOperationWrite,
		FilePath: filePath,
		Data:     data,
		Perm:     perm,
		Result:   resultChan,
	}

	if err := manager.Enqueue(op); err != nil {
		// Fallback to direct write if queue is unavailable
		dsiaProvider := NewDSIAStorageProvider()
		if writeErr := dsiaProvider.AtomicWriteFile(filePath, data, perm); writeErr != nil {
			return errfmt.Errorf(ErrMsgEnqueueWriteFail, err, writeErr)
		}
		return nil
	}

	// Wait for result (with timeout)
	select {
	case result := <-resultChan:
		if result.Err != nil {
			return result.Err
		}
		return nil
	case <-time.After(5 * time.Second):
		// Fallback to direct write on timeout
		dsiaProvider := NewDSIAStorageProvider()
		if writeErr := dsiaProvider.AtomicWriteFile(filePath, data, perm); writeErr != nil {
			return errfmt.Newf(ErrMsgWriteTimeout).Wrap(writeErr)
		}
		return nil
	}
}

// readObjectFile reads an object from a YAML file
// Uses I/O queue for deadlock prevention and load distribution
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) readObjectFile(ctx context.Context, filePath string) (map[string]any, error) {
	// Route through I/O queue (on-demand workers, deadlock prevention)
	return f.readObjectFileViaQueue(ctx, filePath)
}

// readObjectFileFast reads an object from a YAML file or from a stream segment (path with "::offset").
// Use from List bulk reads to meet sub-second target for 5k items; avoids per-file Sync and queue overhead.
func (f *FileObjectStorage) readObjectFileFast(filePath string) (map[string]any, error) {
	if segmentPath, offset, ok := StreamPathAndOffset(filePath); ok && segmentPath != emptyValue {
		return ReadRecordAt(segmentPath, offset)
	}
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return nil, ErrObjectNotFound
	}
	if err != nil {
		return nil, errfmt.Newf(ErrMsgReadFile).Wrap(err)
	}
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, errfmt.Newf(ErrMsgParseYAML).Wrap(err)
	}
	if err := VerifyEmbeddedChecksum(data, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// readObjectFileViaQueue reads an object via I/O queue
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) readObjectFileViaQueue(ctx context.Context, filePath string) (map[string]any, error) {
	manager := GetGlobalIOQueueManager(ctx)

	// Create result channel
	resultChan := make(chan IOResult, 1)

	// Enqueue read operation
	op := &IOOperation{
		Type:     IOOperationRead,
		FilePath: filePath,
		Result:   resultChan,
	}

	if err := manager.Enqueue(op); err != nil {
		// Fallback to direct read if queue is unavailable
		obj, directErr := f.readObjectFileNoCache(filePath)
		if directErr != nil {
			return nil, errfmt.Errorf(ErrMsgEnqueueReadFail, err, directErr)
		}
		return obj, nil
	}

	// Wait for result (with timeout)
	select {
	case result := <-resultChan:
		if result.Err != nil {
			return nil, result.Err
		}
		return result.Obj, nil
	case <-time.After(5 * time.Second):
		// Fallback to direct read on timeout
		obj, directErr := f.readObjectFileNoCache(filePath)
		if directErr != nil {
			return nil, errfmt.Newf(ErrMsgReadTimeout).Wrap(directErr)
		}
		return obj, nil
	}
}

// readObjectFileNoCache reads an object from a YAML file bypassing OS cache
// This is used in test mode to ensure we read the latest data from disk
// It opens the file directly and reads it, which helps bypass some OS caching
func (f *FileObjectStorage) readObjectFileNoCache(filePath string) (map[string]any, error) {
	// Open file with O_RDONLY
	file, err := os.OpenFile(filePath, os.O_RDONLY, 0)
	if os.IsNotExist(err) {
		return nil, ErrObjectNotFound
	}
	if err != nil {
		return nil, errfmt.Newf(ErrMsgOpenFile).Wrap(err)
	}
	defer file.Close()

	// Sync the directory to ensure file system metadata is up to date
	// This helps ensure we see the latest file state
	dir := filepath.Dir(filePath)
	if dirFile, err := os.Open(dir); err == nil {
		var err_swallow_30 = dirFile.Sync()
		if // Best effort - helps ensure file system sees latest state
		err_swallow_30 != nil {
			logging.LogSwallowedError(err_swallow_30)
		}
		var err_swallow_31 = dirFile.Close()
		if err_swallow_31 !=

			// Read file data directly from the open file handle
			// This bypasses some caching layers compared to os.ReadFile
			nil {
			logging.LogSwallowedError(err_swallow_31)
		}
	}

	var data []byte
	stat, err := file.Stat()
	if err == nil {
		data = make([]byte, stat.Size())
		n, err := file.Read(data)
		if err != nil && err.Error() != "EOF" {
			return nil, errfmt.Newf(ErrMsgReadFile).Wrap(err)
		}
		data = data[:n]
	} else {
		// Fallback to os.ReadFile if stat fails
		data, err = os.ReadFile(filePath)
		if err != nil {
			return nil, errfmt.Newf(ErrMsgReadFile).Wrap(err)
		}
	}

	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, errfmt.Newf(ErrMsgParseYAML).Wrap(err)
	}
	if err := VerifyEmbeddedChecksum(data, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// VerifyEmbeddedChecksum verifies the sha256_checksum if present in the object
func VerifyEmbeddedChecksum(data []byte, obj map[string]any) error {
	if obj == nil {
		return nil
	}
	checksumVal, ok := obj["sha256_checksum"]
	if !ok {
		return nil
	}
	expectedChecksum, ok := checksumVal.(string)
	if !ok || expectedChecksum == "" {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var filtered []string
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "sha256_checksum:") {
			filtered = append(filtered, line)
		}
	}
	filteredData := []byte(strings.Join(filtered, "\n"))
	actualChecksum := CalculateSHA256Hash(filteredData)
	if actualChecksum != expectedChecksum {
		return errfmt.Errorf("integrity verification failed: expected checksum %s, got %s", expectedChecksum, actualChecksum)
	}
	return nil
}

// ============================================================================
// Keystore Operations
// ============================================================================

// prepareKeystoreEntry prepares a keystore entry with proper account_id and permission checks
func (f *FileObjectStorage) prepareKeystoreEntry(obj map[string]any, secCtx *pkgctx.SecurityContext) error {
	if accountID := objects.GetString(obj, objects.FieldKeyAccountID); accountID == emptyValue {
		if secCtx != nil && secCtx.AccountID != emptyValue && secCtx.AccountID != pkgctx.SystemAccountID {
			obj[objects.FieldKeyAccountID] = secCtx.AccountID
		} else {
			return errfmt.Errorf(ErrMsgAccountIDRequired)
		}
	}

	// Ensure credential_hash and salt are only set by system
	if secCtx != nil && secCtx.AccountID != pkgctx.SystemAccountID {
		if _, hasHash := obj[objects.FieldKeyCredentialHash]; hasHash {
			return errfmt.Errorf(ErrMsgPermUpdateCredHash)
		}
		if _, hasSalt := obj[objects.FieldKeySalt]; hasSalt {
			return errfmt.Errorf(ErrMsgPermUpdateSalt)
		}
	}

	return nil
}

// applyKeystoreAccessControl applies access control rules for keystore entries
// - System (account:system) can see all fields including credential_hash and salt
// - Admins can see all entries but credential_hash and salt are hidden
// - Users can only see their own entries (matching account_id) and credential_hash/salt are hidden
func (f *FileObjectStorage) applyKeystoreAccessControl(obj map[string]any, secCtx *pkgctx.SecurityContext) map[string]any {
	if secCtx == nil {
		// No security context - return empty object
		return map[string]any{
			objects.FieldKeyID:   obj[objects.FieldKeyID],
			objects.FieldKeyKind: obj[objects.FieldKeyKind],
		}
	}

	// System account can see everything
	if secCtx.AccountID == pkgctx.SystemAccountID {
		return obj
	}

	// Check if user is admin
	isAdmin := slices.Contains(secCtx.Roles, "admin")

	// Get account_id from keystore entry
	entryAccountID, _ := obj[objects.FieldKeyAccountID].(string)

	// Check if user owns this entry
	userOwnsEntry := entryAccountID != emptyValue && entryAccountID == secCtx.AccountID

	// If user doesn't own entry and is not admin, deny access
	if !userOwnsEntry && !isAdmin {
		// Return minimal object (just id and kind)
		return map[string]any{
			objects.FieldKeyID:   obj[objects.FieldKeyID],
			objects.FieldKeyKind: obj[objects.FieldKeyKind],
		}
	}

	// User owns entry or is admin - return object but hide sensitive fields
	filtered := make(map[string]any)
	for k, v := range obj {
		// Hide credential_hash and salt from all non-system users
		// These fields are only accessible to account:system
		if k == objects.FieldKeyCredentialHash || k == objects.FieldKeySalt {
			// Do not include these fields
			continue
		}
		// Include all other fields (metadata and non-sensitive fields)
		filtered[k] = v
	}

	return filtered
}

// ============================================================================
// Tracking
// ============================================================================

// trackPersistenceStep tracks a step in the persistence layer using audit events
// This provides visibility into where Create/Update/Delete operations spend time or fail.
// operation: OpCreate, OpUpdate, OpDelete, or OpWriteContentAddressed/OpValidateBeforeWrite/OpValidateObject/OpGetBucketStrategyRegistry.
// step: use PersistenceStep* constants from operations.go.
func (f *FileObjectStorage) trackPersistenceStep(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	operation string,
	step PersistenceStep,
	objMetadata map[string]any,
	stepMetadata map[string]any,
	duration time.Duration,
	err error,
) error {
	// Skip tracking if project root is not set
	if f.projectRoot == emptyValue {
		return nil
	}

	// NOTE: We do NOT check IsCreatingAuditEvent() here because:
	// 1. Persistence tracking uses structured logging, not audit events, so there's no recursion risk
	// 2. We want to track persistence steps even when audit events are being created
	// 3. The original check was to prevent audit events from creating audit events, but we're not creating audit events here

	// Use structured logging instead of audit events for persistence tracking
	// This avoids creating audit events for audit events and provides better performance
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Build metadata
	metadata := make(map[string]any, len(objMetadata)+len(stepMetadata))
	maps.Copy(metadata, objMetadata)
	for k, v := range stepMetadata {
		metadata[fmt.Sprintf("step_%s", k)] = v
	}
	metadata[objects.FieldKeyOperation] = operation
	metadata["step"] = string(step)
	metadata["duration_ns"] = duration.Nanoseconds()
	metadata["duration_ms"] = float64(duration.Nanoseconds()) / 1e6

	kind, _ := metadata[objects.FieldKeyKind].(string)
	id, _ := metadata[objects.FieldKeyID].(string)

	if err != nil {
		// "Object already exists" on check_exists is idempotent for these kinds (caller updates or retries) - log at Debug to reduce noise
		alreadyExists := step == PersistenceStepCheckExists &&
			(err == ErrObjectExists || strings.Contains(err.Error(), ErrMsgAlreadyExists))
		isIdempotentKind := kind == objects.KindAuditEvent || kind == objects.KindSchedulerJob
		if alreadyExists && isIdempotentKind {
			StorageLog(logger).Debug(LogEventStorageObjectHelpersPersistenceFailedIdempotent).
				WithError(err).
				String("operation", operation).
				String("step", string(step)).
				Kind(kind).
				String("id", id).
				String("duration", duration.String()).
				WithFields(logging.Field{Key: "metadata", Value: metadata}).
				Log()
		} else {
			StorageLog(logger).Error(LogEventStorageObjectHelpersPersistenceFailed, err).
				String("operation", operation).
				String("step", string(step)).
				Kind(kind).
				String("id", id).
				String("duration", duration.String()).
				WithFields(logging.Field{Key: "metadata", Value: metadata}).
				Log()
		}
	} else {
		// Log entry/exit at Debug to avoid flooding scheduler-events with many lines per object (one create = entry + N steps + exit).
		// Slow steps and errors still surface at Info/Error.
		if step == PersistenceStepEntry || step == PersistenceStepExit {
			StorageLog(logger).Debug(LogEventStorageObjectHelpersPersistenceStepDebug).
				String("operation", operation).
				String("step", string(step)).
				Kind(kind).
				String("id", id).
				String("duration", duration.String()).
				WithFields(logging.Field{Key: "metadata", Value: metadata}).
				Log()
		} else {
			// One-time init steps: log at Debug so they don't appear as repeated "slow" operations
			isOneTimeInit := operation == OpGetBucketStrategyRegistry && step == PersistenceStepInitialize
			// Other steps: only log if duration > 100ms or if debug is enabled
			// This reduces noise while still capturing slow operations
			when.When(func() bool { return duration > 100*time.Millisecond && !isOneTimeInit }).Then(func() {
				StorageLog(logger).Info(LogEventStorageObjectHelpersPersistenceSlowInfo).
					String("operation", operation).
					String("step", string(step)).
					Kind(kind).
					String("id", id).
					String("duration", duration.String()).
					WithFields(logging.Field{Key: "metadata", Value: metadata}).
					Log()
			}).OrElseWhen(func() bool { return duration > 100*time.Millisecond && isOneTimeInit }).Then(func() {
				StorageLog(logger).Debug(LogEventStorageObjectHelpersPersistenceOneTimeInitDebug).
					String("operation", operation).
					String("step", string(step)).
					String("duration", duration.String()).
					WithFields(logging.Field{Key: "metadata", Value: metadata}).
					Log()
			}).OrElse(func() {
				StorageLog(logger).Debug(LogEventStorageObjectHelpersPersistenceCompletedDebug).
					String("operation", operation).
					String("step", string(step)).
					Kind(kind).
					String("id", id).
					String("duration", duration.String()).
					WithFields(logging.Field{Key: "metadata", Value: metadata}).
					Log()
			}).Run()
		}
	}

	return nil
}

// ============================================================================
// Utilities
// ============================================================================

// touchProcessDirectory updates the mtime of the docs/process directory
// This is necessary because writing files to subdirectories only updates the
// subdirectory mtime, not the parent docs/process directory mtime. The cache
// staleness check relies on docs/process mtime, so we need to explicitly touch it.
func (f *FileObjectStorage) touchProcessDirectory() error {
	processDir := datacell.ProcessPrimaryDir(f.projectRoot)

	// Check if process directory exists
	if _, err := os.Stat(processDir); os.IsNotExist(err) {
		// Process directory doesn't exist - nothing to touch
		return nil
	}

	// Touch the directory by opening and closing it (this updates mtime)
	// We use O_RDONLY to avoid any permission issues
	dirFile, err := os.Open(processDir)
	if err != nil {
		return errfmt.Newf(ErrMsgOpenProcessDir).Wrap(err)
	}
	defer dirFile.Close()

	// Use Chtimes to explicitly update the mtime
	now := time.Now()
	if err := os.Chtimes(processDir, now, now); err != nil {
		return errfmt.Newf(ErrMsgTouchProcessDir).Wrap(err)
	}

	return nil
}

// generateID generates a unique ID for an object kind
// Uses thread-safe batch generator for consistency and performance
func (f *FileObjectStorage) generateID(ctx context.Context, kind string) (string, error) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Info(LogEventStorageObjectHelpersGenerateIDStartingInfo).
		Kind(kind).
		ProjectRoot(f.projectRoot).
		Log()

	// Use the new ID generation system with default sequential strategy
	generator := idgen.NewGenerator(f.idValidator, f.projectRoot)
	StorageLog(logger).Info(LogEventStorageObjectHelpersGenerateIDCreatedGeneratorInfo).
		Kind(kind).
		Log()

	// Use default sequential strategy (backward compatible with existing behavior)
	config := idgen.DefaultStrategyConfig()
	StorageLog(logger).Info(LogEventStorageObjectHelpersGenerateIDDefaultStrategyInfo).
		Kind(kind).
		Log()

	StorageLog(logger).Info(LogEventStorageObjectHelpersGenerateIDBeforeNextIDInfo).
		Kind(kind).
		Log()
	generatedID, err := generator.GenerateNextID(ctx, kind, config)
	if err != nil {
		StorageLog(logger).Error(LogEventStorageObjectHelpersGenerateIDNextFailed, err).
			Kind(kind).
			Log()
		return "", err
	}
	StorageLog(logger).Info(LogEventStorageObjectHelpersGenerateIDNextOKInfo).
		Kind(kind).
		String("generated_id", generatedID).
		Log()
	return generatedID, nil
}

// generateIDs generates multiple IDs for an object kind in a single batch
// Uses thread-safe batch generator for consistency and performance
func (f *FileObjectStorage) generateIDs(ctx context.Context, kind string, count int) ([]string, error) {
	if count <= 0 {
		return nil, errfmt.Errorf(ErrMsgCountPositive, count)
	}
	if count > 500 {
		return nil, errfmt.Errorf(ErrMsgCountExceed, count)
	}

	// Load ID patterns (only if not already loaded)
	if err := f.idValidator.LoadPatterns(); err != nil {
		return nil, errfmt.Newf(ErrMsgLoadIDPatterns).Wrap(err)
	}

	// Get valid prefixes for this kind
	prefixes := f.idValidator.GetValidPrefixes(kind)
	if len(prefixes) == 0 {
		return nil, errfmt.Errorf(ErrMsgNoIDPrefix, kind)
	}

	// Use the first prefix (most common)
	prefix := prefixes[0]
	// Remove trailing dash if present (validator returns "ITEM-", we need "BLI")
	if len(prefix) > 0 && prefix[len(prefix)-1] == '-' {
		prefix = prefix[:len(prefix)-1]
	}

	// Get directory for this kind
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return nil, errfmt.Errorf(ErrMsgUnknownKind, kind)
	}

	kindDir := filepath.Join(f.processDir, dirName)

	// Use thread-safe batch generator (consistent with audit ID generation)
	batchGenerator := idgen.GetBatchIDGenerator(ctx, kindDir, kind, prefix, 3, 1) // Default: 3 digits, start at 1
	return batchGenerator.GenerateBatchIDs(count)
}

// getContentAddressableStorageIDs returns all IDs from the CAS index for a kind.
func (f *FileObjectStorage) getContentAddressableStorageIDs(kind string) ([]string, error) {
	cas, err := f.getContentAddressableStorage(kind)
	if err != nil {
		return nil, err
	}
	return cas.ListIDs()
}

// isExpectedMissingErr checks if an error is an expected "not found" error,
// which is common during cleanup or cache invalidation and can be safely ignored
// to reduce log noise without breaking the fail-fast mandate.
func isExpectedMissingErr(err error) bool {
	if err == nil {
		return false
	}
	if os.IsNotExist(err) || err == ErrObjectNotFound {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, ErrMsgNoExist) ||
		strings.Contains(errStr, ErrMsgObjectNotFound) ||
		strings.Contains(errStr, "not in stream") ||
		strings.Contains(errStr, ErrMsgNoExist) ||
		strings.Contains(errStr, NoteStaleCASIndex) ||
		strings.Contains(errStr, ErrMsgReadFile)
}
