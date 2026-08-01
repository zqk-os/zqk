package system

import (
	"context"
	"time"

	schedulerpkg "github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/validation"
)

// validationScannerForScheduler implements scheduler.ValidationScanner.
// It enqueues all objects from the object ID cache into the AsyncValidator so
// the shared validation state cache is populated in the background by the scheduler's
// cache_prewarm Tier 4 step.
type validationScannerForScheduler struct{}

var _ schedulerpkg.ValidationScanner = (*validationScannerForScheduler)(nil)

// EnqueueAll enqueues all objects in the object ID cache for background validation.
// Uses ShouldEnqueue to skip objects whose validation state is still fresh (mtime unchanged).
// Workers write results to the shared validation state cache; this call returns quickly.
func (validationScannerForScheduler) EnqueueAll(ctx context.Context, projectRoot string) (int, error) {
	if projectRoot == emptyValue {
		return 0, nil
	}

	cache := GetGlobalObjectIDCache()
	if !cache.IsPopulatedForProject(projectRoot) {
		return 0, nil
	}

	validator := validation.NewAsyncValidator(ctx, projectRoot, 0, 0*time.Second)
	if err := validator.Start(); err != nil {
		return 0, nil
	}
	defer func() { _ = validator.Stop() }()

	enqueued := 0
	for _, kind := range cache.GetKinds() {
		if ctx.Err() != nil {
			break
		}
		for _, entry := range cache.GetEntriesByKind(kind) {
			if validator.ShouldEnqueue(entry.ID, kind, entry.FilePath) {
				if validator.Enqueue(entry.ID, kind, entry.FilePath, 0) {
					enqueued++
				}
			}
		}
	}

	return enqueued, nil
}

// NewValidationScannerForScheduler returns a ValidationScanner for use by the scheduler.
// Pass it via CachePrewarmHandler.WithValidationScanner so cache_prewarm Tier 4 enqueues
// all cached objects for background validation after the object ID cache is built.
func NewValidationScannerForScheduler() schedulerpkg.ValidationScanner {
	return &validationScannerForScheduler{}
}
