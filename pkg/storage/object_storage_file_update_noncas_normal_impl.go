package storage

import (
	"context"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/config"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func (f *FileObjectStorage) updateNonCASPathNormal(ctx context.Context, secCtx *pkgctx.SecurityContext, kind, id, oldFilePath string, existing, updates, previousStateForJournal map[string]any, oldState, newState string, effectiveUpdates map[string]any, expectedUpdatedAt string) error {

	// Normal update (no ID change)
	// Re-check optimistic locking right before writing (for true concurrency safety)
	// This prevents race conditions where multiple goroutines read the same updated_at
	// and all pass the initial check, then all try to write
	if expectedUpdatedAt != emptyValue {
		// In test mode, serialize the critical section (re-check + write) to ensure
		// only one update can succeed at a time, making optimistic locking tests more reliable
		// The mutex must cover BOTH the re-check AND the write operation
		if config.TestingMode().OrDefault(false) || zqkenv.TestMode().Get() == "true" {
			err := concurrency.RunInLockWithLogger(&testModeUpdateMutex, locknames.LockNameTestModeUpdateSerialize, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
				// Re-read object to get latest updated_at right before writing (inside mutex)
				latest, err := f.readObjectFileNoCache(oldFilePath)
				if err != nil {
					return errfmt.Newf(ErrMsgReadOptLock).Wrap(err)
				}
				latestUpdatedAt, _ := latest[objects.FieldKeyUpdatedAt].(string)
				if latestUpdatedAt != expectedUpdatedAt {
					return ErrVersionConflict
				}

				// Merge latest values into existing (preserving our updates)
				// This ensures we have the latest state while keeping our changes
				// IMPORTANT: We must merge updated_at from the latest read to ensure we're working
				// with the actual on-disk value, not the one that was set by ensureObjectMetadata earlier
				for k, v := range latest {
					// Only update fields we didn't explicitly update
					// BUT: always use the latest updated_at from disk (not the one from ensureObjectMetadata)
					if _, wasUpdated := updates[k]; !wasUpdated {
						// Always use the latest on-disk value
						existing[k] = v
					}
				}

				// Update metadata inside mutex to ensure each goroutine gets a unique timestamp
				// This ensures that after the first write, subsequent goroutines will see a different updated_at
				f.ensureObjectMetadata(ctx, existing, secCtx, false)
				if pkgctx.HasLifecycleBreakGlass(ctx) {
					if explicitUpdatedAt, has := updates[objects.FieldKeyUpdatedAt]; has {
						existing[objects.FieldKeyUpdatedAt] = explicitUpdatedAt
					}
					if explicitUpdatedBy, has := updates[objects.FieldKeyUpdatedBy]; has {
						existing[objects.FieldKeyUpdatedBy] = explicitUpdatedBy
					}
				}

				// Write file (within mutex protection in test mode)
				if err := f.writeObjectFile(ctx, oldFilePath, existing); err != nil {
					return err
				}

				// In test mode, force file system sync to ensure the write is fully persisted
				// This is critical for concurrent tests where the next goroutine must see this write
				config := GetStorageConfig()
				if file, err := fileutil.OpenFile(oldFilePath, fileutil.O_RDWR, config.DefaultFilePerm); err == nil {
					//nolint:errcheck // Intentional error ignored
					_ = file.Sync()
					_ = file.Close()
				}
				// Also sync the directory to ensure metadata is flushed
				dir := filepath.Dir(oldFilePath)
				if dirFile, err := fileutil.Open(dir); err == nil {
					//nolint:errcheck // Intentional error ignored
					_ = dirFile.Sync()
					_ = dirFile.Close()
				}
				// Part of the transaction: list cache must reflect the write
				f.invalidateNonCASUpdateCaches(kind, id)
				// BLI-643: Notify subscribers of object update (test mode path)
				executeChangeNotification(ctx, OpUpdate, kind, id, existing)
				return nil
			})
			if err != nil {
				StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
					Warn(LogEventStorageObjectUpdateTestModeSerializationWarn).
					WithError(err).
					Log()
				return errfmt.Newf(ErrMsgTimeoutTestUpdate).Wrap(err)
			}
			// Small delay to ensure OS has processed the sync
			time.Sleep(10 * time.Millisecond)

			// Update hash registry
			hashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(oldFilePath))
			if err := hashRegistry.Load(); err != nil {
				// Log warning but don't fail update
			}
			// Calculate hash of file content
			fileData, err := fileutil.ReadFile(oldFilePath)
			when.When(func() bool { return err != nil }).Then(func() {
				data, marshalErr := f.yamlMarshalForPersistence(existing)
				when.When(func() bool { return marshalErr == nil }).Then(func() { fileData = data }).OrElse(func() {
					logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
					StorageLog(logger).Warn(LogEventStorageObjectUpdateCalcHashFailedWarn).
						Kind(kind).
						ObjectID(id).
						WithError(err).
						Log()
				}).Run()
			}).Run()
			if len(fileData) > 0 {
				hash := f.calculateHash(fileData)
				filename := filepath.Base(oldFilePath)
				hashRegistry.SetHash(filename, hash)
				// Save hash registry with retry (critical for integrity - must succeed)
				if err := f.saveHashRegistryWithRetry(hashRegistry, id, filename, hash); err != nil {
					logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
					if IsHashRegistrySaveQueueFull(err) {
						StorageLog(logger).Warn(LogEventStorageObjectUpdateHashQueueFullTestModeRollback).
							WithError(err).
							ObjectID(id).
							Kind(kind).
							Log()
					} else {
						StorageLog(logger).Error(LogEventStorageObjectUpdateHashPersistAfterUpdateTestFailed, err).
							ObjectID(id).
							Kind(kind).
							Log()
					}
					return errfmt.Errorf(ErrMsgPersistHashRegUpdate, id, err)
				}
			}

			// Update object ID cache to reflect the update
			if err := executeCacheOperation(ctx, oldFilePath); err != nil {
				// Log warning but don't fail update - cache is best effort
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				StorageLog(logger).Warn(LogEventStorageObjectUpdateCacheAfterUpdateFailed).
					ObjectID(id).
					Kind(kind).
					WithError(err).
					Log()
			}

			// Update reverse reference index (best effort - don't fail update if this fails)
			// previousStateForJournal contains old state, existing contains new state
			updateReverseReferenceIndexOnUpdate(id, previousStateForJournal, existing)

			// Record state change in command execution tracker
			RecordObjectStateChange(ctx, OpUpdate, id)

			// Extract changed fields for audit event
			changedFields := make([]string, 0, len(updates))
			for field := range updates {
				// Skip metadata fields
				if field != objects.FieldKeyUpdatedAt && field != objects.FieldKeyUpdatedBy && field != FieldKeyExpectedUpdatedAt {
					changedFields = append(changedFields, field)
				}
			}

			// Create audit event for the update
			//nolint:errcheck // Intentional error ignored - audit events are best effort
			if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, oldFilePath, secCtx, changedFields, f); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// Create change journal entry for the update
					//nolint:errcheck // Intentional error ignored
					Error(ErrMsgSwallowedError, err).Log()
			}

			if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, oldFilePath, OpUpdate, previousStateForJournal, updates, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// BLI-643: Notify subscribers of object update (optimistic locking test mode path)
					Error(ErrMsgSwallowedError, err).Log()
			}

			executeChangeNotification(ctx, OpUpdate, kind, id, existing)

			// Trigger lifecycle hooks if status changed
			if newState != emptyValue && oldState != newState {
				hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
				//nolint:errcheck // Intentional error ignored - lifecycle hooks are best effort
				if err := executeLifecycleHook(hookCtx, kind, oldState, newState, existing); err != nil && !IsExpectedMissingErr(err) {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
				}
			}

			return nil
		}

		// Write file (for optimistic locking updates when not in test mode)
		if err := f.writeObjectFile(ctx, oldFilePath, existing); err != nil {
			return err
		}

		// Update hash registry
		hashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(oldFilePath))
		if err := hashRegistry.Load(); err != nil {
			// Log warning but don't fail update
		}
		// Calculate hash of file content
		fileData, err := fileutil.ReadFile(oldFilePath)
		when.When(func() bool { return err != nil }).Then(func() {
			data, marshalErr := f.yamlMarshalForPersistence(existing)
			when.When(func() bool { return marshalErr == nil }).Then(func() { fileData = data }).OrElse(func() {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				StorageLog(logger).Warn(LogEventStorageObjectUpdateCalcHashFailedWarn).
					Kind(kind).
					ObjectID(id).
					WithError(err).
					Log()
			}).Run()
		}).Run()
		if len(fileData) > 0 {
			hash := f.calculateHash(fileData)
			filename := filepath.Base(oldFilePath)
			hashRegistry.SetHash(filename, hash)
			// Save hash registry with retry (critical for integrity - must succeed)
			if err := f.saveHashRegistryWithRetry(hashRegistry, id, filename, hash); err != nil {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				if IsHashRegistrySaveQueueFull(err) {
					StorageLog(logger).Warn(LogEventStorageObjectUpdateHashQueueFullOptimisticRollback).
						WithError(err).
						ObjectID(id).
						Kind(kind).
						Log()
				} else {
					StorageLog(logger).Error(LogEventStorageObjectUpdateHashPersistAfterUpdateOptFailed, err).
						ObjectID(id).
						Kind(kind).
						Log()
				}
				return errfmt.Errorf(ErrMsgPersistHashRegUpdate, id, err)
			}
		}

		// Update object ID cache to reflect the update
		if err := executeCacheOperation(ctx, oldFilePath); err != nil {
			// Log warning but don't fail update - cache is best effort
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectUpdateCacheAfterUpdateFailed).
				Kind(kind).
				ObjectID(id).
				WithError(err).
				Log()
		}
		// Part of the transaction: list cache must reflect the write
		f.InvalidateCachesForKind(kind)
		_ = RemoveRuntimeDeltaCurrentState(f.projectRoot, kind, id)

		// Record state change in command execution tracker
		RecordObjectStateChange(ctx, OpUpdate, id)

		// Extract changed fields for audit event
		changedFields := make([]string, 0, len(updates))
		for field := range updates {
			// Skip metadata fields
			if field != objects.FieldKeyUpdatedAt && field != objects.FieldKeyUpdatedBy && field != FieldKeyExpectedUpdatedAt {
				changedFields = append(changedFields, field)
			}
		}

		// Create audit event for the update
		//nolint:errcheck // Intentional error ignored - audit events are best effort
		if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, oldFilePath, secCtx, changedFields, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Create change journal entry for the update
				//nolint:errcheck // Intentional error ignored
				Error(ErrMsgSwallowedError, err).Log()
		}

		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, oldFilePath, OpUpdate, previousStateForJournal, updates, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// BLI-643: Notify subscribers of object update (optimistic locking path)
				Error(ErrMsgSwallowedError, err).Log()
		}

		executeChangeNotification(ctx, OpUpdate, kind, id, existing)

		// Trigger lifecycle hooks if status changed
		if newState != emptyValue && oldState != newState {
			// Validate workflow constraints for priority plan activation
			if kind == objects.KindPriorityPlan && newState == "active" && oldState != "active" {
				workflowValidator := NewWorkflowConstraintValidator(f)
				if err := workflowValidator.ValidatePriorityPlanActivation(ctx, secCtx, id); err != nil {
					return errfmt.Newf(ErrMsgWorkflowConstraintPlan).Wrap(err)
				}
			}

			hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
			//nolint:errcheck // Intentional error ignored - lifecycle hooks are best effort
			if err := executeLifecycleHook(hookCtx, kind, oldState, newState, existing); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
		}

		return nil
	}

	// Write file (for updates without optimistic locking)
	if err := f.writeObjectFile(ctx, oldFilePath, existing); err != nil {
		return err
	}

	// Update hash registry
	hashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(oldFilePath))
	if err := hashRegistry.Load(); err != nil {
		// Hash registry doesn't exist yet - will be created on first save
		// This is not an error, just means it's a new registry
	}
	// Calculate hash of file content
	// Use file content directly since object was already written successfully
	fileData, err := fileutil.ReadFile(oldFilePath)
	if err != nil {
		// Fallback to marshaling if file read fails (shouldn't happen)
		if data, marshalErr := f.yamlMarshalForPersistence(existing); marshalErr == nil {
			fileData = data
		} else {
			// Both failed - cannot calculate hash, fail update
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			combinedErr := errfmt.Errorf(ErrMsgFileReadMarshalFallback, err, marshalErr)
			StorageLog(logger).Error(LogEventStorageObjectUpdateCalcHashFailedErr, combinedErr).
				ObjectID(id).
				Kind(kind).
				Log()
			return errfmt.Errorf(ErrMsgCalcHashUpdate, id, err, marshalErr)
		}
	}
	if len(fileData) > 0 {
		hash := f.calculateHash(fileData)
		filename := filepath.Base(oldFilePath)
		hashRegistry.SetHash(filename, hash)
		// Save hash registry with retry (critical for integrity - must succeed)
		if err := f.saveHashRegistryWithRetry(hashRegistry, id, filename, hash); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			if IsHashRegistrySaveQueueFull(err) {
				StorageLog(logger).Warn(LogEventStorageObjectUpdateHashQueueFullRollback).
					WithError(err).
					ObjectID(id).
					Kind(kind).
					String("file", oldFilePath).
					Log()
			} else {
				StorageLog(logger).Error(LogEventStorageObjectUpdateHashPersistAfterUpdateFailed, err).
					ObjectID(id).
					Kind(kind).
					String("file", oldFilePath).
					Log()
			}
			return errfmt.Errorf(ErrMsgPersistHashRegUpdate, id, err)
		}
	}

	// Execute cache operation based on context
	// The context should have been set by the caller with WithCacheUpdate or WithCacheIDChange
	if err := executeCacheOperation(ctx, oldFilePath); err != nil {
		// Log warning but don't fail update - cache is best effort
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageObjectUpdateCacheAfterUpdateFailed).
			ObjectID(id).
			Kind(kind).
			WithError(err).
			Log()
	}
	// Part of the transaction: list cache must reflect the write
	f.invalidateNonCASUpdateCaches(kind, id)

	// Update reverse reference index (best effort - don't fail update if this fails)
	// existing contains the old object state, and after applying updates it contains the new state
	updateReverseReferenceIndexOnUpdate(id, previousStateForJournal, existing)

	// Touch process directory to ensure cache staleness detection works
	// When files are written to subdirectories, only the subdirectory mtime is updated,
	// not the parent .zqk/process directory. This causes cache staleness checks to fail.
	// By explicitly touching the process directory, we ensure the cache knows about changes.
	if err := f.touchProcessDirectory(); err != nil {
		// Log warning but don't fail - this is best effort for cache staleness detection
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Debug(LogEventStorageObjectUpdateTouchProcessDirFailedDebug).
			Kind(kind).
			ObjectID(id).
			WithError(err).
			Log()
	}

	// Record state change in command execution tracker
	RecordObjectStateChange(ctx, OpUpdate, id)

	// Create change journal entry for the update
	// Get file path for change journal (oldFilePath was already computed above)
	//
	//nolint:errcheck // Intentional error ignored
	if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, oldFilePath, OpUpdate, previousStateForJournal, effectiveUpdates, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// BLI-643: Notify subscribers of object update (best-effort)
			Error(ErrMsgSwallowedError, err).Log()
	}

	executeChangeNotification(ctx, OpUpdate, kind, id, existing)

	// Trigger lifecycle hooks if status changed
	if newState != emptyValue && oldState != newState {
		hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
		//nolint:errcheck // Intentional error ignored - lifecycle hooks are best effort
		if err := executeLifecycleHook(hookCtx, kind, oldState, newState, existing); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
	}

	return nil
}

func (f *FileObjectStorage) invalidateNonCASUpdateCaches(kind, id string) {
	f.InvalidateCachesForKind(kind)
	f.InvalidateCASCacheForKind(kind)
	_ = RemoveRuntimeDeltaCurrentState(f.projectRoot, kind, id)
}
