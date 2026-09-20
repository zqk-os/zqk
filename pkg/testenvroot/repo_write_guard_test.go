package testenvroot

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestValidateNoRepoStateMutation(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	testRoot := filepath.Join(repoRoot, "test_root")
	if err := fileutil.MkdirAll(filepath.Join(repoRoot, paths.ProcessDir), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(testRoot, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	// 1. Allowed: target inside testRoot
	testTarget := filepath.Join(testRoot, paths.ProcessBacklogDir, "item.yaml")
	if err := ValidateNoRepoStateMutation(testTarget, repoRoot, testRoot); err != nil {
		t.Fatalf("expected write to testRoot to be allowed, got: %v", err)
	}

	// 2. Prohibited: target directly in repo process dir
	badTarget := filepath.Join(repoRoot, paths.ProcessBacklogDir, "corrupt.yaml")
	if err := ValidateNoRepoStateMutation(badTarget, repoRoot, testRoot); err == nil {
		t.Fatal("expected write to repo process dir to fail closed, got nil error")
	}

	// 3. Prohibited: target directly in repo .zqk
	badZqkTarget := filepath.Join(repoRoot, paths.ProjectDataDir, "cache.json")
	if err := ValidateNoRepoStateMutation(badZqkTarget, repoRoot, testRoot); err == nil {
		t.Fatal("expected write to repo .zqk to fail closed, got nil error")
	}
}

func TestSnapshotRepoState_DetectsDeliberateViolation(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	processDir := filepath.Join(repoRoot, paths.ProcessBacklogDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	existingFile := filepath.Join(processDir, "original.yaml")
	if err := fileutil.WriteFile(existingFile, []byte("id: BLI-ORIGINAL\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := SnapshotRepoState(repoRoot)
	if err != nil {
		t.Fatalf("SnapshotRepoState: %v", err)
	}

	// No mutation -> passes cleanly
	if err := snapshot.VerifyNoMutations(); err != nil {
		t.Fatalf("expected clean verify, got: %v", err)
	}

	// Deliberate violation: create a rogue file in repo process dir
	rogueFile := filepath.Join(processDir, "rogue_leak.yaml")
	if err := fileutil.WriteFile(rogueFile, []byte("id: BLI-ROGUE\n"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}

	// Snapshot must fail closed
	if err := snapshot.VerifyNoMutations(); err == nil {
		t.Fatal("expected snapshot to fail closed on rogue file creation, got nil")
	}
}
