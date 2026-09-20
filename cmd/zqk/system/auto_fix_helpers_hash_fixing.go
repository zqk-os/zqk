package system

import (
	"fmt"
	"path/filepath"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

// fixImmutableObjectHash fixes hash for immutable objects using storage provider.
// Prefer the same storageProvider used for the check so the fix persists to the right backend.
func fixImmutableObjectHash(fixCtx *AutoFixContext, fixKind string, originalHash string, isHashMismatch bool, storageProvider storage.ObjectStorageProvider) (bool, string) {
	projectRoot := fixCtx.Ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	fileDir := filepath.Dir(fixCtx.FilePath)
	kindDir := getKindDirectory(projectRoot, fixKind)
	isBucketedForFix := fileDir != kindDir

	if storageProvider == nil {
		var err error
		var storageFactory *storage.StorageFactory
		storageFactory, err = storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
		if err != nil {
			return false, ""
		}
		storageProvider = storageFactory.GetStorage()
	}
	if storageProvider == nil {
		return false, ""
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	updateErr := storageProvider.Update(pkgctx.NewSystemContext(), secCtx, fixCtx.Obj.ID, map[string]any{})
	if updateErr != nil {
		logging.Fluent(fixCtx.Logger).Warn("Storage provider Update() failed for hash fix, falling back to direct registry update").
			ObjectID(fixCtx.Obj.ID).
			Kind(fixKind).
			WithError(updateErr).
			Log()
		return false, ""
	}

	if isBucketedForFix && fixCtx.HashRegistryCache != nil {
		cacheKey := formatHashRegistryCacheKey(fixKind, fileDir)
		fixCtx.HashRegistryCache.Delete(cacheKey)
	}

	var fixedMsg string
	if fixCtx.Force {
		if auditErr := createHashMismatchFixAuditEvent(fixCtx.Ctx, fixCtx.Obj, fixCtx.FilePath, fixKind, originalHash); auditErr != nil {
			logging.Fluent(fixCtx.Logger).Warn("Failed to create audit event").WithError(auditErr).Log()
		}
		fixedMsg = fmt.Sprintf("Regenerated integrity hash for %s (hash mismatch resolved, audit event and change journal entry created)", fixCtx.Obj.ID)
	} else {
		fixedMsg = fmt.Sprintf("Regenerated integrity hash for %s (hash mismatch resolved, change journal entry created)", fixCtx.Obj.ID)
	}

	return true, fixedMsg
}

// selectRegistryForUpdate selects the appropriate registry for updating
func selectRegistryForUpdate(fixCtx *AutoFixContext, fixKind string, registry storage.HashRegistryProvider) storage.HashRegistryProvider {
	projectRoot := fixCtx.Ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	registryToUpdate := getHashRegistryForFile(fixCtx.Cmd.Context(), fixCtx.FilePath, fixKind, projectRoot, fixCtx.HashRegistryCache)

	if registry != nil && registry == registryToUpdate {
		return registryToUpdate
	}

	if registry != nil {
		fileDir := filepath.Dir(fixCtx.FilePath)
		kindDir := getKindDirectory(projectRoot, fixKind)
		isBucketed := fileDir != kindDir

		if !isBucketed {
			if fixCtx.HashRegistryCache != nil {
				fixCtx.HashRegistryCache.Delete(fixKind)
			}
			return registry
		}
	}

	return registryToUpdate
}

// fixHashViaRegistry fixes hash using direct registry update
func fixHashViaRegistry(fixCtx *AutoFixContext, fixKind string, originalHash string, isHashMismatch bool, registryToUpdate storage.HashRegistryProvider) (bool, string) {
	if registryToUpdate == nil {
		fileDir := filepath.Dir(fixCtx.FilePath)
		kindDir := getKindDirectory(fixCtx.Ctx.ProjectRoot, fixKind)
		isBucketed := fileDir != kindDir
		logging.Fluent(fixCtx.Logger).Error("Cannot auto-fix: registry is nil",
			errfmt.Errorf("registry is nil for %s (kind: %s, fileDir: %s)", fixCtx.Obj.ID, fixKind, fileDir)).
			ObjectID(fixCtx.Obj.ID).
			Kind(fixKind).
			File(fixCtx.FilePath).
			String("fileDir", fileDir).
			String("is_bucketed", fmt.Sprintf("%v", isBucketed)).
			Log()
		return false, ""
	}

	if err := updateHashInRegistryWithInstance(fixCtx.Cmd.Context(), fixCtx.Ctx, fixCtx.Obj, fixCtx.FilePath, fixKind, registryToUpdate); err != nil {
		fileDir := filepath.Dir(fixCtx.FilePath)
		logging.Fluent(fixCtx.Logger).Error("Failed to update hash in registry", err).
			ObjectID(fixCtx.Obj.ID).
			Kind(fixKind).
			File(fixCtx.FilePath).
			String("fileDir", fileDir).
			Log()
		return false, ""
	}

	var fixedMsg string
	if isHashMismatch {
		if auditErr := createHashMismatchFixAuditEvent(fixCtx.Ctx, fixCtx.Obj, fixCtx.FilePath, fixKind, originalHash); auditErr != nil {
			logging.Fluent(fixCtx.Logger).Warn("Failed to create audit event").WithError(auditErr).Log()
		}
		fixedMsg = fmt.Sprintf("Regenerated integrity hash for %s (hash mismatch resolved, audit event created)", fixCtx.Obj.ID)
		logging.Fluent(fixCtx.Logger).Info("Auto-fixed hash mismatch").
			ObjectID(fixCtx.Obj.ID).
			String("message", fixedMsg).
			Log()
	} else {
		fixedMsg = fmt.Sprintf("Updated integrity hash for %s", fixCtx.Obj.ID)
		logging.Fluent(fixCtx.Logger).Info("Auto-fixed missing hash").
			ObjectID(fixCtx.Obj.ID).
			String("message", fixedMsg).
			Log()
	}

	return true, fixedMsg
}
