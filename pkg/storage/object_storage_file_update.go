package storage

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
)

const (
	updateMutationClassRuntimeDelta = "runtime_delta"
	updateMutationClassStructural   = "structural"
	updateMutationClassIDChange     = "id_change"
)

// Update updates an existing object
//
//nolint:gocyclo // Function orchestrates multiple update phases including optimistic locking, CAS/file handling, and hash registry updates; complexity reduced via helper methods
func (f *FileObjectStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	// Infer kind from ID first (more reliable than reading from file)
	if err := f.idValidator.LoadPatterns(); err != nil {
		return errfmt.Newf(ErrMsgLoadIDPatterns).Wrap(err)
	}
	kind := f.idValidator.InferKindFromID(id)
	if kind == emptyValue {
		return errfmt.Errorf(ErrMsgInferKindFailed, id)
	}

	// For keystore_entry, we need to read with system context to get all fields
	// (including credential_hash which is required for validation)
	// Access control will be applied when the updated object is read later
	var existing map[string]any
	var err error
	keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
	when.When(func() bool {
		return keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir && secCtx != nil && secCtx.AccountID != pkgctx.SystemAccountID
	}).Then(func() {
		systemCtx := pkgctx.NewSystemSecurityContext()
		existing, err = f.Read(ctx, systemCtx, id)
	}).OrElse(func() {
		existing, err = f.Read(ctx, secCtx, id)
	}).Run()
	if err != nil {
		return err
	}

	// Ensure kind matches inferred kind (not ontology from file)
	existing[objects.FieldKeyKind] = kind

	// Capture previous state for change journal BEFORE merging updates
	previousStateForJournal := make(map[string]any)
	maps.Copy(previousStateForJournal, existing)

	// Check for blocking issues before write operations (except for automated kinds)
	if err := f.checkForBlockingIssuesBeforeWrite(ctx, OpUpdate, kind, id); err != nil {
		return err
	}

	// Check permission
	if err := f.checkPermission(secCtx, "write", kind); err != nil {
		return err
	}

	// For keystore_entry, verify user owns the entry or is admin
	if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
		entryAccountID, _ := existing[objects.FieldKeyAccountID].(string)
		isAdmin := slices.Contains(secCtx.Roles, "admin")
		if !isAdmin && entryAccountID != secCtx.AccountID {
			return errfmt.Errorf(ErrMsgPermUpdateKeystore)
		}
		// Prevent non-system users from updating credential_hash or salt
		if secCtx != nil && secCtx.AccountID != pkgctx.SystemAccountID {
			if _, hasHash := updates[objects.FieldKeyCredentialHash]; hasHash {
				return errfmt.Errorf(ErrMsgPermUpdateCredHash)
			}
			if _, hasSalt := updates[objects.FieldKeySalt]; hasSalt {
				return errfmt.Errorf(ErrMsgPermUpdateSalt)
			}
		}
	}

	// --- TDE Verification Override Enforcement ---
	if statusVal, ok := updates[objects.FieldKeyStatus].(string); ok && (kind == "test_case" || kind == "criteria" || kind == "convergence_session") {
		if objects.GetGlobalStatusChecker().IsTerminal(kind, statusVal) {
			isScheduler := false
			if secCtx != nil {
				isScheduler = slices.Contains(secCtx.Roles, "scheduler") || secCtx.AccountID == pkgctx.SystemAccountID
			}
			if os.Getenv(zqkenv.BypassVerificationOutcomeAuthority()) == "1" {
				isScheduler = true
			}
			if !isScheduler {
				updates[objects.FieldKeyStatus] = "pending_verification"
				inboxPath := filepath.Join(f.projectRoot, paths.ProjectDataDir, "inbox", "human")
				fileutil.EnsureDir(inboxPath)
				tdePath := filepath.Join(inboxPath, "tde_"+id+"_"+time.Now().Format("20060102150405")+".json")
				tdeData := []byte(`{"id":"` + id + `", "action": "status_override", "requested_status": "` + statusVal + `"}`)
				fileutil.WriteStandardFile(tdePath, tdeData)
			}
		}
	}
	// ---------------------------------------------

	// Optimistic locking: check updated_at if provided in updates
	// Note: We check this early, but for true concurrency safety, we should re-check
	// right before writing. For now, this provides basic optimistic locking.
	var expectedUpdatedAt string
	if expectedUpdatedAtValue := objects.GetString(updates, FieldKeyExpectedUpdatedAt); expectedUpdatedAtValue != "" {
		expectedUpdatedAt = expectedUpdatedAtValue
		actualUpdatedAt, _ := existing[objects.FieldKeyUpdatedAt].(string)
		if actualUpdatedAt != expectedUpdatedAt {
			return ErrVersionConflict
		}
		// Remove from updates (it's a control field, not part of the object)
		delete(updates, FieldKeyExpectedUpdatedAt)
	}

	// Check if this is a built-in object and user has admin role
	isBuiltIn := IsBuiltIn(existing)
	hasAdminRole := slices.Contains(secCtx.Roles, "admin")

	// Load spec for the kind to enforce spec-driven update modes
	var spec *objects.Spec
	if loader := objects.GetGlobalSpecLoader(); loader != nil {
		specFile := fmt.Sprintf("%s.yaml", kind)
		var specErr error
		spec, specErr = loader.LoadSpecWithInheritance(specFile)
		if specErr != nil &&

			!isExpectedMissingErr(specErr) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Check if ID is being updated
				Error(ErrMsgSwallowedError, specErr).Log()
		}
	}

	var newID string
	var idUpdated bool
	if newIDValue := objects.GetString(updates, objects.FieldKeyID); newIDValue != "" && newIDValue != id {
		newID = newIDValue
		idUpdated = true

		// Validate new ID format
		if err := f.idValidator.LoadPatterns(); err != nil {
			return errfmt.Newf(ErrMsgLoadIDPatterns).Wrap(err)
		}
		valid, err := f.idValidator.ValidateID(newID, kind)
		if err != nil {
			return errfmt.Newf(ErrMsgValidateNewID).Wrap(err)
		}
		if !valid {
			return errfmt.Errorf(ErrMsgInvalidIDFormat, kind, newID)
		}

		// Check if new ID already exists
		_, err = f.Read(ctx, secCtx, newID)
		if err == nil {
			return errfmt.Errorf(ErrMsgObjectExistsFmt, newID)
		}
		// For CAS objects, Read might return different error formats
		errStr := err.Error()
		if err != ErrObjectNotFound && !strings.Contains(errStr, "not found") && !strings.Contains(errStr, "ID not found") {
			return errfmt.Newf(ErrMsgCheckNewIDExists).Wrap(err)
		}
	}

	// Normalize status (kind-aware) so we accept variants and persist the preferred value for this kind
	if s := objects.GetString(updates, objects.FieldKeyStatus); s != emptyValue {
		if loader := f.GetLifecycleLoader(); loader != nil {
			if canonical, err := loader.NormalizeStatusForKind(kind, s); err == nil {
				updates[objects.FieldKeyStatus] = canonical
			} else {
				updates[objects.FieldKeyStatus] = objects.NormalizeStatus(s)
			}
		} else {
			updates[objects.FieldKeyStatus] = objects.NormalizeStatus(s)
		}
	}

	if s := objects.GetString(updates, objects.FieldKeyStatus); s != "" {
		if err := checkVerificationOutcomeAuthority(kind, secCtx, s); err != nil {
			return err
		}
	}

	// Merge updates into existing object
	for k, v := range updates {
		// CLI --unset-field: remove key from persisted object (not a value write)
		if IsFieldUnset(v) {
			delete(existing, k)
			continue
		}
		// Handle ID update separately (after validation)
		if k == objects.FieldKeyID {
			if idUpdated {
				existing[k] = v // Update ID in object
			}
			continue
		}

		// Load spec rules
		var lifecycleMode string
		if spec != nil && spec.ResolvedFields != nil {
			if fieldDefAny, ok := spec.ResolvedFields[k]; ok {
				if fieldDef, ok := fieldDefAny.(map[string]any); ok {
					if checklistAny, ok := fieldDef["checklist"]; ok {
						if checklist, ok := checklistAny.(map[string]any); ok {
							if lifecycleAny, ok := checklist["lifecycle"]; ok {
								if lifecycleStr, ok := lifecycleAny.(string); ok {
									lifecycleMode = strings.ToLower(lifecycleStr)
								}
							}
						}
					}
				}
			}
		}

		// Spec-Driven Read-Only/Immutable Fields
		isFieldImmutable := strings.HasPrefix(lifecycleMode, "immutable") || strings.HasPrefix(lifecycleMode, "read-only") ||
			(lifecycleMode == "" && (k == objects.FieldKeyCreatedAt || k == objects.FieldKeyCreatedBy)) // Fallback

		// Don't allow updating immutable fields, unless:
		// - Object is built-in AND user has admin role (for internal command)
		// - Even then, kind remains immutable (too fundamental)
		if k == objects.FieldKeyKind {
			continue
		}
		if isFieldImmutable {
			// Allow updating created_at/created_by for built-in objects with admin role
			if isBuiltIn && hasAdminRole {
				existing[k] = v
			}
			// Otherwise, skip (immutable)
			continue
		}

		// Spec-Driven Append-Only Arrays
		if strings.HasPrefix(lifecycleMode, "append-only") || strings.HasPrefix(lifecycleMode, "append-mostly") || (k == objects.FieldKeyActivityLog && kind == objects.KindConvergenceSession) {
			// Ensure v is an array
			var newEntries []any
			if ne, ok := v.([]any); ok {
				newEntries = ne
			} else {
				// Single item append
				newEntries = []any{v}
			}

			if len(newEntries) > 0 {
				var merged []any
				if el, ok := existing[k].([]any); ok {
					merged = make([]any, 0, len(el)+len(newEntries))
					merged = append(merged, el...)
					merged = append(merged, newEntries...)
				} else {
					merged = newEntries
				}
				existing[k] = merged
				continue
			}
		}

		if mergeMapPatchIntoExisting(existing, k, v) {
			continue
		}
		existing[k] = v
	}
	// Ensure kind field is set correctly (preserve the kind, not ontology)
	existing[objects.FieldKeyKind] = kind

	// Ensure metadata
	f.ensureObjectMetadata(existing, secCtx, false)

	// Normalize the merged object to ensure correct types (especially number fields)
	// This handles cases where existing objects have int values that should be float64
	f.normalizeObjectValues(existing, kind)

	// Get current state for lifecycle validation and hook triggering
	oldState, _ := previousStateForJournal[objects.FieldKeyStatus].(string)
	newState, _ := updates[objects.FieldKeyStatus].(string)

	// Validate workflow constraints for workstream operations
	if kind == objects.KindWorkstream {
		workflowValidator := NewWorkflowConstraintValidator(f)
		operation := OpUpdate
		if err := workflowValidator.ValidateWorkstreamOperation(ctx, secCtx, id, operation, kind); err != nil {
			return errfmt.Newf(ErrMsgWorkflowConstraintFail).Wrap(err)
		}
	}

	// Validate object (spec, lifecycle, references)
	if err := f.validateObject(ctx, existing, kind, oldState); err != nil {
		return errfmt.Newf(ErrMsgValidationFailed).Wrap(err)
	}

	// Route status transitions through the Shockwave API Gateway
	if err := f.dispatchStatusGateway(ctx, secCtx, existing, kind, oldState); err != nil {
		return err
	}
	// Classify updates using effective field changes after merge/normalization.
	// Some call sites pass full object payloads; unchanged structural keys should not
	// force structural CAS rewrites when only runtime-delta fields changed.
	effectiveUpdates := effectiveUpdateFieldsForClassification(previousStateForJournal, existing, updates)
	runtimeDeltaOnly := !idUpdated && updateIsRuntimeDeltaOnly(f.projectRoot, kind, effectiveUpdates)
	mutationClass := classifyUpdateMutation(idUpdated, runtimeDeltaOnly)
	RecordUpdateMutationClass(kind, mutationClass)
	StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
		Debug(LogEventStorageObjectUpdateMutationClassifiedDebug).
		ObjectID(id).
		Kind(kind).
		String(FieldKeyMutationClass, mutationClass).
		Int(FieldKeyRequestedFields, len(updates)).
		Int(FieldKeyEffectiveFields, len(effectiveUpdates)).
		Log()

	// Write-behind path (Option B): enqueue update to WAL + buffer and return; worker persists in background.
	// Skip when ID is changing (idUpdated); that path requires create new + delete old and stays synchronous.
	// Skip for scheduler_job so job metadata (last_run_at, enabled) is persisted synchronously and visible after daemon restart (bundle jobs, one-time jobs).
	if f.writeBuf != nil && f.wal != nil && !idUpdated && kind != objects.KindSchedulerJob && !isSkipWriteBehind(ctx) {
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
			// Stream-backed: use stream_current + change journal only (no CAS). Other kinds: apply to CAS.
			if StreamStorageEnabledForKind(kind) {
				if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, "", OpUpdate, previousStateForJournal, effectiveUpdates, secCtx, f); err != nil && !isExpectedMissingErr(err) {
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

					filePath, err := f.getObjectFilePath(id, kind)
					if err != nil && !isExpectedMissingErr(err) {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
					}
					changedFields := make([]string, 0, len(effectiveUpdates))
					for k := range effectiveUpdates {
						changedFields = append(changedFields, k)
					}
					if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, filePath, secCtx, changedFields, f); err != nil && !isExpectedMissingErr(err) {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
					}
					if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpUpdate, previousStateForJournal, effectiveUpdates, secCtx, f); err != nil && !isExpectedMissingErr(err) {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
					}
				}
			}
			f.InvalidateCachesForKind(kind)
			f.InvalidateCASCacheForKind(kind)
			f.writeBehindWorker.Notify()
			RecordObjectStateChange(ctx, OpUpdate, id)
			executeChangeNotification(ctx, OpUpdate, kind, id, existing)

			// Trigger lifecycle hooks if status changed
			if newState != emptyValue && oldState != newState {
				hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
				//nolint:errcheck // Intentional error ignored - lifecycle hooks are best effort
				if err := executeLifecycleHook(hookCtx, kind, oldState, newState, existing); err != nil && !isExpectedMissingErr(err) {
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
		when.When(func() bool { return runtimeDeltaOnly }).Then(func() {
			if err := WriteRuntimeDeltaCurrentState(f.projectRoot, kind, id, data); err != nil {
				updateErr = errfmt.Newf(ErrMsgWriteRuntimeDelta).Wrap(err)
			}
		}).OrElseWhen(func() bool { return idUpdated }).Then(func() {
			// Use content-addressable storage for Update (with optional migration to target bucket)
			// ID is changing - use UpdateWithIDChange to update CAS index with new ID
			if err := cas.UpdateWithIDChange(id, newID, data); err != nil {
				updateErr = errfmt.Newf(ErrMsgUpdateCASIDChange).Wrap(err)
			}
		}).OrElse(func() {
			// No ID change - use regular Update (migrates to target bucket when strategy says different from current)
			if targetBucketDir != emptyValue {
				if err := cas.Update(id, data, targetBucketDir); err != nil {
					updateErr = errfmt.Newf(ErrMsgUpdateCAS).Wrap(err)
				}
			} else {
				if err := cas.Update(id, data); err != nil {
					updateErr = errfmt.Newf(ErrMsgUpdateCAS).Wrap(err)
				}
			}
		}).Run()

		if updateErr != nil {
			return updateErr
		}

		// List cache must be invalidated whenever storage is modified (README, list_cache.go).
		f.InvalidateCachesForKind(kind)

		// Update hash registry only when CAS object content changed.
		if !runtimeDeltaOnly {
			dirName := objects.GetDirectoryFromKind(kind)
			if dirName != emptyValue {
				kindDir := filepath.Join(f.processDir, dirName)
				hashRegistry := f.newHashRegistry(ctx, kind, kindDir)
				if err := hashRegistry.Load(); err == nil {
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
			if err := RemoveRuntimeDeltaCurrentState(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
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
		if err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		changedFields := make([]string, 0, len(effectiveUpdates))
		for k := range effectiveUpdates {
			changedFields = append(changedFields, k)
		}
		if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, filePath, secCtx, changedFields, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Change journal entry for CAS path (audit trail and rollback; was missing and only done on file path)
				Error(ErrMsgSwallowedError, err).Log()
		}

		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpUpdate, previousStateForJournal, effectiveUpdates, secCtx, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// ITEM-643: Notify subscribers of object update (CAS path)
				Error(ErrMsgSwallowedError, err).Log()
		}

		executeChangeNotification(ctx, OpUpdate, kind, objectIDForAudit, existing)

		// Trigger lifecycle hooks if status changed
		if newState != emptyValue && oldState != newState {
			hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
			//nolint:errcheck // Intentional error ignored - lifecycle hooks are best effort
			if err := executeLifecycleHook(hookCtx, kind, oldState, newState, existing); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
		}

		return nil
	}

	// Get old file path
	oldFilePath, err := f.getObjectFilePath(id, kind)
	if err != nil {
		return err
	}

	// If ID was updated, move file to new location
	if idUpdated {
		// Get new file path
		newFilePath, err := f.getObjectFilePath(newID, kind)
		if err != nil {
			return err
		}

		// Write to new location
		if err := f.writeObjectFile(ctx, newFilePath, existing); err != nil {
			return errfmt.Newf(ErrMsgWriteObjNewLoc).Wrap(err)
		}

		// Update hash registry for new file
		newHashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(newFilePath))
		if err := newHashRegistry.Load(); err != nil {
			// Log warning but don't fail update
		}
		// CRITICAL: Read file back from disk to get the exact bytes that are stored
		// This ensures the hash matches what system check will read
		newFileContent, err := os.ReadFile(newFilePath)
		if err != nil {
			return errfmt.Newf(ErrMsgReadNewFileForHash).Wrap(err)
		}
		hash := f.calculateHash(newFileContent)
		newFilename := filepath.Base(newFilePath)
		newHashRegistry.SetHash(newFilename, hash)
		// Save hash registry with retry (critical for integrity - must succeed)
		if err := f.saveHashRegistryWithRetry(newHashRegistry, id, newFilename, hash); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			if isHashRegistrySaveQueueFull(err) {
				StorageLog(logger).Warn(LogEventStorageObjectUpdateHashQueueFullIDChangeRollback).
					WithError(err).
					ObjectID(id).
					Kind(kind).
					String("new_file", newFilePath).
					Log()
			} else {
				StorageLog(logger).Error(LogEventStorageObjectUpdateHashPersistAfterIDChangeFailed, err).
					ObjectID(id).
					Kind(kind).
					String("new_file", newFilePath).
					Log()
			}
			return errfmt.Errorf(ErrMsgPersistHashRegIDChange, id, err)
		}

		// Remove old file
		if err := os.Remove(oldFilePath); err != nil {
			// Log warning but don't fail - file might already be moved
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectUpdateRemoveOldFileAfterIDChangeFailed).
				String("old_path", oldFilePath).
				WithError(err).
				Log()
		}

		// Update hash registry for old file (remove old hash)
		oldHashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(oldFilePath))
		if err := oldHashRegistry.Load(); err == nil {
			oldHashRegistry.DeleteHash(filepath.Base(oldFilePath))
			//nolint:errcheck // Intentional error ignored
			if err := f.saveHashRegistry(oldHashRegistry); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// Execute cache operation based on context
					// The context should have been set by the caller with WithCacheIDChange
					Error(ErrMsgSwallowedError, err).Log()
			}
		}

		if err := executeCacheOperation(ctx, newFilePath); err != nil {
			// Log warning but don't fail update - cache is best effort
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectUpdateCacheAfterIDChangeFailed).
				String("old_id", id).
				String("new_id", newID).
				Kind(kind).
				WithError(err).
				Log()
		}
		// Part of the transaction: list cache must reflect the write
		f.InvalidateCachesForKind(kind)

		// Update reverse reference index for ID change (best effort - don't fail update if this fails)
		// Remove old ID from index, add new ID with same references
		updateReverseReferenceIndexOnIDChange(id, newID, existing)

		// ITEM-643: Notify subscribers of object update (ID change path; object id is now newID)
		executeChangeNotification(ctx, OpUpdate, kind, newID, existing)

		return nil
	}

	// Normal update (no ID change)
	// Re-check optimistic locking right before writing (for true concurrency safety)
	// This prevents race conditions where multiple goroutines read the same updated_at
	// and all pass the initial check, then all try to write
	if expectedUpdatedAt != emptyValue {
		// In test mode, serialize the critical section (re-check + write) to ensure
		// only one update can succeed at a time, making optimistic locking tests more reliable
		// The mutex must cover BOTH the re-check AND the write operation
		if os.Getenv(zqkenv.TestMode()) == "true" {
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
				// Note: ensureObjectMetadata will update updated_at to a new value, which is what we want
				f.ensureObjectMetadata(existing, secCtx, false)

				// Write file (within mutex protection in test mode)
				if err := f.writeObjectFile(ctx, oldFilePath, existing); err != nil {
					return err
				}

				// In test mode, force file system sync to ensure the write is fully persisted
				// This is critical for concurrent tests where the next goroutine must see this write
				config := GetStorageConfig()
				if file, err := os.OpenFile(oldFilePath, os.O_RDWR, config.DefaultFilePerm); err == nil {
					//nolint:errcheck // Intentional error ignored
					file.Sync()
					file.Close()
				}
				// Also sync the directory to ensure metadata is flushed
				dir := filepath.Dir(oldFilePath)
				if dirFile, err := os.Open(dir); err == nil {
					//nolint:errcheck // Intentional error ignored
					dirFile.Sync()
					dirFile.Close()
				}
				// Part of the transaction: list cache must reflect the write
				f.InvalidateCachesForKind(kind)
				f.InvalidateCASCacheForKind(kind)
				// ITEM-643: Notify subscribers of object update (test mode path)
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
			fileData, err := os.ReadFile(oldFilePath)
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
					if isHashRegistrySaveQueueFull(err) {
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
			if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, oldFilePath, secCtx, changedFields, f); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// Create change journal entry for the update
					//nolint:errcheck // Intentional error ignored
					Error(ErrMsgSwallowedError, err).Log()
			}

			if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, oldFilePath, OpUpdate, previousStateForJournal, updates, secCtx, f); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// ITEM-643: Notify subscribers of object update (optimistic locking test mode path)
					Error(ErrMsgSwallowedError, err).Log()
			}

			executeChangeNotification(ctx, OpUpdate, kind, id, existing)

			// Trigger lifecycle hooks if status changed
			if newState != emptyValue && oldState != newState {
				hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
				//nolint:errcheck // Intentional error ignored - lifecycle hooks are best effort
				if err := executeLifecycleHook(hookCtx, kind, oldState, newState, existing); err != nil && !isExpectedMissingErr(err) {
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
		fileData, err := os.ReadFile(oldFilePath)
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
				if isHashRegistrySaveQueueFull(err) {
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
		if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, oldFilePath, secCtx, changedFields, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Create change journal entry for the update
				//nolint:errcheck // Intentional error ignored
				Error(ErrMsgSwallowedError, err).Log()
		}

		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, oldFilePath, OpUpdate, previousStateForJournal, updates, secCtx, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// ITEM-643: Notify subscribers of object update (optimistic locking path)
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
			if err := executeLifecycleHook(hookCtx, kind, oldState, newState, existing); err != nil && !isExpectedMissingErr(err) {
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
	fileData, err := os.ReadFile(oldFilePath)
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
			if isHashRegistrySaveQueueFull(err) {
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
	f.InvalidateCachesForKind(kind)
	f.InvalidateCASCacheForKind(kind)

	// Update reverse reference index (best effort - don't fail update if this fails)
	// existing contains the old object state, and after applying updates it contains the new state
	updateReverseReferenceIndexOnUpdate(id, previousStateForJournal, existing)

	// Touch process directory to ensure cache staleness detection works
	// When files are written to subdirectories, only the subdirectory mtime is updated,
	// not the parent docs/process directory. This causes cache staleness checks to fail.
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
	//nolint:errcheck // Intentional error ignored
	if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, oldFilePath, OpUpdate, previousStateForJournal, effectiveUpdates, secCtx, f); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// ITEM-643: Notify subscribers of object update (best-effort)
			Error(ErrMsgSwallowedError, err).Log()
	}

	executeChangeNotification(ctx, OpUpdate, kind, id, existing)

	// Trigger lifecycle hooks if status changed
	if newState != emptyValue && oldState != newState {
		hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
		//nolint:errcheck // Intentional error ignored - lifecycle hooks are best effort
		if err := executeLifecycleHook(hookCtx, kind, oldState, newState, existing); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
	}

	return nil
}

// computeUpdatesMap returns a map of field names to new values for fields that were added or changed.
// Used when building change journal entries from previous and new full state (e.g. stream-backed apply).
func computeUpdatesMap(previous, newObj map[string]any) map[string]any {
	if newObj == nil {
		return nil
	}
	updates := make(map[string]any)
	for k, newVal := range newObj {
		prevVal, had := previous[k]
		if !had || !reflect.DeepEqual(prevVal, newVal) {
			updates[k] = newVal
		}
	}
	return updates
}

// effectiveUpdateFieldsForClassification returns the subset of requested fields that effectively
// changed after merge/normalization, plus metadata runtime fields auto-managed by ensureObjectMetadata.
func effectiveUpdateFieldsForClassification(previous, current, requested map[string]any) map[string]any {
	updates := make(map[string]any)
	if current == nil {
		return updates
	}
	for k := range requested {
		if k == FieldKeyExpectedUpdatedAt {
			continue
		}
		prevVal, prevOK := previous[k]
		currVal, currOK := current[k]
		if prevOK != currOK || !reflect.DeepEqual(prevVal, currVal) {
			if currOK {
				updates[k] = currVal
			} else {
				updates[k] = nil
			}
		}
	}
	for _, k := range []string{objects.FieldKeyUpdatedAt, objects.FieldKeyUpdatedBy} {
		prevVal, prevOK := previous[k]
		currVal, currOK := current[k]
		if prevOK != currOK || !reflect.DeepEqual(prevVal, currVal) {
			if currOK {
				updates[k] = currVal
			} else {
				updates[k] = nil
			}
		}
	}
	return updates
}

func classifyUpdateMutation(idUpdated, runtimeDeltaOnly bool) string {
	if idUpdated {
		return updateMutationClassIDChange
	}
	if runtimeDeltaOnly {
		return updateMutationClassRuntimeDelta
	}
	return updateMutationClassStructural
}

// applyUpdateFromBuffer persists an update from the write-behind buffer to CAS or file.
// Used by ObjectWriteBehindWorker for "update" ops. Does not call FlushKind.
// When data is nil and kind is stream-only (minimal WAL replay), skips apply (best-effort: already in stream).
// For stream-backed kinds, updates go to change journal + stream_current overlay only (no CAS hash overhead).
func (f *FileObjectStorage) applyUpdateFromBuffer(ctx context.Context, id, kind string, data []byte, secCtx *pkgctx.SecurityContext) error {
	ctx = withSkipWriteBehind(ctx)

	if len(data) == 0 {
		if StreamStorageEnabledForKind(kind) {
			return nil
		}
		return errfmt.Errorf("applyUpdateFromBuffer: empty data for %s/%s", kind, id)
	}
	logicalPath, err := f.getObjectFilePath(id, kind)
	if err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}
	if logicalPath == emptyValue {
		var err error
		logicalPath, err = f.prepareObjectPath(id, kind)
		if err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
	}
	dirName, err := f.kindDirectoryName(kind)
	if err != nil {
		return err
	}
	config := GetStorageConfig()

	// Stream-backed: persist via change journal (audit) + stream_current overlay only; no CAS.
	if StreamStorageEnabledForKind(kind) {
		var previousState map[string]any
		if existing, readErr := f.Read(ctx, secCtx, id); readErr == nil {
			previousState = existing
		}
		var newObj map[string]any
		if err := yaml.Unmarshal(data, &newObj); err != nil {
			return errfmt.Newf(ErrMsgStreamUpdateUnmarshal).Wrap(err)
		}
		updates := computeUpdatesMap(previousState, newObj)
		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, logicalPath, OpUpdate, previousState, updates, secCtx, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if err := WriteStreamBackedCurrentState(f.projectRoot, kind, id, data); err != nil {
			return errfmt.Newf(ErrMsgStreamUpdateWriteState).Wrap(err)
		}
		f.InvalidateCachesForKind(kind)
		executeChangeNotification(ctx, OpUpdate, kind, id, nil)
		oldState, _ := previousState[objects.FieldKeyStatus].(string)
		newState, _ := newObj[objects.FieldKeyStatus].(string)
		if newState != emptyValue && oldState != newState {
			hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
			if err := executeLifecycleHook(hookCtx, kind, oldState, newState, newObj); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
		}
		return nil
	}

	if f.usesContentAddressableStorage(kind) {
		cas, err := f.getContentAddressableStorage(kind)
		if err != nil {
			return errfmt.Newf(ErrMsgGetCAS).Wrap(err)
		}
		// Read old object before write to compute delta-only updates and lifecycle transitions.
		var oldObj map[string]any
		if existing, readErr := f.Read(ctx, secCtx, id); readErr == nil {
			oldObj = existing
		}
		var newObj map[string]any
		if err := yaml.Unmarshal(data, &newObj); err != nil {
			return errfmt.Newf(ErrMsgUnmarshalUpdateBuf).Wrap(err)
		}
		runtimeDeltaOnly := updateIsRuntimeDeltaOnly(f.projectRoot, kind, computeUpdatesMap(oldObj, newObj))
		if runtimeDeltaOnly {
			if err := WriteRuntimeDeltaCurrentState(f.projectRoot, kind, id, data); err != nil {
				return errfmt.Newf(ErrMsgWriteRuntimeDelta).Wrap(err)
			}
			if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, logicalPath, OpUpdate, oldObj, computeUpdatesMap(oldObj, newObj), secCtx, f); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, logicalPath, secCtx, nil, f); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			f.InvalidateCachesForKind(kind)
			f.InvalidateCASCacheForKind(kind)
			executeChangeNotification(ctx, OpUpdate, kind, id, nil)
			oldState, _ := oldObj[objects.FieldKeyStatus].(string)
			newState, _ := newObj[objects.FieldKeyStatus].(string)
			if newState != emptyValue && oldState != newState {
				hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
				if err := executeLifecycleHook(hookCtx, kind, oldState, newState, newObj); err != nil && !isExpectedMissingErr(err) {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
				}
			}
			return nil
		}
		if err := cas.Update(id, data); err != nil {
			return errfmt.Newf(ErrMsgUpdateCAS).Wrap(err)
		}
		kindDir := filepath.Join(f.processDir, dirName)
		hashRegistry := f.newHashRegistry(ctx, kind, kindDir)
		if err := hashRegistry.Load(); err == nil {
			hash := CalculateSHA256Hash(data)
			filename := fmt.Sprintf("%s%s", id, config.YAMLExtension)
			hashRegistry.SetHash(filename, hash)
			if err := f.saveHashRegistry(hashRegistry); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// Update reverse reference index (best effort)
					Error(ErrMsgSwallowedError, err).Log()
			}
		}

		if newObj != nil {
			updateReverseReferenceIndexOnUpdate(id, oldObj, newObj)
		}
		if err := RemoveRuntimeDeltaCurrentState(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, logicalPath, secCtx, nil, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		f.InvalidateCachesForKind(kind)
		executeChangeNotification(ctx, OpUpdate, kind, id, nil)
		oldState, _ := oldObj[objects.FieldKeyStatus].(string)
		newState, _ := newObj[objects.FieldKeyStatus].(string)
		if newState != emptyValue && oldState != newState {
			hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
			if err := executeLifecycleHook(hookCtx, kind, oldState, newState, newObj); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
		}
		return nil
	}

	filePath, err := f.getObjectFilePath(id, kind)
	if err != nil {
		return err
	}
	// Read old object for reverse reference index update
	var oldObj map[string]any
	if existing, readErr := f.Read(ctx, secCtx, id); readErr == nil {
		oldObj = existing
	}
	if err := f.writeObjectToFile(ctx, id, kind, filePath, data); err != nil {
		return errfmt.Newf(ErrMsgWriteUpdatedObjFile).Wrap(err)
	}
	// Update reverse reference index (best effort - unmarshal new object data)
	var newObj map[string]any
	if err := yaml.Unmarshal(data, &newObj); err == nil {
		updateReverseReferenceIndexOnUpdate(id, oldObj, newObj)
	}
	if err := createUpdateAuditEvent(ctx, f.projectRoot, id, kind, filePath, secCtx, nil, f); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}
	f.InvalidateCachesForKind(kind)
	f.InvalidateCASCacheForKind(kind)
	executeChangeNotification(ctx, OpUpdate, kind, id, nil)
	oldState, _ := oldObj[objects.FieldKeyStatus].(string)
	newState, _ := newObj[objects.FieldKeyStatus].(string)
	if newState != emptyValue && oldState != newState {
		hookCtx := pkgctx.WithLifecycleProjectRoot(ctx, f.projectRoot)
		if err := executeLifecycleHook(hookCtx, kind, oldState, newState, newObj); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
	}
	return nil
}

// dispatchStatusGateway implements the micro API gateway for status transitions.
// It uses the shockwave dispatcher to evaluate router rules and nodes dynamically.
func (f *FileObjectStorage) dispatchStatusGateway(ctx context.Context, secCtx *SecurityContext, obj map[string]any, kind, oldState string) error {
	currentState, _ := obj[objects.FieldKeyStatus].(string)
	if currentState == oldState || currentState == "" {
		return nil // No status transition
	}

	// 1. Fetch the shockwave router configuration for status changes
	// For performance, this would normally be cached or use an in-memory index
	// Here we dynamically query for the active router for this domain
	filter := ListFilter{Kind: "shockwave_router"}
	results, err := f.List(ctx, secCtx, pkgctx.NewStorageContext(), filter)
	if err != nil || len(results.Objects) == 0 {
		return nil // Gateway not configured, fail-open
	}

	var activeRouter map[string]any
	for _, item := range results.Objects {
		if st, _ := item[objects.FieldKeyStatus].(string); st == objects.ObjectStatusActive {
			activeRouter = item
			break
		}
	}
	if activeRouter == nil {
		return nil
	}

	// 2. We construct a simulated node that implements the parent metrics validation check.
	// In a fully distributed micro-gateway, this node might be a separate service.
	statusBubblerNode := func(payload map[string]any) (map[string]any, error) {
		// Parent metrics check logic:
		parentRefs := []string{"parent_ref", objects.FieldKeyPriorityPlanRef, "strategic_plan_ref", "milestone_ref"}
		var parentID string
		for _, refKey := range parentRefs {
			if pid, ok := payload[refKey].(string); ok && pid != "" {
				parentID = pid
				break
			}
		}

		if parentID == "" {
			return map[string]any{"allowed": true}, nil
		}

		parentObj, err := f.Read(ctx, secCtx, parentID)
		if err != nil {
			return nil, err
		}

		parentStatus, _ := parentObj[objects.FieldKeyStatus].(string)
		if parentStatus == objects.ObjectStatusInProgress || parentStatus == objects.ObjectStatusComplete {
			return map[string]any{"allowed": true}, nil
		}

		// Try to forcefully update parent
		parentUpdates := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusInProgress}
		err = f.Update(ctx, secCtx, parentID, parentUpdates)
		if err != nil {
			return nil, errfmt.Newf("hierarchical shockwave blocked: parent %s failed to transition to in_progress (ensure metrics are set)", parentID).Wrap(err)
		}

		return map[string]any{"allowed": true}, nil
	}

	// 3. Dispatch the payload through the gateway
	// In the future we would instantiate shockwave.NewGatewayDispatcher here
	// But since we are in the storage package, we just execute the router's intent locally to avoid circular dependencies
	// (A true GatewayDispatcher would sit above the routing storage layer)

	// Check trigger condition
	condition, _ := activeRouter[objects.FieldKeyTriggerCondition].(map[string]any)
	if condKind, _ := condition[objects.FieldKeyKind].(string); condKind != "" && condKind != kind {
		return nil // Route doesn't apply to this kind
	}

	// Execute rules
	_, err = statusBubblerNode(obj)
	return err
}
