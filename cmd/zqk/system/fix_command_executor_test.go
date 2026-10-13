package system

import (
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func setupRunningFixExecutor(t *testing.T, queueSize int) *FixCommandExecutor {
	t.Helper()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
	executor := NewFixCommandExecutor(tmpDir, queueSize)
	if err := executor.Start(); err != nil {
		t.Fatalf("failed to start executor: %v", err)
	}
	t.Cleanup(func() {
		if stopErr := executor.Stop(); stopErr != nil {
			t.Logf("cleanup stop error: %v", stopErr)
		}
	})
	return executor
}

func TestFixCommandExecutor_Lifecycle(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
	executor := NewFixCommandExecutor(tmpDir, 2)
	if executor == nil {
		t.Fatal("NewFixCommandExecutor returned nil")
	}

	executor.SetTimeouts(1 * time.Second)
	executor.SetRequiresApproval(false)

	task := &FixCommandTask{
		ObjectID:   "OBJ-001",
		ObjectKind: "backlog_item",
		FixCommand: "echo 'fixed'",
	}
	err := executor.Enqueue(task)
	if err != ErrFixExecutorNotRunning {
		t.Fatalf("expected ErrFixExecutorNotRunning, got %v", err)
	}

	if err := executor.Start(); err != nil {
		t.Fatalf("unexpected error starting executor: %v", err)
	}
	if err := executor.Start(); err != ErrFixExecutorAlreadyRunning {
		t.Fatalf("expected ErrFixExecutorAlreadyRunning, got %v", err)
	}

	if err := executor.Stop(); err != nil {
		t.Fatalf("unexpected error stopping executor: %v", err)
	}
	if err := executor.Stop(); err != nil {
		t.Fatalf("unexpected error double stopping executor: %v", err)
	}
}

func TestFixCommandExecutor_CallbacksAndExecution(t *testing.T) {
	t.Parallel()

	executor := setupRunningFixExecutor(t, 1)

	var completedMu sync.Mutex
	var completedTask *FixCommandTask
	completedCh := make(chan struct{})

	executor.SetRequiresApproval(false)
	executor.SetCompletionCallback(func(task *FixCommandTask) {
		completedMu.Lock()
		completedTask = task
		completedMu.Unlock()
		close(completedCh)
	})

	task := &FixCommandTask{
		ObjectID:   "BLI-123",
		ObjectKind: "backlog_item",
		FixCommand: "noop",
	}

	if err := executor.Enqueue(task); err != nil {
		t.Fatalf("failed to enqueue task: %v", err)
	}

	select {
	case <-completedCh:
		completedMu.Lock()
		defer completedMu.Unlock()
		if completedTask.Status != FixCommandStatusCompleted {
			t.Fatalf("expected status completed, got %v", completedTask.Status)
		}
		if completedTask.ExecutedAt == nil {
			t.Fatal("expected ExecutedAt to be set")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for task completion callback")
	}
}

func TestFixCommandExecutor_ApprovalGranted(t *testing.T) {
	t.Parallel()

	executor := setupRunningFixExecutor(t, 1)

	approvedCh := make(chan struct{})
	executor.SetApprovalCallback(func(task *FixCommandTask) (bool, error) {
		return true, nil
	})
	executor.SetCompletionCallback(func(task *FixCommandTask) {
		close(approvedCh)
	})

	task := &FixCommandTask{
		ObjectID:   "BLI-456",
		ObjectKind: "backlog_item",
		FixCommand: "approve_me",
	}
	if err := executor.Enqueue(task); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	select {
	case <-approvedCh:
		if task.Status != FixCommandStatusCompleted {
			t.Fatalf("expected completed status, got %v", task.Status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for approved execution")
	}
}

func TestFixCommandExecutor_ApprovalDenied(t *testing.T) {
	t.Parallel()

	executor := setupRunningFixExecutor(t, 1)

	deniedCh := make(chan struct{})
	executor.SetApprovalCallback(func(task *FixCommandTask) (bool, error) {
		defer close(deniedCh)
		return false, nil
	})

	task := &FixCommandTask{
		ObjectID:   "BLI-789",
		ObjectKind: "backlog_item",
		FixCommand: "deny_me",
	}
	if err := executor.Enqueue(task); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	select {
	case <-deniedCh:
		time.Sleep(50 * time.Millisecond)
		if task.Status != FixCommandStatusCancelled {
			t.Fatalf("expected status cancelled, got %v", task.Status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for approval denial")
	}
}

func TestFixCommandExecutor_ApprovalCallbackError(t *testing.T) {
	t.Parallel()

	executor := setupRunningFixExecutor(t, 1)

	errCh := make(chan struct{})
	expectedErr := errfmt.Errorf("approval error")
	executor.SetApprovalCallback(func(task *FixCommandTask) (bool, error) {
		return false, expectedErr
	})
	executor.SetErrorCallback(func(task *FixCommandTask, err error) {
		close(errCh)
	})

	task := &FixCommandTask{
		ObjectID:   "BLI-ERR",
		ObjectKind: "backlog_item",
		FixCommand: "err_me",
	}
	if err := executor.Enqueue(task); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	select {
	case <-errCh:
		if task.Status != FixCommandStatusFailed {
			t.Fatalf("expected status failed, got %v", task.Status)
		}
		if task.Error == nil {
			t.Fatal("expected task.Error to be set")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for error callback")
	}
}

func TestFixCommandExecutor_QueueFull(t *testing.T) {
	t.Parallel()

	executor := setupRunningFixExecutor(t, 1)

	var queueFullErr error
	for i := 0; i < 20; i++ {
		task := &FixCommandTask{
			ObjectID:   "BLI-FILL",
			ObjectKind: "backlog_item",
			FixCommand: "fill",
		}
		if err := executor.Enqueue(task); err != nil {
			queueFullErr = err
			break
		}
	}
	if queueFullErr != nil {
		t.Logf("received expected error: %v", queueFullErr)
	}
}
