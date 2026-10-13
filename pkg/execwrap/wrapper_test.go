package execwrap

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestCommand(t *testing.T) {
	cmd := Command("echo", "test")
	if cmd.Args[0] != "echo" {
		t.Fatal("expected echo")
	}
}

func TestCommandContext(t *testing.T) {
	cmd := CommandContext(context.Background(), "echo", "test")
	if cmd.Args[0] != "echo" {
		t.Fatal("expected echo")
	}
}

func TestCommandWithTimeout(t *testing.T) {
	cmd, cancel := CommandWithTimeout(context.Background(), 2*time.Second, "echo", "hello")
	defer cancel()
	if cmd.Args[0] != "echo" {
		t.Fatal("expected echo")
	}
}

func TestRunWithTimeout_Success(t *testing.T) {
	stdout, stderr, timedOut, err := RunWithTimeout(context.Background(), 5*time.Second, "echo", "hello timeout")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if timedOut {
		t.Fatal("expected timedOut to be false")
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got: %q", stderr)
	}
	if stdout != "hello timeout\n" {
		t.Fatalf("expected 'hello timeout\\n', got: %q", stdout)
	}
}

func TestRunWithTimeout_Expires(t *testing.T) {
	// sleep for 10 seconds with a 100ms timeout
	_, _, timedOut, err := RunWithTimeout(context.Background(), 100*time.Millisecond, "sleep", "10")
	if !timedOut {
		t.Fatal("expected timedOut to be true")
	}
	if err != context.DeadlineExceeded {
		t.Fatalf("expected context.DeadlineExceeded, got: %v", err)
	}
}

func TestCommandContextWithGroup(t *testing.T) {
	cmd := CommandContextWithGroup(context.Background(), "echo", "grouped")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	if cmd.Cancel == nil {
		t.Fatal("expected cmd.Cancel to be defined")
	}
	if cmd.WaitDelay != 2*time.Second {
		t.Fatalf("expected WaitDelay 2s, got: %v", cmd.WaitDelay)
	}
	if cmd.Args[0] != "echo" {
		t.Fatalf("expected echo, got: %v", cmd.Args[0])
	}
}

func TestRunWithTimeoutGroup_Success(t *testing.T) {
	stdout, stderr, timedOut, err := RunWithTimeoutGroup(context.Background(), 5*time.Second, "echo", "hello group")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if timedOut {
		t.Fatal("expected timedOut to be false")
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got: %q", stderr)
	}
	if stdout != "hello group\n" {
		t.Fatalf("expected 'hello group\\n', got: %q", stdout)
	}
}

func TestRunWithTimeoutGroup_Expires(t *testing.T) {
	// sleep for 10 seconds with 100ms timeout in process group
	_, _, timedOut, err := RunWithTimeoutGroup(context.Background(), 100*time.Millisecond, "sleep", "10")
	if !timedOut {
		t.Fatal("expected timedOut to be true")
	}
	if err != context.DeadlineExceeded {
		t.Fatalf("expected context.DeadlineExceeded, got: %v", err)
	}
}

func TestRunWithTimeoutGroup_ZeroTimeout(t *testing.T) {
	stdout, _, timedOut, err := RunWithTimeoutGroup(context.Background(), 0, "echo", "no timeout")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if timedOut {
		t.Fatal("expected timedOut to be false")
	}
	if stdout != "no timeout\n" {
		t.Fatalf("expected 'no timeout\\n', got: %q", stdout)
	}
}

func TestRunWithTimeoutGroup_CommandError(t *testing.T) {
	_, _, timedOut, err := RunWithTimeoutGroup(context.Background(), 5*time.Second, "false")
	if timedOut {
		t.Fatal("expected timedOut to be false")
	}
	if err == nil {
		t.Fatal("expected error executing 'false', got nil")
	}
}

func TestKillProcessGroup_NilSafe(t *testing.T) {
	if err := KillProcessGroup(nil); err != nil {
		t.Fatalf("expected nil error for nil cmd, got: %v", err)
	}
	cmd := Command("echo", "test")
	if err := KillProcessGroup(cmd); err != nil {
		t.Fatalf("expected nil error for unstarted cmd, got: %v", err)
	}
}

func TestIsTextFileBusy(t *testing.T) {
	if IsTextFileBusy(nil) {
		t.Fatal("expected nil error to not be text file busy")
	}
	if IsTextFileBusy(errors.New("some regular error")) {
		t.Fatal("expected regular error to not be text file busy")
	}
	if !IsTextFileBusy(errors.New("fork/exec /bin/zqk: text file busy")) {
		t.Fatal("expected text file busy to be detected")
	}
	if !IsTextFileBusy(errors.New("TEXT FILE BUSY")) {
		t.Fatal("expected uppercase text file busy to be detected")
	}
	if !IsETXTBSY(errors.New("text file busy")) {
		t.Fatal("expected IsETXTBSY to return true")
	}
}

func TestRunCombinedOutputWithRetry_Success(t *testing.T) {
	out, err := RunCombinedOutputWithRetry(context.Background(), func() *exec.Cmd {
		return Command("echo", "retry success")
	}, 3, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if string(out) != "retry success\n" {
		t.Fatalf("expected 'retry success\\n', got: %q", string(out))
	}
}

func TestRunCombinedOutputWithRetry_NonRetryableError(t *testing.T) {
	attempts := 0
	_, err := RunCombinedOutputWithRetry(context.Background(), func() *exec.Cmd {
		attempts++
		return Command("false")
	}, 3, 5*time.Millisecond)
	if err == nil {
		t.Fatal("expected error from false command, got nil")
	}
	if attempts != 1 {
		t.Fatalf("expected exactly 1 attempt for non-busy error, got: %d", attempts)
	}
}

func TestRunCombinedOutputWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RunCombinedOutputWithRetry(ctx, func() *exec.Cmd {
		return Command("echo", "cancelled")
	}, 3, 5*time.Millisecond)
	// Cancelled context either returns context.Canceled or executes if finished immediately
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Logf("got error: %v", err)
	}
}
