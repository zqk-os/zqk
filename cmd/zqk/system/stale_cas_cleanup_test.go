package system

import (
	"github.com/lanceman/zqk/pkg/datacell"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"errors"
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
		{ObjectID: "BLI-1", ObjectKind: "backlog_item", Issues: []Issue{
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
		{ObjectID: "BLI-1", ObjectKind: "backlog_item", Issues: []Issue{
			{Message: "Stale CAS version detected for object BLI-1: current index hash abc... differs from this file hash def...."},
		}},
		{ObjectID: "BLI-2", ObjectKind: "backlog_item", Issues: []Issue{
			{Message: "Stale CAS version detected for object BLI-2."},
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
	if err := fileutil.MkdirAll(dirPath, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// One real hash-named file (64 hex + .yaml)
	hashName := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.yaml"
	realPath := filepath.Join(dirPath, hashName)
	if err := fileutil.WriteFile(realPath, []byte("id: SCH-test\n"), paths.FilePerm644); err != nil {
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
	if caspkg.GetGlobalListingIndexWriteQueue() == nil {
		t.Skip("CAS write queue not set (run with storage initialized)")
	}
	duplicates := map[string][]string{"SCH-test": {realPath, fakePath}}
	_, _, errs := cleanupHashDuplicatesForKind(ctx, "scheduler_job", dirPath, duplicates)
	for _, e := range errs {
		if e != nil && errors.Is(e, fileutil.ErrNotExist) {
			t.Errorf("missing path must not be reported as error (skip only): %v", e)
		}
	}
}

func TestRunStaleCASCleanupForResults_PrunesIndex(t *testing.T) {
	tmp := t.TempDir()
	// Create kind directory paths.ProcessDir/scheduler_jobs
	kindDir := filepath.Join(tmp, paths.ProcessDir, "scheduler_jobs")
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

// TestCASMembraneHealing_FunctionalAcceptance verifies that duplicate CAS blob violations
// are marked AutoFixable: true and that RunStaleCASCleanupForResults identifies duplicate CAS
// issues and schedules cleanup for those kinds (CRIT-REDACTED).
func TestCASMembraneHealing_FunctionalAcceptance(t *testing.T) {
	t.Parallel()

	// Verify appendCASDuplicateIDCheckResults marks issues as AutoFixable
	inv := caspkg.CASDuplicateIDInventory{
		DuplicateCount: 1,
		Hits: []caspkg.CASDuplicateIDHit{
			{
				Kind:       "backlog_item",
				Dir:        "backlog",
				ObjectID:   "BLI-DUP-TEST-001",
				Paths:      []string{"/tmp/a.yaml", "/tmp/b.yaml"},
				KeeperPath: "/tmp/b.yaml",
			},
		},
	}
	results := appendCASDuplicateIDCheckResults(nil, inv)
	if len(results) == 0 || len(results[0].Issues) == 0 {
		t.Fatalf("expected check results with issues for duplicate CAS inventory")
	}
	issue := results[0].Issues[0]
	if !issue.AutoFixable {
		t.Errorf("expected duplicate CAS issue to be AutoFixable=true, got false")
	}
	if issue.FixCommand == "" {
		t.Errorf("expected non-empty FixCommand on AutoFixable duplicate CAS issue")
	}

	// Verify RunStaleCASCleanupForResults collects kinds for duplicate CAS blobs
	logger := logging.GetLoggerFromProfile("test")
	cleanResult := RunStaleCASCleanupForResults("", results, logger)
	foundKind := false
	for _, k := range cleanResult.KindsRun {
		if k == "backlog_item" {
			foundKind = true
			break
		}
	}
	// With empty projectRoot, RunHashDuplicatesCleanupForKinds returns early, but KindsRun is populated
	if !foundKind {
		t.Errorf("expected kindsRun to include 'backlog_item', got: %v", cleanResult.KindsRun)
	}
}

// TestCASMembraneHealing_BoundaryAndErrorHandling verifies that when a CAS index entry
// points to a missing hash file, stale CAS cleanup uses DiscoverCASFilePathByScanning
// to locate any live hash file for that object ID and re-links the index; if none exists,
// it cleanly removes the dead mapping without erroring (CRIT-REDACTED).
func TestCASMembraneHealing_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	kindDir := filepath.Join(tmp, paths.ProcessBacklogDir)
	if err := fileutil.MkdirAll(kindDir, 0o755); err != nil {
		t.Fatalf("failed to create kind dir: %v", err)
	}

	// Write a live on-disk hash file for BLI-RELINK-001 with hash "1111222233334444"
	liveHash := "1111222233334444555566667777888899990000aaaabbbbccccddddeeeeffff"
	liveFile := filepath.Join(kindDir, liveHash+".yaml")
	fileBody := "id: BLI-RELINK-001\nkind: backlog_item\ntitle: Relinked Item\n"
	if err := fileutil.WriteFile(liveFile, []byte(fileBody), 0o644); err != nil {
		t.Fatalf("failed to write live file: %v", err)
	}

	// Initialize CAS index pointing BLI-RELINK-001 to a missing hash
	cas := storage.NewContentAddressableStorage(kindDir, "backlog_item")
	index := cas.GetIndex()
	if index == nil {
		t.Fatal("expected non-nil index")
	}
	index.Mappings = map[string]string{
		"BLI-RELINK-001": "deaddeaddeaddeaddeaddeaddeaddeaddeaddeaddeaddeaddeaddeaddeaddead",
		"BLI-DEAD-002":   "0000000000000000000000000000000000000000000000000000000000000000",
	}
	if err := index.RemoveMapping("nonexistent"); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}

	// Run cleanup with missing CAS file issues for both
	results := []CheckResult{
		{
			ObjectID:   "BLI-RELINK-001",
			ObjectKind: "backlog_item",
			Issues:     []Issue{{Message: "Missing CAS file detected: stale index entry for object BLI-RELINK-001"}},
		},
		{
			ObjectID:   "BLI-DEAD-002",
			ObjectKind: "backlog_item",
			Issues:     []Issue{{Message: "no such file or directory: failed to read file"}},
		},
	}

	logger := logging.GetLoggerFromProfile("test")
	_ = RunStaleCASCleanupForResults(tmp, results, logger)

	// Re-load the CAS index
	cas2 := storage.NewContentAddressableStorage(kindDir, "backlog_item")
	index2 := cas2.GetIndex()
	if index2 == nil {
		t.Fatal("expected non-nil index2")
	}

	// BLI-RELINK-001 should be re-linked to liveHash
	if gotHash := index2.Mappings["BLI-RELINK-001"]; gotHash != liveHash {
		t.Errorf("expected BLI-RELINK-001 to be re-linked to %s, got %s", liveHash, gotHash)
	}

	// BLI-DEAD-002 had no live hash file, so it should be pruned cleanly
	if _, exists := index2.Mappings["BLI-DEAD-002"]; exists {
		t.Errorf("expected BLI-DEAD-002 to be pruned from index mappings")
	}
}

// TestCASMembraneHealing_IntegrationAndConformance verifies that CheckResult issue deduplication
// and clean result structures conform to system check standards without error (CRIT-REDACTED).
func TestCASMembraneHealing_IntegrationAndConformance(t *testing.T) {
	t.Parallel()

	rawIssues := []Issue{
		{Tier: 1, Category: "registration", Message: "Duplicate blob for ID A"},
		{Tier: 1, Category: "registration", Message: "Duplicate blob for ID A"},
		{Tier: 2, Category: "lifecycle", Message: "Unrelated lifecycle warning"},
	}

	deduped := DedupeCheckResultIssues(rawIssues)
	if len(deduped) != 2 {
		t.Errorf("expected 2 unique issues after dedupe, got %d", len(deduped))
	}
}
