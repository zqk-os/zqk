package dispatch

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// WorkItem describes a single job for the dispatch loop: run one CLI command or tool
// with timeout and progress. Used by the CLI entrypoint (inline, one item) and by MCP
// when using in-process execution. See docs/architecture/DISPATCH_LOOP_AND_MCP.md.
type WorkItem struct {
	// OperationID is a unique id for this run (e.g. "object_list_1234567890").
	OperationID string
	// OperationType is a stable type for logs/metrics (e.g. "object_list", "spec_list").
	OperationType string
	// ProjectRoot is the project root for diagnostics and event context.
	ProjectRoot string
	// Profile is the logging profile (e.g. "human", "mcp").
	Profile string
}

// Run runs one job with the shared progress pattern: progress callback on context and
// heartbeat to the Coordinator. The caller must provide an execution context (execCtx)
// that is already bounded (e.g. by the timeout hook or MCP tool timeout). Runner receives
// a context with progress attached and runs the actual work (e.g. rootCmd.ExecuteContext).
// Concurrency: Run is safe to call from multiple goroutines; each call runs one job to
// completion. For a single-loop design, callers serialize (e.g. one tool call at a time).
func Run(
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
	return runWithProgress(execCtx, item, runner)
}
