package scheduler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestNativeExecutor(t *testing.T) {
	executor := &NativeExecutor{}
	ctx := context.Background()

	cmd := executor.CommandContext(ctx, "echo", "hello", "world")
	var stdout bytes.Buffer
	cmd.SetStdout(&stdout)

	err := cmd.Run()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	result := strings.TrimSpace(stdout.String())
	if result != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", result)
	}
}

func TestMockExecutor(t *testing.T) {
	t.Run("default behavior", func(t *testing.T) {
		executor := &MockExecutor{}
		ctx := context.Background()

		cmd := executor.CommandContext(ctx, "somecmd")
		output, err := cmd.Output()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if len(output) > 0 {
			t.Errorf("expected empty output, got %v", string(output))
		}
	})

	t.Run("memory executor custom behavior", func(t *testing.T) {
		expectedErr := errors.New("mock error")
		responses := map[string]*MockCmd{
			"failcmd": {
				OutputBytes: nil,
				Err:         expectedErr,
			},
			"successcmd": {
				OutputBytes: []byte("mocked output"),
				Err:         nil,
			},
		}
		executor := NewMemoryExecutor(responses)
		ctx := context.Background()

		// Test success case
		cmd := executor.CommandContext(ctx, "successcmd")
		var buf bytes.Buffer
		cmd.SetStdout(&buf)
		err := cmd.Start()
		if err != nil {
			t.Errorf("expected no error from start, got %v", err)
		}
		err = cmd.Wait()
		if err != nil {
			t.Errorf("expected no error from wait, got %v", err)
		}
		if buf.String() != "mocked output" {
			t.Errorf("expected 'mocked output', got '%s'", buf.String())
		}

		// Test failure case
		cmdFail := executor.CommandContext(ctx, "failcmd")
		err = cmdFail.Run()
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

func TestInProcessExecutor(t *testing.T) {
	mockFallback := &MockExecutor{
		CommandContextFn: func(ctx context.Context, name string, args ...string) Cmd {
			return &MockCmd{OutputBytes: []byte("fallback: " + name)}
		},
	}
	inproc := NewInProcessExecutor(mockFallback)

	// Register an in-process handler for "scheduler convergence measure"
	inproc.RegisterHandler("scheduler convergence measure", func(ctx context.Context, dir string, env []string, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
		_, _ = stdout.Write([]byte(`{"rollup_status":"success","in_process":true}`))
		return nil
	})

	ctx := context.Background()

	t.Run("Executes registered command in-process without process fork", func(t *testing.T) {
		cmd := inproc.CommandContext(ctx, "bin/zqk", "scheduler", "convergence", "measure", "--session-id", "CVS-123")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !strings.Contains(string(out), `"rollup_status":"success"`) {
			t.Errorf("expected in-process json result, got %s", string(out))
		}
		if cmd.GetPid() != 0 {
			t.Errorf("expected in-process pid 0, got %d", cmd.GetPid())
		}
	})

	t.Run("Unregistered commands fall back to fallback executor", func(t *testing.T) {
		cmd := inproc.CommandContext(ctx, "git", "status")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if string(out) != "fallback: git" {
			t.Errorf("expected fallback output, got %s", string(out))
		}
	})
}

