package pipeline

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestProvisionObjectBranchStage(t *testing.T) {
	// A basic test to make sure the stage compiles and can be invoked.
	// Since it runs git worktree add, we cannot fully test it without a git repo unless we mock exec.
	// This test just ensures we get an error on invalid payload.

	opts := ProvisionObjectBranchOptions{
		ProjectRoot: "/tmp",
	}

	stage := ProvisionObjectBranchStage(opts)
	ctx := &Context{Ctx: context.Background()}

	_, err := stage(ctx, nil)
	if err == nil {
		t.Errorf("expected error on nil payload")
	}
}

func TestProvisionObjectBranchStage_Locking(t *testing.T) {
	tmpDir := t.TempDir()

	// Init a dummy git repo
	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	if err := initCmd.Run(); err != nil {
		t.Fatalf("failed to init dummy git repo: %v", err)
	}

	// Create a dummy commit so show-ref and worktree have something to anchor to
	configCmd1 := exec.Command("git", "config", "user.name", "test")
	configCmd1.Dir = tmpDir
	_ = configCmd1.Run()
	configCmd2 := exec.Command("git", "config", "user.email", "test@example.com")
	configCmd2.Dir = tmpDir
	_ = configCmd2.Run()

	// Ensure base branch is main
	checkoutCmd := exec.Command("git", "checkout", "-b", "main")
	checkoutCmd.Dir = tmpDir
	_ = checkoutCmd.Run()

	commitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "initial commit")
	commitCmd.Dir = tmpDir
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("failed to create dummy commit: %v", err)
	}

	lockedIDs := make(map[string]bool)
	var lockMu sync.Mutex

	tryLockMock := func(ctx context.Context, id string) (func() error, bool, error) {
		lockMu.Lock()
		defer lockMu.Unlock()
		if lockedIDs[id] {
			return nil, false, nil
		}
		lockedIDs[id] = true
		release := func() error {
			lockMu.Lock()
			defer lockMu.Unlock()
			delete(lockedIDs, id)
			return nil
		}
		return release, true, nil
	}

	opts := ProvisionObjectBranchOptions{
		ProjectRoot: tmpDir,
		ObjectReader: func(ctx context.Context, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyID:   id,
				objects.FieldKeyKind: "task",
			}, nil
		},
		TryLock: tryLockMock,
	}

	stage := ProvisionObjectBranchStage(opts)
	releaseStage := ReleaseObjectBranchStage()

	ctx1 := &Context{Ctx: context.Background()}
	payload := "TSK-locktest"

	// 1. First execution should succeed (lock acquired)
	_, err := stage(ctx1, payload)
	if err != nil {
		t.Fatalf("expected first stage run to succeed, got: %v", err)
	}

	// 2. Second execution on same payload with different context should fail (lock held)
	ctx2 := &Context{Ctx: context.Background()}
	_, err = stage(ctx2, payload)
	if err == nil {
		t.Fatal("expected second stage run to fail due to lock contention, but it succeeded")
	}
	if !strings.Contains(err.Error(), "already held by another process") {
		t.Errorf("expected lock held error, got: %v", err)
	}

	// 3. Release lock from first context
	_, err = releaseStage(ctx1, payload)
	if err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}

	// 4. Third execution should now succeed (lock released)
	ctx3 := &Context{Ctx: context.Background()}
	_, err = stage(ctx3, payload)
	if err != nil {
		t.Fatalf("expected third stage run to succeed after release, got: %v", err)
	}

	// Clean up third lock
	_, _ = releaseStage(ctx3, payload)
}
