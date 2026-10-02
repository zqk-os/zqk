package storage

import (
	"context"
	"path/filepath"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

func (f *FileObjectStorage) writeObjectToStorage(ctx context.Context, id, kind, filePath string, data []byte, secCtx *pkgctx.SecurityContext, useDraftPlane bool) error {
	if err := f.CheckTestRepoWriteGuard(); err != nil {
		return err
	}
	if StreamStorageEnabledForKind(kind) && f.usesContentAddressableStorage(kind) {
		return f.writeObjectToStream(ctx, id, kind, data)
	}
	if useDraftPlane && f.usesContentAddressableStorage(kind) {
		return f.writeCASThroughMembrane(ctx, id, kind, data, true, func() error {
			return f.WriteObjectToDraftPlane(id, kind, data)
		})
	}
	if f.usesContentAddressableStorage(kind) {
		return f.writeCASThroughMembrane(ctx, id, kind, data, false, func() error {
			return f.writeObjectToCAS(ctx, id, kind, filePath, data, secCtx)
		})
	}
	return f.writeObjectToFile(ctx, id, kind, filePath, data)
}

// writeObjectToStream appends the object to the kind's stream segment and updates stream registry + high-volume cache.
// No CAS, no hash registry; retention uses the same index contract (Count, OldestIDs, IDsOlderThan) via cache.
func (f *FileObjectStorage) writeObjectToStream(_ context.Context, id, kind string, data []byte) error {
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
				if info, err := fileutil.Stat(segmentPath); err == nil {
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
	if err := AppendStreamLocationToRegistry(f.projectRoot, kind, id, loc); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Update high-volume cache so retention and count use same index contract (Count, OldestIDs, IDsOlderThan)
			Error(ErrMsgSwallowedError, err).Log()
	}

	cache := GetGlobalHighVolumeEventCache()
	if cache != nil {
		eventType, _ := obj[objects.FieldKeyEventType].(string)
		status, _ := obj[objects.FieldKeyStatus].(string)
		mtime := time.Now().UTC()
		if info, err := fileutil.Stat(segmentPath); err == nil {
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
		if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepGetContentAddressedStorage, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(casStart), err); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return errfmt.Newf(ErrMsgGetCAS).Wrap(err)
	}
	if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepGetContentAddressedStorage, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(casStart), nil); err != nil && !IsExpectedMissingErr(err) {
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
			if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepGetBucketStrategyReg, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"duration_ms": float64(registryDuration.Nanoseconds()) / 1e6}, registryDuration, nil); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
		}
		if registry != nil {
			strategyStart := time.Now()
			strategy, err := registry.GetStrategyForKind(ctx, kind)
			strategyDuration := time.Since(strategyStart)
			if strategyDuration > 100*time.Millisecond {
				if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepGetStrategyForKind, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"duration_ms": float64(strategyDuration.Nanoseconds()) / 1e6}, strategyDuration, err); err != nil && !IsExpectedMissingErr(err) {
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
		if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepBucketStrategyLookup, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"duration_ms": float64(bucketStrategyDuration.Nanoseconds()) / 1e6, "bucket_dir": bucketDir}, bucketStrategyDuration, nil); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Fallback: for chrono-bucketed kinds, if strategy returned no bucket (e.g. during registry init),
				// derive bucket from created_at so we never write to the top-level kind dir (e.g. audit/).
				Error(ErrMsgSwallowedError, err).Log()
		}
	}

	if bucketDir == emptyValue && obj != nil && !StreamStorageEnabledForKind(kind) {
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
			if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepContentAddressedPut, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"bucket_dir": bucketDir}, time.Since(casCreateStart), err); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			return errfmt.Newf(ErrMsgCreateCAS).Wrap(err)
		}
	} else {
		if err := cas.Create(id, data); err != nil {
			if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepContentAddressedPut, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, nil, time.Since(casCreateStart), err); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			return errfmt.Newf(ErrMsgCreateCAS).Wrap(err)
		}
	}
	casCreateDuration := time.Since(casCreateStart)
	if casCreateDuration > 100*time.Millisecond {
		if err := f.trackPersistenceStep(ctx, secCtx, OpWriteContentAddressed, PersistenceStepContentAddressedPut, map[string]any{objects.FieldKeyKind: kind, objects.FieldKeyID: id}, map[string]any{"duration_ms": float64(casCreateDuration.Nanoseconds()) / 1e6, "bucket_dir": bucketDir}, casCreateDuration, nil); err != nil && !IsExpectedMissingErr(err) {
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
			if err := fileutil.Chmod(hashFilePath, config.KeystoreFilePerm); err != nil {
				// Log but don't fail - best effort
			}
		}
	}

	// Register hash in the hash registry for integrity-check compatibility.
	// The CAS write above already succeeded; the object is visible via List().
	// If the hash registry save fails (e.g. transient I/O error), log a warning and
	// continue — the object IS created. `system check --auto-fix` repairs the missing
	// hash entry on the next run. Do NOT roll back the create here.
	if !StreamStorageEnabledForKind(kind) {
		hash := CalculateSHA256Hash(data)
		kindDir := f.GetKindDir(kind)
		if kindDir != "" {
			hashRegistry := f.newHashRegistry(ctx, kind, kindDir)
			// Load existing registry (ignore error if file doesn't exist yet)
			if err := hashRegistry.Load(); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
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

	filename := filepath.Base(filePath)
	hashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(filePath))
	hashRegistry.LoadAndSetHash(filename, hash)

	// Save hash registry with retry (critical for integrity - must succeed)
	if err := f.saveHashRegistryWithRetry(hashRegistry, id, filename, hash); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		if IsHashRegistrySaveQueueFull(err) {
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
	// Non-CAS files couple here. Live CAS blobs couple in PostSyncCallback (hash path).
	// Draft-plane CAS kinds never hit PostSync; the id still belongs in object-id-cache.
	if f.objectDraftPlaneExists(kind, id) {
		if err := coupleObjectIDCacheLivePath(id, kind, f.objectDraftPlanePath(kind, id)); err != nil {
			_ = f.deleteObjectDraftPlane(kind, id)
			return err
		}
	} else if !f.usesContentAddressableStorage(kind) {
		if err := executeCacheOperationWithID(ctx, filePath, id); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectCreateCacheOperationFailed).
				ObjectID(id).
				Kind(kind).
				WithError(err).
				Log()
		}
	}

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
			if err := fileutil.Remove(filePath); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			return errfmt.Newf(ErrMsgWorkflowConstraintFail).Wrap(err)
		}
	}

	// Record state change in command execution tracker
	RecordObjectStateChange(ctx, OpCreate, id)

	actualPath := f.resolveActualObjectFilePath(id, kind, filePath)

	// Create audit event for the creation (pass ctx so bulk-create can defer CAS flush)
	//nolint:errcheck // Intentional error ignored - audit events are best effort
	if err := createCreateAuditEvent(ctx, f.projectRoot, id, kind, actualPath, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
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
	useDraftPlane := false
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err == nil {
		useDraftPlane = caspkg.UseObjectDraftPlane(kind, obj, pkgctx.GetPromoteOnCreate(ctx))
	}
	if useDraftPlane {
		if err := f.errIfDraftCreateWouldDualPlane(id, kind); err != nil {
			// In write-behind apply: if CAS already maps this id, the object was already materialized
			// to CAS (or promoted). The write-behind draft create is superseded by CAS reality;
			// mark as complete rather than stalling write-behind replay.
			return nil
		}
	}
	if err := f.writeObjectToStorage(ctx, id, kind, filePath, data, secCtx, useDraftPlane); err != nil {
		return err
	}
	// Update reverse reference index (best effort - unmarshal object data)
	if obj != nil {
		updateReverseReferenceIndexOnCreate(id, obj)
	}
	return f.finalizeObjectCreation(ctx, id, kind, filePath, secCtx)
}

func (f *FileObjectStorage) resolveActualObjectFilePath(id, kind, fallbackPath string) string {
	actualPath, err := f.getObjectFilePath(id, kind)
	if err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}
	if actualPath == emptyValue {
		return fallbackPath
	}
	return actualPath
}
