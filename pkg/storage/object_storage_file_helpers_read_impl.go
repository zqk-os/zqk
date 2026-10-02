package storage

import (
	"context"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	idgen "github.com/zqk-os/zqk/pkg/storage/id_generation"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/crud"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
	"gopkg.in/yaml.v3"
)

func (f *FileObjectStorage) readObjectFile(ctx context.Context, filePath string) (map[string]any, error) {
	// Route through I/O queue (on-demand workers, deadlock prevention)
	return f.readObjectFileViaQueue(ctx, filePath)
}

func unmarshalAndVerifyYAMLObject(data []byte) (map[string]any, error) {
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, errfmt.Newf(ErrMsgParseYAML).Wrap(err)
	}
	if err := crud.VerifyEmbeddedChecksum(data, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// readObjectFileFast reads an object from a YAML file or from a stream segment (path with "::offset").
// Use from List bulk reads to meet sub-second target for 5k items; avoids per-file Sync and queue overhead.
func (f *FileObjectStorage) readObjectFileFast(filePath string) (map[string]any, error) {
	if segmentPath, offset, ok := StreamPathAndOffset(filePath); ok && segmentPath != emptyValue {
		return ReadRecordAt(segmentPath, offset)
	}
	base := filepath.Base(filePath)
	hash := strings.TrimSuffix(base, filepath.Ext(base))
	isHashFile := filecas.CasHashFilenameRe.MatchString(base)

	if isHashFile {
		if cached, ok := GetGlobalParseCache().Get(hash); ok && cached != nil && cached.Raw != nil {
			cloned := make(map[string]any, len(cached.Raw))
			for k, v := range cached.Raw {
				cloned[k] = v
			}
			return cloned, nil
		}
	}

	var data []byte
	var err error
	if cached, ok := filecas.LookupCASBlob(hash); ok {
		data = cached
	} else {
		data, err = fileutil.ReadFileGated(filePath)
		if fileutil.IsNotExist(err) {
			return nil, ErrObjectNotFound
		}
		if err != nil {
			return nil, errfmt.Newf(ErrMsgReadFile).Wrap(err)
		}
		filecas.StoreCASBlob(hash, data)
	}

	obj, err := unmarshalAndVerifyYAMLObject(data)
	if err != nil {
		return nil, err
	}
	if isHashFile {
		if parsed, parseErr := objects.ParseObject(obj); parseErr == nil && parsed != nil {
			GetGlobalParseCache().Put(hash, parsed)
		}
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
	file, err := fileutil.OpenFile(filePath, fileutil.O_RDONLY, 0)
	if fileutil.IsNotExist(err) {
		return nil, ErrObjectNotFound
	}
	if err != nil {
		return nil, errfmt.Newf(ErrMsgOpenFile).Wrap(err)
	}
	defer file.Close()

	// Sync the directory to ensure file system metadata is up to date
	// This helps ensure we see the latest file state
	dir := filepath.Dir(filePath)
	if dirFile, err := fileutil.Open(dir); err == nil {
		logging.LogSwallowedError(dirFile.Sync())
		var err_swallow_31 = dirFile.Close()
		if err_swallow_31 !=

			// Read file data directly from the open file handle
			// This bypasses some caching layers compared to os.ReadFile
			nil {
			logging.LogSwallowedError(err_swallow_31)
		}
	}

	data, err := io.ReadAll(file)
	if err != nil {
		// Fallback to fileutil.ReadFile if direct stream read fails
		data, err = fileutil.ReadFile(filePath)
		if err != nil {
			return nil, errfmt.Newf(ErrMsgReadFile).Wrap(err)
		}
	}

	return unmarshalAndVerifyYAMLObject(data)
}

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
// System () can see all fields including credential_hash and salt
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
		// These fields are only accessible to
		if k == objects.FieldKeyCredentialHash || k == objects.FieldKeySalt {
			// Do not include these fields
			continue
		}
		// Include all other fields (metadata and non-sensitive fields)
		filtered[k] = v
	}

	return filtered
}

// trackPersistenceStep tracks a step in the persistence layer using audit events
// This provides visibility into where Create/Update/Delete operations spend time or fail.
// operation: OpCreate, OpUpdate, OpDelete, or OpWriteContentAddressed/OpValidateBeforeWrite/OpValidateObject/OpGetBucketStrategyRegistry.
// step: use PersistenceStep* constants from operations.go.
func (f *FileObjectStorage) trackPersistenceStep(
	_ context.Context,
	_ *pkgctx.SecurityContext,
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

// touchProcessDirectory updates the mtime of the .zqk/process directory
// This is necessary because writing files to subdirectories only updates the
// subdirectory mtime, not the parent .zqk/process directory mtime. The cache
// staleness check relies on .zqk/process mtime, so we need to explicitly touch it.
func (f *FileObjectStorage) touchProcessDirectory() error {
	processDir := datacell.ProcessPrimaryDir(f.projectRoot)

	// Check if process directory exists
	if _, err := fileutil.Stat(processDir); fileutil.IsNotExist(err) {
		// Process directory doesn't exist - nothing to touch
		return nil
	}

	// Touch the directory by opening and closing it (this updates mtime)
	// We use O_RDONLY to avoid any permission issues
	dirFile, err := fileutil.Open(processDir)
	if err != nil {
		return errfmt.Newf(ErrMsgOpenProcessDir).Wrap(err)
	}
	defer dirFile.Close()

	// Use Chtimes to explicitly update the mtime
	now := time.Now()
	if err := fileutil.Chtimes(processDir, now, now); err != nil {
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

// getContentAddressableStorageIDs returns all IDs from the CAS index for a kind.
func (f *FileObjectStorage) getContentAddressableStorageIDs(kind string) ([]string, error) {
	cas, err := f.getContentAddressableStorage(kind)
	if err != nil {
		return nil, err
	}
	return cas.ListIDs()
}
