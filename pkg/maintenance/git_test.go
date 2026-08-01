package maintenance

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestPruneQuarantinedRefs(t *testing.T) {
	tmpDir := t.TempDir()

	// Init dummy git repo
	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	if err := initCmd.Run(); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}

	configCmd1 := exec.Command("git", "config", "user.name", "test")
	configCmd1.Dir = tmpDir
	_ = configCmd1.Run()
	configCmd2 := exec.Command("git", "config", "user.email", "test@example.com")
	configCmd2.Dir = tmpDir
	_ = configCmd2.Run()

	checkoutCmd := exec.Command("git", "checkout", "-b", "main")
	checkoutCmd.Dir = tmpDir
	_ = checkoutCmd.Run()

	commitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "initial commit")
	commitCmd.Dir = tmpDir
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Create a branch to quarantine
	branchName := "test-quarantine"
	branchCmd := exec.Command("git", "branch", branchName)
	branchCmd.Dir = tmpDir
	if err := branchCmd.Run(); err != nil {
		t.Fatalf("failed to create branch: %v", err)
	}

	// Soft-delete to refs/archive/test-quarantine
	archiveRef := "refs/archive/" + branchName
	updateCmd := exec.Command("git", "update-ref", archiveRef, branchName)
	updateCmd.Dir = tmpDir
	if err := updateCmd.Run(); err != nil {
		t.Fatalf("failed to update-ref: %v", err)
	}

	// Delete local branch
	delCmd := exec.Command("git", "branch", "-D", branchName)
	delCmd.Dir = tmpDir
	_ = delCmd.Run()

	svc := NewGitMaintenanceService(tmpDir)
	ctx := context.Background()

	// 1. Prune with 1 hour threshold (should prune since the commit we created is brand new but within 1 hour threshold)
	count, err := svc.PruneQuarantinedRefs(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 pruned refs, got: %d", count)
	}

	// Verify the ref still exists
	showCmd := exec.Command("git", "show-ref", "--verify", "--quiet", archiveRef)
	showCmd.Dir = tmpDir
	if err := showCmd.Run(); err != nil {
		t.Errorf("expected archived ref to still exist")
	}

	// 2. Prune with negative duration or 0 threshold (should prune since 0s is >= age of commit)
	count, err = svc.PruneQuarantinedRefs(ctx, -1*time.Second)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 pruned ref, got: %d", count)
	}

	// Verify the ref was deleted
	showCmd = exec.Command("git", "show-ref", "--verify", "--quiet", archiveRef)
	showCmd.Dir = tmpDir
	if err := showCmd.Run(); err == nil {
		t.Errorf("expected archived ref to be deleted")
	}
}
