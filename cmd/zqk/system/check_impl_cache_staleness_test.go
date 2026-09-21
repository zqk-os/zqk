package system

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// TestHashRegistryCacheStaleness reproduces the issue where the hash registry file
// on disk has the correct hash, but the cache has stale data, causing false-positive
// hash mismatch reports.
//
// This test verifies that:
// 1. The registry file on disk has the correct hash
// 2. The cache can have stale data
// 3. Running check without --auto-fix should reload from disk and not report false mismatches
func TestHashRegistryCacheStaleness(t *testing.T) {
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

	// Create a test change journal entry
	month := "2025-12"
	journalDir := filepath.Join(datacell.CellCASPrimaryDir(projectRoot, "change_journal"), month)
	if err := fileutil.MkdirAll(journalDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create journal dir: %v", err)
	}

	// Create CHA-999.yaml with content
	chaTestPath := filepath.Join(journalDir, "CHA-999.yaml")
	chaTestContent := `id: CHA-999
kind: change_journal_entry
schema_version: "` + objects.DefaultSchemaVersion + `"
status: completed
object_ref: backlog_item:BLI-999
change_type: update
title: "Update: backlog_item:BLI-999"
diff_summary: "Test hash mismatch"
created_at: "2025-12-29T16:00:00Z"
created_by: ACC-SYSTEM
updated_at: "2025-12-29T16:00:00Z"
updated_by: ACC-SYSTEM
origin_system: zqk
origin_project: zqk
previous_state:
  status: validated
`
	if err := fileutil.WriteFile(chaTestPath, []byte(chaTestContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to write CHA-999: %v", err)
	}

	// Calculate the CORRECT hash for CHA-999
	chaTestHash := sha256.Sum256([]byte(chaTestContent))
	chaTestHashStr := hex.EncodeToString(chaTestHash[:])

	// Create hash registry and save the CORRECT hash to disk
	hashRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "change_journal_entry", journalDir)
	hashRegistry.SetSkipShutdownCoordinatorCheck(true)
	hashRegistry.SetHash("CHA-999.yaml", chaTestHashStr)
	if err := hashRegistry.Save(); err != nil {
		t.Fatalf("Failed to save hash registry: %v", err)
	}

	// Verify the registry file on disk has the correct hash
	registryFile := filepath.Join(journalDir, ".change_journal_entry.hashes")
	registryData, err := fileutil.ReadFile(registryFile)
	if err != nil {
		t.Fatalf("Failed to read registry file: %v", err)
	}
	if !strings.Contains(string(registryData), chaTestHashStr) {
		t.Fatalf("Registry file does not contain correct hash. Expected: %s", chaTestHashStr[:16])
	}

	// Now simulate cache staleness: create a NEW registry instance with WRONG hash in memory
	// This simulates what happens when the cache has stale data
	staleRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "change_journal_entry", journalDir)
	wrongHash := "b9b7887952db4c44" + "0000000000000000000000000000000000000000000000000000000000000000"
	staleRegistry.SetHash("CHA-999.yaml", wrongHash)
	// Don't save - this is stale in-memory data

	// Create hash registry cache with stale data
	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}
	hashRegistryCache.Set("change_journal_entry", staleRegistry)

	// Now check integrity - it should reload from disk and use the correct hash
	// Get registry from cache (this should have stale data)
	cachedReg, ok := hashRegistryCache.Get("change_journal_entry")
	if !ok {
		t.Fatal("Registry not in cache")
	}

	// BEFORE FIX: Check if cache has stale data
	staleHash := cachedReg.GetHash("CHA-999.yaml")
	if staleHash != wrongHash {
		t.Fatalf("Expected cache to have stale hash %s, got %s", wrongHash[:16], staleHash[:16])
	}

	// Reload from disk (this is what should happen in CheckKindObjectsWithCache)
	if err := cachedReg.Load(); err != nil {
		t.Fatalf("Failed to reload registry: %v", err)
	}

	// AFTER RELOAD: Check if cache now has correct data
	reloadedHash := cachedReg.GetHash("CHA-999.yaml")
	if reloadedHash != chaTestHashStr {
		t.Errorf("After reload, expected hash %s, got %s", chaTestHashStr[:16], reloadedHash[:16])
		t.Errorf("This indicates the cache reload is not working correctly")
	}

	// Now simulate the integrity check
	// This should use the reloaded hash and NOT report a mismatch
	content, err := fileutil.ReadFile(chaTestPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	fileHash := sha256.Sum256(content)
	fileHashStr := hex.EncodeToString(fileHash[:])

	// The integrity check should pass because:
	// 1. File hash matches registry hash (both are chaTestHashStr)
	// 2. Cache was reloaded from disk
	if reloadedHash != fileHashStr {
		t.Errorf("Hash mismatch detected incorrectly: expected=%s, got=%s", reloadedHash[:16], fileHashStr[:16])
		t.Errorf("This is the bug: cache staleness causes false-positive hash mismatches")
	} else {
		t.Logf("✅ Cache reload works correctly: hash matches after reload (hash=%s)", reloadedHash[:16])
	}
}
