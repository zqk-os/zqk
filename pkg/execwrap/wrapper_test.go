package execwrap

import (
	"context"
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
