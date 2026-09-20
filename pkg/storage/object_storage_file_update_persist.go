package storage

import (
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	"github.com/zqk-os/zqk/pkg/when"
)

func (f *FileObjectStorage) persistFileObjectUpdate(p *fileObjectUpdatePrep) error {
	ctx := p.ctx
	secCtx := p.secCtx
	id := p.id
	updates := p.updates
	kind := p.kind
	existing := p.existing
	previousStateForJournal := p.previousStateForJournal
	newID := p.newID
	idUpdated := p.idUpdated
	expectedUpdatedAt := p.expectedUpdatedAt
	oldState := p.oldState
	newState := p.newState
	effectiveUpdates := p.effectiveUpdates
	runtimeDeltaOnly := p.runtimeDeltaOnly
	// Draft-plane / leave-preliminary: sync only (no write-behind). Promote materializes into CAS.
	effectiveStatus, _ := existing[objects.FieldKeyStatus].(string)
	onDraftPlane := f.objectDraftPlaneExists(kind, id)
	stayOnDraftPlane := shouldUseObjectDraftPlane(kind, effectiveStatus) || caspkg.ParkCriteriaWithoutCategory(kind, existing) || caspkg.ParkObjectWithoutDescription(kind, existing)
	if onDraftPlane || stayOnDraftPlane {
		data, marshalErr := f.yamlMarshalForPersistence(existing)
		if marshalErr != nil {
			return errfmt.Newf(ErrMsgMarshalUpdatedObj).Wrap(marshalErr)
		}
		if stayOnDraftPlane {
			if err := f.WriteObjectToDraftPlane(id, kind, data); err != nil {
				return err
			}
			if err := coupleObjectIDCacheLivePath(id, kind, f.objectDraftPlanePath(kind, id)); err != nil {
				return err
			}
			// If demoting from CAS into draft, remove CAS blob after draft write succeeds.
			if !onDraftPlane && f.usesContentAddressableStorage(kind) && !StreamStorageEnabledForKind(kind) {
				if cas, casErr := f.getContentAddressableStorage(kind); casErr == nil && cas != nil {
					if delErr := cas.Delete(id); delErr != nil && !strings.Contains(delErr.Error(), "not found") {
						return errfmt.Newf("object draft plane: demote remove CAS").Wrap(delErr)
					}
					if kindDir := f.GetKindDir(kind); kindDir != emptyValue {
						removeOrphanCASFilesForObjectID(id, kindDir)
					}
				}
			}
		} else {
			// Leaving preliminary: materialize into CAS then drop draft file (fail closed).
			if err := f.writeCASThroughMembrane(ctx, id, kind, data, false, func() error {
				return f.writeObjectToCAS(ctx, id, kind, "", data, secCtx)
			}); err != nil {
				return errfmt.Newf("object draft plane: materialize to CAS").Wrap(err)
			}
			// Use membrane counterpart for deletion if needed, but since it's draft, we drop it directly locally,
			// or through a daemon method if draft plane deletion is privileged.
			// Currently draft plane edits aren't restricted, so local delete is fine.
			if err := f.deleteObjectDraftPlane(kind, id); err != nil {
				return errfmt.Newf("object draft plane: remove after CAS materialize").Wrap(err)
			}
			if f.projectRoot != emptyValue {
				if flushErr := caspkg.GetListingIndexWriteQueueForProjectRoot(f.projectRoot).FlushKindContext(ctx, kind, IndexFlushAfterCreateTimeout); flushErr != nil {
					StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
						Warn(LogEventStorageObjectCreateListingIndexFlushFailed).
						Kind(kind).
						ObjectID(id).
						WithError(flushErr).
						Log()
				}
			}
		}
		f.InvalidateCachesForKind(kind)
		f.InvalidateCASCacheForKind(kind)
		filePath, _ := f.getObjectFilePath(id, kind)
		changedFields := make([]string, 0, len(effectiveUpdates))
		for k := range effectiveUpdates {
			changedFields = append(changedFields, k)
		}
		if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, filePath, secCtx, changedFields, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpUpdate, previousStateForJournal, effectiveUpdates, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		RecordObjectStateChange(ctx, OpUpdate, id)
		executeChangeNotificationWithPath(ctx, OpUpdate, kind, id, filePath, existing)
		if newState != emptyValue && oldState != newState {
			hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
			if err := executeLifecycleHook(hookCtx, kind, oldState, newState, existing); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
		}
		return nil
	}

	// Write-behind path (Option B): enqueue update to WAL + buffer and return; worker persists in background.
	// Skip when ID is changing (idUpdated); that path requires create new + delete old and stays synchronous.
	// Skip for scheduler_job and keystore_entry so metadata (credential_hash, revoked, etc.) is persisted synchronously and visible immediately across contexts.
	if f.writeBuf != nil && f.wal != nil && !idUpdated && kind != objects.KindSchedulerJob && kind != objects.KindKeystoreEntry && !isSkipWriteBehind(ctx) {
		data, err := f.yamlMarshalForPersistence(existing)
		if err != nil {
			return errfmt.Newf(ErrMsgMarshalUpdatedObjWB).Wrap(err)
		}
		var appendErr error
		var didAppend bool
		if err := concurrency.RunInLockWithLogger(&f.walMu, locknames.LockNameUpdateAppendWal, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			if f.writeBuf != nil && f.wal != nil {
				didAppend = true
				appendErr = AppendToWALAndBuffer(f.wal, f.writeBuf, "update", kind, id, data, true)
			}
			return nil
		}); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGen, err).Log()
		}
		if didAppend {
			if appendErr != nil {
				return errfmt.Newf(ErrMsgEnqueueUpdateWB).Wrap(appendErr)
			}
			// Apply immediately so List/Get see new data; WAL remains the durability record.
			var filePath string
			// Stream-backed: use stream_current + change journal only (no CAS). Other kinds: apply to CAS.
			if StreamStorageEnabledForKind(kind) {
				if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, "", OpUpdate, previousStateForJournal, effectiveUpdates, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
				}
				if err := WriteStreamBackedCurrentState(f.projectRoot, kind, id, data); err != nil {
					StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
						Warn(LogEventStorageObjectUpdateWBStreamCurrentFailed).
						ObjectID(id).
						Kind(kind).
						WithError(err).
						Log()
				}
			} else if f.usesContentAddressableStorage(kind) {
				if cas, err := f.getContentAddressableStorage(kind); err == nil {
					if runtimeDeltaOnly {
						if err := WriteRuntimeDeltaCurrentState(f.projectRoot, kind, id, data); err != nil {
							StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
								Warn(LogEventStorageObjectUpdateWBRuntimeDeltaFailed).
								ObjectID(id).
								Kind(kind).
								WithError(err).
								Log()
						}
					} else {
						if err := cas.Update(id, data); err != nil {
							StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
								Warn(LogEventStorageObjectUpdateWBCASApplyFailed).
								ObjectID(id).
								Kind(kind).
								WithError(err).
								Log()
						}
					}

					var pathErr error
					filePath, pathErr = f.getObjectFilePath(id, kind)
					if pathErr != nil && !IsExpectedMissingErr(pathErr) {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, pathErr).Log()
					}
					changedFields := make([]string, 0, len(effectiveUpdates))
					for k := range effectiveUpdates {
						changedFields = append(changedFields, k)
					}
					if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, filePath, secCtx, changedFields, f); err != nil && !IsExpectedMissingErr(err) {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
					}
					if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpUpdate, previousStateForJournal, effectiveUpdates, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
					}
				}
			}
			f.InvalidateCachesForKind(kind)
			f.InvalidateCASCacheForKind(kind)
			f.writeBehindWorker.Notify()
			RecordObjectStateChange(ctx, OpUpdate, id)
			executeChangeNotificationWithPath(ctx, OpUpdate, kind, id, filePath, existing)

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
	}

	// Stream-backed kinds: update stream_current + change journal only; no CAS or hash registry.
	if StreamStorageEnabledForKind(kind) {
		return f.updateStreamBacked(ctx, id, kind, idUpdated, newID, existing, updates, previousStateForJournal, secCtx)
	}

	// Check if this kind uses content-addressable storage
	if f.usesContentAddressableStorage(kind) {
		cas, err := f.getContentAddressableStorage(kind)
		if err != nil {
			return errfmt.Newf(ErrMsgGetCAS).Wrap(err)
		}
		data, err := f.yamlMarshalForPersistence(existing)
		if err != nil {
			return errfmt.Newf(ErrMsgMarshalUpdatedObj).Wrap(err)
		}
		// Compute target bucket from strategy so any kind with bucketing migrates to the correct bucket on update.
		var targetBucketDir string
		if registry := f.getBucketStrategyRegistry(ctx); registry != nil {
			if strategy, regErr := registry.GetStrategyForKind(ctx, kind); regErr == nil && strategy != nil {
				if bk := strategy.GetBucketKey(existing, ""); bk != emptyValue {
					if dirName := objects.GetDirectoryFromKind(kind); dirName != emptyValue {
						kindDir := filepath.Join(f.processDir, dirName)
						targetBucketDir = strategy.GetBucketDirectory(kindDir, bk)
						if targetBucketDir != emptyValue {
							if currentPath, pathErr := f.getObjectFilePath(id, kind); pathErr == nil {
								currentBucketDir := filepath.Dir(currentPath)
								if targetBucketDir == currentBucketDir {
									targetBucketDir = "" // no migration needed
								}
							}
						} else {
							targetBucketDir = ""
						}
					}
				}
			}
		}
		// Runtime-delta-only update path: skip CAS rewrite and persist current runtime overlay + journal.
		var updateErr error
		var oldHash string
		if cas != nil {
			oldHash, _ = cas.GetHashForID(id)
		}
		when.When(func() bool { return runtimeDeltaOnly }).Then(func() {
			if err := WriteRuntimeDeltaCurrentState(f.projectRoot, kind, id, data); err != nil {
				updateErr = errfmt.Newf(ErrMsgWriteRuntimeDelta).Wrap(err)
			}
		}).OrElseWhen(func() bool { return idUpdated }).Then(func() {
			// Use content-addressable storage for Update (with optional migration to target bucket)
			// ID is changing - use UpdateWithIDChange to update CAS index with new ID
			if err := f.renameCASThroughMembrane(ctx, id, newID, kind, func() error {
				if err := cas.UpdateWithIDChange(id, newID, data); err != nil {
					return errfmt.Newf(ErrMsgUpdateCASIDChange).Wrap(err)
				}
				return nil
			}); err != nil {
				updateErr = err
			}
		}).OrElse(func() {
			// No ID change - use regular Update (migrates to target bucket when strategy says different from current)
			if err := f.writeCASThroughMembrane(ctx, id, kind, data, false, func() error {
				if targetBucketDir != emptyValue {
					if err := cas.Update(id, data, targetBucketDir); err != nil {
						return errfmt.Newf(ErrMsgUpdateCAS).Wrap(err)
					}
				} else {
					if err := cas.Update(id, data); err != nil {
						return errfmt.Newf(ErrMsgUpdateCAS).Wrap(err)
					}
				}
				return nil
			}); err != nil {
				updateErr = err
			}
		}).Run()

		if updateErr != nil {
			return updateErr
		}

		// List cache must be invalidated whenever storage is modified (README, list_cache.go).
		f.InvalidateCachesForKind(kind)

		// Update hash registry only when CAS object content changed.
		if !runtimeDeltaOnly {
			kindDir := f.GetKindDir(kind)
			if kindDir != emptyValue {
				hashRegistry := f.newHashRegistry(ctx, kind, kindDir)
				if hashRegistry != nil {
					_ = hashRegistry.Load() // missing file is OK (empty map)
					hash := CalculateSHA256Hash(data)
					objectIDForRegistry := id
					if idUpdated {
						objectIDForRegistry = newID
					}
					config := GetStorageConfig()
					filename := fmt.Sprintf("%s%s", objectIDForRegistry, config.YAMLExtension)
					hashRegistry.SetHash(filename, hash)
					if err := f.saveHashRegistry(hashRegistry); err != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSaveHashRegRetries, err).Log()
					}
				}
			}
			if err := RemoveRuntimeDeltaCurrentState(f.projectRoot, kind, id); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// Create audit event
					//nolint:errcheck // Intentional error ignored - audit events are best effort
					Error(ErrMsgSwallowedError, err).Log()
			}
		}

		objectIDForAudit := id
		if idUpdated {
			objectIDForAudit = newID
		}
		filePath, err := f.getObjectFilePath(objectIDForAudit, kind)
		if err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		// Prefer the live CAS path for CacheContext updates (promote/update set
		// WithCacheUpdate with empty FilePath). Post-sync on cas.Update is primary;
		// this covers callers that only rely on executeCacheOperation.
		// TRACK: BLI-1785723654802038000-b14064bc
		if filePath != emptyValue {
			if cacheErr := executeCacheOperation(ctx, filePath); cacheErr != nil {
				StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
					Warn(LogEventStorageObjectUpdateCacheAfterUpdateFailed).
					ObjectID(objectIDForAudit).
					Kind(kind).
					WithError(cacheErr).
					Log()
			}
		}
		changedFields := make([]string, 0, len(effectiveUpdates))
		for k := range effectiveUpdates {
			changedFields = append(changedFields, k)
		}
		if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, filePath, secCtx, changedFields, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Change journal entry for CAS path (audit trail and rollback; was missing and only done on file path)
				Error(ErrMsgSwallowedError, err).Log()
		}

		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpUpdate, previousStateForJournal, effectiveUpdates, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// BLI-643: Notify subscribers of object update (CAS path)
				Error(ErrMsgSwallowedError, err).Log()
		}

		executeChangeNotificationWithHashes(ctx, OpUpdate, kind, objectIDForAudit, filePath, oldHash, existing)

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

	return f.updateNonCASPath(ctx, secCtx, kind, id, newID, idUpdated, existing, updates, previousStateForJournal, oldState, newState, effectiveUpdates, expectedUpdatedAt)
}
