package concurrency

import (
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// RunWithMaxWait runs fn in a new goroutine and returns when fn completes or maxWait elapses,
// whichever comes first. Use when you must avoid blocking indefinitely on fn (e.g. writing to
// stdout/pipe that may be broken or full) while still allowing fn to complete when it can.
//
// The goroutine is not cancelled when maxWait elapses; fn may still run to completion in the
// background. Callers use this to cap how long the current goroutine waits, not to abort fn.
//
// Uses goroutinelabels so the goroutine is named and observable (profiling, debugging);
// see pkg/goroutinelabels and docs/architecture/concurrency-patterns-v1.0.md.
//
// Example: scheduler start writes success message to cmd.OutOrStdout(); if that is a pipe
// with no reader or full buffer, Fprintf blocks. RunWithMaxWait ensures the CLI returns
// after maxWait so the process can exit instead of staying hung.
//
//	concurrency.RunWithMaxWait(func() {
//	    fmt.Fprintf(w, "Scheduler started (PID: %d).\n", pid)
//	}, 2*time.Second)
func RunWithMaxWait(fn func(), maxWait time.Duration) {
	done := make(chan bool, 1)
	goroutinelabels.NewGoroutine("run_with_max_wait", "run fn with max wait cap; returns when fn completes or timeout").
		WithSignalOnExit(done).
		StartSimple(fn)
	timer := time.NewTimer(maxWait)
	defer timer.Stop()
	select {
	case <-done:
		return
	case <-timer.C:
		return
	}
}
