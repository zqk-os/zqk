package operational

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func registerCongruenceStorageTeardown(t *testing.T, tmpDir string, sp *storage.FileObjectStorage) {
	t.Helper()
	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, sp)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})
}

const (
	testProcessDirName = "process"
	testAuditDirName   = "audit"

	testChangeJournalDir = "change_journal"

	testDirPerm  = 0o750
	testFilePerm = 0o600
)

// TestRun_toleratesMissingFilesDuringWalk verifies that Run completes successfully
// when a YAML file is missing during filepath.Walk (e.g. deleted after ReadDir, or TOCTOU).
// Previously the walk would abort with "lstat ... no such file or directory".
func TestRun_toleratesMissingFilesDuringWalk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	processDir := filepath.Join(tmpDir, "docs", testProcessDirName)
	auditDir := filepath.Join(processDir, testAuditDirName)
	if err := fileutil.MkdirAll(auditDir, testDirPerm); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Two YAML files; we will delete one before running so Walk sees a missing file.
	f1 := filepath.Join(auditDir, "a.yaml")
	f2 := filepath.Join(auditDir, "b.yaml")
	if err := fileutil.WriteFile(f1, []byte("id: a\n"), testFilePerm); err != nil {
		t.Fatalf("write f1: %v", err)
	}
	if err := fileutil.WriteFile(f2, []byte("id: b\n"), testFilePerm); err != nil {
		t.Fatalf("write f2: %v", err)
	}
	// Remove one file so that when Walk runs it may encounter the missing file
	// (depending on walk order). Either way, Run should complete and count only existing files.
	if err := fileutil.Remove(f1); err != nil {
		t.Fatalf("remove f1: %v", err)
	}

	// Use real file storage; we pass cache so Count is never called.
	storageProvider, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Skipf("file storage init (e.g. specs): %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)
	registerCongruenceStorageTeardown(t, tmpDir, storageProvider)
	secCtx := pkgctx.NewSystemSecurityContext()
	opts := RunOptions{
		ProjectRoot:                tmpDir,
		IncludeInternal:            false,
		ZQKBin:                     "",
		ObjectCountByKindFromCache: map[string]int{"audit_event": 1},
	}
	report, err := Run(ctx, storageProvider, secCtx, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.DiskCountByDir[testAuditDirName] != 1 {
		t.Errorf("expected disk count 1 for audit (one file remaining), got %d", report.DiskCountByDir[testAuditDirName])
	}
	if report.TotalDisk != 1 {
		t.Errorf("expected total disk 1 (audit leftover only; _internal schema seeds are not instance inventory), got %d", report.TotalDisk)
	}
}

// TestRun_streamBackedDirs_skipsDiskWalk verifies that when StreamBackedDirs is set, Run skips
// the filepath.Walk for those dirs (disk count = 0) so the command stays fast even when those
// dirs contain thousands of legacy YAML files. Negative disparity must not trigger an alert.
func TestRun_streamBackedDirs_skipsDiskWalk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	processDir := filepath.Join(tmpDir, "docs", testProcessDirName)
	changeJournalDir := filepath.Join(processDir, testChangeJournalDir)
	if err := fileutil.MkdirAll(changeJournalDir, testDirPerm); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Legacy YAML files that would previously be counted during the walk.
	// With the optimization they should be skipped (disk count = 0).
	for _, name := range []string{"legacy-1.yaml", "legacy-2.yaml", "legacy-3.yaml"} {
		if err := fileutil.WriteFile(filepath.Join(changeJournalDir, name), []byte("id: "+name+"\n"), testFilePerm); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
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
		IncludeInternal:            false,
		ZQKBin:                     "",
		ObjectCountByKindFromCache: map[string]int{"change_journal_entry": 10},
		StreamBackedDirs:           []string{testChangeJournalDir},
	}
	report, err := Run(ctx, storageProvider, secCtx, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Stream-backed dir: disk walk is skipped; disk count is 0 (not walked).
	if report.DiskCountByDir[testChangeJournalDir] != 0 {
		t.Errorf("disk count change_journal: got %d, want 0 (walk skipped for stream-backed dir)", report.DiskCountByDir[testChangeJournalDir])
	}
	// Object count from cache is authoritative.
	if report.ObjectCountByDir[testChangeJournalDir] != 10 {
		t.Errorf("object count change_journal: got %d, want 10", report.ObjectCountByDir[testChangeJournalDir])
	}
	// Disparity = 0 (disk) - 10 (object) = -10; negative is expected for stream-backed dirs.
	if disp := report.DisparityByDir[testChangeJournalDir]; disp != -10 {
		t.Errorf("disparity change_journal: got %d, want -10 (disk 0 - object 10)", disp)
	}
	if len(report.StreamBackedDirs) != 1 || report.StreamBackedDirs[0] != testChangeJournalDir {
		t.Errorf("StreamBackedDirs: got %v, want [change_journal]", report.StreamBackedDirs)
	}
	// Negative disparity must not trigger an alert (stream-backed; expected state).
	for _, dir := range report.KindsWithDisparity {
		if dir == testChangeJournalDir {
			t.Error("change_journal should not be in KindsWithDisparity (negative disparity expected for stream-backed dir)")
		}
	}

	// LegacyStreamBackedByDir must be populated with the 3 legacy files we created.
	if n := report.LegacyStreamBackedByDir[testChangeJournalDir]; n != 3 {
		t.Errorf("LegacyStreamBackedByDir[change_journal]: got %d, want 3", n)
	}
}

// TestRun_legacyStreamBacked_nestedSubdirs verifies that countYAMLFilesInDir descends into
// subdirectories (e.g. monthly-bucketed audit/YYYY-MM/ structure) and counts all YAML files.
func TestRun_legacyStreamBacked_nestedSubdirs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	processDir := filepath.Join(tmpDir, "docs", testProcessDirName)

	// Simulate monthly-bucketed audit layout: audit/2026-01/ and audit/2026-02/
	for _, sub := range []string{"2026-01", "2026-02"} {
		d := filepath.Join(processDir, testAuditDirName, sub)
		if err := fileutil.MkdirAll(d, testDirPerm); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
		for i := range 5 {
			name := filepath.Join(d, filepath.Clean("legacy-"+string(rune('a'+i))+".yaml"))
			if err := fileutil.WriteFile(name, []byte("id: x\n"), testFilePerm); err != nil {
				t.Fatalf("write: %v", err)
			}
		}
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
		IncludeInternal:            false,
		ObjectCountByKindFromCache: map[string]int{"audit_event": 100},
		StreamBackedDirs:           []string{testAuditDirName},
	}
	report, err := Run(ctx, storageProvider, secCtx, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Disk walk is skipped; DiskCountByDir must remain 0.
	if report.DiskCountByDir[testAuditDirName] != 0 {
		t.Errorf("DiskCountByDir[audit]: got %d, want 0", report.DiskCountByDir[testAuditDirName])
	}
	// LegacyStreamBackedByDir must count all 10 files across both monthly subdirs.
	if n := report.LegacyStreamBackedByDir[testAuditDirName]; n != 10 {
		t.Errorf("LegacyStreamBackedByDir[audit]: got %d, want 10 (5 per month × 2 months)", n)
	}
}

// TestRun_legacyStreamBacked_emptyDir verifies that LegacyStreamBackedByDir has no entry
// (or 0) for a stream-backed dir that contains no legacy YAML files.
func TestRun_legacyStreamBacked_emptyDir(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	processDir := filepath.Join(tmpDir, "docs", testProcessDirName)
	auditDir := filepath.Join(processDir, testAuditDirName)
	if err := fileutil.MkdirAll(auditDir, testDirPerm); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// No YAML files in the dir — only a hidden metadata file.
	if err := fileutil.WriteFile(filepath.Join(auditDir, ".audit.index"), []byte("{}"), testFilePerm); err != nil {
		t.Fatalf("write index: %v", err)
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
		IncludeInternal:            false,
		ObjectCountByKindFromCache: map[string]int{"audit_event": 50},
		StreamBackedDirs:           []string{testAuditDirName},
	}
	report, err := Run(ctx, storageProvider, secCtx, opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// No legacy files → LegacyStreamBackedByDir should have no entry for this dir.
	if n, ok := report.LegacyStreamBackedByDir[testAuditDirName]; ok && n != 0 {
		t.Errorf("LegacyStreamBackedByDir[audit]: got %d, want 0 (no legacy files)", n)
	}
}

// TestRun_strayUnmappedProcessDir alerts when YAML sits in a top-level folder
// that is not a kind mapping (docs/process/decision/ vs canonical decisions/).
// TRACK: BLI-CEF-OCR-STRAY-KIND-DIR-001
func TestRun_strayUnmappedProcessDir(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()
	strayDir := filepath.Join(tmpDir, "docs", testProcessDirName, "decision")
	if err := fileutil.MkdirAll(strayDir, testDirPerm); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(strayDir, "stub.yaml"), []byte("id: DEC-STUB\nkind: decision\n"), testFilePerm); err != nil {
		t.Fatalf("write: %v", err)
	}

	storageProvider, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Skipf("file storage init: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)
	registerCongruenceStorageTeardown(t, tmpDir, storageProvider)
	secCtx := pkgctx.NewSystemSecurityContext()
	report, err := Run(ctx, storageProvider, secCtx, RunOptions{
		ProjectRoot:                tmpDir,
		IncludeInternal:            false,
		ObjectCountByKindFromCache: map[string]int{"decision": 0},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	found := false
	for _, dir := range report.StrayDirs {
		if dir == "decision" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("StrayDirs = %v, want to include %q", report.StrayDirs, "decision")
	}
	if report.DiskCountByDir["decision"] != 1 {
		t.Errorf("DiskCountByDir[decision] = %d, want 1", report.DiskCountByDir["decision"])
	}
}
