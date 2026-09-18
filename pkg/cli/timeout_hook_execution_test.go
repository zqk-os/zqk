package cli

import (
	"context"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/process"
)

func TestExecuteCommandWithTimeout_Success(t *testing.T) {
	hook := NewTimeoutHook()
	hook.SetEnabled(true)

	ctx := pkgctx.NewSystemContext()
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	timedOut, exitCode, err := hook.executeCommandWithTimeout(
		timeoutCtx,
		5*time.Second,
		"test_command",
		"test command",
		func() error {
			return nil
		},
	)

	if timedOut {
		t.Error("Expected command to complete, not timeout")
	}
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestExecuteCommandWithTimeout_Error(t *testing.T) {
	hook := NewTimeoutHook()
	hook.SetEnabled(true)

	ctx := pkgctx.NewSystemContext()
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	expectedErr := errors.New("command failed")
	timedOut, exitCode, err := hook.executeCommandWithTimeout(
		timeoutCtx,
		5*time.Second,
		"test_command",
		"test command",
		func() error {
			return expectedErr
		},
	)

	if timedOut {
		t.Error("Expected command to complete with error, not timeout")
	}
	if exitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitCode)
	}
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestExecuteCommandWithTimeout_Timeout(t *testing.T) {
	hook := NewTimeoutHook()
	hook.SetEnabled(true)

	ctx := pkgctx.NewSystemContext()
	timeoutCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	timedOut, exitCode, err := hook.executeCommandWithTimeout(
		timeoutCtx,
		100*time.Millisecond,
		"test_command",
		"test command",
		func() error {
			// Sleep longer than timeout
			time.Sleep(500 * time.Millisecond)
			return nil
		},
	)
	duration := time.Since(start)

	if !timedOut {
		t.Error("Expected command to timeout")
	}
	if exitCode != 124 {
		t.Errorf("Expected exit code 124 (timeout), got %d", exitCode)
	}
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
	if duration < 100*time.Millisecond || duration > 300*time.Millisecond {
		t.Errorf("Expected timeout around 100ms, got %v", duration)
	}
}

func TestExecuteCommandWithTimeout_ContextAlreadyCancelled(t *testing.T) {
	hook := NewTimeoutHook()
	hook.SetEnabled(true)

	ctx := pkgctx.NewSystemContext()
	timeoutCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately

	timedOut, exitCode, err := hook.executeCommandWithTimeout(
		timeoutCtx,
		5*time.Second,
		"test_command",
		"test command",
		func() error {
			t.Error("Command function should not be called when context is already cancelled")
			return nil
		},
	)

	if timedOut {
		t.Error("Expected immediate return, not timeout")
	}
	if exitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitCode)
	}
	if err == nil {
		t.Error("Expected error for cancelled context, got nil")
	}
	if err != nil && err.Error() == emptyValue {
		t.Error("Expected error message, got empty string")
	}
}

func TestExecuteCommandWithTimeout_PanicRecovery(t *testing.T) {
	hook := NewTimeoutHook()
	hook.SetEnabled(true)

	ctx := pkgctx.NewSystemContext()
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	panicMsg := "test panic"
	timedOut, exitCode, err := hook.executeCommandWithTimeout(
		timeoutCtx,
		5*time.Second,
		"test_command",
		"test command",
		func() error {
			panic(panicMsg)
		},
	)

	if timedOut {
		t.Error("Expected command to complete with panic, not timeout")
	}
	if exitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitCode)
	}
	if err == nil {
		t.Error("Expected panic error, got nil")
	}
	if err != nil && err.Error() == emptyValue {
		t.Error("Expected panic error message, got empty string")
	}
	// Implementation includes stack trace; require label and panic message in error
	if err != nil && (!strings.Contains(err.Error(), "panic in command executor:") || !strings.Contains(err.Error(), panicMsg)) {
		t.Errorf("Expected panic error to contain %q and %q, got %v", "panic in command executor:", panicMsg, err)
	}
}

func TestExecuteCommandWithTimeout_GoroutineAlwaysSendsToErrChan(t *testing.T) {
	// This test ensures the goroutine always sends to errChan, preventing hangs.
	// The bug we fixed: if WithContext was passed and context was cancelled,
	// the goroutine would exit early without sending to errChan.
	hook := NewTimeoutHook()
	hook.SetEnabled(true)

	ctx := pkgctx.NewSystemContext()
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Use a channel to track if the function was called
	funcCalled := make(chan bool, 1)
	done := make(chan bool, 1)

	goroutinelabels.NewGoroutine("cli_test", "execute command with timeout").StartSimple(func() {
		timedOut, exitCode, err := hook.executeCommandWithTimeout(
			timeoutCtx,
			5*time.Second,
			"test_command",
			"test command",
			func() error {
				funcCalled <- true
				return nil
			},
		)
		// Verify we got a result (not hanging)
		if timedOut || exitCode != 0 || err != nil {
			t.Errorf("Unexpected result: timedOut=%v, exitCode=%d, err=%v", timedOut, exitCode, err)
		}
		done <- true
	})

	// Wait for function to be called
	select {
	case <-funcCalled:
		// Good, function was called
	case <-time.After(1 * time.Second):
		t.Error("Command function was not called within 1 second")
	}

	// Wait for execution to complete
	select {
	case <-done:
		// Good, execution completed
	case <-time.After(2 * time.Second):
		t.Error("executeCommandWithTimeout did not complete within 2 seconds (possible hang)")
	}
}

func TestExecuteCommandWithTimeout_NoTimeoutWhenTimeoutIsZero(t *testing.T) {
	hook := NewTimeoutHook()
	hook.SetEnabled(true)

	ctx := pkgctx.NewSystemContext()
	// Use context without timeout (timeout is 0)
	timeoutCtx := ctx

	start := time.Now()
	timedOut, exitCode, err := hook.executeCommandWithTimeout(
		timeoutCtx,
		0, // No timeout
		"test_command",
		"test command",
		func() error {
			// Sleep for a bit to ensure we're not timing out
			time.Sleep(200 * time.Millisecond)
			return nil
		},
	)
	duration := time.Since(start)

	if timedOut {
		t.Error("Expected command to complete, not timeout (timeout is 0)")
	}
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if duration < 200*time.Millisecond {
		t.Errorf("Expected command to run for at least 200ms, got %v", duration)
	}
}

// TestExecuteCommandWithTimeout_SignalInterrupt tests signal handling.
// Note: This test may be flaky in CI environments, so we'll skip it if signals aren't available.
func TestExecuteCommandWithTimeout_SignalInterrupt(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping signal test in short mode")
	}

	hook := NewTimeoutHook()
	hook.SetEnabled(true)

	ctx := pkgctx.NewSystemContext()
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	done := make(chan bool, 1)
	var resultErr error
	var resultExitCode int

	goroutinelabels.NewGoroutine("cli_test", "execute command with signal interrupt").StartSimple(func() {
		_, resultExitCode, resultErr = hook.executeCommandWithTimeout(
			timeoutCtx,
			5*time.Second,
			"test_command",
			"test command",
			func() error {
				// Sleep long enough for signal to be sent
				time.Sleep(500 * time.Millisecond)
				return nil
			},
		)
		done <- true
	})

	// Give goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Send interrupt signal to current process
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Skipf("Cannot find process for signal test: %v", err)
	}

	// Send SIGINT (interrupt signal)
	if err := proc.Signal(syscall.SIGINT); err != nil {
		t.Skipf("Cannot send signal for test: %v", err)
	}

	// Wait for execution to complete
	select {
	case <-done:
		if resultExitCode != 130 {
			t.Errorf("Expected exit code 130 (interrupt), got %d", resultExitCode)
		}
		if resultErr == nil {
			t.Error("Expected interrupt error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Error("executeCommandWithTimeout did not complete within 2 seconds after signal")
	}
}

func TestExecuteCommandWithTimeout_DisconnectedMonitor(t *testing.T) {
	hook := NewTimeoutHook()
	hook.SetEnabled(true)

	reconnect := process.DisconnectTimeoutMonitor()
	defer reconnect()

	ctx := pkgctx.NewSystemContext()
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()

	timedOut, exitCode, err := hook.executeCommandWithTimeout(
		timeoutCtx,
		30*time.Millisecond,
		"test_dashboard",
		"test dashboard --pager",
		func() error {
			// Sleep longer than timeoutCtx to simulate user reading pager past timeout
			time.Sleep(100 * time.Millisecond)
			return nil
		},
	)

	if timedOut {
		t.Error("Expected command NOT to time out when timeout monitor is disconnected")
	}
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}
}
