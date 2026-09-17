package system

import (
	stdcontext "context"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// clearValidationCacheForAutoFix clears the validation cache when auto-fix is enabled
// This ensures that validation results reflect the current state after fixes were applied
// in previous runs, preventing stale cache from masking resolved issues
// ctx: parent context from command entry point (should not be created here)
func clearValidationCacheForAutoFix(ctx stdcontext.Context, projectRoot string) error {
	projectRoot = ProjectRootOrResolve(projectRoot)

	// Get the async validator if available and clear its cache
	// The async validator manages its own state cache, so clearing it will clear the validation cache
	asyncValidator := GetAsyncValidator(ctx, projectRoot, 0)
	if asyncValidator != nil {
		if err := asyncValidator.ClearCache(); err != nil {
			return errfmt.Newf("failed to clear async validator cache").Wrap(err)
		}
		return nil
	}

	// If async validator isn't available (sync mode), we can't clear the cache directly
	// In sync mode, validation happens on-demand and doesn't use a persistent cache
	// The cache will be naturally invalidated on the next check since files will be re-validated
	return nil
}
