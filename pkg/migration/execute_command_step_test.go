package migration

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

func setupExecuteCommandTest(t *testing.T) (*Executor, func()) {
	t.Helper()
	tmpDir := t.TempDir()

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	_ = os.MkdirAll(processDir, paths.DirPerm755)
	specsDir := filepath.Join(processDir, "_internal", "object_specs")
	_ = os.MkdirAll(specsDir, paths.DirPerm755)

	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	logger := logging.GetLoggerFromProfile("test")
	executor := NewExecutor(storageProvider, tmpDir, logger)
	return executor, func() {
		_ = storageProvider.Shutdown(context.Background())
	}
}

func TestExecuteCommandStep_Success(t *testing.T) {
	t.Parallel()
	executor, cleanup := setupExecuteCommandTest(t)
	defer cleanup()

	ctx := context.Background()
	step := &Step{
		ID:   "run-echo",
		Type: "execute_command",
		Config: map[string]any{
			objects.FieldKeyCommand: "echo",
			"args":                  []any{"hello"},
		},
	}

	result := executor.executeCommandStep(ctx, step, nil)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Output["stdout"] != "hello\n" && result.Output["stdout"] != "hello\r\n" {
		t.Errorf("expected stdout 'hello\\n', got %q", result.Output["stdout"])
	}
	if result.Output["exit_code"] != 0 {
		t.Errorf("expected exit_code 0, got %v", result.Output["exit_code"])
	}
}

func TestExecuteCommandStep_RequiresCommand(t *testing.T) {
	t.Parallel()
	executor, cleanup := setupExecuteCommandTest(t)
	defer cleanup()

	ctx := context.Background()
	step := &Step{
		ID:     "no-command",
		Type:   "execute_command",
		Config: map[string]any{},
	}

	result := executor.executeCommandStep(ctx, step, nil)

	if result.Success {
		t.Fatal("expected failure when command is missing")
	}
	if result.Error == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestExecuteCommandStep_NonZeroExit(t *testing.T) {
	t.Parallel()
	executor, cleanup := setupExecuteCommandTest(t)
	defer cleanup()

	ctx := context.Background()
	step := &Step{
		ID:   "fail",
		Type: "execute_command",
		Config: map[string]any{
			objects.FieldKeyCommand: "false",
		},
	}

	result := executor.executeCommandStep(ctx, step, nil)

	if result.Success {
		t.Fatal("expected failure when command exits non-zero")
	}
	if result.Output["exit_code"] != 1 {
		t.Errorf("expected exit_code 1, got %v", result.Output["exit_code"])
	}
}
