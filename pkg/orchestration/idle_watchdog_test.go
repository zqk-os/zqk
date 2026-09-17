package orchestration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/rollback"
)

// TestIdleWatchdog_StalledSubagent_CancelsExercises the core idle-prevention
// guarantee: a subagent that stops progressing and ignores cancellation is
// bounded by the watchdog, which cancels its context. The watchdog loop must
// exit immediately once it declares the subagent stalled — even if the task
// goroutine is still leaking — so the runner can retry/rollback.
func TestIdleWatchdog_StalledSubagent_IsBounded(t *testing.T) {
	const (
		timeout        = 40 * time.Millisecond
		probeTick      = 5 * time.Millisecond
		taskBlockDelay = 400 * time.Millisecond
		overallBudget  = 200 * time.Millisecond
	)

	w := NewIdleWatchdog(timeout, probeTick)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockErr := errors.New("task blocked")
	execStarted := make(chan struct{})

	done := make(chan error, 1)
	goroutinelabels.NewGoroutine("test.idle_watchdog_stalled", "test worker for stalled subagent").StartSimple(func() {
		done <- w.Execute(ctx, "subagent-stalled", func(tctx context.Context) error {
			close(execStarted)
			<-time.After(taskBlockDelay)
			return blockErr
		})
	})

	<-execStarted

	select {
	case err := <-done:
		if !strings.Contains(err.Error(), idleErrMsgSubagentStalled) {
			t.Fatalf("expected stalled error, got: %v", err)
		}
	case <-time.After(overallBudget):
		t.Fatalf("watchdog did not bound a stalled subagent within %s", overallBudget)
	}
}

// TestIdleWatchdog_RespectingSubagent_PromptlyCancelled verifies that a
// well-behaved subagent that observes context cancellation returns promptly
// once the watchdog cancels its execution context.
func TestIdleWatchdog_RespectingSubagent_PromptlyCancelled(t *testing.T) {
	const (
		timeout     = 40 * time.Millisecond
		probeTick   = 5 * time.Millisecond
		overallBudget = 200 * time.Millisecond
	)

	w := NewIdleWatchdog(timeout, probeTick)
	ctx := context.Background()

	warnErr := errors.New("context canceled")
	done := make(chan error, 1)
	goroutinelabels.NewGoroutine("test.idle_watchdog_polite", "test worker for polite subagent").StartSimple(func() {
		done <- w.Execute(ctx, "subagent-polite", func(tctx context.Context) error {
			// Simulate long work in small increments that honour cancellation.
			for i := 0; i < 40; i++ {
				select {
				case <-tctx.Done():
					return tctx.Err()
				default:
				}
				time.Sleep(5 * time.Millisecond)
			}
			return warnErr
		})
	})

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected non-nil error from cancelled subagent")
		}
	case <-time.After(overallBudget):
		t.Fatalf("polite subagent did not surface within %s", overallBudget)
	}
}

// TestIdleWatchdog_HealthySubagent_Completes verifies the false-positive
// guard: a subagent that completes inside its budget is not touched by the
// watchdog, and its return value (including nil) is propagated to the caller.
func TestIdleWatchdog_HealthySubagent_Completes(t *testing.T) {
	w := NewIdleWatchdog(250 * time.Millisecond, 20 * time.Millisecond)
	ctx := context.Background()

	beatIn := 0
	err := w.Execute(ctx, "subagent-healthy", func(_ context.Context) error {
		beatIn++
		return nil
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if beatIn != 1 {
		t.Fatalf("expected task to run exactly once, got %d", beatIn)
	}
}

// TestIdleWatchdog_DeadlineHonoured verifies that a pre-existing shorter
// parent deadline is not overridden by the watchdog config.
func TestIdleWatchdog_DeadlineHonoured(t *testing.T) {
	const (
		parentTimeout = 20 * time.Millisecond
		watchTimeout  = 5 * time.Second
		watchTick     = 5 * time.Millisecond
	)

	w := NewIdleWatchdog(watchTimeout, watchTick)
	ctx, cancel := context.WithTimeout(context.Background(), parentTimeout)
	defer cancel()

	blockDelay := 400 * time.Millisecond
	start := time.Now()
	err := w.Execute(ctx, "subagent-deadline", func(_ context.Context) error {
		<-time.After(blockDelay)
		return errors.New("late")
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error from deadline, got nil")
	}
	// The watchdog must respect the tighter parent deadline; do not
	// allow the whole budget to be consumed by an uncancelled blocked task.
	if elapsed > 150 * time.Millisecond {
		t.Fatalf("expected parent deadline to win, took %s", elapsed)
	}
}

// TestNewIdleWatchdog_Defaults pins the zero-value behaviour: a zero or
// negative timeout/tick falls back to safe defaults so the watchdog can
// never disable itself via a misconfig'd zero time.
func TestNewIdleWatchdog_Defaults(t *testing.T) {
	w := NewIdleWatchdog(0, 0)
	if w.IdleTimeout <= 0 {
		t.Fatalf("expected default idle timeout to be positive, got %s", w.IdleTimeout)
	}
	if w.ProbeTick <= 0 {
		t.Fatalf("expected default probe tick to be positive, got %s", w.ProbeTick)
	}
	if w.IdleTimeout < w.ProbeTick {
		t.Fatalf("idle timeout %s must be >= probe tick %s", w.IdleTimeout, w.ProbeTick)
	}
}

// TestRunSubagentTask_IdleWatchdog_TriggeredAndRolledBack verifies the
// end-to-end integration: with the watchdog enabled, a stalled subagent is
// cancelled, retried (per MaxRetries), and finally produces exactly one CAP
// alert containing a stall violation.
func TestRunSubagentTask_IdleWatchdog_TriggeredAndRolledBack(t *testing.T) {
	cfg := SentinelConfig{
		MaxRetries:          1,
		IdleWatchdogEnabled: true,
		IdleWatchdogTimeout: 40 * time.Millisecond,
		IdleWatchdogTick:    5 * time.Millisecond,
	}
	mockCap := &MockCAPLoop{}
	runner := NewSubagentRunner(cfg, "/tmp/mock", nil, mockCap)

	taskCalls := 0
	task := func(ctx context.Context) error {
		taskCalls++
		// Blocked on a channel that will never complete; the watchdog must
		// cancel the context so we do not hang.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(120 * time.Millisecond):
			return errors.New("late completion")
		}
	}

	getStates := func() ([]rollback.ObjectState, error) {
		return nil, nil
	}

	start := time.Now()
	err := runner.RunSubagentTask(context.Background(), "subagent-1", task, getStates)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if taskCalls != 2 {
		t.Fatalf("expected task to be called 2 times (initial + 1 retry), got %d", taskCalls)
	}
	if len(mockCap.Alerts) != 1 {
		t.Fatalf("expected exactly 1 CAP alert, got %d", len(mockCap.Alerts))
	}
	if len(mockCap.Alerts[0].Violations) == 0 {
		t.Fatalf("expected stall violation in CAP alert")
	}
	if elapsed > 800*time.Millisecond {
		t.Fatalf("watchdog did not bound the stalled subagent promptly, elapsed %s", elapsed)
	}
}

// TestRunSubagentTask_IdleWatchdog_Disabled_NoEffect confirms that when the
// watchdog flag is off, the task runs unbounded and no watchdog violation is
// appended.
func TestRunSubagentTask_IdleWatchdog_Disabled_NoEffect(t *testing.T) {
	cfg := SentinelConfig{
		MaxRetries:        0,
		IdleWatchdogEnabled: false,
	}
	mockCap := &MockCAPLoop{}
	runner := NewSubagentRunner(cfg, "/tmp/mock", nil, mockCap)

	task := func(_ context.Context) error {
		time.Sleep(20 * time.Millisecond)
		return nil
	}
	getStates := func() ([]rollback.ObjectState, error) { return nil, nil }

	if err := runner.RunSubagentTask(context.Background(), "subagent-1", task, getStates); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if len(mockCap.Alerts) != 0 {
		t.Fatalf("expected 0 alerts, got %d", len(mockCap.Alerts))
	}
}
