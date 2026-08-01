package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestIsProbableGitWorktreeRoot(t *testing.T) {
	tmp := t.TempDir()
	if IsProbableGitWorktreeRoot(tmp) {
		t.Fatal("empty temp dir should not look like a git worktree root")
	}
	gitPath := filepath.Join(tmp, paths.GitWorktreeMetadataEntry)
	if err := os.WriteFile(gitPath, []byte("gitdir: ../.git/modules/foo\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write .git file: %v", err)
	}
	if !IsProbableGitWorktreeRoot(tmp) {
		t.Fatal("dir with .git file should be detected as git worktree root")
	}
}

func TestScrubProjectRootForTempCleanup_skipsGitWorktreeRoot(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, paths.GitWorktreeMetadataEntry), []byte("ref: refs/heads/main\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write .git: %v", err)
	}
	preserve := filepath.Join(tmp, "must_remain.txt")
	if err := os.WriteFile(preserve, []byte("x"), paths.FilePerm644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	ScrubProjectRootForTempCleanup(tmp, 10, 25)
	if _, err := os.Stat(preserve); err != nil {
		t.Fatalf("scrub must not remove siblings when .git present: %v", err)
	}
}

func TestRunProjectTestTeardown_skipsDestructiveStagesOnGitRoot(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, paths.GitWorktreeMetadataEntry), []byte("gitdir: ../.git\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write .git: %v", err)
	}
	docs := datacell.CellCASPrimaryDir(tmp, "x")
	if err := os.MkdirAll(docs, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	zqkData := filepath.Join(tmp, paths.ProjectDataDir, "wal")
	if err := os.MkdirAll(zqkData, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir .zqk: %v", err)
	}
	marker := filepath.Join(tmp, "KEEP_ME")
	if err := os.WriteFile(marker, []byte("ok"), paths.FilePerm644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs, err := NewFileObjectStorageForTest(tmp)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	defer func() { _ = fs.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = fs.Shutdown(ctx)
	})

	opts := TempProjectTeardown(tmp, fs)
	if err := RunProjectTestTeardown(opts); err != nil {
		t.Fatalf("RunProjectTestTeardown: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("destructive teardown must not scrub a git worktree root")
	}
	if _, err := os.Stat(docs); err != nil {
		t.Fatalf("strip must not run on git worktree root (%s should remain)", paths.ProcessDir)
	}
}

// TestRunProjectTestTeardown_scrubsWhenStripWithoutAggressive verifies SCRUB_PROJECT_ROOT runs whenever
// StripProcessArtifacts is true, so callers need not set AggressiveTempProjectCleanup to empty t.TempDir.
func TestRunProjectTestTeardown_scrubsWhenStripWithoutAggressive(t *testing.T) {
	tmp := t.TempDir()
	docs := datacell.CellCASPrimaryDir(tmp, "x")
	if err := os.MkdirAll(docs, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	zqkData := filepath.Join(tmp, paths.ProjectDataDir, "wal")
	if err := os.MkdirAll(zqkData, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir .zqk: %v", err)
	}
	orphan := filepath.Join(tmp, "leftover.txt")
	if err := os.WriteFile(orphan, []byte("x"), paths.FilePerm644); err != nil {
		t.Fatalf("write: %v", err)
	}

	opts := ProjectTestTeardownOptions{
		ProjectRoot:                  tmp,
		StripProcessArtifacts:        true,
		AggressiveTempProjectCleanup: false,
		WALTimeout:                   2 * time.Second,
		ShutdownTimeout:              2 * time.Second,
	}
	if err := RunProjectTestTeardown(opts); err != nil {
		t.Fatalf("RunProjectTestTeardown: %v", err)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty temp root after strip+scrub, remaining: %v", entries)
	}
}
