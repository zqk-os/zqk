package system

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// TestHashMismatchDetectionAndFix tests the hash mismatch detection and fix flow
// This reproduces the issue where files are modified but system check still reports mismatches
func TestHashMismatchDetectionAndFix(t *testing.T) {
	t.Parallel()
	// Create a temporary test directory
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	// Create metrics directory structure
	metricsDir := datacell.CellCASPrimaryDir(projectRoot, "metrics")
	if err := fileutil.MkdirAll(metricsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create metrics directory: %v", err)
	}

	// Create a test scheduler_health_metric file with integer value (original state)
	testFile := filepath.Join(metricsDir, "SHM-TEST-001.yaml")
	originalContent := `id: SHM-TEST-001
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

	// Step 1: Register the original hash
	hashRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "scheduler_health_metric", metricsDir)
	hashRegistry.SetSkipShutdownCoordinatorCheck(true)
	if err := hashRegistry.Load(); err != nil {
		// Registry doesn't exist yet, that's fine
	}

	// Calculate and register original hash
	originalHash := calculateFileHash(testFile)
	filename := filepath.Base(testFile)
	hashRegistry.SetHash(filename, originalHash)
	if err := hashRegistry.Save(); err != nil {
		t.Fatalf("Failed to save hash registry: %v", err)
	}

	// Verify hash is registered
	registeredHash := hashRegistry.GetHash(filename)
	if registeredHash != originalHash {
		t.Fatalf("Hash registration failed: expected %s, got %s", originalHash, registeredHash)
	}

	// Step 2: Modify the file (change integer to float - simulates the fix we applied)
	modifiedContent := `id: SHM-TEST-001
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

	// Step 3: Verify the file hash changed
	newHash := calculateFileHash(testFile)
	if newHash == originalHash {
		t.Fatalf("File hash should have changed after modification")
	}

	// Step 4: Check if system check would detect the mismatch
	// Load the registry again (simulating what system check does)
	hashRegistry2 := storage.NewHashRegistry(pkgctx.NewSystemContext(), "scheduler_health_metric", metricsDir)
	hashRegistry2.SetSkipShutdownCoordinatorCheck(true)
	if err := hashRegistry2.Load(); err != nil {
		t.Fatalf("Failed to load hash registry: %v", err)
	}

	registeredHash2 := hashRegistry2.GetHash(filename)
	if registeredHash2 != originalHash {
		t.Fatalf("Hash registry should still have original hash: expected %s, got %s", originalHash, registeredHash2)
	}

	// This should detect a mismatch
	if registeredHash2 == newHash {
		t.Fatalf("Hash mismatch should be detected: registry has %s, file has %s", registeredHash2, newHash)
	}

	// Step 5: Update the hash registry with the new hash (simulating fix-hashes)
	hashRegistry2.SetHash(filename, newHash)
	if err := hashRegistry2.Save(); err != nil {
		t.Fatalf("Failed to update hash registry: %v", err)
	}

	// Step 6: Verify the hash is now correct
	hashRegistry3 := storage.NewHashRegistry(pkgctx.NewSystemContext(), "scheduler_health_metric", metricsDir)
	if err := hashRegistry3.Load(); err != nil {
		t.Fatalf("Failed to reload hash registry: %v", err)
	}

	registeredHash3 := hashRegistry3.GetHash(filename)
	if registeredHash3 != newHash {
		t.Fatalf("Hash registry should have new hash: expected %s, got %s", newHash, registeredHash3)
	}

	// Step 7: Verify current file hash matches registry
	currentHash := calculateFileHash(testFile)
	if registeredHash3 != currentHash {
		t.Fatalf("Hash mismatch still exists after fix: registry has %s, file has %s", registeredHash3, currentHash)
	}

	t.Logf("✅ Test passed: Hash mismatch detection and fix works correctly")
	t.Logf("   Original hash: %s", originalHash[:16])
	t.Logf("   New hash: %s", newHash[:16])
	t.Logf("   Final registry hash: %s", registeredHash3[:16])
}

// calculateFileHash calculates SHA256 hash of a file
func calculateFileHash(filePath string) string {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
