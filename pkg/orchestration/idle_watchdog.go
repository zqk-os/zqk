package orchestration

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

const (
	idleErrMsgSubagentStalled = "subagent execution stalled (idle watchdog triggered)"
	defaultIdleTimeout        = 5 * time.Minute
	defaultProbeTick          = 500 * time.Millisecond
)

// IdleWatchdog bounds subagent execution to prevent unacknowledged idle intervals and deadlocks.
type IdleWatchdog struct {
	IdleTimeout time.Duration
	ProbeTick   time.Duration
}

// NewIdleWatchdog creates an IdleWatchdog with default fallbacks for non-positive values.
func NewIdleWatchdog(timeout, probeTick time.Duration) *IdleWatchdog {
	if timeout <= 0 {
		timeout = defaultIdleTimeout
	}
	if probeTick <= 0 {
		probeTick = defaultProbeTick
	}
	if timeout < probeTick {
		timeout = probeTick
	}
	return &IdleWatchdog{
		IdleTimeout: timeout,
		ProbeTick:   probeTick,
	}
}

// Execute runs the given task protected by the watchdog timer and context cancellation.
func (w *IdleWatchdog) Execute(ctx context.Context, subagentID string, task func(tctx context.Context) error) error {
	taskCtx, taskCancel := context.WithCancel(ctx)
	defer taskCancel()

	done := make(chan error, 1)
	goroutinelabels.NewGoroutine("idle_watchdog.worker", "subagent execution under idle watchdog").StartSimple(func() {
		done <- task(taskCtx)
	})

	timer := time.NewTimer(w.IdleTimeout)
	defer timer.Stop()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		taskCancel()
		return ctx.Err()
	case <-timer.C:
		taskCancel()
		return fmt.Errorf("%s: subagent %s timed out after %s", idleErrMsgSubagentStalled, subagentID, w.IdleTimeout)
	}
}
