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

// TestHashRegistryCacheInvalidation reproduces the issue where cached hash registries
// prevent system check from seeing updated hashes after fix-hashes runs
func TestHashRegistryCacheInvalidation(t *testing.T) {
	t.Parallel()
	// Create a temporary test directory
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	// Create metrics directory structure
	metricsDir := datacell.CellCASPrimaryDir(projectRoot, "metrics")
	if err := fileutil.MkdirAll(metricsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create metrics directory: %v", err)
	}

	// Create a test scheduler_health_metric file
	testFile := filepath.Join(metricsDir, "SHM-TEST-003.yaml")
	content := `id: SHM-TEST-003
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

	if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Step 1: Create and register original hash (simulating initial state)
	hashRegistry1 := storage.NewHashRegistry(pkgctx.NewSystemContext(), "scheduler_health_metric", metricsDir)
	hashRegistry1.SetSkipShutdownCoordinatorCheck(true)
	hash1 := calculateFileHash(testFile)
	filename := filepath.Base(testFile)
	hashRegistry1.SetHash(filename, hash1)
	if err := hashRegistry1.Save(); err != nil {
		t.Fatalf("Failed to save hash registry: %v", err)
	}

	// Step 2: Simulate system check loading and caching the registry
	// This is what happens in CheckKindObjectsWithCache
	cachedRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "scheduler_health_metric", metricsDir)
	cachedRegistry.SetSkipShutdownCoordinatorCheck(true)
	if err := cachedRegistry.Load(); err != nil {
		t.Fatalf("Failed to load cached registry: %v", err)
	}

	// Verify cached registry has original hash
	cachedHash := cachedRegistry.GetHash(filename)
	if cachedHash != hash1 {
		t.Fatalf("Cached registry hash mismatch: expected %s, got %s", hash1, cachedHash)
	}

	// Step 3: Modify file (simulating the fix we applied - integer to float)
	modifiedContent := `id: SHM-TEST-003
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

	// Step 4: Update hash registry on disk (simulating fix-hashes command)
	// This creates a NEW registry instance and updates it
	updatedRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "scheduler_health_metric", metricsDir)
	updatedRegistry.SetSkipShutdownCoordinatorCheck(true)
	if err := updatedRegistry.Load(); err != nil {
		t.Fatalf("Failed to load registry for update: %v", err)
	}

	hash2 := calculateFileHash(testFile)
	updatedRegistry.SetHash(filename, hash2)
	if err := updatedRegistry.Save(); err != nil {
		t.Fatalf("Failed to update hash registry: %v", err)
	}

	// Step 5: The problem - cached registry still has old hash!
	// This simulates what happens when system check uses a cached registry
	cachedHashAfterUpdate := cachedRegistry.GetHash(filename)
	if cachedHashAfterUpdate == hash2 {
		t.Logf("✅ Cached registry was automatically updated (unexpected but good)")
	} else {
		t.Logf("⚠️  Cached registry still has old hash: %s (expected: %s)", cachedHashAfterUpdate[:16], hash2[:16])
		t.Logf("   This is the bug - cached registry needs to be reloaded!")
	}

	// Step 6: Reload the cached registry (this is what should happen)
	if err := cachedRegistry.Load(); err != nil {
		t.Fatalf("Failed to reload cached registry: %v", err)
	}

	reloadedHash := cachedRegistry.GetHash(filename)
	if reloadedHash != hash2 {
		t.Fatalf("Reloaded registry hash mismatch: expected %s, got %s", hash2, reloadedHash)
	}

	// Step 7: Verify current file hash matches
	currentHash := calculateFileHash(testFile)
	if reloadedHash != currentHash {
		t.Fatalf("Hash mismatch after reload: registry has %s, file has %s", reloadedHash, currentHash)
	}

	t.Logf("✅ Test passed: Hash registry cache invalidation works after reload")
	t.Logf("   Original hash: %s", hash1[:16])
	t.Logf("   Updated hash: %s", hash2[:16])
	t.Logf("   Reloaded hash: %s", reloadedHash[:16])
	t.Logf("")
	t.Logf("   Issue: Cached registries need to be reloaded after hash updates")
	t.Logf("   Solution: Clear hash registry cache or reload after fix-hashes")
}
