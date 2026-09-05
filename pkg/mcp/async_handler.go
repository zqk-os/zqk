package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// AsyncHandler wraps a handler to execute it asynchronously while maintaining
// synchronous response ordering for stdio MCP protocol compliance
type AsyncHandler struct {
	handler          Handler
	maxConcurrent    int
	timeout          time.Duration
	activeOps        atomic.Int64
	totalOpsExecuted atomic.Int64
}

// GetAsyncHandlerStats returns current active operations and total operations executed.
func (a *AsyncHandler) GetAsyncHandlerStats() (active, total int64) {
	if a == nil {
		return 0, 0
	}
	return a.activeOps.Load(), a.totalOpsExecuted.Load()
}

// AsyncHandlerConfig configures async handler behavior
type AsyncHandlerConfig struct {
	// MaxConcurrent limits the number of concurrent operations
	// 0 = unlimited (not recommended)
	MaxConcurrent int

	// Timeout is the maximum duration for an operation
	// 0 = no timeout (not recommended for long operations)
	Timeout time.Duration
}

// NewAsyncHandler creates a new async handler wrapper
func NewAsyncHandler(handler Handler, config AsyncHandlerConfig) *AsyncHandler {
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 10 // Default limit
	}
	if config.Timeout <= 0 {
		config.Timeout = DefaultOperationTimeout
	}

	return &AsyncHandler{
		handler:       handler,
		maxConcurrent: config.MaxConcurrent,
		timeout:       config.Timeout,
	}
}

// Handle executes the handler asynchronously but waits for completion
// This allows long operations to run without blocking the server loop
// while maintaining protocol compliance (synchronous responses)
func (a *AsyncHandler) Handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	// Check if we're at capacity and atomically increment activeOps
	for {
		current := a.activeOps.Load()
		if current >= int64(a.maxConcurrent) {
			return nil, &JSONRPCError{
				Code:    ServerError,
				Message: fmt.Sprintf("Server at capacity (%d concurrent operations)", a.maxConcurrent),
				Data:    map[string]any{"max_concurrent": a.maxConcurrent},
			}
		}
		if a.activeOps.CompareAndSwap(current, current+1) {
			break
		}
	}
	a.totalOpsExecuted.Add(1)

	// Decrement activeOps when done
	defer func() {
		a.activeOps.Add(-1)
	}()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	// Execute handler in goroutine but wait for result
	// This allows the handler to be cancelled/timeout while still maintaining
	// synchronous response ordering
	resultChan := make(chan asyncResult, 1)

	goroutinelabels.NewGoroutine("mcp_async_handler", fmt.Sprintf("handling async MCP request: %s", method)).
		WithPanicHandler(func(r any) {
			// Log panic details (will be logged via error response)
			stackTrace := string(debug.Stack())
			err := errfmt.Errorf("panic in handler: %v\n\nStack trace:\n%s", r, stackTrace)
			select {
			case resultChan <- asyncResult{result: nil, err: err}:
			case <-ctx.Done():
				// Context cancelled, skip sending result
			}
		}).
		StartWithContext(ctx, func(ctx context.Context) error {
			result, err := a.handler.Handle(ctx, method, params)
			select {
			case resultChan <- asyncResult{result: result, err: err}:
			case <-ctx.Done():
				// Context cancelled, skip sending result
			}
			return nil
		})

	// Wait for result or timeout
	select {
	case res := <-resultChan:
		return res.result, res.err
	case <-ctx.Done():
		// Operation timed out or was cancelled
		if ctx.Err() == context.DeadlineExceeded {
			timeoutData := map[string]any{"timeout": a.timeout.String()}
			// Give the handler goroutine a moment to send partial result (with stderr) after process kill.
			// Stderr may contain [MCP_INIT_TRACE] lines showing where CLI init hung.
			select {
			case res := <-resultChan:
				if res.result != nil {
					if m, ok := res.result.(map[string]any); ok {
						if s, ok := m["stderr"].(string); ok && s != emptyValue {
							timeoutData["stderr"] = s
							timeoutData["hint"] = "CLI subprocess stderr (look for [MCP_INIT_TRACE] to see where init hung)"
						}
					}
				}
			case <-time.After(200 * time.Millisecond):
			}
			return nil, &JSONRPCError{
				Code:    ServerError,
				Message: fmt.Sprintf("Operation timed out after %v", a.timeout),
				Data:    timeoutData,
			}
		}
		return nil, &JSONRPCError{
			Code:    ServerError,
			Message: "Operation was cancelled",
			Data:    map[string]any{objects.FieldKeyReason: ctx.Err().Error()},
		}
	}
}

type asyncResult struct {
	result any
	err    error
}

// GetActiveOperations returns the number of currently active operations
func (a *AsyncHandler) GetActiveOperations() int {
	if a == nil {
		return 0
	}
	return int(a.activeOps.Load())
}

// OperationTracker tracks active operations for monitoring and cancellation
type OperationTracker struct {
	operations map[string]*TrackedOperation
	mu         sync.RWMutex
}

// TrackedOperation represents an active operation
type TrackedOperation struct {
	ID        string
	Method    string
	StartTime time.Time
	Context   context.Context
	Cancel    context.CancelFunc
}

// NewOperationTracker creates a new operation tracker
func NewOperationTracker() *OperationTracker {
	return &OperationTracker{
		operations: make(map[string]*TrackedOperation),
	}
}

// StartOperation tracks a new operation
func (ot *OperationTracker) StartOperation(id, method string, baseCtx context.Context) (context.Context, context.CancelFunc) {
	if baseCtx == nil {
		baseCtx = pkgctx.NewSystemContext()
	}
	ctx, cancel := context.WithCancel(baseCtx)

	_ = concurrency.RunInLockWithLogger(
		&ot.mu, LockNameOperationTrackerStart, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			ot.operations[id] = &TrackedOperation{
				ID:        id,
				Method:    method,
				StartTime: time.Now(),
				Context:   ctx,
				Cancel:    cancel,
			}
			return nil
		},
	)

	return ctx, func() {
		cancel()
		_ = concurrency.RunInLockWithLogger(
			&ot.mu, LockNameOperationTrackerCleanup, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				delete(ot.operations, id)
				return nil
			},
		)
	}
}

// CancelOperation cancels a specific operation
func (ot *OperationTracker) CancelOperation(id string) bool {
	var op *TrackedOperation
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&ot.mu, LockNameOperationTrackerCancelCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			op, ok = ot.operations[id]
			exists = ok
			return nil
		},
	)

	if !exists {
		return false
	}

	op.Cancel()

	_ = concurrency.RunInLockWithLogger(
		&ot.mu, LockNameOperationTrackerCancelRemove, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			delete(ot.operations, id)
			return nil
		},
	)

	return true
}

// GetOperations returns all active operations
func (ot *OperationTracker) GetOperations() []TrackedOperation {
	var operations map[string]*TrackedOperation
	_ = concurrency.RunInRLockWithLogger(
		&ot.mu, LockNameOperationTrackerGetOperations, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			operations = make(map[string]*TrackedOperation)
			for k, v := range ot.operations {
				operations[k] = v
			}
			return nil
		},
	)

	return mapValuesDeref(operations)
}

// GetOperationCount returns the number of active operations
func (ot *OperationTracker) GetOperationCount() int {
	var count int
	_ = concurrency.RunInRLockWithLogger(
		&ot.mu, LockNameOperationTrackerGetCount, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			count = len(ot.operations)
			return nil
		},
	)
	return count
}
