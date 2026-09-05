package ambient

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
)

func TestAutoMergeDaemon_Initialization(t *testing.T) {
	logger := logging.GetLoggerFromContext(context.Background())
	daemon := NewAutoMergeDaemon(logger.Logger())
	if daemon == nil {
		t.Fatal("Expected NewAutoMergeDaemon to return a valid instance")
	}
}

func TestAutoMergeDaemon_CancelPropagation(t *testing.T) {
	logger := logging.GetLoggerFromContext(context.Background())
	daemon := NewAutoMergeDaemon(logger.Logger())

	// mock execFn to output empty json array
	daemon.execFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo", "[]")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	done := make(chan struct{})
	go func() {
		daemon.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Passed
	case <-time.After(1 * time.Second):
		t.Fatal("Daemon did not shut down on context cancellation")
	}
}

func TestAutoMergeDaemon_Poll(t *testing.T) {
	logger := logging.GetLoggerFromContext(context.Background())
	daemon := NewAutoMergeDaemon(logger.Logger())

	daemon.execFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		if name == ghCmd && len(arg) > 0 && arg[0] == "pr" && arg[1] == "list" {
			return exec.CommandContext(ctx, "echo", `[{"number": 123, "title": "Test PR", "headRefName": "feat/test", "mergeable": "MERGEABLE"}]`)
		}
		return exec.CommandContext(ctx, "echo", "success")
	}

	// poll should not panic or error out
	daemon.poll(context.Background())
}

func TestAutoMergeDaemon_VerifyAndMerge(t *testing.T) {
	logger := logging.GetLoggerFromContext(context.Background())
	daemon := NewAutoMergeDaemon(logger.Logger())

	daemon.execFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo", "success")
	}

	err := daemon.verifyAndMerge(context.Background(), PR{
		Number:      123,
		Title:       "Test PR",
		HeadRefName: "feat/test",
		Mergeable:   "MERGEABLE",
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}
