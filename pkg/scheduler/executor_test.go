package scheduler

import (
	"bytes"
	"context"
	"errors"
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
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}
