package pipeline

import (
	"context"
	"errors"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// HyperScaleOrchestrator manages the execution of tasks across interconnected agent networks.
type HyperScaleOrchestrator struct {
	MaxConcurrentAgents int
}

// NewHyperScaleOrchestrator creates a new HyperScaleOrchestrator.
func NewHyperScaleOrchestrator(maxConcurrent int) *HyperScaleOrchestrator {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &HyperScaleOrchestrator{
		MaxConcurrentAgents: maxConcurrent,
	}
}

// Orchestrate starts the orchestration process for the given task graphs in parallel.
// TRACK: BLI-CEF-REL-GOROUTINE-LEAKS — REQ-CEF-REL-002 / CRIT-CEF-REL-002A:
// concurrency is a labeled budgeted Pool (not raw go + WaitGroup/semaphore) so cancel
// cannot strand accounting and MaxConcurrentAgents stays a real worker bound.
func (h *HyperScaleOrchestrator) Orchestrate(ctx context.Context, taskIDs []string, dispatchFn func(ctx context.Context, taskID string) error) error {
	if len(taskIDs) == 0 {
		return nil
	}
	if dispatchFn == nil {
		return errors.New("hyper_scale: nil dispatchFn")
	}

	budget := goroutinelabels.DefaultBudget()
	if budget == nil {
		// Honor MaxConcurrentAgents when the process has no DefaultBudget.
		budget = goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{})
	}

	pool := goroutinelabels.NewPool(
		budget,
		"hyper_scale",
		"dispatching hyper-scale orchestration tasks",
		h.MaxConcurrentAgents,
		len(taskIDs),
	)
	pool.Start(ctx)
	defer pool.Stop()

	errCh := make(chan error, len(taskIDs))
	for _, taskID := range taskIDs {
		tID := taskID
		if err := pool.Submit(ctx, func(workerCtx context.Context) error {
			if err := dispatchFn(workerCtx, tID); err != nil {
				errCh <- err
				return err
			}
			return nil
		}); err != nil {
			errCh <- err
		}
	}

	// Drain workers before collecting errors (Stop is idempotent with defer).
	pool.Stop()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
