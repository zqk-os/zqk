package agent_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/cmd/zqk/agent"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAgentNext_FailClosedOnWorktreeExists(t *testing.T) {
	root, store := setupOrchestrateTest(t)

	// Create a dummy ATK task
	taskID := "ATK-TEST-WORKTREE"
	task := map[string]any{
		objects.FieldKeyID:     taskID,
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		objects.FieldKeyTitle:  "Test task with un-removed worktree",
	}

	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	secCtx := pkgctx.NewSystemSecurityContext()
	err := store.Create(ctx, secCtx, task)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Create a dummy worktree directory
	worktreeDir := paths.AgentWorktreeDir(root, taskID)
	err = fileutil.MkdirAll(worktreeDir, paths.DirPerm755)
	if err != nil {
		t.Fatalf("failed to create worktree dir: %v", err)
	}

	cmd := agent.NewNextCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))
	cmd.SetArgs([]string{taskID})

	err = cmd.Execute()
	if err == nil {
		t.Fatalf("expected next command to fail when worktree exists, but it succeeded")
	}

	if !strings.Contains(err.Error(), "still exists") {
		t.Errorf("expected error about worktree existing, got: %v", err)
	}
}

func TestAgentNext_FailClosedOnNotMerged(t *testing.T) {
	root, store := setupOrchestrateTest(t)

	// Init a git repo in root
	initCmd := testkit.ManagedCommand(t, t.Context(), "git", "init")
	initCmd.Dir = root
	if err := initCmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Create an initial commit
	commitCmd := testkit.ManagedCommand(t, t.Context(), "git", "commit", "--allow-empty", "-m", "init")
	commitCmd.Dir = root
	commitCmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Create a dummy branch
	branchCmd := testkit.ManagedCommand(t, t.Context(), "git", "branch", "integration/pri-123")
	branchCmd.Dir = root
	if err := branchCmd.Run(); err != nil {
		t.Fatalf("git branch failed: %v", err)
	}

	// Get a commit hash that is NOT in integration/pri-123
	// We make a new commit on main
	commitCmd2 := testkit.ManagedCommand(t, t.Context(), "git", "commit", "--allow-empty", "-m", "unmerged")
	commitCmd2.Dir = root
	commitCmd2.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	if err := commitCmd2.Run(); err != nil {
		t.Fatalf("git commit 2 failed: %v", err)
	}

	revCmd := testkit.ManagedCommand(t, t.Context(), "git", "rev-parse", "HEAD")
	revCmd.Dir = root
	out, err := revCmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	hash := strings.TrimSpace(string(out))

	// Create a dummy ATK task with priority_plan_ref and commit_hashes
	taskID := "ATK-TEST-NOT-MERGED"
	task := map[string]any{
		objects.FieldKeyID:              taskID,
		objects.FieldKeyKind:            objects.KindAgentTask,
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyTitle:           "Test task not merged",
		objects.FieldKeyPriorityPlanRef: "pri-123",
		objects.FieldKeyCommitHashes:    []any{hash},
	}

	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	secCtx := pkgctx.NewSystemSecurityContext()
	err = store.Create(ctx, secCtx, task)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	cmd := agent.NewNextCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))
	cmd.SetArgs([]string{taskID})

	err = cmd.Execute()
	if err == nil {
		t.Fatalf("expected next command to fail when commit is not merged, but it succeeded")
	}

	if !strings.Contains(err.Error(), "is not merged into") {
		t.Errorf("expected error about not merged, got: %v", err)
	}
}

func TestAgentNext_FailClosedOnMissingPriorityPlanRef(t *testing.T) {
	root, store := setupOrchestrateTest(t)

	taskID := "ATK-TEST-NO-PRI-REF"
	task := map[string]any{
		objects.FieldKeyID:     taskID,
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		objects.FieldKeyTitle:  "Test task without priority_plan_ref",
	}

	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	secCtx := pkgctx.NewSystemSecurityContext()
	err := store.Create(ctx, secCtx, task)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	cmd := agent.NewNextCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))
	cmd.SetArgs([]string{taskID})

	err = cmd.Execute()
	if err == nil {
		t.Fatalf("expected next command to fail when priority_plan_ref is missing, but it succeeded")
	}

	if !strings.Contains(err.Error(), "priority_plan_ref is missing") {
		t.Errorf("expected error about missing priority_plan_ref, got: %v", err)
	}
}

func TestAgentNext_FailClosedOnNotMerged_Lowercase(t *testing.T) {
	root, store := setupOrchestrateTest(t)

	// Init a git repo in root
	initCmd := testkit.ManagedCommand(t, t.Context(), "git", "init")
	initCmd.Dir = root
	if err := initCmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	commitCmd := testkit.ManagedCommand(t, t.Context(), "git", "commit", "--allow-empty", "-m", "init")
	commitCmd.Dir = root
	commitCmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// create unmerged commit
	commitCmd2 := testkit.ManagedCommand(t, t.Context(), "git", "commit", "--allow-empty", "-m", "unmerged")
	commitCmd2.Dir = root
	commitCmd2.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	if err := commitCmd2.Run(); err != nil {
		t.Fatalf("git commit 2 failed: %v", err)
	}

	revCmd := testkit.ManagedCommand(t, t.Context(), "git", "rev-parse", "HEAD")
	revCmd.Dir = root
	out, err := revCmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	hash := strings.TrimSpace(string(out))

	taskID := "ATK-TEST-NOT-MERGED-LOWER"
	task := map[string]any{
		objects.FieldKeyID:              taskID,
		objects.FieldKeyKind:            objects.KindAgentTask,
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyTitle:           "Test task not merged lowercase check",
		objects.FieldKeyPriorityPlanRef: "PRI-ABC",
		objects.FieldKeyCommitHashes:    []any{hash},
	}

	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	secCtx := pkgctx.NewSystemSecurityContext()
	err = store.Create(ctx, secCtx, task)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	cmd := agent.NewNextCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))
	cmd.SetArgs([]string{taskID})

	err = cmd.Execute()
	if err == nil {
		t.Fatalf("expected next command to fail when commit is not merged, but it succeeded")
	}

	if !strings.Contains(err.Error(), "integration/pri-abc") {
		t.Errorf("expected error to check for integration/pri-abc, got: %v", err)
	}
}
