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

// TestHashRegistryCacheIssue reproduces the issue where hash registry cache
// prevents system check from seeing updated hashes
func TestHashRegistryCacheIssue(t *testing.T) {
	// Not t.Parallel(): global shutdown coordinator + hash registry Save() interact across parallel tests.
	// Create a temporary test directory
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	// Create metrics directory structure
	metricsDir := datacell.CellCASPrimaryDir(projectRoot, "metrics")
	if err := fileutil.MkdirAll(metricsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create metrics directory: %v", err)
	}

	// Create a test scheduler_health_metric file
	testFile := filepath.Join(metricsDir, "SHM-TEST-002.yaml")
	content := `id: SHM-TEST-002
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

	if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Step 1: Create and register hash (simulating initial state)
	hashRegistry1 := storage.NewHashRegistry(pkgctx.NewSystemContext(), "scheduler_health_metric", metricsDir)
	hashRegistry1.SetSkipShutdownCoordinatorCheck(true)
	hash1 := calculateFileHash(testFile)
	filename := filepath.Base(testFile)
	hashRegistry1.SetHash(filename, hash1)
	if err := hashRegistry1.Save(); err != nil {
		t.Fatalf("Failed to save hash registry: %v", err)
	}

	// Step 2: Load registry again (simulating system check loading from cache)
	hashRegistry2 := storage.NewHashRegistry(pkgctx.NewSystemContext(), "scheduler_health_metric", metricsDir)
	hashRegistry2.SetSkipShutdownCoordinatorCheck(true)
	if err := hashRegistry2.Load(); err != nil {
		t.Fatalf("Failed to load hash registry: %v", err)
	}

	// Verify hash is loaded
	loadedHash := hashRegistry2.GetHash(filename)
	if loadedHash != hash1 {
		t.Fatalf("Hash not loaded correctly: expected %s, got %s", hash1, loadedHash)
	}

	// Step 3: Modify file (simulating the fix we applied)
	modifiedContent := `id: SHM-TEST-002
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
updated_by: ACC-1785920548450214012-68b850c0
created_by: ACC-1785920548450214012-68b850c0
`

	if err := fileutil.WriteFile(testFile, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Step 4: Update hash registry (simulating fix-hashes command)
	hashRegistry3 := storage.NewHashRegistry(pkgctx.NewSystemContext(), "scheduler_health_metric", metricsDir)
	hashRegistry3.SetSkipShutdownCoordinatorCheck(true)
	if err := hashRegistry3.Load(); err != nil {
		t.Fatalf("Failed to load hash registry for update: %v", err)
	}

	hash2 := calculateFileHash(testFile)
	hashRegistry3.SetHash(filename, hash2)
	if err := hashRegistry3.Save(); err != nil {
		t.Fatalf("Failed to update hash registry: %v", err)
	}

	// Step 5: Load registry again (simulating system check after fix)
	// This should see the updated hash
	hashRegistry4 := storage.NewHashRegistry(pkgctx.NewSystemContext(), "scheduler_health_metric", metricsDir)
	hashRegistry4.SetSkipShutdownCoordinatorCheck(true)
	if err := hashRegistry4.Load(); err != nil {
		t.Fatalf("Failed to reload hash registry: %v", err)
	}

	finalHash := hashRegistry4.GetHash(filename)
	if finalHash != hash2 {
		t.Fatalf("Hash registry not updated: expected %s, got %s", hash2, finalHash)
	}

	// Step 6: Verify current file hash matches
	currentHash := calculateFileHash(testFile)
	if finalHash != currentHash {
		t.Fatalf("Hash mismatch: registry has %s, file has %s", finalHash, currentHash)
	}

	t.Logf("✅ Test passed: Hash registry update works correctly")
	t.Logf("   Original hash: %s", hash1[:16])
	t.Logf("   Updated hash: %s", hash2[:16])
	t.Logf("   Final registry hash: %s", finalHash[:16])
}
