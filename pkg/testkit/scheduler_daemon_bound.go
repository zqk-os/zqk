package testkit

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// DefaultMaxCLISchedulerDaemonLifetime is the hard wall-clock ceiling for a
// detached scheduler daemon started by a Go test (CLI `scheduler start`).
// Tests should finish earlier via job completion callbacks and stop the daemon;
// this bound exists so a hung wait cannot leave orphans for hours.
const DefaultMaxCLISchedulerDaemonLifetime = 3 * time.Minute

// DefaultMaxInProcessSchedulerLifetime is the hard ceiling for in-process
// `Scheduler.Start(ctx)` used by package tests. Prefer canceling on the first
// successful assertion (callback / condition), not waiting out the full budget.
const DefaultMaxInProcessSchedulerLifetime = 2 * time.Minute

// BoundCLISchedulerOpts configures [StartBoundCLIScheduler].
type BoundCLISchedulerOpts struct {
	// CLIBinary is the path to the zqk (or test) binary that implements scheduler start/stop.
	CLIBinary string
	// ProjectRoot is the isolated project root (ZQK_TEST_ROOT / cwd for WireExec).
	ProjectRoot string
	// Env is the child process environment. If nil, [WireCLISubprocessForIsolatedProject] alone sets Dir/Env.
	Env []string
	// TestID is passed as --test-id (default: t.Name()) so teardown does not friendly-fire parallel tests.
	TestID string
	// MaxLifetime overrides [DefaultMaxCLISchedulerDaemonLifetime] when > 0.
	MaxLifetime time.Duration
	// PreferCallback documents intent: wait on a completion callback (or equivalent) and stop
	// early rather than polling until MaxLifetime. Does not change StartBoundCLIScheduler behavior.
	PreferCallback bool
}

// BoundCLISchedulerHandle is returned by [StartBoundCLIScheduler].
type BoundCLISchedulerHandle struct {
	TestID      string
	Deadline    time.Time
	BoundCtx    context.Context
	cancelBound context.CancelFunc
}

// StartBoundCLIScheduler starts `scheduler start --test-id=…` in the isolated project,
// registers t.Cleanup that always runs `scheduler stop --force`, and arms a context
// deadline equal to MaxLifetime. When the deadline fires while the test is still
// running, a watchdog still force-stops; callers should also select on BoundCtx.Done()
// (or use [WaitChanOrBound] / [WaitChanOrBoundWithCap]) so the test fails fast instead of hanging.
//
// TRACK: REDACTED — remove ad-hoc start/stop pairs once call sites use this helper.
func StartBoundCLIScheduler(t testing.TB, opts BoundCLISchedulerOpts) *BoundCLISchedulerHandle {
	t.Helper()
	if opts.CLIBinary == "" {
		t.Fatal("StartBoundCLIScheduler: CLIBinary is required")
	}
	if opts.ProjectRoot == "" {
		t.Fatal("StartBoundCLIScheduler: ProjectRoot is required")
	}
	testID := opts.TestID
	if testID == "" {
		testID = t.Name()
	}
	max := opts.MaxLifetime
	if max <= 0 {
		max = DefaultMaxCLISchedulerDaemonLifetime
	}

	boundCtx, cancelBound := context.WithTimeout(context.Background(), max)
	h := &BoundCLISchedulerHandle{
		TestID:      testID,
		Deadline:    time.Now().Add(max),
		BoundCtx:    boundCtx,
		cancelBound: cancelBound,
	}

	startCmd := exec.Command(opts.CLIBinary, "scheduler", "start", "--test-id="+testID) //nolint:gosec // test binary path from caller
	WireCLISubprocessForIsolatedProject(startCmd, opts.ProjectRoot)
	if opts.Env != nil {
		startCmd.Env = opts.Env
	}
	out, err := startCmd.CombinedOutput()
	if err != nil {
		cancelBound()
		t.Fatalf("scheduler start (--test-id=%s): %v\n%s", testID, err, out)
	}

	t.Cleanup(func() {
		cancelBound()
		stopCmd := exec.Command(opts.CLIBinary, "scheduler", "stop", "--test-id="+testID, "--force") //nolint:gosec // test binary path from caller
		WireCLISubprocessForIsolatedProject(stopCmd, opts.ProjectRoot)
		if opts.Env != nil {
			stopCmd.Env = opts.Env
		}
		if stopOut, stopErr := stopCmd.CombinedOutput(); stopErr != nil {
			t.Logf("bound CLI scheduler cleanup: stop --force: %v\n%s", stopErr, stopOut)
		}
	})

	// Watchdog: if the test ignores BoundCtx and hangs past MaxLifetime, still tear down.
	// Do not call t.Fail from this goroutine; WaitChanOrBound / the test deadline should fail the case.
	go func() {
		<-boundCtx.Done()
		if boundCtx.Err() != context.DeadlineExceeded {
			return
		}
		stopCmd := exec.Command(opts.CLIBinary, "scheduler", "stop", "--test-id="+testID, "--force") //nolint:gosec // test binary path from caller
		WireCLISubprocessForIsolatedProject(stopCmd, opts.ProjectRoot)
		if opts.Env != nil {
			stopCmd.Env = opts.Env
		}
		_ = stopCmd.Run() //nolint:errcheck // best-effort orphan prevention
	}()

	return h
}

// BoundSchedulerStartContext returns a child context canceled on test cleanup and after max
// (default [DefaultMaxInProcessSchedulerLifetime]). Pass it to `Scheduler.Start(ctx)` so the
// daemon loop cannot outlive the test budget. Prefer canceling early after the assertion
// (job callback / condition), not waiting for the full max.
func BoundSchedulerStartContext(t testing.TB, parent context.Context, max time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	if parent == nil {
		parent = context.Background()
	}
	if max <= 0 {
		max = DefaultMaxInProcessSchedulerLifetime
	}
	ctx, cancel := context.WithTimeout(parent, max)
	t.Cleanup(cancel)
	return ctx, cancel
}

// WaitChanOrBound waits for done or boundCtx.Done(). On deadline, returns context.DeadlineExceeded.
// Use for job completion callbacks so tests fail fast and Cleanup stops the daemon.
func WaitChanOrBound(done <-chan struct{}, boundCtx context.Context) error {
	if boundCtx == nil {
		<-done
		return nil
	}
	select {
	case <-done:
		return nil
	case <-boundCtx.Done():
		return boundCtx.Err()
	}
}

// WaitChanOrBoundWithCap waits for done, or whichever comes first of waitCap and boundCtx.
// Prefer this for a single job-completion callback (e.g. 60s) under a longer daemon ceiling.
func WaitChanOrBoundWithCap(done <-chan struct{}, boundCtx context.Context, waitCap time.Duration) error {
	if waitCap <= 0 {
		return WaitChanOrBound(done, boundCtx)
	}
	timer := time.NewTimer(waitCap)
	defer timer.Stop()
	if boundCtx == nil {
		select {
		case <-done:
			return nil
		case <-timer.C:
			return context.DeadlineExceeded
		}
	}
	select {
	case <-done:
		return nil
	case <-timer.C:
		return context.DeadlineExceeded
	case <-boundCtx.Done():
		return boundCtx.Err()
	}
}
