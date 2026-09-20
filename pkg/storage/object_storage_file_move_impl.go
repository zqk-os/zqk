package storage

import (
	"context"
	"path/filepath"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/crud"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Move moves an object to a different directory/kind while preserving history
func (f *FileObjectStorage) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, newKind string, updateReferences bool) error {
	// Require CLI authorization for moves
	if !IsCLIOperation(ctx, secCtx) {
		return errfmt.Errorf(ConstStreamMoveOperationsMustBePerformedThroughCli)
	}

	// Read existing object
	existing, err := f.Read(ctx, secCtx, id)
	if err != nil {
		return err
	}

	oldKind, ok := existing[objects.FieldKeyKind].(string)
	if !ok {
		return errfmt.Errorf(ConstStreamObjectMissingKindField2)
	}

	// Validate new kind
	if newKind == emptyValue {
		return errfmt.Errorf(ConstStreamNewKindCannotBeEmpty)
	}

	// Check if kind is actually changing
	if oldKind == newKind {
		return errfmt.Errorf(ConstStreamObjectIsAlreadyOfKindStrNoMoveNeeded, newKind)
	}

	// Validate new kind exists (check if directory mapping exists)
	newDir := objects.GetDirectoryFromKind(newKind)
	if newDir == emptyValue {
		return errfmt.Errorf(ConstStreamInvalidKindStrNoDirectoryMappingFound, newKind)
	}

	// Check for blocking issues before write operations
	if err := f.checkForBlockingIssuesBeforeWrite(ctx, "move", oldKind, id); err != nil {
		return err
	}

	// Check permissions (requires write permission for both old and new kinds)
	if err := f.checkPermission(secCtx, "write", oldKind); err != nil {
		return errfmt.Errorf(ConstStreamPermissionDeniedForSourceKindStrErr, oldKind, err)
	}
	if err := f.checkPermission(secCtx, "write", newKind); err != nil {
		return errfmt.Errorf(ConstStreamPermissionDeniedForTargetKindStrErr, newKind, err)
	}

	// Get old file path
	oldFilePath, err := f.getObjectFilePath(id, oldKind)
	if err != nil {
		return err
	}

	// Get new file path
	config := GetStorageConfig()
	newDirPath := datacell.CellCASPrimaryDir(f.projectRoot, newDir)
	if err := fileutil.MkdirAll(newDirPath, config.DefaultDirPerm); err != nil {
		return errfmt.Newf(ConstStreamFailedToCreateTargetDirectory).Wrap(err)
	}

	newFilePath := filepath.Join(newDirPath, filepath.Base(oldFilePath))

	// Check if target file already exists
	if _, err := fileutil.Stat(newFilePath); err == nil {
		return errfmt.Errorf(ConstStreamTargetFileAlreadyExistsStr, newFilePath)
	}

	// Read old file content for hash calculation
	fileData, err := fileutil.ReadFile(oldFilePath)
	if err != nil {
		return errfmt.Newf(ConstStreamFailedToReadSourceFile).Wrap(err)
	}

	// Verify source file integrity when kind uses CAS index (same guarantee as Read)
	if f.usesContentAddressableStorage(oldKind) {
		cas, err := f.getContentAddressableStorage(oldKind)
		if err == nil {
			expectedHash, hashErr := cas.GetHashForID(id)
			if hashErr == nil {
				if err := VerifyContentHash(fileData, expectedHash); err != nil {
					return errfmt.Newf(ConstStreamSourceFileIntegrityCheckFailed).Wrap(err)
				}
			}
		}
	} else if isHashBasedFilename(filepath.Base(oldFilePath)) {
		// Hash-named file in a non-CAS-index kind dir (e.g. discovered file): verify content matches filename
		if err := f.VerifyOrReconcileCASHash(ctx, oldKind, id, oldFilePath, fileData); err != nil {
			return errfmt.Newf(ConstStreamSourceFileIntegrityCheckFailed).Wrap(err)
		}
	}
	// Plain ID-based files (no CAS index, no 64-char hash filename): already validated by Read at start of Move

	// Update object kind (preserve all other fields including history)
	existing[objects.FieldKeyKind] = newKind
	// Update updated_at and updated_by (move is an update operation)
	f.ensureObjectMetadata(ctx, existing, secCtx, false)

	// Validate object with new kind
	status, _ := existing[objects.FieldKeyStatus].(string)

	// If status is invalid for new kind, try to find a valid default status
	// This allows moving objects between kinds with different lifecycle states
	if status != emptyValue {
		// Try validating with current status first
		if err := f.validateObject(ctx, existing, newKind, status); err != nil {
			// Status is invalid for new kind - try to get a default status
			lifecycleLoader := objects.NewLifecycleLoader("")
			lifecycle, err := lifecycleLoader.LoadLifecycle(newKind)
			if err == nil && lifecycle != nil && len(lifecycle.Statuses) > 0 {
				// Try to find an initial status first
				var defaultStatus string
				for _, s := range lifecycle.Statuses {
					if s.Origin {
						defaultStatus = s.Value
						break
					}
				}
				// If no initial status, use the first one
				if defaultStatus == emptyValue {
					defaultStatus = lifecycle.Statuses[0].Value
				}
				oldStatus := status
				existing[objects.FieldKeyStatus] = defaultStatus
				status = defaultStatus
				// Log the status change
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				StorageLog(logger).Info(LogEventStorageObjectMoveStatusLifecycleInfo).
					ObjectID(id).
					String("old_kind", oldKind).
					String("new_kind", newKind).
					String("old_status", oldStatus).
					String("new_status", defaultStatus).
					Log()
			} else {
				// No lifecycle found or no statuses - clear status
				delete(existing, objects.FieldKeyStatus)
				status = ""
			}
		}
	}

	// Final validation
	if err := f.validateObject(ctx, existing, newKind, status); err != nil {
		return errfmt.Newf(ConstStreamValidationFailedForNewKind).Wrap(err)
	}

	// Write to new location
	if err := f.writeObjectFile(ctx, newFilePath, existing); err != nil {
		return errfmt.Newf(ConstStreamFailedToWriteObjectToNewLocation).Wrap(err)
	}

	// Update hash registry for new file
	newHashRegistry := f.newHashRegistry(ctx, newKind, newDirPath)
	if err := newHashRegistry.Load(); err != nil {
		// Log warning but continue
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageObjectMoveHashRegistryLoadFailed).WithError(err).Log()
	}
	hash := f.calculateHash(fileData)
	newFilename := filepath.Base(newFilePath)
	newHashRegistry.SetHash(newFilename, hash)
	if err := f.saveHashRegistryWithRetry(newHashRegistry, id, newFilename, hash); err != nil {
		logging.LogSwallowedError(fileutil.Remove(newFilePath))
		return errfmt.Errorf(ConstStreamFailedToPersistHashRegistryForNewLocationErrMove, err)
	}

	// Remove old file
	if err := fileutil.Remove(oldFilePath); err != nil {
		// Log warning but don't fail - file might already be moved
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageObjectMoveRemoveOldFileFailed).
			String("old_path", oldFilePath).
			WithError(err).
			Log()
	}

	// Update hash registry for old file (remove old hash)
	oldHashRegistry := f.newHashRegistry(ctx, oldKind, filepath.Dir(oldFilePath))
	if err := oldHashRegistry.Load(); err == nil {
		oldHashRegistry.DeleteHash(filepath.Base(oldFilePath))
		logging.LogSwallowedError(f.saveHashRegistry(oldHashRegistry))
	}

	if updateReferences {
		if err := f.updateReferencesForMovedObject(ctx, secCtx, id, oldKind, newKind); err != nil {
			// Log warning but don't fail move - references can be updated later
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectMoveUpdateRefsFailed).WithError(err).Log()
		}
	}

	// Execute cache operation (update cache for moved object)
	if err := executeCacheOperation(ctx, newFilePath); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageObjectMoveCacheFailed).
			ObjectID(id).
			String("old_kind", oldKind).
			String("new_kind", newKind).
			WithError(err).
			Log()
	}

	// Invalidate list cache for both old and new kinds (move affects both)
	// executeCacheOperation already invalidates newKind, but we need to invalidate oldKind too
	if oldKind != newKind {
		f.InvalidateCachesForKind(oldKind)
	}

	// Record state change
	RecordObjectStateChange(ctx, "move", id)

	// Create change journal entry for the move
	previousStateForJournal := map[string]any{
		objects.FieldKeyKind: oldKind,
	}
	updatesForJournal := map[string]any{
		objects.FieldKeyKind: newKind,
	}
	var err_swallow_32 = createChangeJournalEntry(ctx, f.projectRoot, id, newKind, newFilePath, "move", previousStateForJournal, updatesForJournal, secCtx, f)
	if err_swallow_32 !=

		// Create audit event for move
		//nolint:errcheck // Intentional error ignored
		nil {
		logging.LogSwallowedError(err_swallow_32)
	}
	logging.LogSwallowedError(createMoveAuditEvent(ctx, f.projectRoot, id, oldKind, newKind, oldFilePath, newFilePath, secCtx, f))

	return nil
}

func (f *FileObjectStorage) updateReferencesForMovedObject(ctx context.Context, secCtx *pkgctx.SecurityContext, objectID string, oldKind, newKind string) error {
	// Find all objects that reference this object
	dependents, err := f.findDependents(ctx, objectID, oldKind)
	if err != nil {
		return errfmt.Newf(ConstStreamFailedToFindDependents).Wrap(err)
	}

	// Update each dependent's references
	for _, dependentID := range dependents {
		dependent, err := f.Read(ctx, secCtx, dependentID)
		if err != nil {
			// Skip if not found
			continue
		}

		// Find all reference fields and update them
		updated := false
		for fieldName, fieldValue := range dependent {
			if !strings.HasSuffix(fieldName, "_ref") && !strings.HasSuffix(fieldName, "_refs") {
				continue
			}
			switch

			// Handle single reference
			v := fieldValue.(type) {
			case string:
				if crud.ReferenceMatches(v, objectID, oldKind) {
					newRef := crud.BuildNewReference(v, objectID, newKind)
					dependent[fieldName] = newRef
					updated = true
				}
			case

				// Handle reference arrays
				[]any:
				updatedArray := false
				for i, refItem := range v {
					if refStr, ok := refItem.(string); ok {
						if crud.ReferenceMatches(refStr, objectID, oldKind) {
							v[i] = crud.BuildNewReference(refStr, objectID, newKind)
							updatedArray = true
						}
					}
				}
				if updatedArray {
					dependent[fieldName] = v
					updated = true
				}
			}
		}

		// Handle string arrays (common for reference fields)

		// Update dependent if references changed
		if updated {
			// Build updates map with changed reference fields
			updates := make(map[string]any)
			for fieldName, fieldValue := range dependent {
				if strings.HasSuffix(fieldName, "_ref") || strings.HasSuffix(fieldName, "_refs") {
					updates[fieldName] = fieldValue
				}
			}
			if err := f.Update(ctx, secCtx, dependentID, updates); err != nil {
				// Log but continue with other dependents
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				StorageLog(logger).Warn(LogEventStorageObjectMoveUpdateDependentsFailed).
					String("dependent_id", dependentID).
					WithError(err).
					Log()
			}
		}
	}

	return nil
}

// Rename changes an object's ID (same kind). BLI-851.
