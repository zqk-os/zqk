package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
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
	if err := os.MkdirAll(newDirPath, config.DefaultDirPerm); err != nil {
		return errfmt.Newf(ConstStreamFailedToCreateTargetDirectory).Wrap(err)
	}

	newFilePath := filepath.Join(newDirPath, filepath.Base(oldFilePath))

	// Check if target file already exists
	if _, err := os.Stat(newFilePath); err == nil {
		return errfmt.Errorf(ConstStreamTargetFileAlreadyExistsStr, newFilePath)
	}

	// Read old file content for hash calculation
	fileData, err := os.ReadFile(oldFilePath)
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
	f.ensureObjectMetadata(existing, secCtx, false)

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
		var err_swallow_34 = os.Remove(newFilePath)
		if err_swallow_34 != nil {
			logging.LogSwallowedError(err_swallow_34)
		}
		return errfmt.Errorf(ConstStreamFailedToPersistHashRegistryForNewLocationErrMove, err)
	}

	// Remove old file
	if err := os.Remove(oldFilePath); err != nil {
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
		var err_swallow_35 = f.saveHashRegistry(oldHashRegistry)
		if err_swallow_35 !=

			// Update references if requested
			nil {
			logging.LogSwallowedError(err_swallow_35)
		}
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
	var err_swallow_33 = createMoveAuditEvent(ctx, f.projectRoot, id, oldKind, newKind, oldFilePath, newFilePath, secCtx, f)
	if err_swallow_33 !=

		// updateReferencesForMovedObject updates all references pointing to a moved object
		nil {
		logging.LogSwallowedError(err_swallow_33)
	}

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
				if f.referenceMatches(v, objectID, oldKind) {
					newRef := f.buildNewReference(v, objectID, newKind)
					dependent[fieldName] = newRef
					updated = true
				}
			case

				// Handle reference arrays
				[]any:
				updatedArray := false
				for i, refItem := range v {
					if refStr, ok := refItem.(string); ok {
						if f.referenceMatches(refStr, objectID, oldKind) {
							v[i] = f.buildNewReference(refStr, objectID, newKind)
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

// Rename changes an object's ID (same kind). ITEM-851.
func (f *FileObjectStorage) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	if !IsCLIOperation(ctx, secCtx) {
		return errfmt.Errorf(ConstStreamRenameOperationsMustBePerformedThroughCli)
	}

	existing, err := f.Read(ctx, secCtx, oldID)
	if err != nil {
		return err
	}

	kind, ok := existing[objects.FieldKeyKind].(string)
	if !ok {
		return errfmt.Errorf(ConstStreamObjectMissingKindField2)
	}

	if newID == oldID {
		return errfmt.Errorf(ConstStreamNewIdIsTheSameAsCurrentIdNoRenameNeeded)
	}

	// Validate new ID format and that it doesn't already exist (Update does this too; we do it here for clear errors)
	if err := f.idValidator.LoadPatterns(); err != nil {
		return errfmt.Newf(ConstStreamFailedToLoadIdPatterns).Wrap(err)
	}
	valid, err := f.idValidator.ValidateID(newID, kind)
	if err != nil {
		return errfmt.Newf(ErrMsgValidateNewID).Wrap(err)
	}
	if !valid {
		return errfmt.Errorf(ErrMsgInvalidIDFormat, kind, newID)
	}
	exists, err := f.Exists(ctx, secCtx, newID)
	if err != nil {
		return errfmt.Newf(ConstStreamFailedToCheckIfNewIdExists).Wrap(err)
	}
	if exists {
		return errfmt.Errorf(ConstStreamObjectWithIdStrAlreadyExists, newID)
	}

	// Check for blocking issues and permission
	if err := f.checkForBlockingIssuesBeforeWrite(ctx, "rename", kind, oldID); err != nil {
		return err
	}
	if err := f.checkPermission(secCtx, "write", kind); err != nil {
		return errfmt.Errorf(ConstStreamPermissionDeniedForKindStrErr, kind, err)
	}

	// Perform ID change via Update (handles file move, hash registry, reverse index, audit, cache)
	opCtx := pkgctx.WithCacheIDChange(ctx, oldID, newID, kind, "")
	if err := f.Update(opCtx, secCtx, oldID, map[string]any{objects.FieldKeyID: newID}); err != nil {
		return err
	}

	if updateReferences {
		if err := f.updateReferencesForRenamedObject(ctx, secCtx, oldID, newID, kind); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectMoveUpdateRefsAfterRenameFail).WithError(err).Log()
		}
	}

	return nil
}

// updateReferencesForRenamedObject updates all references pointing to oldID to use newID (same kind).
func (f *FileObjectStorage) updateReferencesForRenamedObject(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID, kind string) error {
	dependents, err := f.findDependents(ctx, oldID, kind)
	if err != nil {
		return errfmt.Newf(ConstStreamFailedToFindDependents).Wrap(err)
	}

	for _, dependentID := range dependents {
		dependent, err := f.Read(ctx, secCtx, dependentID)
		if err != nil {
			continue
		}

		updated := false
		for fieldName, fieldValue := range dependent {
			if !strings.HasSuffix(fieldName, "_ref") && !strings.HasSuffix(fieldName, "_refs") {
				continue
			}
			switch v := fieldValue.(type) {
			case string:
				if f.referenceMatches(v, oldID, kind) {
					dependent[fieldName] = f.buildReferenceWithNewID(v, oldID, newID)
					updated = true
				}
			case []any:
				updatedArray := false
				for i, refItem := range v {
					if refStr, ok := refItem.(string); ok && f.referenceMatches(refStr, oldID, kind) {
						v[i] = f.buildReferenceWithNewID(refStr, oldID, newID)
						updatedArray = true
					}
				}
				if updatedArray {
					dependent[fieldName] = v
					updated = true
				}
			}
		}

		if updated {
			updates := make(map[string]any)
			for fieldName, fieldValue := range dependent {
				if strings.HasSuffix(fieldName, "_ref") || strings.HasSuffix(fieldName, "_refs") {
					updates[fieldName] = fieldValue
				}
			}
			if err := f.Update(ctx, secCtx, dependentID, updates); err != nil {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				StorageLog(logger).Warn(LogEventStorageObjectMoveUpdateDependentsAfterRenameFailed).
					String("dependent_id", dependentID).
					WithError(err).
					Log()
			}
		}
	}

	return nil
}

// buildReferenceWithNewID replaces oldID with newID in a reference string, preserving format (kind:id or plain id).
func (f *FileObjectStorage) buildReferenceWithNewID(oldRef, oldID, newID string) string {
	parsed := validation.ParseNamespace(oldRef)
	if parsed != nil && parsed.ObjectID == oldID {
		if parsed.Layer != emptyValue && parsed.Domain != emptyValue {
			return fmt.Sprintf("%s:%s:%s:%s", parsed.Layer, parsed.Domain, parsed.ObjectType, newID)
		}
		if parsed.Layer != emptyValue {
			return fmt.Sprintf("%s:%s:%s", parsed.Layer, parsed.ObjectType, newID)
		}
		if parsed.ObjectType != emptyValue {
			return fmt.Sprintf("%s:%s", parsed.ObjectType, newID)
		}
		return newID
	}
	if strings.Contains(oldRef, ":") {
		parts := strings.SplitN(oldRef, ":", 2)
		if len(parts) == 2 && parts[1] == oldID {
			return fmt.Sprintf("%s:%s", parts[0], newID)
		}
	}
	if oldRef == oldID {
		return newID
	}
	return oldRef
}

// referenceMatches checks if a reference string matches the given object ID and kind
func (f *FileObjectStorage) referenceMatches(refStr, objectID, kind string) bool {
	// Parse reference to extract ID
	parsed := validation.ParseNamespace(refStr)
	if parsed != nil && parsed.ObjectID == objectID {
		// Check if kind matches (or is empty/legacy format)
		if parsed.ObjectType == kind || parsed.ObjectType == emptyValue {
			return true
		}
	}

	// Check legacy format "kind:id"
	if strings.Contains(refStr, ":") {
		parts := strings.SplitN(refStr, ":", 2)
		if len(parts) == 2 && parts[0] == kind && parts[1] == objectID {
			return true
		}
	}

	// Check if it's just the ID (legacy format)
	return refStr == objectID
}

// buildNewReference builds a new reference string with the updated kind
func (f *FileObjectStorage) buildNewReference(oldRef, objectID, newKind string) string {
	// Parse old reference to preserve namespace if present
	parsed := validation.ParseNamespace(oldRef)
	if parsed != nil {
		// Preserve namespace, update kind
		if parsed.Layer != emptyValue && parsed.Domain != emptyValue {
			return fmt.Sprintf("%s:%s:%s:%s", parsed.Layer, parsed.Domain, newKind, objectID)
		}
		if parsed.Layer != emptyValue {
			return fmt.Sprintf("%s:%s:%s", parsed.Layer, newKind, objectID)
		}
	}

	// Default: use "kind:id" format
	return fmt.Sprintf("%s:%s", newKind, objectID)
}

// createMoveAuditEvent creates an audit event for a move operation
// fileStorage is optional - if provided and CAS is enabled, routes through CAS
//
//nolint:unparam // Always returns nil error - audit events are best-effort
func createMoveAuditEvent(ctx context.Context, projectRoot, id, oldKind, newKind, oldPath, newPath string, secCtx *pkgctx.SecurityContext, fileStorage *FileObjectStorage) error {
	// CRITICAL: Use fileStorage's project root if provided and projectRoot is empty
	// This ensures deterministic project root resolution - no auto-discovery fallback
	if projectRoot == emptyValue && fileStorage != nil {
		projectRoot = fileStorage.GetProjectRoot()
	}

	if projectRoot == emptyValue {
		// Can't create audit event without project root
		// Don't fall back to auto-discovery - this causes non-deterministic behavior
		return nil // Best effort - don't fail move
	}

	// Get relative file paths
	oldRelPath, err := filepath.Rel(projectRoot, oldPath)
	if err != nil {
		oldRelPath = oldPath
	}
	newRelPath, err := filepath.Rel(projectRoot, newPath)
	if err != nil {
		newRelPath = newPath
	}

	// Build operation description
	operation := fmt.Sprintf(ConstStreamMovedObjectStrFromKindStrToStr, id, oldKind, newKind)

	// Build metadata
	metadata := map[string]any{
		"old_kind":             oldKind,
		"old_path":             oldRelPath,
		"new_kind":             newKind,
		"new_path":             newRelPath,
		objects.FieldKeySource: "cli",
		"project_root":         projectRoot,
	}

	// Use instance builder helper to create audit event
	// Use the context from Move operation for proper cancellation/timeout propagation
	options := &AuditEventOptions{
		EventType:  "object_move",
		Operation:  operation,
		TargetKind: newKind,
		TargetID:   id,
		TargetPath: newRelPath,
		Severity:   "medium",
		Metadata:   metadata,
	}

	// Use the provided fileStorage if available (file backend)
	// If fileStorage is nil, pass nil to CreateAuditEventWithBuilder which will create
	// the appropriate storage provider (file or graph) via NewStorageFactory
	// CRITICAL: Do NOT create FileObjectStorage here - that would set up global singletons
	// even when using graph backend. Let CreateAuditEventWithBuilder handle backend detection.
	var storageProvider ObjectStorageProvider = fileStorage
	// If fileStorage is nil, pass nil - CreateAuditEventWithBuilder will detect backend type
	// and create the appropriate storage provider (file or graph) via NewStorageFactory

	// Use the provided context for proper cancellation/timeout propagation
	return CreateAuditEventWithBuilder(ctx, projectRoot, secCtx, storageProvider, options)
}
