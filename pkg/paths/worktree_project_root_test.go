package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindWorktreeProjectRoot_SimulatedLinkedWorktree(t *testing.T) {
	tempBase := t.TempDir()

	// 1. Create main repo with .zqk and .git/worktrees/wt1
	mainRepo := filepath.Join(tempBase, "main-repo")
	mainZqk := filepath.Join(mainRepo, ProjectDataDir)
	mainGitWorktreeDir := filepath.Join(mainRepo, ".git", "worktrees", "wt1")

	if err := os.MkdirAll(mainZqk, DirPerm755); err != nil {
		t.Fatalf("failed to create main .zqk: %v", err)
	}
	if err := os.MkdirAll(mainGitWorktreeDir, DirPerm755); err != nil {
		t.Fatalf("failed to create main git worktree dir: %v", err)
	}

	// Write commondir in the worktree git dir pointing to ../.. (i.e. mainRepo/.git)
	commondirFile := filepath.Join(mainGitWorktreeDir, "commondir")
	if err := os.WriteFile(commondirFile, []byte("../..\n"), FilePerm644); err != nil {
		t.Fatalf("failed to write commondir: %v", err)
	}

	// 2. Create secondary worktree directory without .zqk
	secondaryWorktree := filepath.Join(tempBase, "linked-worktree")
	worktreeSubDir := filepath.Join(secondaryWorktree, "pkg", "subpkg")
	if err := os.MkdirAll(worktreeSubDir, DirPerm755); err != nil {
		t.Fatalf("failed to create worktree sub dir: %v", err)
	}

	// Write .git file in secondary worktree pointing to mainGitWorktreeDir
	gitFile := filepath.Join(secondaryWorktree, GitWorktreeMetadataEntry)
	gitFileContent := "gitdir: " + mainGitWorktreeDir + "\n"
	if err := os.WriteFile(gitFile, []byte(gitFileContent), FilePerm644); err != nil {
		t.Fatalf("failed to write .git file: %v", err)
	}

	// Test from root of secondary worktree
	resolvedRoot := FindWorktreeProjectRoot(secondaryWorktree)
	if resolvedRoot != mainRepo {
		t.Errorf("FindWorktreeProjectRoot(%s) = %q, want %q", secondaryWorktree, resolvedRoot, mainRepo)
	}

	// Test from deep subdirectory in secondary worktree
	resolvedSub := FindWorktreeProjectRoot(worktreeSubDir)
	if resolvedSub != mainRepo {
		t.Errorf("FindWorktreeProjectRoot(%s) = %q, want %q", worktreeSubDir, resolvedSub, mainRepo)
	}

	// Test FindWorkspaceRoot and ResolveProjectRoot from within the worktree
	wsRoot := FindWorkspaceRoot(worktreeSubDir)
	if wsRoot != mainRepo {
		t.Errorf("FindWorkspaceRoot(%s) = %q, want %q", worktreeSubDir, wsRoot, mainRepo)
	}

	projRoot := ResolveProjectRoot(worktreeSubDir)
	if projRoot != mainRepo {
		t.Errorf("ResolveProjectRoot(%s) = %q, want %q", worktreeSubDir, projRoot, mainRepo)
	}
}
