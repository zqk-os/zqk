package dispatch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

const (
	// DefaultPoolWorkerCount is the default number of workers in the dispatch pool (20–30 range).
	DefaultPoolWorkerCount = 24
	// DefaultPoolQueueSize is the default work queue size for the dispatch pool.
	DefaultPoolQueueSize = 64
	// dispatchBudgetMaxTotal reserves space for pool workers plus a small buffer.
	dispatchBudgetMaxTotal = 32
)

var (
	globalPoolMu sync.Mutex
	globalPool   *goroutinelabels.Pool
	globalBudget *goroutinelabels.Budget
)

// StartGlobalPool starts the global dispatch pool for concurrent tool execution (e.g. MCP).
// Uses a dedicated budget so it works when the scheduler is not running. Idempotent;
// subsequent calls are no-ops if already started. Call StopGlobalPool when the server exits.
func StartGlobalPool(ctx context.Context) {
	globalPoolMu.Lock()
	defer globalPoolMu.Unlock()
	if globalPool != nil {
		return
	}
	globalBudget = goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: dispatchBudgetMaxTotal})
	globalPool = goroutinelabels.NewPool(globalBudget, "dispatch_worker", "dispatch pool for CLI/tool runs", DefaultPoolWorkerCount, DefaultPoolQueueSize)
	globalPool.Start(ctx)
}

// StopGlobalPool stops the global dispatch pool and releases its budget. Idempotent.
// Call when the MCP server (or other pool owner) shuts down.
func StopGlobalPool() {
	globalPoolMu.Lock()
	pool := globalPool
	globalPool = nil
	globalBudget = nil
	globalPoolMu.Unlock()
	if pool != nil {
		pool.Stop()
	}
}

// GlobalPoolStarted returns true if the global dispatch pool has been started and not stopped.
func GlobalPoolStarted() bool {
	globalPoolMu.Lock()
	defer globalPoolMu.Unlock()
	return globalPool != nil
}

// RunViaPool runs one job via the global dispatch pool when started; otherwise runs inline (Run).
// Caller blocks until the job completes or execCtx is cancelled. Safe to call from multiple goroutines.
// In-process CLI execution is serialized elsewhere (e.g. mutex in the runner) because rootCmd is shared.
func RunViaPool(
	execCtx context.Context,
	item *WorkItem,
	runner func(ctx context.Context) error,
) error {
	if item == nil {
		return errfmt.Errorf("dispatch: work item is nil")
	}
	if item.OperationID == emptyValue {
		item.OperationID = fmt.Sprintf("%s_%d", item.OperationType, time.Now().UnixNano())
	}
	globalPoolMu.Lock()
	pool := globalPool
	globalPoolMu.Unlock()
	if pool == nil {
		return runWithProgress(execCtx, item, runner)
	}
	resultCh := make(chan error, 1)
	fn := func(ctx context.Context) error {
		err := runWithProgress(ctx, item, runner)
		resultCh <- err
		return err
	}
	if err := pool.Submit(execCtx, fn); err != nil {
		return err
	}
	return <-resultCh
}
