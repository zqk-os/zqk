// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package testenvroot

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSetup_ErrorBranches(t *testing.T) {
	t.Parallel()

	// Case 1: testRoot is a file instead of a directory
	filePath := filepath.Join(t.TempDir(), "file_collision.txt")
	if err := fileutil.WriteFile(filePath, []byte("content"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to create file collision: %v", err)
	}
	if _, err := Setup(filePath); err == nil {
		t.Fatal("expected Setup to fail when testRoot is a file")
	}

	// Case 2: test settings target path is an existing directory
	testDir := t.TempDir()
	configDir := filepath.Join(testDir, paths.ConfigDir)
	if err := fileutil.MkdirAll(configDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	targetFileAsDir := filepath.Join(configDir, paths.ZqkTestConfigFileName)
	if err := fileutil.Mkdir(targetFileAsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create dir collision for config: %v", err)
	}
	if _, err := Setup(testDir); err == nil {
		t.Fatal("expected Setup to fail when settings target path is a directory")
	}
}

func TestRepoWriteGuard_UnwatchedAndModifications(t *testing.T) {
	t.Parallel()

	// Snapshot directory with missing process directory
	emptyDir := t.TempDir()
	snap, err := SnapshotRepoState(emptyDir)
	if err != nil {
		t.Fatalf("SnapshotRepoState on empty dir failed: %v", err)
	}
	if err := snap.VerifyNoMutations(); err != nil {
		t.Fatalf("VerifyNoMutations on empty snapshot failed: %v", err)
	}

	// Verify with empty repoRoot on snapshot struct
	emptySnap := &GuardStateSnapshot{repoRoot: ""}
	if err := emptySnap.VerifyNoMutations(); err != nil {
		t.Fatalf("VerifyNoMutations with empty repoRoot failed: %v", err)
	}

	// Verify with repoRoot that lacks process dir
	missingProcSnap := &GuardStateSnapshot{repoRoot: t.TempDir(), files: make(map[string]int64)}
	if err := missingProcSnap.VerifyNoMutations(); err != nil {
		t.Fatalf("VerifyNoMutations with missing process dir failed: %v", err)
	}

	// Detect file modification (mtime / content changed)
	repo := t.TempDir()
	procDir := filepath.Join(repo, paths.ProcessDir)
	if err := fileutil.MkdirAll(procDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to make procDir: %v", err)
	}
	itemFile := filepath.Join(procDir, "item.yaml")
	if err := fileutil.WriteFile(itemFile, []byte("version: 1\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write item file: %v", err)
	}

	modSnap, err := SnapshotRepoState(repo)
	if err != nil {
		t.Fatalf("SnapshotRepoState failed: %v", err)
	}

	// Ensure different timestamp
	time.Sleep(20 * time.Millisecond)
	if err := fileutil.WriteFile(itemFile, []byte("version: 2\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to rewrite item file: %v", err)
	}

	if err := modSnap.VerifyNoMutations(); err == nil {
		t.Fatal("expected VerifyNoMutations to fail on file modification")
	}

	// Verify short-circuit on multiple added files
	freshRepo := t.TempDir()
	freshProc := filepath.Join(freshRepo, paths.ProcessDir)
	if err := fileutil.MkdirAll(freshProc, paths.DirPerm755); err != nil {
		t.Fatalf("failed to make freshProc: %v", err)
	}
	multiSnap, err := SnapshotRepoState(freshRepo)
	if err != nil {
		t.Fatalf("SnapshotRepoState failed: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(freshProc, "a.yaml"), []byte("a"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write a.yaml: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(freshProc, "b.yaml"), []byte("b"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write b.yaml: %v", err)
	}
	if err := multiSnap.VerifyNoMutations(); err == nil {
		t.Fatal("expected VerifyNoMutations to fail on rogue files")
	}
}

func TestRepoWriteGuard_UnreadableSubdirDuringWalk(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	procDir := filepath.Join(repo, paths.ProcessDir)
	subDir := filepath.Join(procDir, "unreadable")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create unreadable dir: %v", err)
	}
	testFile := filepath.Join(subDir, "item.yaml")
	if err := fileutil.WriteFile(testFile, []byte("a: 1\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if err := fileutil.Chmod(subDir, 0000); err != nil {
		t.Fatalf("failed to chmod subDir: %v", err)
	}
	t.Cleanup(func() {
		if err := fileutil.Chmod(subDir, paths.DirPerm755); err != nil {
			t.Logf("cleanup chmod failed: %v", err)
		}
	})

	snap, err := SnapshotRepoState(repo)
	if err != nil {
		t.Fatalf("SnapshotRepoState failed: %v", err)
	}
	if err := snap.VerifyNoMutations(); err != nil {
		t.Fatalf("VerifyNoMutations failed: %v", err)
	}
}

func TestCopyPackSubdirYAML_Variations(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	projectRoot := t.TempDir()
	packsDir := filepath.Join(projectRoot, "packs")
	packSpecs := filepath.Join(packsDir, "test_pack", "specs")
	if err := fileutil.MkdirAll(packSpecs, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create pack specs dir: %v", err)
	}
	specFile := filepath.Join(packSpecs, "spec1.yaml")
	if err := fileutil.WriteFile(specFile, []byte("kind: spec\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write pack spec: %v", err)
	}

	// Add a non-directory entry to packs
	noteFile := filepath.Join(packsDir, "readme.txt")
	if err := fileutil.WriteFile(noteFile, []byte("packs readme"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write pack note: %v", err)
	}

	// Add an empty pack dir without specs
	if err := fileutil.MkdirAll(filepath.Join(packsDir, "empty_pack"), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create empty pack dir: %v", err)
	}

	copyPackSubdirYAML(testRoot, projectRoot, "specs", paths.ProcessInternalObjectSpecsDir)

	copiedFile := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir, "test_pack", "spec1.yaml")
	if _, err := fileutil.Stat(copiedFile); err != nil {
		t.Fatalf("expected copied pack spec file at %s: %v", copiedFile, err)
	}
}

func TestLinkOrCopyYAML_TargetWriteError(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "valid.yaml")
	if err := fileutil.WriteFile(src, []byte("data: true\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write valid.yaml: %v", err)
	}

	targetDir := t.TempDir()
	if err := linkOrCopyYAML(src, targetDir); err == nil {
		t.Fatal("expected linkOrCopyYAML to fail when target is an existing directory")
	}
}

func TestCopyObjectSpecsAndLifecycles_ErrorBranches(t *testing.T) {
	t.Parallel()

	// CopyObjectSpecsFromProject error
	testRoot := t.TempDir()
	projectRoot := t.TempDir()
	srcSpecs := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(srcSpecs, paths.DirPerm755); err != nil {
		t.Fatalf("failed to make srcSpecs: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(srcSpecs, "obj.yaml"), []byte("a: 1\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write obj.yaml: %v", err)
	}
	if err := fileutil.MkdirAll(filepath.Join(testRoot, ".zqk", "specs"), paths.DirPerm755); err != nil {
		t.Fatalf("failed to make specs parent: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir), []byte("blocker"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write blocker file: %v", err)
	}

	if err := CopyObjectSpecsFromProject(testRoot, projectRoot); err == nil {
		t.Fatal("expected CopyObjectSpecsFromProject to fail when target dir is a file")
	}

	// CopyLifecyclesFromProject error
	testRoot2 := t.TempDir()
	projectRoot2 := t.TempDir()
	srcLc := filepath.Join(projectRoot2, paths.ProcessInternalLifecyclesDir)
	if err := fileutil.MkdirAll(srcLc, paths.DirPerm755); err != nil {
		t.Fatalf("failed to make srcLc: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(srcLc, "lc.yaml"), []byte("a: 1\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write lc.yaml: %v", err)
	}
	if err := fileutil.MkdirAll(filepath.Join(testRoot2, ".zqk", "specs"), paths.DirPerm755); err != nil {
		t.Fatalf("failed to make specs parent: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(testRoot2, paths.ProcessInternalLifecyclesDir), []byte("blocker"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write blocker file: %v", err)
	}

	if err := CopyLifecyclesFromProject(testRoot2, projectRoot2); err == nil {
		t.Fatal("expected CopyLifecyclesFromProject to fail when target dir is a file")
	}
}

func TestCopySpecIndexFromProject_TargetDirError(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	projectRoot := t.TempDir()
	srcIndexDir := filepath.Join(projectRoot, paths.ProcessInternalDir)
	if err := fileutil.MkdirAll(srcIndexDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to make srcIndexDir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(srcIndexDir, "spec_index.json"), []byte("{}"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write spec_index.json: %v", err)
	}

	// Block testRoot/.zqk by making it a file
	if err := fileutil.WriteFile(filepath.Join(testRoot, ".zqk"), []byte("blocker"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write .zqk blocker: %v", err)
	}

	if err := CopySpecIndexFromProject(testRoot, projectRoot); err == nil {
		t.Fatal("expected CopySpecIndexFromProject to fail when .zqk is a file")
	}
}

func TestCopyYAMLFiles_WalkErrors(t *testing.T) {
	t.Parallel()

	// Permission error during Walk
	src := t.TempDir()
	subDir := filepath.Join(src, "unreadable")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create subDir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(subDir, "item.yaml"), []byte("val: 1\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write item.yaml: %v", err)
	}
	if err := fileutil.Chmod(subDir, 0000); err != nil {
		t.Fatalf("failed to chmod subDir: %v", err)
	}
	t.Cleanup(func() {
		if err := fileutil.Chmod(subDir, paths.DirPerm755); err != nil {
			t.Logf("cleanup chmod failed: %v", err)
		}
	})

	dst := t.TempDir()
	if err := copyYAMLFiles(src, dst); err == nil {
		t.Fatal("expected copyYAMLFiles to fail with permission error")
	}

	// Target EnsureDir failure
	validSrc := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(validSrc, "test.yaml"), []byte("k: v\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test.yaml: %v", err)
	}
	dstDir := t.TempDir()
	blockerFile := filepath.Join(dstDir, "blocked_dir")
	if err := fileutil.WriteFile(blockerFile, []byte("blocker"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write blocker: %v", err)
	}

	// Passing blockerFile as targetDir: EnsureDir(filepath.Dir(targetPath)) will try to make directory under blockerFile
	if err := copyYAMLFiles(validSrc, filepath.Join(blockerFile, "subdir")); err == nil {
		t.Fatal("expected copyYAMLFiles to fail when target path cannot be created")
	}
}

func TestCopyYAMLFiles_RemovedDuringWalk(t *testing.T) {
	t.Parallel()

	// Verify the fileutil.IsNotExist(err) branch in copyYAMLFiles
	for i := 0; i < 50; i++ {
		src := t.TempDir()
		sub := filepath.Join(src, "transient")
		if err := fileutil.MkdirAll(sub, paths.DirPerm755); err != nil {
			continue
		}
		if err := fileutil.WriteFile(filepath.Join(sub, "test.yaml"), []byte("k: v\n"), paths.FilePerm644); err != nil {
			continue
		}
		dst := t.TempDir()

		stop := make(chan struct{})
		goroutinelabels.NewGoroutine("test_copy_yaml_remover", "concurrent transient dir remover").StartSimple(func() {
			time.Sleep(10 * time.Microsecond)
			if remErr := fileutil.RemoveAll(sub); remErr != nil && !fileutil.IsNotExist(remErr) {
				t.Logf("transient sub remove: %v", remErr)
			}
			close(stop)
		})

		if copyErr := copyYAMLFiles(src, dst); copyErr != nil && !fileutil.IsNotExist(copyErr) {
			t.Logf("copy during removal: %v", copyErr)
		}
		<-stop
	}
}

func TestBootstrapRoot_ErrorBranches(t *testing.T) {
	t.Parallel()

	// Setup fails because testRoot is a file
	testFile := filepath.Join(t.TempDir(), "file.txt")
	if err := fileutil.WriteFile(testFile, []byte("content"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if err := BootstrapRoot(testFile, t.TempDir()); err == nil {
		t.Fatal("expected BootstrapRoot to fail on invalid testRoot")
	}

	// ProcessInternalDir copying fails at line 129
	testRoot := t.TempDir()
	projectRoot := t.TempDir()
	srcInternalSub := filepath.Join(projectRoot, paths.ProcessInternalDir, "blocked_sub")
	if err := fileutil.MkdirAll(srcInternalSub, paths.DirPerm755); err != nil {
		t.Fatalf("failed to make srcInternalSub: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(srcInternalSub, "spec.yaml"), []byte("a: 1\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write spec.yaml: %v", err)
	}
	if _, err := Setup(testRoot); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(testRoot, paths.ProcessInternalDir, "blocked_sub"), []byte("blocker"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write blocker file: %v", err)
	}
	if err := BootstrapRoot(testRoot, projectRoot); err == nil {
		t.Fatal("expected BootstrapRoot to fail when ProcessInternalDir cannot be written")
	}

	// packs copying fails
	testRoot2 := t.TempDir()
	projectRoot2 := t.TempDir()
	packsDir := filepath.Join(projectRoot2, "packs")
	if err := fileutil.MkdirAll(packsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to make packsDir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(packsDir, "pack.yaml"), []byte("a: 1\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write pack.yaml: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(testRoot2, "packs"), []byte("blocker"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write blocker for packs: %v", err)
	}
	if err := BootstrapRoot(testRoot2, projectRoot2); err == nil {
		t.Fatal("expected BootstrapRoot to fail when packs dir cannot be written")
	}
}

func TestOSWrappers_Operations(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	testDir := filepath.Join(tmp, "nested", "sub")
	if err := MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	filePath := filepath.Join(testDir, "output.txt")
	testData := []byte("hermetic test data\n")
	if err := WriteFile(filePath, testData, paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	readData, err := fileutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(readData) != string(testData) {
		t.Fatalf("content mismatch: got %q, want %q", string(readData), string(testData))
	}
}

func TestConcurrent_SetupAndSnapshots(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	const workerCount = 8

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine("test_concurrent_setup_worker", "concurrent setup and snapshot worker").StartSimple(func() {
			defer wg.Done()

			root := t.TempDir()
			absRoot, err := Setup(root)
			if err != nil {
				t.Errorf("worker %d: Setup failed: %v", workerID, err)
				return
			}
			if absRoot != root {
				t.Errorf("worker %d: unexpected absRoot: %s", workerID, absRoot)
				return
			}

			snap, err := SnapshotRepoState(root)
			if err != nil {
				t.Errorf("worker %d: SnapshotRepoState failed: %v", workerID, err)
				return
			}

			if err := snap.VerifyNoMutations(); err != nil {
				t.Errorf("worker %d: VerifyNoMutations failed: %v", workerID, err)
				return
			}
		})
	}

	wg.Wait()
}
