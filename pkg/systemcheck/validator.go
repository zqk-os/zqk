package systemcheck

import (
	stdcontext "context"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

// GetAsyncValidator creates a new async validator instance for the given context.
// ctx: parent context from caller - will be derived hierarchically.
// projectRoot: the root directory of the zqk project.
// workerCount: number of workers, or 0 for auto-configuration from concurrency config.
func GetAsyncValidator(ctx stdcontext.Context, projectRoot string, workerCount int) *validation.AsyncValidator {
	if workerCount <= 0 {
		cfg := concurrency.GetGlobalConcurrencyConfig()
		workerCount = cfg.ValidatorMaxWorkers
	}
	if workerCount <= 0 {
		workerCount = 2
	}
	cfg := validation.DefaultAsyncValidatorConfig()
	cfg.ProgressChannelSize = 50000
	return validation.NewAsyncValidator(ctx, projectRoot, workerCount, validation.DefaultValidationStateCacheMaxAge, cfg)
}

// GetRecommendedSemaphoreCapacity calculates recommended semaphore capacity based on metrics.
// This helps inform validator creation for future runs.
func GetRecommendedSemaphoreCapacity() int {
	specLoader := objects.GetGlobalSpecLoader()
	if specLoader == nil {
		return 16 // Default: NumCPU * 2
	}

	metrics := specLoader.GetMetrics()
	if metrics.TotalWaits == 0 {
		return 16 // No data yet, use default
	}

	baseSemaphore := 16 // Current default (NumCPU * 2)

	// Adjust based on contention rate
	if metrics.ContentionRate > 0.3 {
		return baseSemaphore * 2
	} else if metrics.ContentionRate > 0.1 {
		return int(float64(baseSemaphore) * 1.5)
	}

	return baseSemaphore
}

// DetermineValidationPriority determines validation priority for an object.
func DetermineValidationPriority(kind, objectID string, validator *validation.AsyncValidator) int {
	if validator != nil {
		// Check cache for previous violations
		if state, ok := validator.GetCachedState(objectID); ok {
			// If object had Tier 1 issues, prioritize it
			for _, issue := range state.Issues {
				if issue.Tier == 1 && issue.ResolvedAt == nil {
					return 1 // Highest priority
				}
			}
			// If object had Tier 2 issues, medium-high priority
			for _, issue := range state.Issues {
				if issue.Tier == 2 && issue.ResolvedAt == nil {
					return 2
				}
			}
		}
	}

	// Default priority based on object kind importance
	// Critical objects get higher priority
	criticalKinds := map[string]bool{
		objects.KindRole:         true,
		objects.KindAccount:      true,
		objects.KindPolicy:       true,
		objects.KindSchedulerJob: true,
	}

	if criticalKinds[kind] {
		return 2 // High priority for critical objects
	}

	return 3 // Default priority
}
