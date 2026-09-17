package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/process"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// isAggressiveRetentionTolerance returns true when the command is retention-tolerance with
// --batch-size >= 5000 and --max-batches >= 20 (one-off aggressive cleanup).
func isAggressiveRetentionTolerance(normalizedCmd string, args []string) bool {
	if !strings.Contains(normalizedCmd, "retention-tolerance") {
		return false
	}
	var batchSize, maxBatches int
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "--batch-size":
			if n, err := strconv.Atoi(args[i+1]); err == nil {
				batchSize = n
			}
		case "--max-batches":
			if n, err := strconv.Atoi(args[i+1]); err == nil {
				maxBatches = n
			}
		}
	}
	return batchSize >= 5000 && maxBatches >= 20
}

// WrapCommand wraps a command execution with timeout and metrics tracking.
// The runner receives the execution context (with timeout when applicable) so that blocking
// operations (e.g. list/count semaphore) can be cancelled when the timeout fires.
func (h *TimeoutHook) WrapCommand(ctx context.Context, command string, args []string, fn func(execCtx context.Context) error) error {
	return h.WrapCommandWithContext(ctx, command, args, nil, fn)
}

// parseTimeoutFlag extracts timeout value from command context and args
func (h *TimeoutHook) parseTimeoutFlag(cmdCtx *CommandContext, args []string) (timeout time.Duration, explicitlySet bool) {
	// Check cmdCtx.Flags first (highest priority)
	if cmdCtx != nil && cmdCtx.Flags != nil {
		if timeoutVal, ok := cmdCtx.Flags["timeout"]; ok {
			explicitlySet = true
			switch v := timeoutVal.(type) {
			case time.Duration:
				timeout = v
			case string:
				// Try to parse as duration string (e.g., "10m", "30s")
				if parsed, err := time.ParseDuration(v); err == nil {
					timeout = parsed
				}
			}
		}
	}

	// Also check args directly for --timeout flag (fallback if not in cmdCtx)
	if timeout == 0 {
		for i, arg := range args {
			if arg == "--timeout" && i+1 < len(args) {
				if !explicitlySet {
					explicitlySet = true
				}
				if parsed, err := time.ParseDuration(args[i+1]); err == nil {
					timeout = parsed
					break
				}
			}
		}
	}

	// Check if explicitly set (even if value is 0)
	if !explicitlySet {
		if cmdCtx != nil && cmdCtx.Flags != nil {
			if _, exists := cmdCtx.Flags["timeout"]; exists {
				explicitlySet = true
			}
		}
		if !explicitlySet {
			for i, arg := range args {
				if arg == "--timeout" && i+1 < len(args) {
					explicitlySet = true
					break
				}
			}
		}
	}

	return timeout, explicitlySet
}

// applyUserTimeoutOverride applies user-specified timeout override if provided
func (h *TimeoutHook) applyUserTimeoutOverride(timeout time.Duration, userTimeout time.Duration, timeoutExplicitlySet bool, normalizedCmd string) time.Duration {
	if !timeoutExplicitlySet {
		return timeout
	}

	if userTimeout > 0 {
		// CRITICAL: For system check with --auto-fix, enforce minimum 2-minute timeout
		// Metrics show 100% error rate for --auto-fix with short timeouts (30s, 60s)
		// Auto-fix operations require time to apply fixes, so short timeouts always fail
		if strings.Contains(normalizedCmd, "system check") && strings.Contains(normalizedCmd, "--auto-fix") {
			minAutoFixTimeout := 2 * time.Minute
			if userTimeout < minAutoFixTimeout {
				h.logger.LogDebug("User timeout too short for --auto-fix, enforcing minimum",
					logging.String("command", normalizedCmd),
					logging.String("user_timeout", userTimeout.String()),
					logging.String("enforced_timeout", minAutoFixTimeout.String()))
				return minAutoFixTimeout
			}
		}
		h.logger.LogDebug("Using user-specified timeout",
			logging.String("command", normalizedCmd),
			logging.String("timeout", userTimeout.String()))
		return userTimeout
	}

	// User explicitly set timeout to 0 (disable timeout)
	h.logger.LogDebug("Timeout disabled by user (--timeout 0)",
		logging.String("command", normalizedCmd))
	return 0
}

// mcpLongLivedCommands are MCP commands that must outlive the child timeout: ensure/supervise
// start them with a non-zqk parent (python), which otherwise hits the 2m childMaxTimeout and kills
// the TCP listener (~"transport closed"). Same lifecycle as mcp serve → idle_timeout in
// .zqk/mcp/config.yaml.
var mcpLongLivedCommands = []string{
	"mcp serve",
	"mcp daemon",
	"mcp proxy",
	"mcp supervise",   // supervise --loop must not die under childMaxTimeout when spawned detached
	"mcp ide-adapter", // Cursor/IDE stdio bridge; parent is the IDE, not zqk
	"mcp cursor-adapter",
}

// otherDaemonCommands are the non-MCP commands that run until stopped.
var otherDaemonCommands = []string{
	"scheduler start",
	"system truth-sentinel",
	zqkenv.PrivilegedWriterDaemonCommandFragment(),
	"feed watch",        // long-lived MCP subscriber; auto-timeout must not kill the seat watcher
	"agent orchestrate", // multi-task priority plan orchestrator; runs until plan completes
}

// isMCPLongLivedCommand reports whether the command is an MCP process that must not be killed by
// the child timeout.
func isMCPLongLivedCommand(normalizedCmd string) bool {
	return containsAnyCommand(normalizedCmd, mcpLongLivedCommands)
}

// isDaemonCommand reports whether the command runs until stopped rather than to completion.
func isDaemonCommand(normalizedCmd string) bool {
	return isMCPLongLivedCommand(normalizedCmd) || containsAnyCommand(normalizedCmd, otherDaemonCommands)
}

// containsAnyCommand reports whether normalizedCmd contains any of the given command fragments.
func containsAnyCommand(normalizedCmd string, fragments []string) bool {
	for _, fragment := range fragments {
		if strings.Contains(normalizedCmd, fragment) {
			return true
		}
	}
	return false
}

// handleDaemonCommandTimeout handles timeout logic for daemon/server commands
func (h *TimeoutHook) handleDaemonCommandTimeout(ctx context.Context, command, normalizedCmd string, args []string, cmdCtx *CommandContext, fn func(execCtx context.Context) error, startTime time.Time, timeout time.Duration, timeoutExplicitlySet bool) (bool, error) {
	isMCPLongLived := isMCPLongLivedCommand(normalizedCmd)

	if !isDaemonCommand(normalizedCmd) {
		return false, nil
	}

	if isMCPLongLived {
		err := h.wrapCommandWithResettableTimeout(ctx, command, normalizedCmd, args, cmdCtx, func() error { return fn(ctx) }, startTime, timeout)
		return true, err
	}

	// For scheduler start and other daemon commands, disable timeout if:
	// 1. User explicitly set timeout to 0, OR
	// 2. Timeout wasn't explicitly set (use baseline timeout, but scheduler start should run indefinitely)
	if timeout == 0 || !timeoutExplicitlySet {
		// No timeout - command runs indefinitely
		return true, fn(ctx)
	}

	return false, nil
}

// executeCommandWithTimeout executes a command with timeout and signal handling
func (h *TimeoutHook) executeCommandWithTimeout(timeoutCtx context.Context, timeout time.Duration, normalizedCmd, command string, fn func() error) (timedOut bool, exitCode int, execErr error) {
	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	defer signal.Stop(sigChan)

	// Channel to track execution result
	errChan := make(chan error, 1)

	// Log context state before starting goroutine
	h.logger.LogDebug("Starting command executor goroutine",
		logging.String("command", normalizedCmd),
		logging.String("timeout", timeout.String()),
		logging.Bool("timeout_ctx_done", timeoutCtx.Err() != nil))

	// Check if context is already cancelled before starting goroutine
	if timeoutCtx.Err() != nil {
		if timeoutCtx.Err() == context.Canceled {
			return false, 1, errfmt.Errorf("command context cancelled before execution: %s", timeoutCtx.Err())
		}
		return false, 124, errfmt.Errorf("command context cancelled before execution: %s", timeoutCtx.Err())
	}

	// Execute command in goroutine
	// NOTE: We don't pass WithContext here because we want the goroutine to run even if timeoutCtx
	// is cancelled (the select will handle timeout). Passing WithContext would cause the goroutine
	// to exit early without sending to errChan, causing a hang.
	goroutinelabels.NewGoroutine("command_executor", fmt.Sprintf("executing command: %s", normalizedCmd)).
		StartSimple(func() {
			h.logger.LogDebug("Command executor goroutine started",
				logging.String("command", normalizedCmd))
			defer func() {
				if r := recover(); r != nil {
					// Capture stack trace for debugging
					buf := make([]byte, 4096)
					n := runtime.Stack(buf, false)
					stackTrace := string(buf[:n])

					h.logger.LogWarning("Panic in command executor",
						logging.String("command", normalizedCmd),
						logging.String("panic", fmt.Sprintf("%v", r)),
						logging.String("stack_trace", stackTrace))
					// Ensure we send to errChan even on panic
					select {
					case errChan <- errfmt.Errorf("panic in command executor: %v\n\nStack trace:\n%s", r, stackTrace):
					default:
						// Channel already has a value (shouldn't happen with buffered channel of size 1)
					}
				}
			}()
			result := fn()
			h.logger.LogDebug("Command executor completed",
				logging.String("command", normalizedCmd),
				logging.Bool("has_error", result != nil))
			// Ensure we send to errChan
			select {
			case errChan <- result:
			default:
				// Channel already has a value (shouldn't happen with buffered channel of size 1)
			}
		})

	// Wait for completion or timeout
	select {
	case execErr = <-errChan:
		// Command completed
		h.logger.LogDebug("Command execution completed",
			logging.String("command", normalizedCmd),
			logging.Bool("has_error", execErr != nil))
		if execErr != nil {
			exitCode = 1
			var exitCoder interface{ ExitCode() int }
			if errors.As(execErr, &exitCoder) {
				exitCode = exitCoder.ExitCode()
			}
		}
	case <-timeoutCtx.Done():
		if process.IsTimeoutMonitorDisconnected() {
			// Timeout monitor was disconnected for an interactive session (e.g. pager).
			// Wait for the command to finish or receive an interrupt signal instead of killing it.
			select {
			case execErr = <-errChan:
				if execErr != nil {
					exitCode = 1
					var exitCoder interface{ ExitCode() int }
					if errors.As(execErr, &exitCoder) {
						exitCode = exitCoder.ExitCode()
					}
				}
			case <-sigChan:
				execErr = errfmt.Errorf("command interrupted")
				exitCode = 130
			}
			return timedOut, exitCode, execErr
		}
		if timeoutCtx.Err() == context.Canceled {
			timedOut = false
			execErr = errfmt.Errorf("command context cancelled: %s", normalizedCmd)
			exitCode = 1
			h.logger.LogDebug("Command context cancelled",
				logging.String("command", normalizedCmd),
				logging.String("full_command", command))
		} else {
			// Timeout occurred
			timedOut = true
			execErr = errfmt.Errorf("command timed out after %v: %s", timeout, normalizedCmd)
			exitCode = 124 // Standard timeout exit code
			baseline := h.getBaselineDuration(normalizedCmd)
			h.logger.LogWarning("Command timed out",
				logging.String("command", normalizedCmd),
				logging.String("full_command", command),
				logging.String("timeout", timeout.String()),
				logging.String("baseline_duration", baseline.String()))
		}
	case <-sigChan:
		// Interrupted
		execErr = errfmt.Errorf("command interrupted")
		exitCode = 130 // Standard interrupt exit code
		h.logger.LogDebug("Command interrupted",
			logging.String("command", normalizedCmd))
	}

	return timedOut, exitCode, execErr
}

func (h *TimeoutHook) buildCommandMetric(command, normalizedCmd string, args []string, cmdCtx *CommandContext, startTime, endTime time.Time, execErr error, timedOut bool, exitCode int) *CommandMetric {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	metric := &CommandMetric{
		Command:        command,
		NormalizedCmd:  normalizedCmd,
		Duration:       time.Since(startTime),
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
		MaxMemoryBytes: m.Sys,
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

	return metric
}

// recordMetricsAsync records command metrics asynchronously (non-blocking)
func (h *TimeoutHook) recordMetricsAsync(metric *CommandMetric, normalizedCmd string) {
	if h.metricsStore == nil {
		return
	}

	h.wg.Add(1)
	// Record metrics asynchronously (fire-and-forget) to prevent blocking command completion
	goroutinelabels.NewGoroutine("command_metrics_recorder", fmt.Sprintf("recording metrics for command: %s", normalizedCmd)).
		StartSimple(func() {
			defer h.wg.Done()
			// Use a separate timeout context for metrics recording
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
				// Metrics recording timed out - log but don't block
				h.logger.LogWarning("Metrics recording timed out (non-blocking)",
					logging.String("command", normalizedCmd),
					logging.String("timeout", config.MetricsRecordingTimeout.String()))
			}
		})
}

// WrapCommandWithContext wraps a command execution with timeout, metrics tracking, and context.
// The runner fn receives the execution context (execCtx). When a timeout is active, execCtx
// is cancelled when the timeout fires so that blocking operations (e.g. storage list/count
// semaphore) can exit instead of blocking until process exit.
func (h *TimeoutHook) WrapCommandWithContext(ctx context.Context, command string, args []string, cmdCtx *CommandContext, fn func(execCtx context.Context) error) error {
	if !h.enabled {
		return fn(ctx)
	}

	startTime := time.Now()
	normalizedCmd := h.normalizeCommand(command, args)

	// Parse user timeout flag
	userTimeout, timeoutExplicitlySet := h.parseTimeoutFlag(cmdCtx, args)

	// Get baseline timeout from metrics if available
	timeout := h.getTimeoutForCommand(normalizedCmd, args)

	// Override with user-specified timeout if provided
	timeout = h.applyUserTimeoutOverride(timeout, userTimeout, timeoutExplicitlySet, normalizedCmd)

	// Aggressive one-off retention: use 2h so the run is not killed (even when baseline timeout is 0)
	if isAggressiveRetentionTolerance(normalizedCmd, args) {
		if timeout <= 0 || timeout < 2*time.Hour {
			timeout = 2 * time.Hour
		}
	}

	// scheduler stop --wait/--max-wait can exceed the default "scheduler stop" rule (often 30s in
	// command_timeouts.yaml); align outer hook with stopScheduler inner wall — see timeout_hook_scheduler_stop.go.
	timeout = adjustOuterTimeoutForSchedulerStopWait(normalizedCmd, args, timeout)

	// Handle daemon commands (may return early)
	if handled, err := h.handleDaemonCommandTimeout(ctx, command, normalizedCmd, args, cmdCtx, fn, startTime, timeout, timeoutExplicitlySet); handled {
		return err
	}

	// When parent is not the main zqk process (e.g. script or IDE), cap timeout so we don't burn 10m on a stuck command.
	// Exemptions are declared per-command in config/command_timeouts.yaml via child_max_timeout_exempt
	// and child_max_timeout_exempt_if_explicit rather than hardcoded here.
	const childMaxTimeout = 2 * time.Minute
	if !process.IsParentZqk() && timeout > 0 && timeout > childMaxTimeout {
		exempt := h.isChildMaxTimeoutExempt(normalizedCmd, timeoutExplicitlySet) ||
			schedulerStopWaitExemptFromChildCap(normalizedCmd, args)
		if !exempt {
			timeout = childMaxTimeout
		}
	}

	// Create context with timeout (skip if timeout is 0 - disabled)
	var execCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	} else {
		execCtx = ctx
	}

	// When parent is not zqk, run an idle watchdog: cancel context if no meaningful activity for idleShutdownDuration.
	// Per-command overrides are declared in config/command_timeouts.yaml via idle_shutdown_duration.
	const defaultIdleShutdownDuration = 10 * time.Second
	idleShutdownDuration := h.getIdleShutdownDuration(normalizedCmd, defaultIdleShutdownDuration)
	runCtx := execCtx
	if !process.IsParentZqk() {
		process.TouchMeaningfulActivity()
		idleCtx, idleCancel := context.WithCancel(execCtx)
		defer idleCancel()
		runCtx = idleCtx
		goroutinelabels.NewGoroutine("cli_child_idle_watchdog", "cancel command when idle and parent is not zqk").
			StartWithContext(execCtx, func(ctx context.Context) error {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-ticker.C:
						if process.IsTimeoutMonitorDisconnected() {
							process.TouchMeaningfulActivity()
							continue
						}
						if time.Since(process.GetLastMeaningfulActivity()) > idleShutdownDuration {
							idleCancel()
							return nil
						}
					}
				}
			})
	}

	// Run the command with runCtx so it can be cancelled when timeout or idle fires
	runWithExecCtx := func() error { return fn(runCtx) }
	timedOut, exitCode, execErr := h.executeCommandWithTimeout(execCtx, timeout, normalizedCmd, command, runWithExecCtx)

	endTime := time.Now()

	// Build and record metrics
	metric := h.buildCommandMetric(command, normalizedCmd, args, cmdCtx, startTime, endTime, execErr, timedOut, exitCode)
	h.recordMetricsAsync(metric, normalizedCmd)

	return execErr
}
