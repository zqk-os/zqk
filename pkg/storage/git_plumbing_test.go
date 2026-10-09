package storage_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func initTestGitRepo(t *testing.T, dir string) {
	t.Helper()
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "config", "user.name", "Test Agent")
	runCmd(t, dir, "git", "config", "user.email", "test@zqk.internal")
	runCmd(t, dir, "git", "config", "commit.gpgsign", "false")

	// Commit initial dummy file on main branch
	dummy := filepath.Join(dir, "README.md")
	if err := fileutil.WriteFile(dummy, []byte("# Test Repo\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write dummy: %v", err)
	}
	runCmd(t, dir, "git", "add", "README.md")
	runCmd(t, dir, "git", "commit", "-m", "initial commit")
}

func runCmd(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := execwrap.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed %s %v (%s): %v", name, args, strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out))
}

func TestGitPlumbingEngine_SnapshotAndRestore(t *testing.T) {
	repoDir := t.TempDir()
	initTestGitRepo(t, repoDir)

	// Create fake .zqk directory
	zqkDir := filepath.Join(repoDir, ".zqk")
	if err := fileutil.MkdirAll(filepath.Join(zqkDir, "objects"), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir .zqk/objects: %v", err)
	}
	sampleObj := filepath.Join(zqkDir, "objects", "sample.yaml")
	sampleContent := "id: BLI-TEST-001\nkind: backlog_item\ntitle: Sample Test\n"
	if err := fileutil.WriteFile(sampleObj, []byte(sampleContent), paths.FilePerm644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	headBefore := runCmd(t, repoDir, "git", "rev-parse", "HEAD")

	engine, err := storage.NewGitPlumbingEngine(storage.GitPlumbingOptions{
		RepoRoot:   repoDir,
		StorageDir: ".zqk",
	})
	if err != nil {
		t.Fatalf("NewGitPlumbingEngine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Snapshot into refs/zqk/snapshots/main
	snap, err := engine.Snapshot(ctx, "snapshots/main", "test snapshot 1")
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	if snap.CommitHash == "" {
		t.Fatal("expected non-empty commit hash")
	}

	// Invariant: HEAD must not have moved and working tree status must not show staged files
	headAfter := runCmd(t, repoDir, "git", "rev-parse", "HEAD")
	if headBefore != headAfter {
		t.Fatalf("HEAD moved during snapshot! before=%s, after=%s", headBefore, headAfter)
	}

	// 2. List snapshots
	snapshots, err := engine.ListSnapshots(ctx)
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].CommitHash != snap.CommitHash {
		t.Fatalf("expected commit hash %s, got %s", snap.CommitHash, snapshots[0].CommitHash)
	}

	// 3. Restore to an alternate directory
	restoreDir := t.TempDir()
	if err := engine.Restore(ctx, "snapshots/main", restoreDir); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	restoredFile := filepath.Join(restoreDir, "objects", "sample.yaml")
	restoredBytes, err := fileutil.ReadFile(restoredFile)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(restoredBytes) != sampleContent {
		t.Fatalf("restored content mismatch: expected %q, got %q", sampleContent, string(restoredBytes))
	}
}

func TestGitPlumbingEngine_PushAndFetchDistributed(t *testing.T) {
	// Remote bare repo
	remoteDir := t.TempDir()
	runCmd(t, remoteDir, "git", "init", "--bare")

	// Node 1 (producer)
	node1 := t.TempDir()
	initTestGitRepo(t, node1)
	runCmd(t, node1, "git", "remote", "add", "origin", remoteDir)

	// Create state on Node 1
	zqkDir1 := filepath.Join(node1, ".zqk")
	if err := fileutil.MkdirAll(zqkDir1, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir zqkDir1: %v", err)
	}
	node1Obj := filepath.Join(zqkDir1, "node1_state.yaml")
	content := "state: distributed_ok\n"
	if err := fileutil.WriteFile(node1Obj, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("write node1Obj: %v", err)
	}

	engine1, err := storage.NewGitPlumbingEngine(storage.GitPlumbingOptions{
		RepoRoot:   node1,
		StorageDir: ".zqk",
	})
	if err != nil {
		t.Fatalf("engine1: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	snap1, err := engine1.Snapshot(ctx, "sync/cluster", "cluster node 1 snapshot")
	if err != nil {
		t.Fatalf("engine1.Snapshot: %v", err)
	}

	// Push refs/zqk/sync/cluster to remote
	if err := engine1.Push(ctx, "origin", "refs/zqk/sync/cluster:refs/zqk/sync/cluster"); err != nil {
		t.Fatalf("engine1.Push: %v", err)
	}

	// Node 2 (consumer)
	node2 := t.TempDir()
	initTestGitRepo(t, node2)
	runCmd(t, node2, "git", "remote", "add", "origin", remoteDir)

	engine2, err := storage.NewGitPlumbingEngine(storage.GitPlumbingOptions{
		RepoRoot:   node2,
		StorageDir: ".zqk",
	})
	if err != nil {
		t.Fatalf("engine2: %v", err)
	}

	// Fetch refs/zqk/sync/cluster from remote
	if err := engine2.Fetch(ctx, "origin", "refs/zqk/sync/cluster:refs/zqk/sync/cluster"); err != nil {
		t.Fatalf("engine2.Fetch: %v", err)
	}

	// Verify Node 2 has the snapshot
	snapshots2, err := engine2.ListSnapshots(ctx)
	if err != nil {
		t.Fatalf("engine2.ListSnapshots: %v", err)
	}
	if len(snapshots2) != 1 {
		t.Fatalf("expected 1 snapshot on node2, got %d", len(snapshots2))
	}
	if snapshots2[0].CommitHash != snap1.CommitHash {
		t.Fatalf("node2 commit mismatch: expected %s, got %s", snap1.CommitHash, snapshots2[0].CommitHash)
	}

	// Restore into node 2 .zqk
	zqkDir2 := filepath.Join(node2, ".zqk")
	if err := engine2.Restore(ctx, "sync/cluster", zqkDir2); err != nil {
		t.Fatalf("engine2.Restore: %v", err)
	}

	restoredFile2 := filepath.Join(zqkDir2, "node1_state.yaml")
	b, err := fileutil.ReadFile(restoredFile2)
	if err != nil {
		t.Fatalf("read restoredFile2: %v", err)
	}
	if string(b) != content {
		t.Fatalf("expected %q, got %q", content, string(b))
	}
}
