package operational

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFilesystemSnapshot_ScopeAndEdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// ParseFilesystemSnapshotScope
	if s, err := ParseFilesystemSnapshotScope(""); err != nil || s != FSSnapshotScopeFull {
		t.Errorf("expected full scope, got %s %v", s, err)
	}
	if s, err := ParseFilesystemSnapshotScope("full"); err != nil || s != FSSnapshotScopeFull {
		t.Errorf("expected full scope, got %s %v", s, err)
	}
	if s, err := ParseFilesystemSnapshotScope("zqk"); err != nil || s != FSSnapshotScopeZqk {
		t.Errorf("expected zqk scope, got %s %v", s, err)
	}
	if s, err := ParseFilesystemSnapshotScope("docs"); err != nil || s != FSSnapshotScopeDocs {
		t.Errorf("expected docs scope, got %s %v", s, err)
	}
	if s, err := ParseFilesystemSnapshotScope("zqk-and-docs"); err != nil || s != FSSnapshotScopeZqkAndDocs {
		t.Errorf("expected zqk-and-docs scope, got %s %v", s, err)
	}
	if _, err := ParseFilesystemSnapshotScope("invalid-scope"); err == nil {
		t.Errorf("expected error on invalid scope")
	}

	// RunFilesystemProjectSnapshot error cases
	if _, err := RunFilesystemProjectSnapshot(ctx, "", FSSnapshotScopeFull); err == nil {
		t.Errorf("expected error on empty projectRoot")
	}
	if _, err := RunFilesystemProjectSnapshot(ctx, "/nonexistent/path/never/here", FSSnapshotScopeFull); err == nil {
		t.Errorf("expected error on non-existent path")
	}

	// Run with narrow scopes on a test layout
	root := t.TempDir()
	_ = fileutil.MkdirAll(filepath.Join(root, paths.ProjectDataDir, "sub1"), paths.DirPerm750)
	_ = fileutil.WriteFile(filepath.Join(root, paths.ProjectDataDir, "sub1", "f.txt"), []byte("data"), paths.FilePerm600)
	_ = fileutil.MkdirAll(filepath.Join(root, paths.DocsDir, "sub2"), paths.DirPerm750)
	_ = fileutil.WriteFile(filepath.Join(root, paths.DocsDir, "sub2", "f.md"), []byte("doc"), paths.FilePerm600)

	// FSSnapshotScopeZqk
	snapZqk, err := RunFilesystemProjectSnapshot(ctx, root, FSSnapshotScopeZqk)
	if err != nil {
		t.Fatalf("RunFilesystemProjectSnapshot zqk: %v", err)
	}
	if snapZqk.TotalFiles != 1 {
		t.Errorf("expected 1 file in zqk scope, got %d", snapZqk.TotalFiles)
	}

	// FSSnapshotScopeDocs
	snapDocs, err := RunFilesystemProjectSnapshot(ctx, root, FSSnapshotScopeDocs)
	if err != nil {
		t.Fatalf("RunFilesystemProjectSnapshot docs: %v", err)
	}
	if snapDocs.TotalFiles != 1 {
		t.Errorf("expected 1 file in docs scope, got %d", snapDocs.TotalFiles)
	}

	// FSSnapshotScopeZqkAndDocs
	snapBoth, err := RunFilesystemProjectSnapshot(ctx, root, FSSnapshotScopeZqkAndDocs)
	if err != nil {
		t.Fatalf("RunFilesystemProjectSnapshot zqk-and-docs: %v", err)
	}
	if snapBoth.TotalFiles != 2 {
		t.Errorf("expected 2 files in both scope, got %d", snapBoth.TotalFiles)
	}

	// Non-directory projectRoot error
	filePath := filepath.Join(root, "file.txt")
	_ = fileutil.WriteFile(filePath, []byte("test"), paths.FilePerm600)
	if _, err := RunFilesystemProjectSnapshot(ctx, filePath, FSSnapshotScopeFull); err == nil {
		t.Errorf("expected error when projectRoot is a regular file")
	}
}

func TestVolumeStats_NilOrEmpty(t *testing.T) {
	t.Parallel()
	fillVolumeStats(nil, "/tmp")
	snap := &FilesystemProjectSnapshot{}
	fillVolumeStats(snap, "")
	if snap.Volume != nil {
		t.Errorf("expected nil volume on empty path")
	}
	fillVolumeStats(snap, "/nonexistent/path/for/statfs")
}

func TestGetInternalCountByKind_ErrorBranch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Calling with non-existent binary returns error
	_, _, err := getInternalCountByKind(ctx, t.TempDir(), "/nonexistent/zqk-bin", 100*time.Millisecond)
	if err == nil {
		t.Errorf("expected error on non-existent binary")
	}
}

func TestCongruenceRun_AdditionalBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Missing project root
	if _, err := Run(ctx, nil, secCtx, RunOptions{}); err == nil {
		t.Errorf("expected error on empty project root")
	}

	// 2. Missing process directory
	nonExistentRoot := t.TempDir()
	if _, err := Run(ctx, nil, secCtx, RunOptions{ProjectRoot: nonExistentRoot}); err == nil {
		t.Errorf("expected error when process dir does not exist")
	}

	// 3. Setup process dir with stream backed, stray, and cache
	root := t.TempDir()
	procDir := datacell.ProcessPrimaryDir(root)
	_ = fileutil.MkdirAll(procDir, paths.DirPerm750)

	// Stream backed directory with some YAML files
	auditDir := filepath.Join(procDir, "audit")
	_ = fileutil.MkdirAll(auditDir, paths.DirPerm750)
	_ = fileutil.WriteFile(filepath.Join(auditDir, "event1.yaml"), []byte("id: 1\n"), paths.FilePerm600)

	// Stray directory with YAML files (no kind mapping)
	strayDir := filepath.Join(procDir, "unknown_stray_dir")
	_ = fileutil.MkdirAll(strayDir, paths.DirPerm750)
	_ = fileutil.WriteFile(filepath.Join(strayDir, "item1.yaml"), []byte("id: 2\n"), paths.FilePerm600)

	// Known directory with large disparity
	bliDir := filepath.Join(procDir, "backlog_items")
	_ = fileutil.MkdirAll(bliDir, paths.DirPerm750)
	for i := 0; i < 5; i++ {
		_ = fileutil.WriteFile(filepath.Join(bliDir, fmt.Sprintf("t%d.yaml", i)), []byte("id: t\n"), paths.FilePerm600)
	}

	opts := RunOptions{
		ProjectRoot:        root,
		StreamBackedDirs:   []string{"audit"},
		DisparityThreshold: 2, // trigger disparity alert when disp > 2
		ObjectCountByKindFromCache: map[string]int{
			"backlog_item": 0, // disk=5 vs object=0 -> disparity=5 > threshold(2)
		},
		IncludeInternal: true,
		ZQKBin:          "/nonexistent/bin", // exercise internal count failure path safely
		InternalTimeout: 50 * time.Millisecond,
	}

	report, err := Run(ctx, nil, secCtx, opts)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(report.StrayDirs) == 0 {
		t.Errorf("expected unknown_stray_dir in StrayDirs")
	}
	if report.LegacyStreamBackedByDir["audit"] != 1 {
		t.Errorf("expected 1 legacy file in audit dir, got %d", report.LegacyStreamBackedByDir["audit"])
	}
	if len(report.KindsWithDisparity) == 0 {
		t.Errorf("expected disparity for backlog_items directory; report: DiskCountByDir=%v, ObjectCountByDir=%v, DisparityByDir=%v, StrayDirs=%v, Alerts=%v",
			report.DiskCountByDir, report.ObjectCountByDir, report.DisparityByDir, report.StrayDirs, report.Alerts)
	}
}

func TestPlanWalkRootsFanout_Branches(t *testing.T) {
	t.Parallel()

	// 1. Base does not exist
	r1, err := planWalkRootsFanout("/nonexistent/path/fanout")
	if err != nil || r1 != nil {
		t.Errorf("expected nil, nil for nonexistent base, got %v, %v", r1, err)
	}

	// 2. Base is a regular file
	root := t.TempDir()
	fpath := filepath.Join(root, "regular_file.txt")
	if err := fileutil.WriteFile(fpath, []byte("data"), paths.FilePerm600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	r2, err := planWalkRootsFanout(fpath)
	if err != nil || r2 != nil {
		t.Errorf("expected nil, nil when base is regular file, got %v, %v", r2, err)
	}

	// 3. Base is an empty directory (no child dirs) -> should return []string{base}
	emptyDir := filepath.Join(root, "emptydir")
	if err := fileutil.MkdirAll(emptyDir, paths.DirPerm750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	r3, err := planWalkRootsFanout(emptyDir)
	if err != nil || len(r3) != 1 || r3[0] != emptyDir {
		t.Errorf("expected [emptyDir], got %v, err %v", r3, err)
	}

	// 4. Base has skip dirs like .git and files only -> returns [emptyDir]
	gitDir := filepath.Join(emptyDir, ".git")
	if err := fileutil.MkdirAll(gitDir, paths.DirPerm750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(emptyDir, "somefile.txt"), []byte("file"), paths.FilePerm600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	r4, err := planWalkRootsFanout(emptyDir)
	if err != nil || len(r4) != 1 || r4[0] != emptyDir {
		t.Errorf("expected [emptyDir] when only skip dirs and files present, got %v, err %v", r4, err)
	}

	// 5. Base has legitimate child dir -> returns child dir
	validSub := filepath.Join(emptyDir, "valid_sub")
	if err := fileutil.MkdirAll(validSub, paths.DirPerm750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	r5, err := planWalkRootsFanout(emptyDir)
	if err != nil || len(r5) != 1 || r5[0] != validSub {
		t.Errorf("expected [valid_sub], got %v, err %v", r5, err)
	}
}

func TestPlanParallelWalkRoots_InvalidScope(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, _, err := planParallelWalkRoots(root, FilesystemSnapshotScope("unrecognized"))
	if err == nil {
		t.Errorf("expected error on invalid scope in planParallelWalkRoots")
	}
}

func TestMergeSegIntoSnapshot_NilSafeties(t *testing.T) {
	t.Parallel()
	mergeSegIntoSnapshot(nil, nil)
	mergeSegIntoSnapshot(&FilesystemProjectSnapshot{}, nil)
	mergeSegIntoSnapshot(nil, newFsSegAgg())
}

func TestRunFilesystemProjectSnapshotParallel_CanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	root := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(root, "file.txt"), []byte("data"), paths.FilePerm600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	snap := &FilesystemProjectSnapshot{}
	err := runFilesystemProjectSnapshotParallel(ctx, root, snap, FSSnapshotScopeFull)
	if err == nil {
		t.Errorf("expected error on canceled context")
	}
}

func TestCongruenceRun_NoCacheStorageCount(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	procDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(procDir, paths.DirPerm750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	storageProvider, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Skipf("file storage init: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)
	registerCongruenceStorageTeardown(t, tmpDir, storageProvider)

	secCtx := pkgctx.NewSystemSecurityContext()
	opts := RunOptions{
		ProjectRoot:                tmpDir,
		DisparityThreshold:         0,   // test default threshold fallback
		ObjectCountByKindFromCache: nil, // exercises registry.LoadFields and storageProvider.Count
	}

	report, err := Run(ctx, storageProvider, secCtx, opts)
	if err != nil {
		t.Fatalf("Run without cache failed: %v", err)
	}
	if report == nil {
		t.Fatalf("expected report, got nil")
	}
}
