package system

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

// invalidateValidationCacheForObject invalidates the validation cache for a specific object
// This is called after successful auto-fix operations to ensure the next check sees the updated object
// and doesn't report hash mismatches due to stale cache
// CRITICAL: After storageProvider.Update completes, the hash registry is updated, but the validation
// cache still has the old state. We must invalidate the cache so the next check sees the new hash.
//
// IMPORTANT: In async mode, when auto-fix runs DURING validation, the state is stored AFTER auto-fix
// completes and already includes AutoFixed results in metadata. Invalidating here would delete the
// state that was just stored, causing AutoFixed results to be lost. So we only invalidate if we're
// NOT in the middle of a validation run (i.e., sync mode or post-validation).
//
// This function is best-effort and should not fail the auto-fix operation if cache invalidation fails.
func invalidateValidationCacheForObject(fixCtx *AutoFixContext, objectID string) {
	projectRoot := ProjectRootOrResolve(fixCtx.Ctx.ProjectRoot)

	// Try to get the global async validator
	// Note: This may be nil if we're in sync mode or validator hasn't been initialized
	// In async mode, the validator is initialized during check, so it should be available
	// CRITICAL: fixCtx.Cmd may be nil in tests - use context from fixCtx.Ctx if available
	var ctx context.Context
	if fixCtx.Cmd != nil {
		ctx = fixCtx.Cmd.Context()
	}
	// Ensure we have a valid context (cmd.Context() may return nil in tests)
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	asyncValidator := GetAsyncValidator(ctx, projectRoot, 0)
	if asyncValidator != nil {
		// CRITICAL: In async mode, when auto-fix runs during validation, the state is stored
		// AFTER auto-fix completes (at line 714 in async_validator.go). The state already includes
		// AutoFixed results in metadata. If we invalidate here, we delete the state that was just
		// stored, causing AutoFixed results to be lost when collecting results.
		//
		// Solution: Don't invalidate in async mode when auto-fix runs during validation.
		// The state is already correct and includes AutoFixed. We only need to invalidate if
		// we're going to re-validate, but we're not - we're collecting the state that was just stored.
		//
		// For sync mode or post-validation scenarios, invalidation is still needed to ensure
		// the next check sees updated state. But in async mode during validation, the state
		// is stored after auto-fix, so it's already correct.
		//
		// Check if we're in async validation mode by checking if the validator is running
		// If running, don't invalidate (state will be stored with AutoFixed)
		// If not running, invalidate (sync mode or post-validation)
		if asyncValidator.IsRunning() {
			// Async validation is running - state will be stored after auto-fix with AutoFixed
			// Don't invalidate, as it would delete the state that's about to be stored
			logging.Fluent(fixCtx.Logger).Debug("Skipping cache invalidation during async validation (state will be stored with AutoFixed)").
				String("object_id", objectID).
				Log()
			return
		}

		// Async validator exists but not running (sync mode or post-validation)
		// Invalidate to ensure next check sees updated state
		asyncValidator.InvalidateCache(objectID)
		logging.Fluent(fixCtx.Logger).Debug("Invalidated validation cache for object after auto-fix").
			String("object_id", objectID).
			Log()
		return
	}

	// If async validator isn't available (sync mode), we can't invalidate the cache
	// In sync mode, validation happens on-demand and doesn't use a persistent cache
	// The cache will be naturally invalidated on the next check since the file mtime/checksum changed
	logging.Fluent(fixCtx.Logger).Debug("Async validator not available for cache invalidation (sync mode - cache will refresh on next check)").
		String("object_id", objectID).
		Log()
}
