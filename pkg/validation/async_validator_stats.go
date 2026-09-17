package validation

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
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
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstMagic8cfdad90, err).Log()
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
	statsBuilder := goroutinelabels.NewGoroutine(ConstMagic69760185, ConstMagic2264261e).
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
		logging.Fluent(av.logger).Debug(ConstMagica74a040f).
			String("timeout", timeout.String()).
			Int(ConstMagice4c6e9b7, int(getActiveGoroutines())).
			Log()
		return false
	}
}
