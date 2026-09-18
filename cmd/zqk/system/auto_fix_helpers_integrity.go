package system

import (
	"fmt"
	"path/filepath"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

// fixIntegrityIssue fixes an integrity issue.
// storageProvider should be the same instance used for the check so fixes persist to the right backend.
func fixIntegrityIssue(fixCtx *AutoFixContext, issue Issue, registry storage.HashRegistryProvider, storageProvider storage.ObjectStorageProvider) (bool, string) {
	isHashMismatch := strings.Contains(issue.Message, "Hash mismatch detected")
	isMissingHash := strings.Contains(issue.Message, "No integrity hash recorded")
	isCASIndexOutOfSync := strings.Contains(issue.Message, "CAS index out of sync")
	isCASFileCorruption := strings.Contains(issue.Message, "CAS file corruption detected")

	if !isHashMismatch && !isMissingHash && !isCASIndexOutOfSync && !isCASFileCorruption {
		return false, ""
	}

	fixKind := determineFixKind(fixCtx)
	if fixKind == emptyValue {
		logging.Fluent(fixCtx.Logger).Warn("Cannot auto-fix: unable to determine object kind").
			ObjectID(fixCtx.Obj.ID).
			File(fixCtx.FilePath).
			Log()
		return false, ""
	}

	logging.Fluent(fixCtx.Logger).Debug("Determined fix kind").
		ObjectID(fixCtx.Obj.ID).
		String("fix_kind", fixKind).
		Log()

	// Use caller's storage to detect backend when available; otherwise create factory (same project root)
	projectRoot := fixCtx.Ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	isCASFile := false
	if storageProvider != nil {
		if fs := storage.UnwrapToFileObjectStorage(storageProvider); fs != nil {
			isCASFile = true
		}
	}
	if !isCASFile {
		storageFactory, err := storage.NewStorageFactory(fixCtx.Cmd.Context(), projectRoot)
		if err == nil && storageFactory != nil {
			isCASFile = storageFactory.IsFileBackend(fixKind)
		} else {
			logging.Fluent(fixCtx.Logger).Warn("Failed to create storage factory for backend check, assuming file backend").WithError(err).Log()
			isCASFile = true
		}
	}

	logging.Fluent(fixCtx.Logger).Info("DEBUG isCASFile").
		ObjectID(fixCtx.Obj.ID).
		Kind(fixKind).
		String("is_cas_file", fmt.Sprintf("%v", isCASFile)).
		String("storage_provider_type", fmt.Sprintf("%T", storageProvider)).
		Log()

	// CAS file corruption (filename hash != content hash) or index out-of-sync: force the index and file to match content.
	// Also treat "Hash mismatch detected" as CAS file corruption when the file is hash-named (64-hex.yaml) so tests and
	// callers that pass a minimal message still get fixCASIndexOutOfSync.
	isHashNamedFile := false
	if base := filepath.Base(fixCtx.FilePath); len(base) == 69 && strings.HasSuffix(base, ".yaml") {
		isHashNamedFile = isHexString(base[:64])
	}
	if isCASFile && (isCASIndexOutOfSync || isCASFileCorruption || (isHashMismatch && isHashNamedFile)) {
		logging.Fluent(fixCtx.Logger).Info("Attempting to auto-fix CAS index drift").
			ObjectID(fixCtx.Obj.ID).
			Kind(fixKind).
			File(fixCtx.FilePath).
			Log()
		success, msg := fixCASIndexOutOfSync(fixCtx, fixKind, storageProvider)
		if success {
			return true, msg
		}
		return false, ""
	}

	// originalHash only needed for non-CAS-drift paths (registry/audit); avoid getOriginalHash when Cmd.Context() may be nil (e.g. tests).
	var originalHash string
	if isHashMismatch {
		originalHash = getOriginalHash(fixCtx.FilePath, fixKind, fixCtx)
	}

	// For CAS files, handle both hash mismatches and missing hashes
	if isCASFile {
		// For hash mismatches on CAS files, automatically fix by removing stale index entry
		// This allows us to then treat it as a "missing hash" case and re-index
		if isHashMismatch {
			logging.Fluent(fixCtx.Logger).Info("Attempting to auto-fix hash mismatch by removing stale index entry").
				ObjectID(fixCtx.Obj.ID).
				Kind(fixKind).
				File(fixCtx.FilePath).
				Log()

			// Use the hash mismatch fix strategy to remove stale index entry
			projectRoot := fixCtx.Ctx.ProjectRoot
			projectRoot = ProjectRootOrResolve(projectRoot)

			if projectRoot != emptyValue {
				// Create strategy and fixer
				strategy := storage.NewRemoveStaleIndexStrategy(fixCtx.Logger)
				fixer := storage.NewHashMismatchFixer(strategy, fixCtx.Logger)

				// Fix the hash mismatch by removing stale index entry
				stdctx := pkgctx.NewSystemContext()
				results, err := fixer.FixHashMismatches(stdctx, []string{fixCtx.Obj.ID}, fixKind, projectRoot)
				if err == nil && len(results) > 0 && results[0].Fixed {
					logging.Fluent(fixCtx.Logger).Info("Successfully removed stale index entry for hash mismatch").
						ObjectID(fixCtx.Obj.ID).
						Kind(fixKind).
						String("strategy", results[0].Strategy).
						Log()

					// Now treat as missing hash and re-index
					// This will be handled by the missing hash logic below
					isMissingHash = true
					isHashMismatch = false
				} else {
					logging.Fluent(fixCtx.Logger).Warn("Failed to remove stale index entry for hash mismatch, trying alternative fix").
						ObjectID(fixCtx.Obj.ID).
						Kind(fixKind).
						WithError(err).
						Log()
					// Fall through to try fixImmutableObjectHash as fallback
					success, msg := fixImmutableObjectHash(fixCtx, fixKind, originalHash, isHashMismatch, storageProvider)
					if success {
						return true, msg
					}
				}
			} else {
				// No project root - try alternative fix
				success, msg := fixImmutableObjectHash(fixCtx, fixKind, originalHash, isHashMismatch, storageProvider)
				if success {
					return true, msg
				}
			}
		}

		// For missing hash on CAS files, we need to index the existing file
		// This also handles cases where we just removed a stale index entry (hash mismatch -> missing hash)
		if isMissingHash {
			logging.Fluent(fixCtx.Logger).Info("Attempting to fix missing hash for CAS file").
				ObjectID(fixCtx.Obj.ID).
				Kind(fixKind).
				File(fixCtx.FilePath).
				Log()
			success, msg := fixCASMissingHash(fixCtx, fixKind, storageProvider)
			if success {
				logging.Fluent(fixCtx.Logger).Info("Successfully fixed missing hash for CAS file").
					ObjectID(fixCtx.Obj.ID).
					String("message", msg).
					Log()
				// CRITICAL: Return immediately - don't fall through
				return true, msg
			}
			logging.Fluent(fixCtx.Logger).Error("Failed to fix missing hash for CAS file",
				errfmt.Errorf("fixCASMissingHash failed: %s", msg)).
				ObjectID(fixCtx.Obj.ID).
				Kind(fixKind).
				File(fixCtx.FilePath).
				Log()
			// Don't fall through - CAS fix must succeed or we error
			return false, ""
		}
	}

	// CRITICAL: All kinds use CAS now - HashRegistry is deprecated
	// If we reach here, it means CAS fix failed - return error to stop masking issues
	logging.Fluent(fixCtx.Logger).Error("CAS auto-fix failed and no fallback available",
		errfmt.Errorf("object %s (kind: %s) uses CAS but auto-fix failed. HashRegistry is deprecated and cannot be used as fallback", fixCtx.Obj.ID, fixKind)).
		ObjectID(fixCtx.Obj.ID).
		Kind(fixKind).
		File(fixCtx.FilePath).
		String("is_hash_mismatch", fmt.Sprintf("%v", isHashMismatch)).
		String("is_missing_hash", fmt.Sprintf("%v", isMissingHash)).
		Log()
	return false, ""
}
