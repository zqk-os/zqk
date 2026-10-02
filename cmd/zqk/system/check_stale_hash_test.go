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

// TestStaleHashInValidationCache reproduces the issue where validation cache
// stores stale expected hashes that don't match the current registry
func TestStaleHashInValidationCache(t *testing.T) {
	t.Parallel()
	// Create a temporary test directory
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	// Create requirements directory structure
	requirementsDir := datacell.CellCASPrimaryDir(projectRoot, "requirements")
	if err := fileutil.MkdirAll(requirementsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create requirements directory: %v", err)
	}

	// Step 1: Create a test file with original content
	testFile := filepath.Join(requirementsDir, "REQ-TEST-001.yaml")
	originalContent := `id: REQ-TEST-001
kind: requirement
schema_version: "` + objects.DefaultSchemaVersion + `"
status: active
title: Test Requirement
created_at: "2026-01-04T20:15:14Z"
updated_at: "2026-01-04T20:15:14Z"
created_by: ACC-SYSTEM
updated_by: ACC-SYSTEM
`

	if err := fileutil.WriteFile(testFile, []byte(originalContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Step 2: Register original hash
	hashRegistry1 := storage.NewHashRegistry(pkgctx.NewSystemContext(), "requirement", requirementsDir)
	hashRegistry1.SetSkipShutdownCoordinatorCheck(true)
	hash1 := calculateFileHash(testFile)
	filename := filepath.Base(testFile)
	hashRegistry1.SetHash(filename, hash1)
	if err := hashRegistry1.Save(); err != nil {
		t.Fatalf("Failed to save hash registry: %v", err)
	}

	t.Logf("Step 2: Registered original hash: %s", hash1[:16])

	// Step 3: Modify file (simulating a fix that changes the hash)
	modifiedContent := `id: REQ-TEST-001
kind: requirement
schema_version: "` + objects.DefaultSchemaVersion + `"
status: active
title: Test Requirement Modified
created_at: "2026-01-04T20:15:14Z"
updated_at: "2026-01-04T20:15:14Z"
created_by: ACC-SYSTEM
updated_by: ACC-SYSTEM
`

	if err := fileutil.WriteFile(testFile, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to modify test file: %v", err)
	}

	hash2 := calculateFileHash(testFile)
	if hash2 == hash1 {
		t.Fatalf("File hash should have changed after modification")
	}

	t.Logf("Step 3: File modified, new hash: %s", hash2[:16])

	// Step 4: Update hash registry (simulating fix-hashes command)
	hashRegistry2 := storage.NewHashRegistry(pkgctx.NewSystemContext(), "requirement", requirementsDir)
	hashRegistry2.SetSkipShutdownCoordinatorCheck(true)
	if err := hashRegistry2.Load(); err != nil {
		t.Fatalf("Failed to load registry for update: %v", err)
	}

	hashRegistry2.SetHash(filename, hash2)
	if err := hashRegistry2.Save(); err != nil {
		t.Fatalf("Failed to update hash registry: %v", err)
	}

	t.Logf("Step 4: Updated registry with new hash: %s", hash2[:16])

	// Step 5: Verify registry has correct hash
	hashRegistry3 := storage.NewHashRegistry(pkgctx.NewSystemContext(), "requirement", requirementsDir)
	if err := hashRegistry3.Load(); err != nil {
		t.Fatalf("Failed to reload registry: %v", err)
	}

	registryHash := hashRegistry3.GetHash(filename)
	if registryHash != hash2 {
		t.Fatalf("Registry should have updated hash: expected %s, got %s", hash2, registryHash)
	}

	t.Logf("Step 5: Verified registry has correct hash: %s", registryHash[:16])

	// Step 6: Simulate validation using directoryRegistryPool
	// This is what system check actually does
	pool := &directoryRegistryPool{}
	registryForValidation := pool.GetOrCreate(pkgctx.NewSystemContext(), "requirement", requirementsDir)

	// Step 7: Check what hash validation would see
	expectedHash := registryForValidation.GetHash(filename)
	if expectedHash != hash2 {
		t.Fatalf("Validation should see updated hash: expected %s, got %s", hash2[:16], expectedHash[:16])
	}

	t.Logf("Step 7: Validation sees correct hash: %s", expectedHash[:16])

	// Step 8: Verify file hash matches
	currentFileHash := calculateFileHash(testFile)
	if expectedHash != currentFileHash {
		t.Fatalf("Hash mismatch: registry has %s, file has %s", expectedHash[:16], currentFileHash[:16])
	}

	t.Logf("✅ Test passed: No hash mismatch detected")
	t.Logf("   Registry hash: %s", expectedHash[:16])
	t.Logf("   File hash: %s", currentFileHash[:16])
}
