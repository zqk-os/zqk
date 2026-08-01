package system

import (
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestRunStaleCASCleanupForResults_EmptyResults(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	result := RunStaleCASCleanupForResults("", nil, logger)
	if len(result.KindsRun) != 0 || result.FilesHandled != 0 {
		t.Errorf("empty results: expected zero kinds and files, got kinds=%d files=%d",
			len(result.KindsRun), result.FilesHandled)
	}
}

func TestRunStaleCASCleanupForResults_NoStaleCASMessage(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	results := []CheckResult{
		{ObjectID: "ITEM-1", ObjectKind: "backlog_item", Issues: []Issue{
			{Message: "CAS index out of sync"},
		}},
	}
	result := RunStaleCASCleanupForResults("/tmp", results, logger)
	if len(result.KindsRun) != 0 || result.FilesHandled != 0 {
		t.Errorf("no Stale CAS message: expected zero kinds and files, got kinds=%v files=%d",
			result.KindsRun, result.FilesHandled)
	}
}

func TestRunStaleCASCleanupForResults_CollectsKinds(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	results := []CheckResult{
		{ObjectID: "ITEM-1", ObjectKind: "backlog_item", Issues: []Issue{
			{Message: "Stale CAS version detected for object ITEM-1: current index hash abc... differs from this file hash def...."},
		}},
		{ObjectID: "ITEM-2", ObjectKind: "backlog_item", Issues: []Issue{
			{Message: "Stale CAS version detected for object ITEM-2."},
		}},
		{ObjectID: "ADR-1", ObjectKind: "decision", Issues: []Issue{
			{Message: "Stale CAS version detected for object ADR-1."},
		}},
	}
	// Use empty project root so RunHashDuplicatesCleanupForKinds returns early (no work)
	result := RunStaleCASCleanupForResults("", results, logger)
	// With empty projectRoot, RunHashDuplicatesCleanupForKinds returns 0, nil without running
	if len(result.KindsRun) != 2 {
		t.Errorf("expected 2 kinds (backlog_item, decision), got %v", result.KindsRun)
	}
	if result.FilesHandled != 0 {
		t.Errorf("empty project root: expected 0 files handled, got %d", result.FilesHandled)
	}
}

func TestRunHashDuplicatesCleanupForKinds_EmptyInputs(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	total, errs := RunHashDuplicatesCleanupForKinds("", nil, false, false, logger, nil, nil)
	if total != 0 || len(errs) != 0 {
		t.Errorf("empty inputs: expected 0, nil, got total=%d errs=%d", total, len(errs))
	}
	total, errs = RunHashDuplicatesCleanupForKinds("/nonexistent", []string{}, false, false, logger, nil, nil)
	if total != 0 || len(errs) != 0 {
		t.Errorf("empty kinds: expected 0, nil, got total=%d errs=%d", total, len(errs))
	}
}

// TestCleanupHashDuplicatesForKind_SkipsMissingPaths_NoError ensures that when the duplicate list
// contains a path that no longer exists (e.g. index stale or file already deleted), we skip it and
// do not report it as an error. This prevents "Stale CAS cleanup reported error ... no such file"
// spam when the CAS index references deleted files.
func TestCleanupHashDuplicatesForKind_SkipsMissingPaths_NoError(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmp)
	kindDir := objects.GetDirectoryFromKind("scheduler_job")
	if kindDir == emptyValue {
		t.Fatal("scheduler_job kind has no directory")
	}
	dirPath := filepath.Join(processDir, kindDir)
	if err := os.MkdirAll(dirPath, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// One real hash-named file (64 hex + .yaml)
	hashName := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.yaml"
	realPath := filepath.Join(dirPath, hashName)
	if err := os.WriteFile(realPath, []byte("id: SCH-test\n"), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Path that does not exist (simulates index/cache listing a file that was later deleted)
	fakePath := filepath.Join(dirPath, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff.yaml")
	ctx := &CleanupDuplicatesContext{
		ProjectRoot:                  tmp,
		ProcessDir:                   processDir,
		HashDuplicates:               true,
		DryRun:                       true,
		QuarantineDir:                filepath.Join(tmp, paths.ProjectDataDir, "system-health", "quarantine", "hash-duplicates"),
		Logger:                       logging.GetLoggerFromProfile("test"),
		DeleteHashDuplicatesForKinds: map[string]bool{"scheduler_job": true},
	}
	if storage.GetGlobalListingIndexWriteQueue() == nil {
		t.Skip("CAS write queue not set (run with storage initialized)")
	}
	duplicates := map[string][]string{"SCH-test": {realPath, fakePath}}
	_, _, errs := cleanupHashDuplicatesForKind(ctx, "scheduler_job", dirPath, duplicates)
	for _, e := range errs {
		if e != nil && errors.Is(e, os.ErrNotExist) {
			t.Errorf("missing path must not be reported as error (skip only): %v", e)
		}
	}
}

func TestRunStaleCASCleanupForResults_PrunesIndex(t *testing.T) {
	tmp := t.TempDir()
	// Create kind directory docs/process/scheduler_jobs
	kindDir := filepath.Join(tmp, "docs/process/scheduler_jobs")
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Create a CAS index with a mapping
	cas := storage.NewContentAddressableStorage(kindDir, "scheduler_job")
	index := cas.GetIndex()
	if index == nil {
		t.Fatal("expected non-nil index from NewContentAddressableStorage")
	}

	// Add mapping to index
	index.Mappings = map[string]string{
		"SCH-run-bundle-21": "abcdef1234567890",
		"SCH-good-job":      "1234567890abcdef",
	}
	if err := index.RemoveMapping("nonexistent"); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}

	// Now run RunStaleCASCleanupForResults on a result with "Missing CAS file"
	results := []CheckResult{
		{
			ObjectID:   "SCH-run-bundle-21",
			ObjectKind: "scheduler_job",
			Issues: []Issue{
				{Message: "Missing CAS file detected: stale index entry for object SCH-run-bundle-21"},
			},
		},
	}

	logger := logging.GetLoggerFromProfile("test")
	_ = RunStaleCASCleanupForResults(tmp, results, logger)

	// Verify that SCH-run-bundle-21 was removed, but SCH-good-job remains
	// Re-load the CAS index from disk
	cas2 := storage.NewContentAddressableStorage(kindDir, "scheduler_job")
	index2 := cas2.GetIndex()
	if index2 == nil {
		t.Fatal("expected non-nil index2")
	}

	mappings := index2.Mappings
	if _, exists := mappings["SCH-run-bundle-21"]; exists {
		t.Error("expected SCH-run-bundle-21 to be removed from index mappings")
	}
	if _, exists := mappings["SCH-good-job"]; !exists {
		t.Error("expected SCH-good-job to remain in index mappings")
	}
}
