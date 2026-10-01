package system

import (
	stdcontext "context"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	"github.com/zqk-os/zqk/pkg/validation"
)

// Type aliases keep cmd/zqk/system call sites stable while check domain types
// live in pkg/systemcheck.
type (
	CheckResult           = systemcheck.CheckResult
	Issue                 = systemcheck.Issue
	CheckSnapshot         = systemcheck.CheckSnapshot
	CheckSnapshotMetadata = systemcheck.CheckSnapshotMetadata
	ResolutionResult      = systemcheck.ResolutionResult
	ViolationResolver     = systemcheck.ViolationResolver
)

// NewViolationResolver creates a violation resolver (pkg/systemcheck).
func NewViolationResolver(projectRoot string, specLoader *objects.SpecLoader, logger logging.Logger) *ViolationResolver {
	return systemcheck.NewViolationResolver(projectRoot, specLoader, logger)
}

// GetAsyncValidator delegates to systemcheck.GetAsyncValidator.
func GetAsyncValidator(ctx stdcontext.Context, projectRoot string, workerCount int) *validation.AsyncValidator {
	return systemcheck.GetAsyncValidator(ctx, projectRoot, workerCount)
}

// getRecommendedSemaphoreCapacity delegates to systemcheck.GetRecommendedSemaphoreCapacity.
func getRecommendedSemaphoreCapacity() int {
	return systemcheck.GetRecommendedSemaphoreCapacity()
}

// determineValidationPriority delegates to systemcheck.DetermineValidationPriority.
func determineValidationPriority(kind, objectID string, validator *validation.AsyncValidator) int {
	return systemcheck.DetermineValidationPriority(kind, objectID, validator)
}
