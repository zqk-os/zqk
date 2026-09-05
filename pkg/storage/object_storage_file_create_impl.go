package storage

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/kernelcas"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"github.com/lanceman/zqk/pkg/storage/crud"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

//nolint:gocyclo
func (f *FileObjectStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	// Kernel Mutation Pipeline entry (COMMIT re-enters with kernelcas.WithCommit).
	// TRACK: REDACTED
	if !kernelcas.IsCommit(ctx) {
		kind, _ := obj[objects.FieldKeyKind].(string)
		id, _ := obj[objects.FieldKeyID].(string)
		status, _ := obj[objects.FieldKeyStatus].(string)
		return kernelcas.RunCreate(ctx, nil, &kernelcas.Mutation{
			Kind:   kind,
			ID:     id,
			Status: status,
			Intent: kernelcas.IntentCreate,
			Reason: pkgctx.GetLifecycleBreakGlassReason(ctx),
			CommitFn: func(c context.Context) error {
				return f.Create(c, secCtx, obj)
			},
		})
	}

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
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepEntry, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, 0, nil); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Normalize status (kind-aware) so we accept variants and persist the preferred value for this kind
			Error(ErrMsgSwallowedError, err).Log()
	}

	if s := objects.GetString(obj, objects.FieldKeyStatus); s != emptyValue {
		canonical, canonErr := canonicalizePersistedLifecycleStatus(f.GetLifecycleLoader(), kind, s)
		if canonErr != nil {
			return canonErr
		}
		obj[objects.FieldKeyStatus] = canonical
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
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepValidateAndPrepare, map[string]any{objects.FieldKeyKind: kind}, nil, time.Since(stepStart), err); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepValidateAndPrepare, map[string]any{objects.FieldKeyKind: kind}, nil, time.Since(stepStart), nil); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Ensure object has valid ID
			Error(ErrMsgSwallowedError, err).Log()
	}

	id, err = f.ensureObjectID(ctx, obj, kind)
	if err != nil {
		StorageLog(logger).Error(LogEventStorageObjectCreateEnsureObjectIDFailed, err).
			Kind(kind).
			ObjectID(id).
			Log()
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepEnsureID, map[string]any{objects.FieldKeyKind: kind}, nil, time.Since(stepStart), err); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepEnsureID, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), nil); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Prepare file path and directory
			Error(ErrMsgSwallowedError, err).Log()
	}

	filePath, err := f.prepareObjectPath(id, kind)
	if err != nil {
		StorageLog(logger).Error(LogEventStorageObjectCreatePreparePathFailed, err).
			Kind(kind).
			ObjectID(id).
			Log()
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepPreparePath, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), err); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepPreparePath, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id, objects.FieldKeyFilePath: filePath}, nil, time.Since(stepStart), nil); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Check if object already exists
			Error(ErrMsgSwallowedError, err).Log()
	}

	if err := f.checkObjectExists(id, kind, filePath); err != nil {
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepCheckExists, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), err); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepCheckExists, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), nil); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Validate object before writing
			Error(ErrMsgSwallowedError, err).Log()
	}

	if err := f.validateObjectBeforeCreation(ctx, obj, kind, secCtx); err != nil {
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepValidateBeforeWriteStep, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), err); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepValidateBeforeWriteStep, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), nil); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Marshal object to YAML
			Error(ErrMsgSwallowedError, err).Log()
	}

	data, err := f.marshalObjectForCreation(obj, filePath)
	if err != nil {
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepMarshal, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), err); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepMarshal, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id, "data_size": len(data)}, nil, time.Since(stepStart), nil); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Write-behind path (Option B): enqueue to WAL + buffer and return; worker persists in background.
			// Skip for scheduler_job when WithSyncCreateForSchedulerJob(ctx), or for any kind when WithSyncCreateForKind(ctx, kind).
			// Lock order: walMu before ObjectWriteBuffer.mu (see LOCK_ORDERING.md).
			Error(ErrMsgSwallowedError, err).Log()
	}

	// Create never uses write-behind. Buffer ACK without durable YAML is the repair_draft ghost
	// factory (CLI create→get under scheduler/test load). Sync write + visibility proof only.
	// TRACK: [REDACTED-ID]

	// Decide draft plane from the live object map (post-metadata), not a re-unmarshal of YAML.
	// Incomplete criteria park off CAS unless --promote, which hits the membrane.
	// TRACK: BLI-KERNEL-CRIT-CATEGORY-MINT-001
	useDraftPlane := caspkg.UseObjectDraftPlane(kind, obj, pkgctx.GetPromoteOnCreate(ctx))
	if StreamStorageEnabledForKind(kind) {
		useDraftPlane = false
	}
	if useDraftPlane {
		if err := f.errIfDraftCreateWouldDualPlane(id, kind); err != nil {
			return err
		}
	}

	// Write object to storage (CAS or file-based)
	if err := f.writeObjectToStorage(ctx, id, kind, filePath, data, secCtx, useDraftPlane); err != nil {
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepWriteToStorage, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id, "uses_cas": f.usesContentAddressableStorage(kind)}, nil, time.Since(stepStart), err); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return err
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepWriteToStorage, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id, "uses_cas": f.usesContentAddressableStorage(kind)}, nil, time.Since(stepStart), nil); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Flush per-kind listing index so List() sees the new object (same or other process).
			// Skip when stream storage was used (no index update). During bulk create, flush is deferred.
			Error(ErrMsgSwallowedError, err).Log()
	}

	if useDraftPlane && !f.objectDraftPlaneExists(kind, id) {
		path := f.objectDraftPlanePath(kind, id)
		_, statErr := fileutil.Stat(path)
		return errfmt.Errorf("create: draft-plane write reported success but file missing for %s. path=%s, statErr=%v", id, path, statErr)
	}
	if !useDraftPlane && f.usesContentAddressableStorage(kind) && !StreamStorageEnabledForKind(kind) && f.projectRoot != emptyValue && !crud.DeferListingIndexFlushForBulkCreate(ctx, kind) {
		stepFlush := time.Now()
		flushErr := caspkg.GetListingIndexWriteQueueForProjectRoot(f.projectRoot).FlushKindContext(ctx, kind, IndexFlushAfterCreateTimeout)
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepIndexFlush, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepFlush), flushErr); err != nil && !IsExpectedMissingErr(err) {
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

	// Visibility proof BEFORE audit/journal/notifications — those side effects contend on CAS
	// indexes / pending visibility and were racing create→get under parallel CLI + scheduler tests.
	// TRACK: [REDACTED-ID]
	totalDuration := time.Since(startTime)
	if proofErr := f.proveCreateVisibility(ctx, secCtx, id, kind, useDraftPlane); proofErr != nil {
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepExit, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"success": false}, totalDuration, proofErr); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return proofErr
	}

	// Perform post-creation operations (after durable proof)
	finalErr := f.finalizeObjectCreation(ctx, id, kind, filePath, secCtx)
	if finalErr != nil {
		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepFinalize, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), finalErr); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Always log exit (even if fast) - critical for understanding flow
				Error(ErrMsgSwallowedError, err).Log()
		}

		if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepExit, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"success": false}, time.Since(startTime), finalErr); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return finalErr
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepFinalize, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(stepStart), nil); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Update reverse reference index (best effort - don't fail create if this fails)
			Error(ErrMsgSwallowedError, err).Log()
	}

	updateReverseReferenceIndexOnCreate(id, obj)

	actualPath, err := f.getObjectFilePath(id, kind)
	if err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}
	if actualPath == emptyValue {
		actualPath = filePath
	}
	if useDraftPlane {
		actualPath = f.objectDraftPlanePath(kind, id)
	}

	// Create change journal entry for creation
	if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, actualPath, OpCreate, nil, obj, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// BLI-643: Notify subscribers of object creation (best-effort)
			Error(ErrMsgSwallowedError, err).Log()
	}

	executeChangeNotification(ctx, OpCreate, kind, id, obj)

	// Always log exit (even if fast) - critical for understanding flow
	if err := f.trackPersistenceStep(ctx, secCtx, OpCreate, PersistenceStepExit, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"success": true}, time.Since(startTime), nil); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Error(ErrMsgSwallowedError, err).Log()
	}
	// Fail closed: a successful Create must leave lifecycle status on the caller's map.
	// Empty status + nil return was observed with repair_draft ghosts (metadata never applied).
	// TRACK: [REDACTED-ID]
	if objects.GetString(obj, objects.FieldKeyStatus) == emptyValue {
		return errfmt.Errorf("create invariant violated: status empty after persist for %s", id)
	}
	return nil
}

// proveCreateVisibility fail-closes create→get: draft plane via Stat+ReadFile (not Read, which
// can hit write-behind/pending), CAS via hash Stat+Read. TRACK: [REDACTED-ID]
