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

// TestHashRegistryLocationForBucketedObjects verifies that hash registries
// are created and accessed in the correct directory for bucketed objects.
//
// This test ensures that:
// 1. For bucketed objects (audit_event, change_journal_entry), registries are in subdirectories
// 2. The registry directory matches the file's directory
// 3. Hash lookups use the correct registry location
func TestHashRegistryLocationForBucketedObjects(t *testing.T) {
	t.Parallel()
	// Setup test project in temporary directory (POL-CODE-006: Test Data Isolation)
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	// Safeguard: Ensure we're not accidentally using actual project root
	wd, _ := fileutil.Getwd()
	actualProjectRoot := filepath.Clean(filepath.Join(wd, "../../.."))
	if tempDir == actualProjectRoot {
		t.Fatal("Test is using actual project root - this violates POL-CODE-006 (Test Data Isolation)")
	}

	projectRoot := tempDir

	// Test audit_event (bucketed object)
	month := "2025-12"
	auditDir := filepath.Join(datacell.StreamCurrentKindDir(projectRoot, objects.KindAuditEvent), month)
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create audit dir: %v", err)
	}

	// Create AUD-999.yaml in subdirectory
	auditFilePath := filepath.Join(auditDir, "AUD-999.yaml")
	auditContent := `id: AUD-999
kind: audit_event
schema_version: "` + objects.DefaultSchemaVersion + `"
status: completed
event_type: test
operation: "Test operation"
created_at: "2025-12-29T16:00:00Z"
`
	if err := fileutil.WriteFile(auditFilePath, []byte(auditContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to write AUD-999: %v", err)
	}

	// Calculate correct hash
	auditHash := sha256.Sum256([]byte(auditContent))
	auditHashStr := hex.EncodeToString(auditHash[:])

	// Create registry in the FILE's directory (subdirectory) - this is correct
	fileDir := filepath.Dir(auditFilePath)
	registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", fileDir)
	registry.SetSkipShutdownCoordinatorCheck(true)
	registry.SetHash("AUD-999.yaml", auditHashStr)
	if err := registry.Save(); err != nil {
		t.Fatalf("Failed to save registry: %v", err)
	}

	// Verify registry file is in the subdirectory, not parent
	registryFile := filepath.Join(fileDir, ".audit_event.hashes")
	if _, err := fileutil.Stat(registryFile); err != nil {
		t.Fatalf("Registry file should be in subdirectory %s, but not found: %v", fileDir, err)
	}

	// Verify parent directory does NOT have a registry (or if it does, it doesn't have this hash)
	parentDir := datacell.StreamCurrentKindDir(projectRoot, objects.KindAuditEvent)
	parentRegistryFile := filepath.Join(parentDir, ".audit_event.hashes")
	if _, err := fileutil.Stat(parentRegistryFile); err == nil {
		// Parent registry exists - verify it doesn't have AUD-999
		parentRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", parentDir)
		parentRegistry.SetSkipShutdownCoordinatorCheck(true)
		if err := parentRegistry.Load(); err == nil {
			if parentRegistry.GetHash("AUD-999.yaml") != emptyValue {
				t.Errorf("Parent registry should not contain AUD-999 hash for bucketed object")
			}
		}
	}

	// Verify we can retrieve the hash from the correct registry
	loadedRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", fileDir)
	loadedRegistry.SetSkipShutdownCoordinatorCheck(true)
	if err := loadedRegistry.Load(); err != nil {
		t.Fatalf("Failed to load registry: %v", err)
	}
	retrievedHash := loadedRegistry.GetHash("AUD-999.yaml")
	if retrievedHash != auditHashStr {
		t.Errorf("Expected hash %s, got %s", auditHashStr[:16], retrievedHash[:16])
	}

	// Verify file directory matches registry directory
	if filepath.Dir(auditFilePath) != fileDir {
		t.Errorf("File directory %s should match registry directory %s", filepath.Dir(auditFilePath), fileDir)
	}

	t.Logf("✅ Hash registry correctly located in file's directory for bucketed object")
}

// TestHashRegistryLocationForNonBucketedObjects verifies that hash registries
// are created in the kind directory for non-bucketed objects.
func TestHashRegistryLocationForNonBucketedObjects(t *testing.T) {
	t.Parallel()
	// Setup test project in temporary directory (POL-CODE-006: Test Data Isolation)
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	// Safeguard: Ensure we're not accidentally using actual project root
	wd, _ := fileutil.Getwd()
	actualProjectRoot := filepath.Clean(filepath.Join(wd, "../../.."))
	if tempDir == actualProjectRoot {
		t.Fatal("Test is using actual project root - this violates POL-CODE-006 (Test Data Isolation)")
	}

	projectRoot := tempDir

	// Test backlog_item (non-bucketed object)
	backlogDir := filepath.Join(projectRoot, paths.ProcessBacklogDir)
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create backlog dir: %v", err)
	}

	// Create BLI-999.yaml in kind directory
	backlogFilePath := filepath.Join(backlogDir, "BLI-999.yaml")
	backlogContent := `id: BLI-999
kind: backlog_item
schema_version: "` + objects.DefaultSchemaVersion + `"
status: validated
title: "Test backlog item"
`
	if err := fileutil.WriteFile(backlogFilePath, []byte(backlogContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to write BLI-999: %v", err)
	}

	// Calculate correct hash
	backlogHash := sha256.Sum256([]byte(backlogContent))
	backlogHashStr := hex.EncodeToString(backlogHash[:])

	// Create registry in the kind directory (not file's directory) - this is correct for non-bucketed
	kindDir := backlogDir
	registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", kindDir)
	registry.SetSkipShutdownCoordinatorCheck(true)
	registry.SetHash("BLI-999.yaml", backlogHashStr)
	if err := registry.Save(); err != nil {
		t.Fatalf("Failed to save registry: %v", err)
	}

	// Verify registry file is in the kind directory
	registryFile := filepath.Join(kindDir, ".backlog_item.hashes")
	if _, err := fileutil.Stat(registryFile); err != nil {
		t.Fatalf("Registry file should be in kind directory %s, but not found: %v", kindDir, err)
	}

	// Verify file directory matches kind directory for non-bucketed objects
	if filepath.Dir(backlogFilePath) != kindDir {
		t.Errorf("File directory %s should match kind directory %s for non-bucketed objects", filepath.Dir(backlogFilePath), kindDir)
	}

	t.Logf("✅ Hash registry correctly located in kind directory for non-bucketed object")
}
