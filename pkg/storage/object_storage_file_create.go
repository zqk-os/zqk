package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	"github.com/lanceman/zqk/pkg/when"
	"gopkg.in/yaml.v3"
)

// bulkCreateCtxKey is the context key for deferring listing-index flush during bulk job creation.
type bulkCreateCtxKey struct{}

// WithBulkCreateDeferFlush marks the context so Create() skips per-object listing-index FlushKind for
// scheduler_job and audit_event. Caller must call FlushKind("scheduler_job") and
// FlushKind("audit_event") once after the bulk create loop (e.g. in GenerateJobs).
func WithBulkCreateDeferFlush(ctx context.Context) context.Context {
	return context.WithValue(ctx, bulkCreateCtxKey{}, true)
}

func deferListingIndexFlushForBulkCreate(ctx context.Context, kind string) bool {
	v, ok := ctx.Value(bulkCreateCtxKey{}).(bool)
	if !ok || !v {
		return false
	}
	return kind == objects.KindSchedulerJob || kind == objects.KindAuditEvent
}

// syncCreateForSchedulerJobKey is the context key for synchronous create of scheduler_job
// so the new job is visible to LoadJobs in the same process (avoids write-behind race).
type syncCreateForSchedulerJobKey struct{}

// WithSyncCreateForSchedulerJob marks the context so Create() for kind scheduler_job
// skips write-behind and applies to storage immediately. Used by the scheduler when
// ensuring SCH-101 and other critical jobs so LoadJobs sees them on first load.
func WithSyncCreateForSchedulerJob(ctx context.Context) context.Context {
	return context.WithValue(ctx, syncCreateForSchedulerJobKey{}, true)
}

func syncCreateForSchedulerJob(ctx context.Context) bool {
	v, ok := ctx.Value(syncCreateForSchedulerJobKey{}).(bool)
	return ok && v
}

// syncCreateForKindKey is the context key for synchronous create of a specific kind (used by migrate-legacy-to-stream).
type syncCreateForKindKey struct{}

// WithSyncCreateForKind marks the context so Create() for the given kind skips write-behind and applies to storage immediately.
// Used when migrating legacy YAML in docs/architecture/<dir> to stream so each object is visible after create.
func WithSyncCreateForKind(ctx context.Context, kind string) context.Context {
	return context.WithValue(ctx, syncCreateForKindKey{}, kind)
}

func syncCreateForKind(ctx context.Context) string {
	v, _ := ctx.Value(syncCreateForKindKey{}).(string)
	return v
}

//nolint:gocyclo
func (f *FileObjectStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	startTime := time.Now()

	// Track persistence layer path - always log entry for visibility
	kind, _ := obj[objects.FieldKeyKind].(string)
	id, _ := obj[objects.FieldKeyID].(string)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	if f.projectRoot == emptyValue {
		StorageLog(logger).Warn(LogEventStorageObjectCreateEmptyProjectRoot).
			Kind(kind).
			ObjectID(id).
			Log()
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepEntry, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, 0, nil); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Normalize status (kind-aware) so we accept variants and persist the preferred value for this kind
			Error(ErrMsgSwallowedError, err).Log()
	}

	if s := objects.GetString(obj, objects.FieldKeyStatus); s != emptyValue {
		if loader := f.GetLifecycleLoader(); loader != nil {
			if canonical, err := loader.NormalizeStatusForKind(kind, s); err == nil {
				obj[objects.FieldKeyStatus] = canonical
			} else {
				obj[objects.FieldKeyStatus] = objects.NormalizeStatus(s)
			}
		} else {
			obj[objects.FieldKeyStatus] = objects.NormalizeStatus(s)
		}
	}

	// Validate and prepare object
	stepStart := time.Now()
	var err error
	kind, err = f.validateAndPrepareObjectForCreation(ctx, obj, secCtx)
	if err != nil {
		StorageLog(logger).Error(LogEventStorageObjectCreateValidatePrepareFailed, err).
			Kind(kind).
			ObjectID(id).
			Log()
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepValidateAndPrepare, map[string]any{objects.FieldKeyKind: kind}, nil, time.Since(stepStart), err); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepValidateAndPrepare, map[string]any{objects.FieldKeyKind: kind}, nil, time.Since(stepStart), nil); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Ensure object has valid ID
			Error(ErrMsgSwallowedError, err).Log()
	}

	stepStart = time.Now()
	id, err = f.ensureObjectID(ctx, obj, kind)
	if err != nil {
		StorageLog(logger).Error(LogEventStorageObjectCreateEnsureObjectIDFailed, err).
			Kind(kind).
			ObjectID(id).
			Log()
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepEnsureID, map[string]any{objects.FieldKeyKind: kind}, nil, time.Since(stepStart), err); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepEnsureID, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), nil); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Prepare file path and directory
			Error(ErrMsgSwallowedError, err).Log()
	}

	stepStart = time.Now()
	filePath, err := f.prepareObjectPath(id, kind)
	if err != nil {
		StorageLog(logger).Error(LogEventStorageObjectCreatePreparePathFailed, err).
			Kind(kind).
			ObjectID(id).
			Log()
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepPreparePath, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), err); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepPreparePath, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id, objects.FieldKeyFilePath: filePath}, nil, time.Since(stepStart), nil); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Check if object already exists
			Error(ErrMsgSwallowedError, err).Log()
	}

	stepStart = time.Now()
	if err := f.checkObjectExists(id, kind, filePath); err != nil {
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepCheckExists, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), err); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepCheckExists, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), nil); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Validate object before writing
			Error(ErrMsgSwallowedError, err).Log()
	}

	stepStart = time.Now()
	if err := f.validateObjectBeforeCreation(ctx, obj, kind, secCtx); err != nil {
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepValidateBeforeWriteStep, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), err); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepValidateBeforeWriteStep, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), nil); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Marshal object to YAML
			Error(ErrMsgSwallowedError, err).Log()
	}

	stepStart = time.Now()
	data, err := f.marshalObjectForCreation(obj, filePath)
	if err != nil {
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepMarshal, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), err); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepMarshal, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id, "data_size": len(data)}, nil, time.Since(stepStart), nil); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Write-behind path (Option B): enqueue to WAL + buffer and return; worker persists in background.
			// Skip for scheduler_job when WithSyncCreateForSchedulerJob(ctx), or for any kind when WithSyncCreateForKind(ctx, kind).
			// Lock order: walMu before ObjectWriteBuffer.mu (see LOCK_ORDERING.md).
			Error(ErrMsgSwallowedError, err).Log()
	}

	stepStart = time.Now()
	skipWriteBehind := (kind == objects.KindSchedulerJob && syncCreateForSchedulerJob(ctx)) || (syncCreateForKind(ctx) != emptyValue && kind == syncCreateForKind(ctx)) || isSkipWriteBehind(ctx)
	var appendErr error
	var didAppend bool
	if !skipWriteBehind && f.writeBuf != nil && f.wal != nil {
		if err := concurrency.RunInLockWithLogger(&f.walMu, locknames.LockNameCreateAppendWal, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			didAppend = true
			appendErr = AppendToWALAndBuffer(f.wal, f.writeBuf, "create", kind, id, data, true)
			return nil
		}); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGen, err).Log()
		}
	}
	if didAppend {
		if appendErr != nil {
			if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepWriteToStorage, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), appendErr); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			return appendErr
		}
		f.writeBehindWorker.Notify()
		RecordObjectStateChange(ctx, OpCreate, id)

		// Stream-backed objects MUST be written synchronously since WAL drops their payload
		// The background worker will see it as a no-op due to stream registry idempotency
		if StreamStorageEnabledForKind(kind) && f.usesContentAddressableStorage(kind) {
			if streamErr := f.writeObjectToStream(ctx, id, kind, data); streamErr != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
					Error(ErrMsgSwallowedError, streamErr).Log()
			}
		}

		executeChangeNotification(ctx, OpCreate, kind, id, obj)

		actualPath, err := f.getObjectFilePath(id, kind)
		if err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if actualPath == emptyValue {
			actualPath = filePath
		}

		// Create change journal entry for creation
		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, actualPath, OpCreate, nil, obj, secCtx, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Create audit event for the creation (pass ctx so bulk-create can defer CAS flush)
				//nolint:errcheck // Intentional error ignored - audit events are best effort
				Error(ErrMsgSwallowedError, err).Log()
		}

		if err := createCreateAuditEvent(ctx, f.projectRoot, id, kind, actualPath, secCtx, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}

		totalDuration := time.Since(startTime)
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepWriteToStorage, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id, "write_behind": true}, nil, time.Since(stepStart), nil); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepExit, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"success": true}, totalDuration, nil); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return nil
	}

	// Write object to storage (CAS or file-based)
	stepStart = time.Now()
	if err := f.writeObjectToStorage(ctx, id, kind, filePath, data, secCtx); err != nil {
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepWriteToStorage, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id, "uses_cas": f.usesContentAddressableStorage(kind)}, nil, time.Since(stepStart), err); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepWriteToStorage, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id, "uses_cas": f.usesContentAddressableStorage(kind)}, nil, time.Since(stepStart), nil); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Flush per-kind listing index so List() sees the new object (same or other process).
			// Skip when stream storage was used (no index update). During bulk create, flush is deferred.
			Error(ErrMsgSwallowedError, err).Log()
	}

	if f.usesContentAddressableStorage(kind) && !StreamStorageEnabledForKind(kind) && f.projectRoot != emptyValue && !deferListingIndexFlushForBulkCreate(ctx, kind) {
		stepFlush := time.Now()
		flushErr := GetListingIndexWriteQueueForProjectRoot(f.projectRoot).FlushKindContext(ctx, kind, IndexFlushAfterCreateTimeout)
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepIndexFlush, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepFlush), flushErr); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if flushErr != nil {
			StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
				Warn(LogEventStorageObjectCreateListingIndexFlushFailed).
				Kind(kind).
				ObjectID(id).
				WithError(flushErr).
				Log()
		}
	}
	// Invalidate list cache for stream-backed kinds so List() sees the new object (same as Update path).
	if StreamStorageEnabledForKind(kind) {
		f.InvalidateCachesForKind(kind)
	}

	// Perform post-creation operations
	stepStart = time.Now()
	finalErr := f.finalizeObjectCreation(ctx, id, kind, filePath, secCtx)
	totalDuration := time.Since(startTime)
	if finalErr != nil {
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepFinalize, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), finalErr); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Always log exit (even if fast) - critical for understanding flow
				Error(ErrMsgSwallowedError, err).Log()
		}

		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepExit, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"success": false}, totalDuration, finalErr); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return finalErr
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepFinalize, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), nil); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Update reverse reference index (best effort - don't fail create if this fails)
			Error(ErrMsgSwallowedError, err).Log()
	}

	updateReverseReferenceIndexOnCreate(id, obj)

	actualPath, err := f.getObjectFilePath(id, kind)
	if err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}
	if actualPath == emptyValue {
		actualPath = filePath
	}

	// Create change journal entry for creation
	if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, actualPath, OpCreate, nil, obj, secCtx, f); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// ITEM-643: Notify subscribers of object creation (best-effort)
			Error(ErrMsgSwallowedError, err).Log()
	}

	executeChangeNotification(ctx, OpCreate, kind, id, obj)

	// Always log exit (even if fast) - critical for understanding flow
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepExit, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"success": true}, totalDuration, nil); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Error(ErrMsgSwallowedError, err).Log()
	}
	return nil
}

func (f *FileObjectStorage) validateAndPrepareObjectForCreation(ctx context.Context, obj map[string]any, secCtx *pkgctx.SecurityContext) (string, error) {
	kind, ok := obj[objects.FieldKeyKind].(string)
	if !ok || kind == emptyValue {
		return "", errfmt.Errorf(ErrMsgObjectNeedsKind)
	}

	if err := f.checkPermission(secCtx, "write", kind); err != nil {
		return "", err
	}

	// For keystore_entry, ensure account_id is set from security context if not provided
	keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
	if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
		if err := f.prepareKeystoreEntry(obj, secCtx); err != nil {
			return "", err
		}
	}

	return kind, nil
}

// ensureObjectID ensures object has a valid ID, generating one if needed
func (f *FileObjectStorage) ensureObjectID(ctx context.Context, obj map[string]any, kind string) (string, error) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	id, ok := obj[objects.FieldKeyID].(string)
	if !ok || id == emptyValue {
		StorageLog(logger).Debug(LogEventStorageObjectCreateEnsureNoIDGenerating).
			Kind(kind).
			Log()

		// For CAS-enabled kinds, use timestamp-based IDs because CAS uses hash-based filenames
		// and sequential ID generation can't scan the directory to find the next sequence number
		if f.usesContentAddressableStorage(kind) {
			// Get ID prefix from validator (strict - no fallback); LoadPatterns waits if another goroutine is loading
			if err := f.idValidator.LoadPatterns(); err != nil {
				return "", errfmt.Newf(ErrMsgLoadIDPatterns).Wrap(err)
			}
			prefixes := f.idValidator.GetValidPrefixes(kind)
			if len(prefixes) == 0 {
				return "", errfmt.Errorf(ErrMsgNoValidIDPrefix, kind)
			}
			// Use first prefix (most common case)
			prefix := prefixes[0]

			// Generate unique timestamp-based ID with random component to prevent collisions
			// Even with nanosecond precision, concurrent goroutines can generate IDs at the same nanosecond
			// Adding a small random component (4 bytes = 8 hex chars) ensures uniqueness
			// Format: PREFIX-timestamp-random (e.g., BAS-1768909936457275000-a1b2c3d4)
			baseTime := time.Now().UnixNano()
			randomBytes := make([]byte, 4) // 4 bytes = 8 hex characters
			if _, err := rand.Read(randomBytes); err != nil {
				// Fallback: use timestamp with nanosecond delay if random fails
				StorageLog(logger).Warn(LogEventStorageObjectCreateEnsureRandomComponentFailed).
					Kind(kind).
					WithError(err).
					Log()
				id = fmt.Sprintf("%s%d", prefix, baseTime)
				obj[objects.FieldKeyID] = id
			} else {
				randomHex := hex.EncodeToString(randomBytes)
				generatedID := fmt.Sprintf("%s%d-%s", prefix, baseTime, randomHex)
				StorageLog(logger).Debug(LogEventStorageObjectCreateEnsureGeneratedCASID).
					Kind(kind).
					String("generated_id", generatedID).
					Log()
				id = generatedID
				obj[objects.FieldKeyID] = id
			}
		} else {
			// Use thread-safe batch generator for non-CAS kinds (consistent with audit IDs and other sequential IDs)
			generatedID, err := f.generateID(ctx, kind)
			if err != nil {
				StorageLog(logger).Error(LogEventStorageObjectCreateEnsureGenerateIDFailed, err).
					Kind(kind).
					Log()
				return "", errfmt.Newf(ErrMsgGenerateID).Wrap(err)
			}
			StorageLog(logger).Debug(LogEventStorageObjectCreateEnsureIDGeneratedOK).
				Kind(kind).
				String("generated_id", generatedID).
				Log()
			id = generatedID
			obj[objects.FieldKeyID] = id
		}
	}

	// Legacy churn ids: SCH-<ts>-scheduler-job-<parent> chains when parent was already a long SCH-* id.
	// Normalize to a short hash-based id so we never persist unbounded recursive names.
	if kind == objects.KindSchedulerJob {
		if normalized := normalizeSchedulerJobIDIfRecursive(kind, id); normalized != id {
			StorageLog(logger).Warn(LogEventStorageObjectCreateEnsureNormalizedRecursiveSchedulerJob).
				Kind(kind).
				String("previous_id", id).
				String("normalized_id", normalized).
				Log()
			id = normalized
			obj[objects.FieldKeyID] = id
		}
	}

	// scheduler_job ids are used as lock filenames; filesystems have NAME_MAX ~255. Reject long ids at create so we never persist them.
	const maxSchedulerJobIDLen = 200
	if kind == objects.KindSchedulerJob && len(id) > maxSchedulerJobIDLen {
		return "", errfmt.Errorf(ErrMsgSchedulerJobIDLength, maxSchedulerJobIDLen, len(id))
	}

	// Validate ID format (strict - must pass validation); LoadPatterns waits if another goroutine is loading
	if err := f.idValidator.LoadPatterns(); err != nil {
		return "", errfmt.Newf(ErrMsgLoadIDPatternsValidation).Wrap(err)
	}
	valid, err := f.idValidator.ValidateID(id, kind)
	if err != nil {
		return "", errfmt.Newf(ErrMsgValidateID).Wrap(err)
	}
	if !valid {
		return "", errfmt.Errorf(ErrMsgInvalidIDFormat, kind, id)
	}

	return id, nil
}

// prepareObjectPath prepares the file path and ensures directory exists.
// For CAS kinds, we build the legacy path directly so Create does not call getObjectFilePath
// for non-existing objects (getObjectFilePath returns an error when the object is not found and kind dir exists).
func (f *FileObjectStorage) prepareObjectPath(id, kind string) (string, error) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return "", errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
	}
	config := GetStorageConfig()
	if err := os.MkdirAll(kindDir, config.DefaultDirPerm); err != nil {
		StorageLog(logger).Error(LogEventStorageObjectCreatePreparePathMkdirFailed, err).
			Kind(kind).
			ObjectID(id).
			String("dir_path", kindDir).
			Log()
		return "", errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}
	var filePath string
	if f.usesContentAddressableStorage(kind) {
		accountDir := objects.GetDirectoryFromKind(objects.KindAccount)
		when.When(func() bool {
			return accountDir == "accounts" && objects.GetDirectoryFromKind(kind) == accountDir && strings.HasPrefix(id, "account:")
		}).Then(func() {
			username := strings.TrimPrefix(id, "account:")
			filePath = filepath.Join(kindDir, fmt.Sprintf("account-%s%s", username, config.YAMLExtension))
		}).OrElse(func() {
			filePath = filepath.Join(kindDir, fmt.Sprintf("%s%s", id, config.YAMLExtension))
		}).Run()
	} else {
		var err error
		filePath, err = f.getObjectFilePath(id, kind)
		if err != nil {
			StorageLog(logger).Error(LogEventStorageObjectCreatePreparePathGetObjectPathFailed, err).
				Kind(kind).
				ObjectID(id).
				Log()
			return "", err
		}
	}
	dirPath := filepath.Dir(filePath)
	if err := os.MkdirAll(dirPath, config.DefaultDirPerm); err != nil {
		StorageLog(logger).Error(LogEventStorageObjectCreatePreparePathMkdirFailed, err).
			Kind(kind).
			ObjectID(id).
			String("dir_path", dirPath).
			Log()
		return "", errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}
	// For keystore, use more restrictive permissions
	keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
	if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
		if err := os.Chmod(filepath.Dir(filePath), config.KeystoreDirPerm); err != nil {
			// Log but don't fail - best effort
		}
	}
	return filePath, nil
}

// checkObjectExists checks if object already exists (stream registry, CAS, or file-based).
// For stream-backed kinds, only the stream registry is authoritative so Create can succeed when the ID
// exists only in legacy CAS/file and not in the stream (e.g. ensure-retention-jobs creating fixed-ID jobs).
func (f *FileObjectStorage) checkObjectExists(id, kind, filePath string) error {
	// Check for blocking issues before write operations (except for automated kinds)
	// Note: This requires context, but we'll pass it through the main Create method
	// For now, we'll skip this check here and do it in validateObjectBeforeCreation

	// For stream-backed kinds, existence is defined only by the stream registry (List reads from there).
	// Do not use CAS or legacy file path so Create can add the ID to the stream when it's missing there.
	if StreamStorageEnabledForKind(kind) {
		if f.getStreamLocation(id, kind) != emptyValue {
			return ErrObjectExists
		}
		return nil
	}

	// For CAS-enabled kinds, check the CAS index instead of file path
	if f.usesContentAddressableStorage(kind) {
		cas, err := f.getContentAddressableStorage(kind)
		if err == nil {
			_, hashErr := cas.GetHashForID(id)
			if hashErr == nil {
				_, pathErr := cas.GetFilePathForID(id)
				if pathErr == nil {
					return ErrObjectExists
				}
				// Stale index: ID maps to a hash whose file is gone — allow Create to repair.
				return nil
			}
		}
		// If CAS check fails, fall through to file-based check as backup
	}

	// For non-CAS kinds, check file existence
	if _, err := os.Stat(filePath); err == nil {
		return ErrObjectExists
	}

	return nil
}

// marshalObjectForCreation marshals object to YAML for writing
func (f *FileObjectStorage) marshalObjectForCreation(obj map[string]any, filePath string) ([]byte, error) {
	// Marshal object to YAML before writing
	// Git's approach: Calculate hash from content BEFORE writing, then trust the write
	// This eliminates race conditions from reading back immediately after writing
	data, err := f.yamlMarshalForPersistence(obj)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgMarshalObjCreation).Wrap(err)
	}

	if len(data) == 0 {
		return nil, errfmt.Errorf(ErrMsgEmptyContent, filePath)
	}

	return data, nil
}

// writeObjectToStorage writes object to storage (stream, CAS, or file-based).
// When stream storage is enabled for the kind, writes to append-only segment (no CAS overhead).
func (f *FileObjectStorage) writeObjectToStorage(ctx context.Context, id, kind, filePath string, data []byte, secCtx *pkgctx.SecurityContext) error {
	// Stream path: append-only segment + index/cache update (no hash, no hash registry)
	if StreamStorageEnabledForKind(kind) && f.usesContentAddressableStorage(kind) {
		return f.writeObjectToStream(ctx, id, kind, data)
	}
	// CAS path
	if f.usesContentAddressableStorage(kind) {
		return f.writeObjectToCAS(ctx, id, kind, filePath, data, secCtx)
	}
	return f.writeObjectToFile(ctx, id, kind, filePath, data)
}

// writeObjectToStream appends the object to the kind's stream segment and updates stream registry + high-volume cache.
// No CAS, no hash registry; retention uses the same index contract (Count, OldestIDs, IDsOlderThan) via cache.
func (f *FileObjectStorage) writeObjectToStream(ctx context.Context, id, kind string, data []byte) error {
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return errfmt.Newf(ErrMsgStreamUnmarshalObj).Wrap(err)
	}

	var createdAt time.Time
	if s := objects.GetString(obj, objects.FieldKeyCreatedAt); s != emptyValue {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			createdAt = t
		}
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	// If this object already has a registered stream location, avoid appending another
	// duplicate record for the same (id, kind). Update the high-volume cache entry only.
	if existingLoc := f.getStreamLocation(id, kind); existingLoc != emptyValue {
		cache := GetGlobalHighVolumeEventCache()
		if cache != nil {
			eventType, _ := obj[objects.FieldKeyEventType].(string)
			status, _ := obj[objects.FieldKeyStatus].(string)
			segmentPath, _, ok := StreamPathAndOffset(existingLoc)
			mtime := time.Now().UTC()
			if ok {
				if info, err := os.Stat(segmentPath); err == nil {
					mtime = info.ModTime()
				}
			}
			entry := &HighVolumeEventCacheEntry{
				ID:        id,
				Kind:      kind,
				CreatedAt: createdAt,
				EventType: eventType,
				Status:    status,
				FilePath:  existingLoc,
				MTime:     mtime,
				Exists:    true,
			}
			cache.Set(entry)
		}
		return nil
	}

	segmentPath, offset, err := AppendToStream(f.projectRoot, kind, id, obj, createdAt)
	if err != nil {
		return err
	}
	loc := FormatStreamLocation(segmentPath, offset)
	f.setStreamLocation(id, kind, loc)
	if err := AppendStreamLocationToRegistry(f.projectRoot, kind, id, loc); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Update high-volume cache so retention and count use same index contract (Count, OldestIDs, IDsOlderThan)
			Error(ErrMsgSwallowedError, err).Log()
	}

	cache := GetGlobalHighVolumeEventCache()
	if cache != nil {
		eventType, _ := obj[objects.FieldKeyEventType].(string)
		status, _ := obj[objects.FieldKeyStatus].(string)
		mtime := time.Now().UTC()
		if info, err := os.Stat(segmentPath); err == nil {
			mtime = info.ModTime()
		}
		entry := &HighVolumeEventCacheEntry{
			ID:        id,
			Kind:      kind,
			CreatedAt: createdAt,
			EventType: eventType,
			Status:    status,
			FilePath:  loc,
			MTime:     mtime,
			Exists:    true,
		}
		cache.Set(entry)
	}
	return nil
}

// writeObjectToCAS writes object using content-addressable storage
// Uses bucket strategies to determine bucket directory (robust, works with any strategy)
func (f *FileObjectStorage) writeObjectToCAS(ctx context.Context, id, kind, filePath string, data []byte, secCtx *pkgctx.SecurityContext) error {
	casStart := time.Now()
	cas, err := f.getContentAddressableStorage(kind)
	if err != nil {
		if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepGetContentAddressedStorage, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(casStart), err); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return errfmt.Newf(ErrMsgGetCAS).Wrap(err)
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepGetContentAddressedStorage, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(casStart), nil); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Parse object data to determine bucket directory using bucket strategy
			// This is robust and works with any bucket strategy (chronological, categorical, state-based, composite, etc.)
			Error(ErrMsgSwallowedError, err).Log()
	}

	var bucketDir string
	var obj map[string]any
	bucketStrategyStart := time.Now()
	if err := yaml.Unmarshal(data, &obj); err == nil {
		// Get bucket strategy for this kind
		registryStart := time.Now()
		registry := f.getBucketStrategyRegistry(ctx)
		registryDuration := time.Since(registryStart)
		if registryDuration > 100*time.Millisecond {
			if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepGetBucketStrategyReg, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"duration_ms": float64(registryDuration.Nanoseconds()) / 1e6}, registryDuration, nil); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
		}
		if registry != nil {
			strategyStart := time.Now()
			strategy, err := registry.GetStrategyForKind(ctx, kind)
			strategyDuration := time.Since(strategyStart)
			if strategyDuration > 100*time.Millisecond {
				if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepGetStrategyForKind, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"duration_ms": float64(strategyDuration.Nanoseconds()) / 1e6}, strategyDuration, err); err != nil && !isExpectedMissingErr(err) {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
				}
			}
			if err == nil && strategy != nil {
				// Calculate bucket key from object data
				bucketKey := strategy.GetBucketKey(obj, filePath)
				if bucketKey != emptyValue {
					// Get kind directory
					kindDir := f.GetKindDir(kind)
					if kindDir != "" {
						// Use strategy to get bucket directory (handles nested structures, composite keys, etc.)
						bucketDir = strategy.GetBucketDirectory(kindDir, bucketKey)
					}
				}
			}
		}
	}
	bucketStrategyDuration := time.Since(bucketStrategyStart)
	if bucketStrategyDuration > 100*time.Millisecond {
		if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepBucketStrategyLookup, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"duration_ms": float64(bucketStrategyDuration.Nanoseconds()) / 1e6, "bucket_dir": bucketDir}, bucketStrategyDuration, nil); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Fallback: for chrono-bucketed kinds, if strategy returned no bucket (e.g. during registry init),
				// derive bucket from created_at so we never write to the top-level kind dir (e.g. audit/).
				Error(ErrMsgSwallowedError, err).Log()
		}
	}

	if bucketDir == emptyValue && obj != nil {
		if kind == objects.KindAuditEvent || kind == objects.KindChangeJournalEntry {
			if createdAt, _ := obj[objects.FieldKeyCreatedAt].(string); createdAt != emptyValue {
				if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
					month := t.Format("2006-01")
					kindDir := f.GetKindDir(kind)
					if kindDir != "" {
						bucketDir = filepath.Join(kindDir, month)
					}
				}
			}
		}
	}

	// Use content-addressable storage for Create, passing bucket directory if applicable
	// CAS handles hash calculation, file writing, and index updates
	casCreateStart := time.Now()
	if bucketDir != emptyValue {
		if err := cas.Create(id, data, bucketDir); err != nil {
			if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepContentAddressedPut, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"bucket_dir": bucketDir}, time.Since(casCreateStart), err); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			return errfmt.Newf(ErrMsgCreateCAS).Wrap(err)
		}
	} else {
		if err := cas.Create(id, data); err != nil {
			if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepContentAddressedPut, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(casCreateStart), err); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			return errfmt.Newf(ErrMsgCreateCAS).Wrap(err)
		}
	}
	casCreateDuration := time.Since(casCreateStart)
	if casCreateDuration > 100*time.Millisecond {
		if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepContentAddressedPut, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"duration_ms": float64(casCreateDuration.Nanoseconds()) / 1e6, "bucket_dir": bucketDir}, casCreateDuration, nil); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// For keystore entries, set file permissions to restrictive after CAS creates the file
				Error(ErrMsgSwallowedError, err).Log()
		}
	}

	config := GetStorageConfig()
	keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
	if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
		hashFilePath, err := cas.GetFilePathForID(id)
		if err == nil {
			// Set restrictive permissions for keystore files
			if err := os.Chmod(hashFilePath, config.KeystoreFilePerm); err != nil {
				// Log but don't fail - best effort
			}
		}
	}

	// Register hash in the hash registry for integrity-check compatibility.
	// The CAS write above already succeeded; the object is visible via List().
	// If the hash registry save fails (e.g. transient I/O error), log a warning and
	// continue — the object IS created. `system check --auto-fix` repairs the missing
	// hash entry on the next run. Do NOT roll back the create here.
	hash := CalculateSHA256Hash(data)
	kindDir := f.GetKindDir(kind)
	if kindDir != "" {
		hashRegistry := f.newHashRegistry(ctx, kind, kindDir)
		// Load existing registry (ignore error if file doesn't exist yet)
		if err := hashRegistry.Load(); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Hash registry key: use path base (for CAS, prepareObjectPath may still pass a placeholder path; registry keys by object ID for integrity lookup).
				Error(ErrMsgSwallowedError, err).Log()
		}

		filename := filepath.Base(filePath)
		hashRegistry.SetHash(filename, hash)
		if err := f.saveHashRegistryWithRetry(hashRegistry, id, filename, hash); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectCreateHashRegistrySaveRetryExceeded).
				WithError(err).
				Kind(kind).
				ObjectID(id).
				Log()
			// Do not return error: CAS create succeeded; hash entry will be repaired by system check.
		}
	}

	// Audit event is created in finalizeObjectCreation (single place for both sync and write-behind apply).
	// Do not call createCreateAuditEvent here or we double-count (every create would emit two audit events).

	return nil
}

// writeObjectToFile writes object to file-based storage
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) writeObjectToFile(ctx context.Context, id, kind, filePath string, data []byte) error {
	// Calculate hash from the data we're about to write (Git's approach)
	hash := f.calculateHash(data)

	// Write file
	config := GetStorageConfig()
	filePerm := config.DefaultFilePerm
	keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
	if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
		filePerm = config.KeystoreFilePerm
	}

	if err := f.writeObjectFileWithPermAndData(ctx, filePath, data, filePerm); err != nil {
		return err
	}

	// Update hash registry (required for integrity checks)
	hashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(filePath))
	if err := hashRegistry.Load(); err != nil {
		// Hash registry doesn't exist yet - will be created on first save
	}

	filename := filepath.Base(filePath)
	hashRegistry.SetHash(filename, hash)

	// Save hash registry with retry (critical for integrity - must succeed)
	if err := f.saveHashRegistryWithRetry(hashRegistry, id, filename, hash); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		if isHashRegistrySaveQueueFull(err) {
			StorageLog(logger).Warn(LogEventStorageObjectCreateHashRegistryQueueFullRollback).
				WithError(err).
				ObjectID(id).
				Kind(kind).
				String("file", filePath).
				Log()
		} else {
			StorageLog(logger).Error(LogEventStorageObjectCreateHashRegistryPersistFailed, err).
				ObjectID(id).
				Kind(kind).
				String("file", filePath).
				Log()
		}
		return errfmt.Errorf(ErrMsgPersistHashRegRollback, id, err)
	}

	return nil
}

// finalizeObjectCreation performs post-creation operations
func (f *FileObjectStorage) finalizeObjectCreation(ctx context.Context, id, kind, filePath string, secCtx *pkgctx.SecurityContext) error {
	// During write-behind worker apply, synchronous Create(audit_event) from here would enqueue
	// WAL work while the worker is still inside apply (deadlock; buffer stuck at N+1 ops).
	if isSkipWriteBehind(ctx) {
		ctx = WithDeferAuditEvents(ctx)
	}
	// For CAS objects, skip cache update here - the CAS PostSyncCallback will handle it
	// with the correct hash-based file path. The filePath parameter here is incorrect for CAS objects
	// (it's constructed from getObjectFilePath which doesn't know the hash yet).
	if !f.usesContentAddressableStorage(kind) {
		// Execute cache operation based on context for non-CAS objects
		// Pass the actual ID to update cache context if it was auto-generated
		if err := executeCacheOperationWithID(ctx, filePath, id); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectCreateCacheOperationFailed).
				ObjectID(id).
				Kind(kind).
				WithError(err).
				Log()
		}
	}
	// For CAS objects, cache update happens in PostSyncCallback with correct file path

	// Touch process directory to ensure cache staleness detection works
	if err := f.touchProcessDirectory(); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Debug(LogEventStorageObjectCreateTouchProcessDirFailed).
			ObjectID(id).
			Kind(kind).
			WithError(err).
			Log()
	}

	// Validate workflow constraints for workstream creation (after ID is known)
	if kind == objects.KindWorkstream {
		workflowValidator := NewWorkflowConstraintValidator(f)
		if err := workflowValidator.ValidateWorkstreamOperation(ctx, secCtx, id, OpCreate, kind); err != nil {
			// Rollback: delete the file we just created
			if err := os.Remove(filePath); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			return errfmt.Newf(ErrMsgWorkflowConstraintFail).Wrap(err)
		}
	}

	// Record state change in command execution tracker
	RecordObjectStateChange(ctx, OpCreate, id)

	actualPath, err := f.getObjectFilePath(id, kind)
	if err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}
	if actualPath == emptyValue {
		actualPath = filePath
	}

	// Create audit event for the creation (pass ctx so bulk-create can defer CAS flush)
	//nolint:errcheck // Intentional error ignored - audit events are best effort
	if err := createCreateAuditEvent(ctx, f.projectRoot, id, kind, actualPath, secCtx, f); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// applyCreateFromBuffer persists a create/update from the write-behind buffer to CAS or file.
			// Used by ObjectWriteBehindWorker. Does not call FlushKind (worker runs after caller returns).
			// When data is nil and kind is stream-only (minimal WAL replay), skips apply (best-effort: record already in stream).
			Error(ErrMsgSwallowedError, err).Log()
	}

	return nil
}

func (f *FileObjectStorage) applyCreateFromBuffer(ctx context.Context, id, kind string, data []byte, secCtx *pkgctx.SecurityContext) error {
	ctx = withSkipWriteBehind(ctx)

	if len(data) == 0 {
		if StreamStorageEnabledForKind(kind) {
			// Minimal WAL record replayed: no payload; assume already persisted to stream
			return nil
		}
		return errfmt.Errorf("applyCreateFromBuffer: empty data for %s/%s", kind, id)
	}
	filePath, err := f.prepareObjectPath(id, kind)
	if err != nil {
		return err
	}
	if err := f.writeObjectToStorage(ctx, id, kind, filePath, data, secCtx); err != nil {
		return err
	}
	// Update reverse reference index (best effort - unmarshal object data)
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err == nil {
		updateReverseReferenceIndexOnCreate(id, obj)
	}
	return f.finalizeObjectCreation(ctx, id, kind, filePath, secCtx)
}
