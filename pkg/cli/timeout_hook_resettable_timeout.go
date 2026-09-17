package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// Global variable to store timeout reset function and context (for server commands)
var (
	timeoutResetFuncMu sync.Mutex
	timeoutResetFunc   func()
	serverContext      context.Context // Context for server commands (cancelled on timeout)
)

// setTimeoutResetFunc sets the timeout reset function (thread-safe)
func setTimeoutResetFunc(fn func()) {
	_ = concurrency.RunInLockWithLogger(
		&timeoutResetFuncMu, LockNameTimeoutHookSetResetFunc, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			timeoutResetFunc = fn
			return nil
		},
	)
}

// ResetTimeout resets the timeout for the current server command (if any)
// This should be called whenever the server receives client activity
func ResetTimeout() {
	var resetFunc func()
	_ = concurrency.RunInLockWithLogger(
		&timeoutResetFuncMu, LockNameTimeoutHookGetResetFunc, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			resetFunc = timeoutResetFunc
			return nil
		},
	)
	if resetFunc != nil {
		resetFunc()
	}
}

// setServerContext stores the server context (for graceful shutdown on timeout)
func setServerContext(ctx context.Context) {
	_ = concurrency.RunInLockWithLogger(
		&timeoutResetFuncMu, LockNameTimeoutHookSetServerContext, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			serverContext = ctx
			return nil
		},
	)
}

// GetServerContext returns the current server context (for checking cancellation)
func GetServerContext() context.Context {
	var ctx context.Context
	_ = concurrency.RunInLockWithLogger(
		&timeoutResetFuncMu, LockNameTimeoutHookGetServerContext, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			ctx = serverContext
			return nil
		},
	)
	return ctx
}

// wrapCommandWithResettableTimeout wraps a command execution with a resettable timeout
// This is used for server commands where the timeout should reset on client activity
// The timeout function is stored in a global variable that the server can access
func (h *TimeoutHook) wrapCommandWithResettableTimeout(ctx context.Context, command, normalizedCmd string, args []string, cmdCtx *CommandContext, fn func() error, startTime time.Time, initialTimeout time.Duration) error {
	// Create a channel to signal timeout reset requests
	resetChan := make(chan struct{}, 1)

	// Create initial timeout context
	// If timeout is 0 or very large (effectively infinite), use background context without timeout
	var timeoutCtx context.Context
	var cancel context.CancelFunc
	if initialTimeout <= 0 || initialTimeout > 1000*time.Hour {
		// Effectively infinite timeout - use system context without timeout
		// Use NewSystemContext() as base to avoid inheriting any timeout from parent context
		timeoutCtx, cancel = context.WithCancel(pkgctx.NewSystemContext())
		// Don't cancel immediately - let it run indefinitely
		// We'll only cancel on explicit shutdown
	} else {
		timeoutCtx, cancel = context.WithTimeout(ctx, initialTimeout)
	}
	defer cancel()

	// Store the reset function and context in a way the server can access it
	// We'll use package-level variables for this
	setTimeoutResetFunc(func() {
		select {
		case resetChan <- struct{}{}:
		default:
			// Channel full, skip reset (shouldn't happen with buffered channel)
		}
	})
	setServerContext(timeoutCtx)
	defer func() {
		setTimeoutResetFunc(nil) // Clear on exit
		setServerContext(context.Background())
	}()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	defer signal.Stop(sigChan)

	// Channel to track execution result
	errChan := make(chan error, 1)
	var timedOut bool
	var exitCode int
	var execErr error

	// Execute command in goroutine
	goroutinelabels.NewGoroutine("command_executor", fmt.Sprintf("executing command: %s", normalizedCmd)).
		StartSimple(func() {
			errChan <- fn()
		})

	// Wait for completion, timeout, reset signal, or interrupt
	for {
		select {
		case execErr = <-errChan:
			// Command completed
			// If we've already timed out, ignore any error from the command
			// (the timeout handler already set execErr = nil and exitCode = 0)
			if timedOut {
				// Already handled timeout gracefully, ignore this error
				execErr = nil
				exitCode = 0
			} else if execErr != nil {
				exitCode = 1
			}
			goto done
		case <-timeoutCtx.Done():
			// Timeout occurred - gracefully shutdown server instead of erroring
			// For server commands, timeout means "no client activity, shutdown gracefully"
			timedOut = true
			execErr = nil // No error - graceful shutdown
			exitCode = 0  // Clean exit
			baseline := h.getBaselineDuration(normalizedCmd)
			timeoutErr := timeoutCtx.Err()
			h.logger.LogInfo("Server shutting down gracefully (no client activity)",
				logging.String("command", normalizedCmd),
				logging.String("full_command", command),
				logging.String("timeout", initialTimeout.String()),
				logging.String("baseline_duration", baseline.String()),
				logging.String("reason", "idle_timeout"),
				logging.String("timeout_error", timeoutErr.Error()),
				logging.String("trace", "timeout_expired_in_timeout_hook"))
			// Signal the server to shutdown gracefully by cancelling the context
			// The server's Serve() method should check for context cancellation
			cancel() // Cancel the context to signal shutdown
			// Wait briefly for the command to finish (with timeout), then exit
			// This prevents the goroutine from overwriting our graceful shutdown
			select {
			case <-errChan:
				// Command finished, but we've already set execErr = nil, so ignore it
			case <-time.After(100 * time.Millisecond):
				// Give it a moment, then proceed with graceful shutdown
			}
			goto done
		case <-resetChan:
			// Client activity detected - reset timeout
			// Log timeout reset for debugging
			h.logger.LogDebug("Resetting server timeout due to client activity",
				logging.String("command", normalizedCmd),
				logging.String("timeout", initialTimeout.String()))
			oldCancel := cancel // Save old cancel
			// Create new timeout context (same logic as initial creation)
			if initialTimeout <= 0 || initialTimeout > 1000*time.Hour {
				// Effectively infinite timeout - use system context without timeout
				// Use NewSystemContext() as base to avoid inheriting any timeout from parent context
				timeoutCtx, cancel = context.WithCancel(pkgctx.NewSystemContext())
			} else {
				timeoutCtx, cancel = context.WithTimeout(ctx, initialTimeout)
			}
			// Update server context with new timeout context
			setServerContext(timeoutCtx)
			if oldCancel != nil {
				oldCancel() // Cancel old timeout AFTER updating server context
			}
			// Continue waiting
		case <-sigChan:
			// Interrupted
			cancel() // Ensure cancel is called
			execErr = errfmt.Errorf("command interrupted")
			exitCode = 130 // Standard interrupt exit code
			goto done
		}
	}

done:
	if cancel != nil {
		cancel() // Ensure cancel is called on all paths
	}
	duration := time.Since(startTime)
	endTime := time.Now()

	// Record metrics with enhanced context
	metric := &CommandMetric{
		Command:        command,
		NormalizedCmd:  normalizedCmd,
		Duration:       duration,
		StartTime:      startTime,
		EndTime:        endTime,
		Success:        execErr == nil && !timedOut,
		ExitCode:       exitCode,
		Error:          h.sanitizeError(execErr),
		TimedOut:       timedOut,
		Timestamp:      startTime,
		Args:           h.sanitizeArgs(args),
		Flags:          make(map[string]any),
		ObjectsCreated: []string{},
		ObjectsUpdated: []string{},
		ObjectsDeleted: []string{},
	}

	// Add context information if provided
	if cmdCtx != nil {
		metric.PriorityPlan = cmdCtx.PriorityPlan
		metric.Workstream = cmdCtx.Workstream
		metric.Milestone = cmdCtx.Milestone
		metric.ActorID = cmdCtx.ActorID
		metric.ActorRoles = cmdCtx.ActorRoles
		if cmdCtx.Flags != nil {
			metric.Flags = cmdCtx.Flags
		}
	}

	if h.metricsStore != nil {
		// Record metrics asynchronously (fire-and-forget)
		goroutinelabels.NewGoroutine("command_metrics_recorder", fmt.Sprintf("recording metrics for command: %s", normalizedCmd)).
			StartSimple(func() {
				config := h.getTimeoutConfig()
				metricsCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), config.MetricsRecordingTimeout)
				defer cancel()

				done := make(chan error, 1)
				goroutinelabels.NewGoroutine("metrics_store_writer", fmt.Sprintf("writing metrics for: %s", normalizedCmd)).
					StartSimple(func() {
						done <- h.metricsStore.RecordCommandExecution(metric)
					})

				select {
				case err := <-done:
					if err != nil {
						h.logger.LogWarning("Failed to record command metrics",
							logging.Error(err),
							logging.String("command", normalizedCmd))
					}
				case <-metricsCtx.Done():
					h.logger.LogWarning("Metrics recording timed out (non-blocking)",
						logging.String("command", normalizedCmd),
						logging.String("timeout", "5s"))
				}
			})
	}

	return execErr
}
