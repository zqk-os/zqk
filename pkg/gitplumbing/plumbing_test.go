package gitplumbing_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/gitplumbing"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// setupGitRepo initializes a git repository with configured user credentials for commit creation.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()

	execGit(t, repoDir, "init")
	execGit(t, repoDir, "config", "user.name", "Test Committer")
	execGit(t, repoDir, "config", "user.email", "committer@example.com")
	execGit(t, repoDir, "config", "commit.gpgSign", "false")

	return repoDir
}

// execGit executes a git command in the specified directory and fails the test if an error occurs.
func execGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := testkit.ManagedCommand(t, t.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v, output: %s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func TestNewPlumbingEngine(t *testing.T) {
	tempRoot := t.TempDir()

	t.Run("error on missing repo root", func(t *testing.T) {
		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{})
		if err == nil {
			t.Fatalf("expected error on empty RepoRoot, got nil")
		}
		if engine != nil {
			t.Fatalf("expected nil engine on error, got %+v", engine)
		}
	})

	t.Run("default options applied", func(t *testing.T) {
		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot: tempRoot,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if engine.RepoRoot() != tempRoot {
			t.Errorf("expected RepoRoot %s, got %s", tempRoot, engine.RepoRoot())
		}
		if engine.RefPrefix() != gitplumbing.DefaultGitRefPrefix {
			t.Errorf("expected RefPrefix %s, got %s", gitplumbing.DefaultGitRefPrefix, engine.RefPrefix())
		}
	})

	t.Run("custom ref prefix without trailing slash", func(t *testing.T) {
		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot:  tempRoot,
			RefPrefix: "refs/custom",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if engine.RefPrefix() != "refs/custom/" {
			t.Errorf("expected 'refs/custom/', got %s", engine.RefPrefix())
		}
	})

	t.Run("custom ref prefix with trailing slash", func(t *testing.T) {
		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot:  tempRoot,
			RefPrefix: "refs/custom/test/",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if engine.RefPrefix() != "refs/custom/test/" {
			t.Errorf("expected 'refs/custom/test/', got %s", engine.RefPrefix())
		}
	})

	t.Run("relative storage dir", func(t *testing.T) {
		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot:   tempRoot,
			StorageDir: "custom/data",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if engine == nil {
			t.Fatalf("expected engine instance")
		}
	})

	t.Run("absolute storage dir", func(t *testing.T) {
		absDir := filepath.Join(tempRoot, "abs/data")
		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot:   tempRoot,
			StorageDir: absDir,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if engine == nil {
			t.Fatalf("expected engine instance")
		}
	})
}

func TestPlumbingEngine_Snapshot_Errors(t *testing.T) {
	repoDir := setupGitRepo(t)
	ctx := context.Background()

	t.Run("missing ref name", func(t *testing.T) {
		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot: repoDir,
		})
		if err != nil {
			t.Fatalf("failed to create engine: %v", err)
		}

		info, err := engine.Snapshot(ctx, "", "test snapshot")
		if err == nil {
			t.Fatalf("expected error on empty refName, got nil")
		}
		if info != nil {
			t.Fatalf("expected nil SnapshotInfo on error")
		}
	})

	t.Run("non-existent storage directory", func(t *testing.T) {
		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot:   repoDir,
			StorageDir: filepath.Join(repoDir, "non_existent_dir_12345"),
		})
		if err != nil {
			t.Fatalf("failed to create engine: %v", err)
		}

		_, err = engine.Snapshot(ctx, "snap1", "test")
		if err == nil {
			t.Fatalf("expected error on non-existent storageDir, got nil")
		}
		if !strings.Contains(err.Error(), "storage dir not found") {
			t.Errorf("expected 'storage dir not found' in error, got: %v", err)
		}
	})

	t.Run("storage dir is a file not a directory", func(t *testing.T) {
		filePath := filepath.Join(repoDir, "regular_file.txt")
		if err := fileutil.WriteFile(filePath, []byte("data"), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot:   repoDir,
			StorageDir: filePath,
		})
		if err != nil {
			t.Fatalf("failed to create engine: %v", err)
		}

		_, err = engine.Snapshot(ctx, "snap1", "test")
		if err == nil {
			t.Fatalf("expected error when storageDir is a file, got nil")
		}
		if !strings.Contains(err.Error(), "storage dir is not a directory") {
			t.Errorf("expected 'storage dir is not a directory' in error, got: %v", err)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		storageDir := filepath.Join(repoDir, paths.ProjectDataDir)
		if err := fileutil.MkdirAll(storageDir, 0755); err != nil {
			t.Fatalf("failed to create storage dir: %v", err)
		}

		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot: repoDir,
		})
		if err != nil {
			t.Fatalf("failed to create engine: %v", err)
		}

		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()

		_, err = engine.Snapshot(canceledCtx, "snap-canceled", "canceled")
		if err == nil {
			t.Fatalf("expected error on canceled context, got nil")
		}
	})

	t.Run("update-ref failure due to ref collision", func(t *testing.T) {
		storageDir := filepath.Join(repoDir, paths.ProjectDataDir)
		if err := fileutil.MkdirAll(storageDir, 0755); err != nil {
			t.Fatalf("failed to create storage dir: %v", err)
		}
		if err := fileutil.WriteFile(filepath.Join(storageDir, "dummy.txt"), []byte("dummy"), 0644); err != nil {
			t.Fatalf("failed to write dummy file: %v", err)
		}

		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot: repoDir,
		})
		if err != nil {
			t.Fatalf("failed to create engine: %v", err)
		}

		// First snapshot creates refs/zqk/parent-ref
		if _, err := engine.Snapshot(ctx, "parent-ref", "first"); err != nil {
			t.Fatalf("first snapshot failed: %v", err)
		}

		// Attempting to create a ref that treats the file ref as a directory will fail git update-ref
		_, err = engine.Snapshot(ctx, "parent-ref/child-collision", "should fail update-ref")
		if err == nil {
			t.Fatalf("expected git update-ref failure on collision, got nil")
		}
		if !strings.Contains(err.Error(), "git update-ref") {
			t.Errorf("expected error to mention 'git update-ref', got: %v", err)
		}
	})
}

func TestPlumbingEngine_Snapshot_And_ListSnapshots(t *testing.T) {
	repoDir := setupGitRepo(t)
	ctx := context.Background()

	storageDir := filepath.Join(repoDir, paths.ProjectDataDir)
	if err := fileutil.MkdirAll(filepath.Join(storageDir, "manifests"), 0755); err != nil {
		t.Fatalf("failed to create manifests dir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(storageDir, "manifests", "meta.json"), []byte(`{"version": 1}`), 0644); err != nil {
		t.Fatalf("failed to write meta.json: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(storageDir, "state.db"), []byte("state-payload-bytes"), 0644); err != nil {
		t.Fatalf("failed to write state.db: %v", err)
	}

	engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
		RepoRoot: repoDir,
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// 1. Initial listing must be empty
	initialList, err := engine.ListSnapshots(ctx)
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(initialList) != 0 {
		t.Fatalf("expected 0 snapshots initially, got %d", len(initialList))
	}

	// 2. Capture first snapshot with custom message and short ref
	snap1, err := engine.Snapshot(ctx, "session-backup-1", "Initial kernel snapshot")
	if err != nil {
		t.Fatalf("Snapshot 1 failed: %v", err)
	}

	if snap1.RefName != "refs/zqk/session-backup-1" {
		t.Errorf("expected ref 'refs/zqk/session-backup-1', got '%s'", snap1.RefName)
	}
	if snap1.CommitHash == "" {
		t.Errorf("expected non-empty commit hash")
	}
	if snap1.TreeHash == "" {
		t.Errorf("expected non-empty tree hash")
	}
	if snap1.Message != "Initial kernel snapshot" {
		t.Errorf("expected message 'Initial kernel snapshot', got '%s'", snap1.Message)
	}
	if time.Since(snap1.Timestamp) > time.Minute {
		t.Errorf("snapshot timestamp unexpected: %v", snap1.Timestamp)
	}

	// 3. Verify ListSnapshots returns the snapshot
	listAfter1, err := engine.ListSnapshots(ctx)
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(listAfter1) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(listAfter1))
	}
	if listAfter1[0].RefName != snap1.RefName {
		t.Errorf("expected ref %s, got %s", snap1.RefName, listAfter1[0].RefName)
	}
	if listAfter1[0].CommitHash != snap1.CommitHash {
		t.Errorf("expected commit %s, got %s", snap1.CommitHash, listAfter1[0].CommitHash)
	}
	if listAfter1[0].Message != snap1.Message {
		t.Errorf("expected message %s, got %s", snap1.Message, listAfter1[0].Message)
	}

	// 4. Update data in storageDir
	if err := fileutil.WriteFile(filepath.Join(storageDir, "state.db"), []byte("state-payload-v2"), 0644); err != nil {
		t.Fatalf("failed to update state.db: %v", err)
	}

	// 5. Capture second snapshot pointing to the SAME ref with an empty message (tests default message and parent commit branch)
	snap2, err := engine.Snapshot(ctx, "session-backup-1", "")
	if err != nil {
		t.Fatalf("Snapshot 2 failed: %v", err)
	}

	if snap2.CommitHash == snap1.CommitHash {
		t.Errorf("expected new commit hash for updated snapshot, got same %s", snap2.CommitHash)
	}
	if !strings.HasPrefix(snap2.Message, "zqk kernel snapshot at") {
		t.Errorf("expected default message prefix, got '%s'", snap2.Message)
	}

	// Verify parent commit using git rev-parse
	parentCommit := execGit(t, repoDir, "rev-parse", snap2.CommitHash+"^1")
	if parentCommit != snap1.CommitHash {
		t.Errorf("expected parent commit %s, got %s", snap1.CommitHash, parentCommit)
	}

	// 6. Capture third snapshot with full ref prefix ("refs/...")
	snap3, err := engine.Snapshot(ctx, "refs/zqk/checkpoints/chk-100", "Checkpoint 100")
	if err != nil {
		t.Fatalf("Snapshot 3 failed: %v", err)
	}
	if snap3.RefName != "refs/zqk/checkpoints/chk-100" {
		t.Errorf("expected full ref kept as-is, got '%s'", snap3.RefName)
	}

	// 7. Verify ListSnapshots lists both refs
	listAfter3, err := engine.ListSnapshots(ctx)
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(listAfter3) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(listAfter3))
	}
}

func TestPlumbingEngine_Restore(t *testing.T) {
	repoDir := setupGitRepo(t)
	ctx := context.Background()

	storageDir := filepath.Join(repoDir, paths.ProjectDataDir)
	nestedDir := filepath.Join(storageDir, "catalog", "objects")
	if err := fileutil.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("failed to create nested dirs: %v", err)
	}

	file1Path := filepath.Join(storageDir, "primary.txt")
	file2Path := filepath.Join(nestedDir, "object-42.json")
	content1 := []byte("primary storage data")
	content2 := []byte(`{"id": 42, "status": "active"}`)

	if err := fileutil.WriteFile(file1Path, content1, 0644); err != nil {
		t.Fatalf("failed to write primary.txt: %v", err)
	}
	if err := fileutil.WriteFile(file2Path, content2, 0644); err != nil {
		t.Fatalf("failed to write object-42.json: %v", err)
	}

	engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
		RepoRoot: repoDir,
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	snap, err := engine.Snapshot(ctx, "restore-point", "Test restore snapshot")
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	t.Run("empty ref name fails", func(t *testing.T) {
		targetDir := t.TempDir()
		if err := engine.Restore(ctx, "", targetDir); err == nil {
			t.Fatalf("expected error on empty refName, got nil")
		}
	})

	t.Run("non-existent ref fails", func(t *testing.T) {
		targetDir := t.TempDir()
		if err := engine.Restore(ctx, "refs/zqk/non-existent-ref", targetDir); err == nil {
			t.Fatalf("expected error on non-existent ref, got nil")
		}
	})

	t.Run("invalid target dir fails", func(t *testing.T) {
		// Create a file and use it as targetDir to cause MkdirAll failure
		filePath := filepath.Join(t.TempDir(), "blocker-file")
		if err := fileutil.WriteFile(filePath, []byte("blocker"), 0644); err != nil {
			t.Fatalf("failed to write blocker file: %v", err)
		}
		badTarget := filepath.Join(filePath, "sub-dir")

		if err := engine.Restore(ctx, snap.RefName, badTarget); err == nil {
			t.Fatalf("expected error when targetDir cannot be created, got nil")
		}
	})

	t.Run("successful restore with short ref", func(t *testing.T) {
		targetDir := filepath.Join(t.TempDir(), "restored-short")

		if err := engine.Restore(ctx, "restore-point", targetDir); err != nil {
			t.Fatalf("Restore failed: %v", err)
		}

		// Verify files
		got1, err := fileutil.ReadFile(filepath.Join(targetDir, "primary.txt"))
		if err != nil {
			t.Fatalf("failed to read restored primary.txt: %v", err)
		}
		if string(got1) != string(content1) {
			t.Errorf("content mismatch in primary.txt: got %q, want %q", string(got1), string(content1))
		}

		got2, err := fileutil.ReadFile(filepath.Join(targetDir, "catalog", "objects", "object-42.json"))
		if err != nil {
			t.Fatalf("failed to read restored object-42.json: %v", err)
		}
		if string(got2) != string(content2) {
			t.Errorf("content mismatch in object-42.json: got %q, want %q", string(got2), string(content2))
		}
	})

	t.Run("successful restore with full ref", func(t *testing.T) {
		targetDir := filepath.Join(t.TempDir(), "restored-full")

		if err := engine.Restore(ctx, snap.RefName, targetDir); err != nil {
			t.Fatalf("Restore failed: %v", err)
		}

		got1, err := fileutil.ReadFile(filepath.Join(targetDir, "primary.txt"))
		if err != nil {
			t.Fatalf("failed to read restored primary.txt: %v", err)
		}
		if string(got1) != string(content1) {
			t.Errorf("content mismatch in primary.txt: got %q, want %q", string(got1), string(content1))
		}
	})

	t.Run("unpack file fails when destination is an existing directory", func(t *testing.T) {
		targetDir := filepath.Join(t.TempDir(), "conflict-dir")
		// Pre-create primary.txt as a directory
		if err := fileutil.MkdirAll(filepath.Join(targetDir, "primary.txt"), 0755); err != nil {
			t.Fatalf("failed to create directory conflict: %v", err)
		}

		if err := engine.Restore(ctx, snap.RefName, targetDir); err == nil {
			t.Fatalf("expected error when file cannot be opened because a directory exists, got nil")
		}
	})

	t.Run("unpack parent dir fails when path is a file", func(t *testing.T) {
		targetDir := filepath.Join(t.TempDir(), "conflict-parent")
		if err := fileutil.MkdirAll(targetDir, 0755); err != nil {
			t.Fatalf("failed to create target dir: %v", err)
		}
		// Pre-create 'catalog' as a regular file so MkdirAll on catalog/objects fails
		if err := fileutil.WriteFile(filepath.Join(targetDir, "catalog"), []byte("not-a-dir"), 0644); err != nil {
			t.Fatalf("failed to write blocker file: %v", err)
		}

		if err := engine.Restore(ctx, snap.RefName, targetDir); err == nil {
			t.Fatalf("expected error when parent directory cannot be created, got nil")
		}
	})
}

func TestPlumbingEngine_Push_And_Fetch(t *testing.T) {
	ctx := context.Background()

	// 1. Set up bare remote repository
	bareRemoteDir := t.TempDir()
	execGit(t, bareRemoteDir, "init", "--bare")

	// 2. Set up local repo 1 and configure remote
	localRepo1 := setupGitRepo(t)
	execGit(t, localRepo1, "remote", "add", "origin", bareRemoteDir)

	storageDir1 := filepath.Join(localRepo1, paths.ProjectDataDir)
	if err := fileutil.MkdirAll(storageDir1, 0755); err != nil {
		t.Fatalf("failed to create storage dir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(storageDir1, "seed.txt"), []byte("seed-data"), 0644); err != nil {
		t.Fatalf("failed to write seed file: %v", err)
	}

	engine1, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
		RepoRoot: localRepo1,
	})
	if err != nil {
		t.Fatalf("failed to create engine1: %v", err)
	}

	snap1, err := engine1.Snapshot(ctx, "push-fetch-test", "Snapshot for push and fetch")
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	t.Run("push with default remote and refspec", func(t *testing.T) {
		if err := engine1.Push(ctx, "", ""); err != nil {
			t.Fatalf("Push with default options failed: %v", err)
		}

		// Verify bare repo has the ref
		bareRef := execGit(t, bareRemoteDir, "rev-parse", snap1.RefName)
		if bareRef != snap1.CommitHash {
			t.Errorf("bare repo ref %s commit mismatch: got %s, want %s", snap1.RefName, bareRef, snap1.CommitHash)
		}
	})

	t.Run("push with explicit remote and refspec", func(t *testing.T) {
		refSpec := fmt.Sprintf("%s*:%s*", engine1.RefPrefix(), engine1.RefPrefix())
		if err := engine1.Push(ctx, "origin", refSpec); err != nil {
			t.Fatalf("Push with explicit options failed: %v", err)
		}
	})

	t.Run("push failure on invalid remote", func(t *testing.T) {
		if err := engine1.Push(ctx, "nonexistent-remote", ""); err == nil {
			t.Fatalf("expected error pushing to nonexistent remote, got nil")
		}
	})

	t.Run("push failure on canceled context", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()
		if err := engine1.Push(canceledCtx, "", ""); err == nil {
			t.Fatalf("expected error pushing with canceled context, got nil")
		}
	})

	t.Run("fetch into local repo 2", func(t *testing.T) {
		localRepo2 := setupGitRepo(t)
		execGit(t, localRepo2, "remote", "add", "origin", bareRemoteDir)

		engine2, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot: localRepo2,
		})
		if err != nil {
			t.Fatalf("failed to create engine2: %v", err)
		}

		// Initially, repo 2 has no snapshots
		listBefore, err := engine2.ListSnapshots(ctx)
		if err != nil {
			t.Fatalf("ListSnapshots before fetch failed: %v", err)
		}
		if len(listBefore) != 0 {
			t.Fatalf("expected 0 snapshots in repo 2 before fetch, got %d", len(listBefore))
		}

		// Fetch default
		if err := engine2.Fetch(ctx, "", ""); err != nil {
			t.Fatalf("Fetch with default options failed: %v", err)
		}

		// Snapshots should now be visible in repo 2
		listAfter, err := engine2.ListSnapshots(ctx)
		if err != nil {
			t.Fatalf("ListSnapshots after fetch failed: %v", err)
		}
		if len(listAfter) != 1 {
			t.Fatalf("expected 1 snapshot in repo 2 after fetch, got %d", len(listAfter))
		}
		if listAfter[0].CommitHash != snap1.CommitHash {
			t.Errorf("commit hash mismatch: got %s, want %s", listAfter[0].CommitHash, snap1.CommitHash)
		}

		// Fetch with explicit remote and refspec
		refSpec := fmt.Sprintf("%s*:%s*", engine2.RefPrefix(), engine2.RefPrefix())
		if err := engine2.Fetch(ctx, "origin", refSpec); err != nil {
			t.Fatalf("Fetch with explicit options failed: %v", err)
		}
	})

	t.Run("fetch failure on invalid remote", func(t *testing.T) {
		if err := engine1.Fetch(ctx, "nonexistent-remote", ""); err == nil {
			t.Fatalf("expected error fetching from nonexistent remote, got nil")
		}
	})

	t.Run("fetch failure on canceled context", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()
		if err := engine1.Fetch(canceledCtx, "", ""); err == nil {
			t.Fatalf("expected error fetching with canceled context, got nil")
		}
	})
}

func TestPlumbingEngine_ListSnapshots_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("canceled context causes error", func(t *testing.T) {
		repoDir := setupGitRepo(t)
		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot: repoDir,
		})
		if err != nil {
			t.Fatalf("failed to create engine: %v", err)
		}

		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()

		_, err = engine.ListSnapshots(canceledCtx)
		if err == nil {
			t.Fatalf("expected error with canceled context, got nil")
		}
	})

	t.Run("invalid repo root causes error", func(t *testing.T) {
		nonRepoDir := t.TempDir()
		engine, err := gitplumbing.NewPlumbingEngine(gitplumbing.PlumbingOptions{
			RepoRoot: nonRepoDir,
		})
		if err != nil {
			t.Fatalf("failed to create engine: %v", err)
		}

		_, err = engine.ListSnapshots(ctx)
		if err == nil {
			t.Fatalf("expected error on non-git repo root, got nil")
		}
	})
}
