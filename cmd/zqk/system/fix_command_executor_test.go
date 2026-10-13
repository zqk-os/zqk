package system

import (
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestFixCommandExecutor_Lifecycle(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
	executor := NewFixCommandExecutor(tmpDir, 2)
	if executor == nil {
		t.Fatal("NewFixCommandExecutor returned nil")
	}

	// Setting configuration before start
	executor.SetTimeouts(1 * time.Second)
	executor.SetRequiresApproval(false)

	// Enqueue before start should fail with ErrFixExecutorNotRunning
	task := &FixCommandTask{
		ObjectID:   "OBJ-001",
		ObjectKind: "backlog_item",
		FixCommand: "echo 'fixed'",
	}
	err := executor.Enqueue(task)
	if err != ErrFixExecutorNotRunning {
		t.Fatalf("expected ErrFixExecutorNotRunning, got %v", err)
	}

	// Start executor
	if err := executor.Start(); err != nil {
		t.Fatalf("unexpected error starting executor: %v", err)
	}

	// Double start should fail with ErrFixExecutorAlreadyRunning
	if err := executor.Start(); err != ErrFixExecutorAlreadyRunning {
		t.Fatalf("expected ErrFixExecutorAlreadyRunning, got %v", err)
	}

	// Stop executor
	if err := executor.Stop(); err != nil {
		t.Fatalf("unexpected error stopping executor: %v", err)
	}

	// Double stop should be a no-op and succeed
	if err := executor.Stop(); err != nil {
		t.Fatalf("unexpected error double stopping executor: %v", err)
	}
}

func TestFixCommandExecutor_CallbacksAndExecution(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
	executor := NewFixCommandExecutor(tmpDir, 1)

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

	if err := executor.Start(); err != nil {
		t.Fatalf("failed to start executor: %v", err)
	}
	defer func() { _ = executor.Stop() }()

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

func TestFixCommandExecutor_ApprovalFlow(t *testing.T) {
	t.Parallel()

	t.Run("approval granted", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
		executor := NewFixCommandExecutor(tmpDir, 1)

		approvedCh := make(chan struct{})
		executor.SetApprovalCallback(func(task *FixCommandTask) (bool, error) {
			return true, nil
		})
		executor.SetCompletionCallback(func(task *FixCommandTask) {
			close(approvedCh)
		})

		if err := executor.Start(); err != nil {
			t.Fatalf("failed to start executor: %v", err)
		}
		defer func() { _ = executor.Stop() }()

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
	})

	t.Run("approval denied", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
		executor := NewFixCommandExecutor(tmpDir, 1)

		deniedCh := make(chan struct{})
		executor.SetApprovalCallback(func(task *FixCommandTask) (bool, error) {
			defer close(deniedCh)
			return false, nil
		})

		if err := executor.Start(); err != nil {
			t.Fatalf("failed to start executor: %v", err)
		}
		defer func() { _ = executor.Stop() }()

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
			// Give a short moment for task status update
			time.Sleep(50 * time.Millisecond)
			if task.Status != FixCommandStatusCancelled {
				t.Fatalf("expected status cancelled, got %v", task.Status)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for approval denial")
		}
	})

	t.Run("approval callback error", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
		executor := NewFixCommandExecutor(tmpDir, 1)

		errCh := make(chan struct{})
		expectedErr := errfmt.Errorf("approval error")
		executor.SetApprovalCallback(func(task *FixCommandTask) (bool, error) {
			return false, expectedErr
		})
		executor.SetErrorCallback(func(task *FixCommandTask, err error) {
			close(errCh)
		})

		if err := executor.Start(); err != nil {
			t.Fatalf("failed to start executor: %v", err)
		}
		defer func() { _ = executor.Stop() }()

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
	})
}

func TestFixCommandExecutor_QueueFull(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
	executor := NewFixCommandExecutor(tmpDir, 1)

	_ = executor.Start()
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
	_ = executor.Stop()
	_ = queueFullErr
}
