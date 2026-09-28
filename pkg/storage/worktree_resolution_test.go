package storage_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestWorktreeKernelResolution_Integration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found in PATH")
	}

	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("failed to eval symlinks for tempDir: %v", err)
	}

	// 1. Initialize main git repo
	mainRepo := filepath.Join(tempDir, "main-repo")
	if err := os.MkdirAll(mainRepo, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create main repo dir: %v", err)
	}

	runGit := func(dir string, args ...string) {
		cmd := testkit.ManagedCommand(t, t.Context(), "git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed in %s: %v\nOutput: %s", args, dir, err, string(out))
		}
	}

	runGit(mainRepo, "init", "-b", "main")

	// Write .gitignore ignoring .zqk so worktrees don't inherit a local .zqk
	gitignorePath := filepath.Join(mainRepo, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(".zqk\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	readmePath := filepath.Join(mainRepo, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Main Repo\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	runGit(mainRepo, "add", ".")
	runGit(mainRepo, "commit", "-m", "initial commit")

	// 2. Create secondary git worktree
	worktreePath := filepath.Join(tempDir, "feature-worktree")
	runGit(mainRepo, "worktree", "add", "-b", "feature/test", worktreePath)

	// Verify that .git in worktree is a file, not a directory
	gitEntry := filepath.Join(worktreePath, paths.GitWorktreeMetadataEntry)
	fi, err := os.Stat(gitEntry)
	if err != nil {
		t.Fatalf("expected .git in worktree: %v", err)
	}
	if fi.IsDir() {
		t.Fatalf("expected .git in secondary worktree to be a file, but got directory")
	}

	// Verify that secondary worktree has NO .zqk
	wtZqk := filepath.Join(worktreePath, paths.ProjectDataDir)
	if _, err := os.Stat(wtZqk); !os.IsNotExist(err) {
		t.Fatalf("expected no .zqk in secondary worktree, but found one")
	}

	// 3. Initialize .zqk in main repo with sample object
	storage.MustEnsureProcessSpecsLayoutForTest(t, mainRepo)
	zqkDir := filepath.Join(mainRepo, paths.ProjectDataDir)
	goalsDir := filepath.Join(zqkDir, "process", "goals")
	if err := os.MkdirAll(goalsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create goals dir: %v", err)
	}

	sampleGoal := filepath.Join(goalsDir, "GOAL-TEST-001.yaml")
	sampleGoalContent := `id: GOAL-TEST-001
kind: goal
title: Test Goal in Main Repo
status: active
created_by: ACC-TEST
namespace_id: zqk:kernel
schema_version: 2.0.0
`
	if err := os.WriteFile(sampleGoal, []byte(sampleGoalContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write sample goal: %v", err)
	}

	// 4. Test path resolution from the secondary worktree root and deep subdirectory
	deepSubDir := filepath.Join(worktreePath, "src", "deep", "nested")
	if err := os.MkdirAll(deepSubDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create deep sub dir: %v", err)
	}

	resolvedRoot := paths.ResolveProjectRoot(worktreePath)
	if resolvedRoot != mainRepo {
		t.Errorf("ResolveProjectRoot(%s) = %q, want %q", worktreePath, resolvedRoot, mainRepo)
	}

	resolvedDeepRoot := paths.ResolveProjectRoot(deepSubDir)
	if resolvedDeepRoot != mainRepo {
		t.Errorf("ResolveProjectRoot(%s) = %q, want %q", deepSubDir, resolvedDeepRoot, mainRepo)
	}

	// 5. Test storage loading from resolved root
	sp, err := storage.NewFileObjectStorageForTest(resolvedDeepRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sp.Shutdown(context.Background())
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSecurityContext(pkgctx.SystemAccountID, []string{"admin", "scheduler"}, []string{"read:*", "write:*"})
	obj, err := sp.Read(ctx, secCtx, "GOAL-TEST-001")
	if err != nil {
		t.Fatalf("failed to read GOAL-TEST-001 through resolved storage: %v", err)
	}
	if obj == nil || obj[objects.FieldKeyID] != "GOAL-TEST-001" {
		t.Errorf("expected GOAL-TEST-001, got %+v", obj)
	}
}
