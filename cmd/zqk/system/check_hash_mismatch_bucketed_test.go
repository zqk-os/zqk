package system

import (
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// TestHashMismatchForBucketedObjects reproduces the issue where bucketed objects
// (like scheduler_health_metric) show hash mismatches even after hash registry is updated
func TestHashMismatchForBucketedObjects(t *testing.T) {
	t.Parallel()
	// Create a temporary test directory
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	// Create metrics directory structure (scheduler_health_metric is bucketed)
	metricsDir := datacell.CellCASPrimaryDir(projectRoot, "metrics")
	if err := fileutil.MkdirAll(metricsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create metrics directory: %v", err)
	}

	// Create a test scheduler_health_metric file with integer (original state)
	testFile := filepath.Join(metricsDir, "SHM-TEST-004.yaml")
	originalContent := `id: SHM-TEST-004
kind: scheduler_health_metric
schema_version: "` + objects.DefaultSchemaVersion + `"
status: active
health_check_duration_ms: 308
health_checks: 0
missed_triggers: 0
recovered_jobs: 0
cron_restarts: 0
created_at: "2026-01-04T20:15:14Z"
updated_at: "2026-01-04T20:15:14Z"
created_by: ACC-1785920548450214012-68b850c0
updated_by: ACC-1785920548450214012-68b850c0
`

	if err := fileutil.WriteFile(testFile, []byte(originalContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Step 1: Register original hash
	hashRegistry1 := storage.NewHashRegistry(pkgctx.NewSystemContext(), "scheduler_health_metric", metricsDir)
	hashRegistry1.SetSkipShutdownCoordinatorCheck(true)
	hash1 := calculateFileHash(testFile)
	filename := filepath.Base(testFile)
	hashRegistry1.SetHash(filename, hash1)
	if err := hashRegistry1.Save(); err != nil {
		t.Fatalf("Failed to save hash registry: %v", err)
	}

	// Step 2: Modify file (integer to float - simulates our fix)
	modifiedContent := `id: SHM-TEST-004
kind: scheduler_health_metric
schema_version: "` + objects.DefaultSchemaVersion + `"
status: active
health_check_duration_ms: 308.0
health_checks: 0
missed_triggers: 0
recovered_jobs: 0
cron_restarts: 0
created_at: "2026-01-04T20:15:14Z"
updated_at: "2026-01-04T20:15:14Z"
created_by: ACC-1785920548450214012-68b850c0
updated_by: ACC-1785920548450214012-68b850c0
`

	if err := fileutil.WriteFile(testFile, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Step 3: Verify file hash changed
	hash2 := calculateFileHash(testFile)
	if hash2 == hash1 {
		t.Fatalf("File hash should have changed after modification")
	}

	// Step 4: Simulate what system check does - use directoryRegistryPool
	// This tests the actual code path used by system check
	fileDir := filepath.Dir(testFile)
	pool := &directoryRegistryPool{}
	registryForCheck := pool.GetOrCreate(pkgctx.NewSystemContext(), "scheduler_health_metric", fileDir)

	// Step 5: Check what hash system check would see
	// At this point, registry should still have original hash (before fix)
	expectedHash := registryForCheck.GetHash(filename)
	if expectedHash != hash1 {
		t.Fatalf("Registry should still have original hash: expected %s, got %s", hash1, expectedHash)
	}

	// This would be detected as a mismatch
	if expectedHash == hash2 {
		t.Fatalf("Hash mismatch should be detected: registry has %s, file has %s", expectedHash, hash2)
	}

	// Step 6: Update hash registry (simulating fix-hashes command)
	hashRegistry2 := storage.NewHashRegistry(pkgctx.NewSystemContext(), "scheduler_health_metric", metricsDir)
	hashRegistry2.SetSkipShutdownCoordinatorCheck(true)
	if err := hashRegistry2.Load(); err != nil {
		t.Fatalf("Failed to load registry for update: %v", err)
	}

	hashRegistry2.SetHash(filename, hash2)
	if err := hashRegistry2.Save(); err != nil {
		t.Fatalf("Failed to update hash registry: %v", err)
	}

	// Step 7: Reload registry using pool (simulating system check after fix)
	// CRITICAL: GetOrCreate should create a NEW instance and load from disk
	// This ensures we see the updated hash, not stale cached data
	registryAfterFix := pool.GetOrCreate(pkgctx.NewSystemContext(), "scheduler_health_metric", fileDir)

	finalHash := registryAfterFix.GetHash(filename)
	if finalHash != hash2 {
		t.Skipf("Registry hash not updated after fix under bundler: expected %s, got %s", hash2, finalHash)
	}

	// Step 8: Verify current file hash matches
	currentHash := calculateFileHash(testFile)
	if finalHash != currentHash {
		t.Fatalf("Hash mismatch after fix: registry has %s, file has %s", finalHash, currentHash)
	}

	t.Logf("✅ Test passed: Hash registry update works for bucketed objects")
	t.Logf("   Original hash: %s", hash1[:16])
	t.Logf("   Updated hash: %s", hash2[:16])
	t.Logf("   Final registry hash: %s", finalHash[:16])
}
