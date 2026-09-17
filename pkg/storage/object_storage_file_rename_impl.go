package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/crud"
)

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
// Rewrites *_ref / *_refs and identity fields (created_by, updated_by, account_id).
func (f *FileObjectStorage) updateReferencesForRenamedObject(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID, kind string) error {
	depSet := make(map[string]struct{})
	dependents, err := f.findDependents(ctx, oldID, kind)
	if err != nil {
		return errfmt.Newf(ConstStreamFailedToFindDependents).Wrap(err)
	}
	for _, id := range dependents {
		depSet[id] = struct{}{}
	}
	// created_by / updated_by / account_id are often missing from the reverse-ref index;
	// scan inventory so account renames rewrite attribution and keystore links.
	for _, id := range f.findIdentityFieldDependents(ctx, secCtx, oldID, kind) {
		depSet[id] = struct{}{}
	}

	for dependentID := range depSet {
		if dependentID == newID || dependentID == oldID {
			continue
		}
		dependent, err := f.Read(ctx, secCtx, dependentID)
		if err != nil {
			continue
		}

		updates := make(map[string]any)
		for _, fieldName := range collectRewritableIdentityFields(dependent) {
			fieldValue := dependent[fieldName]
			switch v := fieldValue.(type) {
			case string:
				if crud.ReferenceMatches(v, oldID, kind) {
					updates[fieldName] = crud.BuildReferenceWithNewID(v, oldID, newID)
				}
			case []any:
				updatedArray := false
				newArr := make([]any, len(v))
				copy(newArr, v)
				for i, refItem := range newArr {
					if refStr, ok := refItem.(string); ok && crud.ReferenceMatches(refStr, oldID, kind) {
						newArr[i] = crud.BuildReferenceWithNewID(refStr, oldID, newID)
						updatedArray = true
					}
				}
				if updatedArray {
					updates[fieldName] = newArr
				}
			case []string:
				updatedArray := false
				newArr := make([]string, len(v))
				copy(newArr, v)
				for i, refStr := range newArr {
					if crud.ReferenceMatches(refStr, oldID, kind) {
						newArr[i] = crud.BuildReferenceWithNewID(refStr, oldID, newID)
						updatedArray = true
					}
				}
				if updatedArray {
					updates[fieldName] = newArr
				}
			}
		}

		if len(updates) == 0 {
			continue
		}
		// created_by is immutable unless force override — required for ACC-* attribution cutover.
		// TRACK: BLI-REDACTED
		updateCtx := pkgctx.WithLifecycleBreakGlass(ctx, "ACC account id cutover rewrite identity fields")
		if err := f.Update(updateCtx, secCtx, dependentID, updates); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectMoveUpdateDependentsAfterRenameFailed).
				String("dependent_id", dependentID).
				WithError(err).
				Log()
		}
	}

	return nil
}

// findIdentityFieldDependents lists object IDs whose created_by, updated_by, or account_id
// (or *_ref/_refs) match oldID. Used when reverse-ref index does not cover attribution fields.
// Full inventory scan runs for the system account (ubiquitous created_by); other account
// renames only scan keystore_entry for account_id links.
func (f *FileObjectStorage) findIdentityFieldDependents(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, kind string) []string {
	if kind != objects.KindAccount {
		return nil
	}
	var kinds []string
	fullInventory := oldID == pkgctx.SystemAccountID ||
		oldID == "account:system" ||
		strings.HasSuffix(oldID, ":system")
	if fullInventory {
		fieldRegistry := objects.GetGlobalFieldRegistry()
		if err := fieldRegistry.LoadFields(); err != nil {
			return nil
		}
		allKinds, err := fieldRegistry.GetAllKinds()
		if err != nil || len(allKinds) == 0 {
			return nil
		}
		kinds = allKinds
	} else {
		kinds = []string{objects.KindKeystoreEntry}
	}
	storageCtx := pkgctx.NewStorageContext()
	seen := make(map[string]struct{})
	var out []string
	for _, k := range kinds {
		result, listErr := f.List(ctx, secCtx, storageCtx, ListFilter{Kind: k})
		if listErr != nil || result == nil {
			continue
		}
		for _, obj := range result.Objects {
			id, _ := obj[objects.FieldKeyID].(string)
			if id == emptyValue || id == oldID {
				continue
			}
			for _, fieldName := range collectRewritableIdentityFields(obj) {
				if crud.ObjectFieldReferencesID(obj[fieldName], oldID, kind) {
					if _, ok := seen[id]; !ok {
						seen[id] = struct{}{}
						out = append(out, id)
					}
					break
				}
			}
		}
	}
	return out
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
