package scheduler

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const schedulerStatusCheckTimeout = 3 * time.Second

// SchedulerStatus represents the status of the scheduler daemon
// This struct is immutable and safe for concurrent access - each call returns a new instance
type SchedulerStatus struct {
	Running     bool
	InProcess   bool // true if running in this process
	ProcessID   int  // PID if running in another process
	ProjectRoot string
}

// getSchedulerStatus checks if scheduler is running (in this process or another)
// This is a common pattern used across multiple commands (start, stop, status, trigger)
//
// Thread-Safety:
//   - GetGlobalScheduler() is protected by globalSchedulerMu (RWMutex) - thread-safe
//   - sched.IsRunning() is protected by s.runningMu (RWMutex) - thread-safe
//   - IsSchedulerRunning() reads PID file (atomic file I/O, no shared mutable state) - thread-safe
//   - Returns a new SchedulerStatus instance (no shared mutable state) - thread-safe
//
// This function is safe to call concurrently from multiple goroutines.
//
//nolint:unparam // Always returns nil error - function is designed to always succeed
func getSchedulerStatus(ctx *cli.Context) (*SchedulerStatus, error) {
	status := &SchedulerStatus{}

	// When ZQK_TEST_ROOT is set, prefer ctx.ProjectRoot so tests (e.g. TestTriggerJob_SchedulerNotRunning)
	// always see status for the test root and never accidentally use the real project root (e.g. when the
	// test bundle runs under the scheduler daemon).
	if zqkenv.TestRoot().Get() != emptyValue && ctx != nil && ctx.ProjectRoot != emptyValue {
		status.ProjectRoot = ctx.ProjectRoot
	} else {
		// CWD-oriented: use current directory's project first so status matches "this" project (see PROJECT_ROOT_ORIENTATION.md)
		status.ProjectRoot = cli.ResolveProjectRoot(".")
		if status.ProjectRoot == emptyValue && ctx != nil {
			status.ProjectRoot = ctx.ProjectRoot
		}
	}
	projectRoot := status.ProjectRoot
	if projectRoot == emptyValue {
		return status, nil // No project root, assume not running
	}

	// First check if scheduler is running in this process (non-blocking so "scheduler status" never hangs
	// when the daemon holds globalSchedulerMu)
	sched, ok := schedulerpkg.GetGlobalSchedulerIfAvailable()
	if ok && sched != nil && sched.IsRunning() {
		status.Running = true
		status.InProcess = true
		return status, nil
	}

	// Check if scheduler is running in another process via PID file.
	// Bound this check so status/start/stop commands never hang indefinitely.
	type runningResult struct {
		running bool
		pid     int
		err     error
	}
	resultCh := make(chan runningResult, 1)
	goroutinelabels.NewGoroutine("scheduler_helpers", "check scheduler status").
		StartSimple(func() {
			running, pid, err := schedulerpkg.IsSchedulerRunning(projectRoot)
			resultCh <- runningResult{running: running, pid: pid, err: err}
		})
	var rr runningResult
	select {
	case rr = <-resultCh:
	case <-time.After(schedulerStatusCheckTimeout):
		return status, nil
	}
	if rr.err != nil {
		// Error reading PID file - assume not running
		return status, nil
	}

	if rr.running {
		status.Running = true
		status.InProcess = false
		status.ProcessID = rr.pid
	}

	return status, nil
}

// requireSchedulerRunning ensures scheduler is running and returns an error if not
// This is used by commands that require the scheduler to be running
//
// Thread-Safety: Safe for concurrent access (calls thread-safe getSchedulerStatus)
func requireSchedulerRunning(ctx *cli.Context) (*SchedulerStatus, error) {
	status, err := getSchedulerStatus(ctx)
	if err != nil {
		return nil, err
	}

	if !status.Running {
		return nil, errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations("scheduler daemon is not running - start it first with 'zqk scheduler start'"))
	}

	return status, nil
}

// requireSchedulerNotRunning ensures scheduler is NOT running and returns an error if it is
// This is used by the start command to prevent duplicate instances
//
// Thread-Safety: Safe for concurrent access (calls thread-safe getSchedulerStatus)
// Note: Race condition possible if two processes call start simultaneously, but PID file
// check in scheduler.Start() provides additional protection.
func requireSchedulerNotRunning(ctx *cli.Context) (*SchedulerStatus, error) {
	status, err := getSchedulerStatus(ctx)
	if err != nil {
		return nil, err
	}

	if status.Running {
		if status.InProcess {
			return status, errfmt.Errorf("scheduler is already running in this process")
		}
		return status, errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("scheduler daemon is already running (PID: %d). Use 'zqk scheduler stop' to stop it first", status.ProcessID)))
	}

	return status, nil
}

// getSchedulerInstance returns the scheduler instance if running in this process,
// or an error if running in another process or not running at all
//
// Thread-Safety:
//   - Calls thread-safe getSchedulerStatus()
//   - GetGlobalScheduler() is protected by globalSchedulerMu - thread-safe
//   - Returns pointer to Scheduler, but caller must use scheduler's own mutexes
//     (jobsMu, runningMu) when accessing scheduler state
func getSchedulerInstance(ctx *cli.Context) (*schedulerpkg.Scheduler, *SchedulerStatus, error) {
	status, err := getSchedulerStatus(ctx)
	if err != nil {
		return nil, nil, err
	}

	if !status.Running {
		return nil, status, errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations("scheduler daemon is not running - start it first with 'zqk scheduler start'"))
	}

	if !status.InProcess {
		return nil, status, errfmt.Errorf("scheduler daemon is running in another process (PID: %d). This operation requires the scheduler to be running in this process", status.ProcessID)
	}

	sched := schedulerpkg.GetGlobalScheduler()
	if sched == nil {
		return nil, status, errfmt.Errorf("scheduler instance not found (this should not happen)")
	}

	return sched, status, nil
}

// enqueueJobTriggerRequest enqueues a job trigger request for cross-process triggering.
// When preCommit is true, the request includes trigger_origin=pre_commit so the job's callback runs.
func enqueueJobTriggerRequest(ctx *cli.Context, cmd *cobra.Command, jobID string, preCommit bool) error {
	status, err := requireSchedulerRunning(ctx)
	if err != nil {
		return err
	}

	queue := schedulerpkg.NewJobTriggerQueue(status.ProjectRoot)
	triggerOrigin := schedulerpkg.TriggerOriginCLISubmit
	if preCommit {
		triggerOrigin = schedulerpkg.TriggerOriginPreCommit
	}
	if err := queue.EnqueueTriggerRequestWithOrigin(jobID, triggerOrigin); err != nil {
		return errfmt.Newf("failed to enqueue trigger request").Wrap(err)
	}

	return cli.WriteOutput(cmd, []byte(fmt.Sprintf(
		"Job trigger request for %s has been queued. The scheduler daemon will process it shortly.\n",
		jobID,
	)))
}
