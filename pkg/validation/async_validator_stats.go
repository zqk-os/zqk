package validation

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

// GetValidationStats returns statistics about validation state
// Note: stateCache and priorityQueue have their own internal mutexes, so no additional synchronization needed
func (av *AsyncValidator) GetValidationStats() (total, stale, withIssues, queueSize int) {
	total, stale, withIssues = av.stateCache.Count()
	queueSize = av.priorityQueue.Size()
	return
}

// GetPendingObjectIDs returns object IDs (and kind) still in the validation queue, for stuck diagnostics.
func (av *AsyncValidator) GetPendingObjectIDs() []string {
	return av.priorityQueue.SnapshotObjectIDs()
}

// GetWorkerCount returns the number of active worker goroutines
func (av *AsyncValidator) GetWorkerCount() int {
	return int(av.activeWorkers.Load())
}

// GetMaxWorkers returns the maximum number of workers
func (av *AsyncValidator) GetMaxWorkers() int {
	var maxWorkers int
	if err := concurrency.RunInRLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorGetMaxWorkers,
		lockLoggerSystem(),
		func() error {
			maxWorkers = av.maxWorkers
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("lock failed in GetMaxWorkers: %v\n", err).Log()
	}
	return maxWorkers
}

// WaitForValidationCompletion waits for all validation goroutines to complete
// This should be called when the queue is empty to ensure all validation work is finished
// before declaring completion. Returns immediately if no validation goroutines are running.
func (av *AsyncValidator) WaitForValidationCompletion(timeout time.Duration) bool {
	done := make(chan struct{})
	waitCtx, waitCancel := context.WithTimeout(av.ctx, timeout)
	defer waitCancel()
	statsBud := goroutinelabels.DefaultBudget()
	statsBuilder := goroutinelabels.NewGoroutine("async_validator_completion_wait", "waiting for validation completion").
		WithCleanup(func() {
			close(done)
		})
	if statsBud != nil {
		statsBuilder = statsBuilder.WithBudget(statsBud)
	}
	statsBuilder.StartWithContext(waitCtx, func(ctx context.Context) error {
		av.validationWg.Wait()
		return nil
	})

	select {
	case <-done:
		// All validation goroutines finished
		return true
	case <-waitCtx.Done():
		// Timeout waiting for validation goroutines
		logging.Fluent(av.logger).Debug("Timeout waiting for validation goroutines to complete").
			String("timeout", timeout.String()).
			Int("active_goroutines", int(getActiveGoroutines())).
			Log()
		return false
	}
}
