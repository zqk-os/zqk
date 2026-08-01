package pipeline

import (
	"context"
	"sync"
)

// HyperScaleOrchestrator manages the execution of tasks across interconnected agent networks.
type HyperScaleOrchestrator struct {
	MaxConcurrentAgents int
}

// NewHyperScaleOrchestrator creates a new HyperScaleOrchestrator.
func NewHyperScaleOrchestrator(maxConcurrent int) *HyperScaleOrchestrator {
	return &HyperScaleOrchestrator{
		MaxConcurrentAgents: maxConcurrent,
	}
}

// Orchestrate starts the orchestration process for the given task graphs in parallel.
func (h *HyperScaleOrchestrator) Orchestrate(ctx context.Context, taskIDs []string, dispatchFn func(ctx context.Context, taskID string) error) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(taskIDs))
	semaphore := make(chan struct{}, h.MaxConcurrentAgents)

	for _, taskID := range taskIDs {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(tID string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			if err := dispatchFn(ctx, tID); err != nil {
				errCh <- err
			}
		}(taskID)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errs[0] // or a multierror
	}
	return nil
}
