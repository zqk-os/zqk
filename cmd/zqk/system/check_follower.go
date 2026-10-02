package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

// CheckCompletionSubscriber subscribes to system_check completion events
type CheckCompletionSubscriber struct {
	operationID string
	mu          sync.Mutex
	completed   bool
	result      error
	done        chan struct{}
}

// NewCheckCompletionSubscriber creates a new subscriber for a specific operation
func NewCheckCompletionSubscriber(operationID string) *CheckCompletionSubscriber {
	return &CheckCompletionSubscriber{
		operationID: operationID,
		done:        make(chan struct{}),
	}
}

// TerminalProgressSubscriber renders system_check progress and milestone events
// to the terminal (stderr) based on coordinator operational events. This keeps
// CLI UX responsive while staying inside the coordination framework.
type TerminalProgressSubscriber struct {
	operationID string
	cmd         *cobra.Command

	mu                sync.Mutex
	active            atomic.Bool
	lastPct           float64
	lastDiscoveryMsg  string
	lastDiscoveryTime time.Time
	hasShownStart     bool
}

// NewTerminalProgressSubscriber creates a new terminal progress subscriber.
func NewTerminalProgressSubscriber(operationID string, cmd *cobra.Command) *TerminalProgressSubscriber {
	sub := &TerminalProgressSubscriber{
		operationID: operationID,
		cmd:         cmd,
	}
	sub.active.Store(true)
	return sub
}

// ID returns the subscriber ID.
func (s *TerminalProgressSubscriber) ID() string {
	return fmt.Sprintf("terminal_progress_%s", s.operationID)
}

// HandleEvent processes progress-related operational events for the operation.
func (s *TerminalProgressSubscriber) HandleEvent(event *coordination.OperationalEvent) error {
	if event == nil || s.cmd == nil {
		return nil
	}

	// Only handle events for this operation and type.
	if event.OperationID != s.operationID || event.OperationType != eventTypeSystemCheck {
		return nil
	}

	switch event.Type {
	case "operation.progress", "operation.unknown", "operation.start":
		if !s.active.Load() {
			return nil
		}
		phaseAny := event.Metadata[objects.FieldKeyPhase]
		phase, hasPhase := phaseAny.(string)

		// Phase "start": system check starting (emitted by follower before background run)
		if hasPhase && phase == eventStatusStart {
			s.mu.Lock()
			defer s.mu.Unlock()
			if !s.active.Load() {
				return nil
			}
			if !s.hasShownStart {
				s.hasShownStart = true
				fmt.Fprintf(s.cmd.ErrOrStderr(), "System check starting...\n")
			}
			return nil
		}

		// Teardown / finalization phase
		if hasPhase && (phase == "teardown" || phase == "finalizing" || phase == "shutdown") {
			msgAny := event.Metadata["message"]
			msg := "Finalizing caches and storage queues..."
			if m, ok := msgAny.(string); ok && m != emptyValue {
				msg = m
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if !s.active.Load() {
				return nil
			}
			fmt.Fprintf(s.cmd.ErrOrStderr(), "%s\n", msg)
			return nil
		}

		// Phase "cache": object ID cache progress (loading/ready/error/timeout) via coordinator
		if hasPhase && phase == targetKindCache {
			msgAny := event.Metadata["message"]
			if msg, ok := msgAny.(string); ok && msg != emptyValue {
				fmt.Fprintf(s.cmd.ErrOrStderr(), "%s\n", msg)
			}
			return nil
		}

		// Discovery phase
		if hasPhase && phase == "discovery" {
			// Discovery start/progress: render discovery-specific message
			filesFoundAny := event.Metadata["files_found"]
			elapsedAny := event.Metadata["elapsed_seconds"]
			kindAny := event.Metadata[objects.FieldKeyKind]
			kindsCountAny := event.Metadata["kinds_count"]

			filesFound := 0
			if f, ok := filesFoundAny.(int); ok {
				filesFound = f
			}
			elapsed := ""
			if e, ok := elapsedAny.(float64); ok {
				elapsed = fmt.Sprintf("%.0fs", e)
			}

			s.mu.Lock()
			defer s.mu.Unlock()
			if !s.active.Load() {
				return nil
			}

			// For discovery start (no files_found yet), show kind count
			if filesFound == 0 && event.Type == "operation.start" {
				kindsCount := 0
				if kc, ok := kindsCountAny.(int); ok {
					kindsCount = kc
				}
				msg := fmt.Sprintf("Starting discovery across %d kinds...", kindsCount)
				if targetKind, ok := event.Metadata[objects.FieldKeyTargetKind].(string); ok && targetKind != emptyValue {
					msg = fmt.Sprintf("Starting discovery for kind: %s...", targetKind)
				}
				fmt.Fprintf(s.cmd.ErrOrStderr(), "%s\n", msg)
				return nil
			}

			// Discovery progress: show files found
			// Skip displaying progress if count is unknown (negative value)
			if filesFound < 0 {
				return nil
			}
			kindStr := ""
			if kindAny != nil {
				if k, ok := kindAny.(string); ok && k != emptyValue {
					kindStr = k
				}
			}

			// Always show progress if we have elapsed time (heartbeat indicator)
			// If no elapsed time, still show if we have files found (count available)
			if elapsed == emptyValue && filesFound == 0 {
				// No elapsed time and no files - skip this update (wait for next tick)
				return nil
			}

			// Build message with count if available, otherwise just show elapsed time as heartbeat
			var msg string
			if filesFound > 0 {
				// We have a count - show it with elapsed time if available
				if kindStr != emptyValue {
					msg = fmt.Sprintf("Discovering %s objects... %d found", kindStr, filesFound)
				} else {
					msg = fmt.Sprintf("Discovering objects... %d found", filesFound)
				}
				if elapsed != emptyValue {
					msg += fmt.Sprintf(" (%s)", elapsed)
				}
			} else if elapsed != emptyValue {
				// No files counted yet, but show heartbeat with elapsed time
				// This provides continuous feedback that discovery is in progress
				if kindStr != emptyValue {
					msg = fmt.Sprintf("Discovering %s objects... (%s)", kindStr, elapsed)
				} else {
					msg = fmt.Sprintf("Discovering objects... (%s)", elapsed)
				}
			} else {
				// Fallback: show generic message if no elapsed time or count
				// This shouldn't happen, but provides safety net
				if kindStr != emptyValue {
					msg = fmt.Sprintf("Discovering %s objects...", kindStr)
				} else {
					msg = "Discovering objects..."
				}
			}

			// Only print if message changed or enough time has passed (avoid spam)
			shouldPrint := false
			if msg != s.lastDiscoveryMsg {
				shouldPrint = true
				s.lastDiscoveryMsg = msg
			} else if time.Since(s.lastDiscoveryTime) >= 1*time.Second {
				// Print heartbeat every 1 second even if count hasn't changed
				shouldPrint = true
			}

			if shouldPrint {
				s.lastDiscoveryTime = time.Now()
				fmt.Fprintf(s.cmd.ErrOrStderr(), "%s\n", msg)
			}
			return nil
		}

		// Aggregation phase: post-validation aggregation and CAS membrane check
		if hasPhase && (phase == "aggregation" || phase == "post_validation") {
			msgAny := event.Metadata["message"]
			msg := "Aggregating validation layers and checking CAS membrane..."
			if m, ok := msgAny.(string); ok && m != emptyValue {
				msg = m
			}
			fmt.Fprintf(s.cmd.ErrOrStderr(), "%s\n", msg)
			return nil
		}

		// Validation progress: extract progress metrics from metadata.
		progressAny := event.Metadata["progress"]
		totalAny := event.Metadata["total_tasks"]
		if totalAny == nil {
			totalAny = event.Metadata["total"]
		}
		queueAny := event.Metadata["queue_size"]

		progress, okP := progressAny.(int)
		total, okT := totalAny.(int)
		if !okP || !okT || total <= 0 {
			return nil
		}

		queueSize := 0
		if q, ok := queueAny.(int); ok {
			queueSize = q
		}

		pct := calculateProgressPercent(progress, total)

		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.active.Load() {
			return nil
		}

		// Avoid spamming if percentage hasn't moved much.
		if pct-s.lastPct < 1.0 && pct < 100.0 {
			return nil
		}
		s.lastPct = pct

		// Render a simple progress line to stderr.
		fmt.Fprintf(s.cmd.ErrOrStderr(), "System check progress: %.1f%% (%d/%d, queue: %d)\n", pct, progress, total, queueSize)

	case "operation.complete", "operation.error":
		// Mark inactive so we stop rendering after completion/error.
		s.active.Store(false)
	}

	return nil
}

// EventTypes returns the event types we're interested in.
func (s *TerminalProgressSubscriber) EventTypes() []string {
	return []string{"operation.start", "operation.progress", "operation.complete", "operation.error", "operation.unknown"}
}

// IsActive returns whether the subscriber is still active.
func (s *TerminalProgressSubscriber) IsActive() bool {
	return s.active.Load()
}

// ID returns the subscriber ID
func (s *CheckCompletionSubscriber) ID() string {
	return fmt.Sprintf("check_follower_%s", s.operationID)
}

// HandleEvent processes completion events for the operation
func (s *CheckCompletionSubscriber) HandleEvent(event *coordination.OperationalEvent) error {
	// Guard against nil event
	if event == nil {
		return nil
	}

	// Only process events for our operation ID and operation type
	if event.OperationID != s.operationID || event.OperationType != eventTypeSystemCheck {
		return nil
	}

	// Process completion or error events
	if event.Type == "operation.complete" || event.Type == "operation.error" {
		s.mu.Lock()
		if !s.completed {
			s.completed = true
			if event.Type == "operation.error" && event.Error != emptyValue {
				s.result = errfmt.Errorf("check failed: %s", event.Error)
			}
			// Safe to close - we check completed flag and hold lock
			// Use recover to prevent panic if channel already closed (defensive)
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Channel already closed - ignore (race condition handled)
					}
				}()
				close(s.done)
			}()
		}
		s.mu.Unlock()
	}

	return nil
}

// EventTypes returns the event types we're interested in
func (s *CheckCompletionSubscriber) EventTypes() []string {
	return []string{"operation.complete", "operation.error"}
}

// IsActive returns whether the subscriber is still active
func (s *CheckCompletionSubscriber) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.completed
}

// Wait waits for the operation to complete with a timeout
func (s *CheckCompletionSubscriber) Wait(timeout time.Duration) error {
	select {
	case <-s.done:
		s.mu.Lock()
		result := s.result
		s.mu.Unlock()
		return result
	case <-time.After(timeout):
		return errfmt.Errorf("timeout waiting for check completion after %v", timeout)
	}
}

// waitForCheckCompletion subscribes to coordinator events and waits for completion
func waitForCheckCompletion(operationID string, timeout time.Duration) error {
	// Get global coordinator
	coordinator := coordination.GetCoordinator()
	if coordinator == nil {
		return errfmt.Errorf("coordinator not available")
	}

	// Create subscriber
	subscriber := NewCheckCompletionSubscriber(operationID)

	// Subscribe
	subscriberID := coordinator.Subscribe(subscriber)
	defer coordinator.Unsubscribe(subscriberID)

	// Wait for completion
	return subscriber.Wait(timeout)
}

// runCheckAsyncWithFollow runs check in background and follows/wait for completion
func runCheckAsyncWithFollow(cmd *cobra.Command, args []string, timeout time.Duration) error {
	cli.TouchMeaningfulActivity() // idle watchdog: check follow started (avoids cancel during storage/coordinator setup when parent is not zqk)
	if cmd != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "System check starting...\n")
	}
	ctx, err := resolveSystemCliContextWithFallback(cmd)
	if err != nil {
		return err
	}

	// Generate operation ID before starting
	operationID := fmt.Sprintf("check_%d", time.Now().UnixNano())

	// Get project root and storage provider for coordination
	projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)
	storageProvider, cleanup := initCheckCoordinationStorage(projectRoot)
	defer cleanup()

	profile := profileOrDefault(ctx.Profile, systemProfileHuman)

	// Subscribe to coordinator events BEFORE starting background work to avoid missing early events
	coordinator := coordination.GetCoordinator()
	var progressSubscriberID string
	if coordinator != nil {
		// Completion subscriber (defensive / redundant with checkDone channel).
		completionSubscriber := NewCheckCompletionSubscriber(operationID)
		completionID := coordinator.Subscribe(completionSubscriber)
		defer coordinator.Unsubscribe(completionID)

		// Terminal progress subscriber for real-time UX when waiting for completion (default).
		// Subscribe BEFORE emitting start event to ensure we catch discovery start.
		progressSubscriber := NewTerminalProgressSubscriber(operationID, cmd)
		progressSubscriber.hasShownStart = true
		progressSubscriberID = coordinator.Subscribe(progressSubscriber)
		defer coordinator.Unsubscribe(progressSubscriberID)
	}

	// Emit start event immediately (after subscription so subscriber can catch it)
	if projectRoot != emptyValue && storageProvider != nil {
		opCallback := coordination.NewCoordinatorOperationCallback(
			pkgctx.NewSystemContext(),
			projectRoot,
			storageProvider,
			eventTypeSystemCheck,
			profile,
		)
		opCallback.OnStart(operationID, map[string]any{
			"operation_type":      eventTypeSystemCheck,
			"mode":                "async_follow",
			objects.FieldKeyPhase: eventStatusStart, // So TerminalProgressSubscriber shows "System check starting..."
		})
	}

	// Interrupt-aware context: cancelled on SIGINT so the check can exit gracefully (defers run, e.g. CPU profile flush).
	// Uses standard pattern: signal.NotifyContext bridges OS signal to context cancellation (see docs/architecture/SIGNAL_AND_CONTEXT.md).
	parent := pkgctx.NewSystemContext()
	if cmd != nil && cmd.Context() != nil {
		parent = cmd.Context()
	}
	runCtx, stopSignal := signal.NotifyContext(parent, os.Interrupt)
	defer stopSignal()

	// Start check in background goroutine. We must always send to checkDone exactly once so the
	// follower never blocks until the 30m timeout when the check panics or defers block.
	// Do NOT use goroutine budget for this goroutine: if budget is exhausted, StartWithContext
	// would return without starting it and checkDone would never be sent, causing a permanent hang.
	cli.TouchMeaningfulActivity() // idle watchdog: about to start check goroutine (after storage/coordinator setup)
	var checkErr error
	checkDone := make(chan error, 1)
	followerBuilder := goroutinelabels.NewGoroutine("system_check_background", fmt.Sprintf("running system check %s in background", operationID))
	followerBuilder.StartWithContext(runCtx, func(bgCtx context.Context) error {
		// Single exit path: defer sends to checkDone so panic or normal return both signal completion.
		defer func() { checkDone <- checkErr }()
		defer func() {
			if r := recover(); r != nil {
				checkErr = errfmt.Errorf("panic in system check: %v", r)
			}
		}()

		started := time.Now()
		checkErr = runCheckAsyncWithContextAndOperationID(cmd, ctx, args, operationID, runCtx)
		emitSystemCheckOperationResult(projectRoot, storageProvider, operationID, profile, checkErr, time.Since(started))

		return checkErr
	})

	// Wait for check completion, timeout, or interrupt (runCtx cancelled on SIGINT)
	select {
	case err := <-checkDone:
		if err != nil {
			return FormatCheckExecutionError(cmd, args, err)
		}
		if cmd != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "System check complete.")
		}
		return nil
	case <-runCtx.Done():
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return FormatCheckTimeoutError(cmd, args, timeout)
		}
		return runCtx.Err() // e.g. context.Canceled when user pressed Ctrl+C
	case <-time.After(timeout):
		return FormatCheckTimeoutError(cmd, args, timeout)
	}
}
