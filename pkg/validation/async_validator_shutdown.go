package validation

import (
	"context"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// InitiateShutdown implements QueueShutdownHandler
// Signals the validator to stop accepting new tasks
func (av *AsyncValidator) InitiateShutdown() error {
	var alreadyStopped bool
	if err := concurrency.RunInLockWithLogger(
		&av.mu,
		LockNameAsyncValidatorInitiateShutdown,
		lockLoggerSystem(),
		func() error {
			if !av.running {
				alreadyStopped = true
				return nil
			}
			return nil
		},
	); err != nil {
		logging.Fluent(av.logger).Error(ConstMagic29d94cd0, err).Log()
	}

	if alreadyStopped {
		return nil // Already stopped
	}

	// Signal shutdown (prevents new tasks from being enqueued)
	select {
	case <-av.shutdown:
		// Already closed
	default:
		close(av.shutdown)
	}

	return nil
}

// Drain implements QueueShutdownHandler
// Processes all pending tasks and waits for workers to complete
func (av *AsyncValidator) Drain(ctx context.Context) error {
	if av.IsDrained() {
		return nil
	}

	if err := av.InitiateShutdown(); err != nil {
		return err
	}

	if av.IsDrained() {
		return nil
	}

	// Wait for workers with timeout (callback-based, timeout is fallback)
	drainComplete := make(chan struct{})

	// Use callback pattern: wait in goroutine, notify via channel
	drainBud := goroutinelabels.DefaultBudget()
	drainBuilder := goroutinelabels.NewGoroutine(ConstMagice0c33bd5, ConstMagic261c9989).
		WithCleanup(func() {
			close(drainComplete)
		})
	if drainBud != nil {
		drainBuilder = drainBuilder.WithBudget(drainBud)
	}
	drainBuilder.StartWithContext(ctx, func(waitCtx context.Context) error {
		av.wg.Wait()
		return nil
	})

	select {
	case <-drainComplete:
		// All workers finished
		return nil
	case <-ctx.Done():
		// Timeout or context cancelled (timeout is fallback)
		return ctx.Err()
	}
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// IsDrained implements QueueShutdownHandler
func (av *AsyncValidator) IsDrained() bool {
	// Check if queue is empty and no workers are running
	return av.priorityQueue.Size() == 0 && av.activeWorkers.Load() == 0
}

// GetPendingCount implements QueueShutdownHandler
func (av *AsyncValidator) GetPendingCount() int64 {
	return int64(av.priorityQueue.Size())
}

// GetName implements QueueShutdownHandler
func (av *AsyncValidator) GetName() string {
	return ConstMagicExtracted_14
}

// IsCritical implements QueueShutdownHandler
// Validation is critical - we should drain before force shutdown
func (av *AsyncValidator) IsCritical() bool {
	return true
}
