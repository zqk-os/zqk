package cas_test

import (
	"github.com/lanceman/zqk/pkg/storage"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestCAS_TamperDetection_HashMismatch tests that tampering is detected
// when the content of a hash-based file is modified
func TestCAS_TamperDetection_HashMismatch(t *testing.T) {
	// Create temporary test environment
	testDir := t.TempDir()
	kindDir := filepath.Join(testDir, "metrics")
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create kind directory: %v", err)
	}

	// Create CAS instance
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, "test_audit_aggregation_metric", casQueue)

	// Create an object
	objectID := "TAM-001"
	originalData := []byte(`id: TAM-001
kind: test_audit_aggregation_metric
title: Test Object
status: exploring
schema_version: "` + objects.DefaultSchemaVersion + `"
collection_count: 1
first_seen: "2030-01-05T00:00:00Z"
last_seen: "2030-01-05T00:00:00Z"`)

	if err := cas.Create(objectID, originalData); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Wait for index updates to be processed
	if err := storage.FlushListingIndexForKind("test_audit_aggregation_metric"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Get the hash and file path
	hash, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get hash: %v", err)
	}
	hashFile := filepath.Join(kindDir, hash+".yaml")

	// Verify object can be read initially
	_, err = cas.Read(objectID)
	if err != nil {
		t.Fatalf("Failed to read object initially: %v", err)
	}

	// Tamper with the file: modify content directly
	tamperedData := []byte(`id: TAM-001
kind: test_audit_aggregation_metric
title: TAMPERED Object
status: exploring
schema_version: "` + objects.DefaultSchemaVersion + `"
collection_count: 1
first_seen: "2030-01-05T00:00:00Z"
last_seen: "2030-01-05T00:00:00Z"`)

	if err := fileutil.WriteFile(hashFile, tamperedData, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to tamper with file: %v", err)
	}

	// Attempt to read - should detect tampering
	_, err = cas.Read(objectID)
	if err == nil {
		t.Error("Expected error when reading tampered file, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("Expected hash mismatch error, got: %v", err)
	}
}

// TestCAS_TamperDetection_IndexTampering tests that tampering with the index is detected
func TestCAS_TamperDetection_IndexTampering(t *testing.T) {
	// Create temporary test environment
	testDir := t.TempDir()
	kindDir := filepath.Join(testDir, "metrics")
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create kind directory: %v", err)
	}

	// Create CAS instance
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, "test_audit_aggregation_metric", casQueue)

	// Create an object
	objectID := "TAM-002"
	originalData := []byte(`id: TAM-002
kind: test_audit_aggregation_metric
title: Test Object
status: exploring
schema_version: "` + objects.DefaultSchemaVersion + `"
collection_count: 1
first_seen: "2030-01-05T00:00:00Z"
last_seen: "2030-01-05T00:00:00Z"`)

	if err := cas.Create(objectID, originalData); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Wait for index updates to be processed
	if err := storage.FlushListingIndexForKind("test_audit_aggregation_metric"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Get the hash
	originalHash, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get hash: %v", err)
	}

	// Tamper with the index: change the hash mapping
	// This simulates someone modifying the index file directly
	cas.Mu.Lock()
	cas.GetIndex().Mu.Lock()
	cas.GetIndex().Mappings[objectID] = "0000000000000000000000000000000000000000000000000000000000000000" // Fake hash
	cas.GetIndex().Mu.Unlock()
	cas.Mu.Unlock()

	// Attempt to read - should fail because hash doesn't match
	_, err = cas.Read(objectID)
	if err == nil {
		t.Error("Expected error when reading with tampered index, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "hash mismatch") && !strings.Contains(err.Error(), "failed to read hash file") {
		t.Errorf("Expected hash mismatch or file not found error, got: %v", err)
	}

	// Restore original hash to verify it works again
	cas.Mu.Lock()
	cas.GetIndex().Mu.Lock()
	cas.GetIndex().Mappings[objectID] = originalHash
	cas.GetIndex().Mu.Unlock()
	cas.Mu.Unlock()

	// Should be able to read again
	_, err = cas.Read(objectID)
	if err != nil {
		t.Errorf("Failed to read object after restoring hash: %v", err)
	}
}

// TestCAS_TamperDetection_FileDeletion tests that deletion of hash file is detected
func TestCAS_TamperDetection_FileDeletion(t *testing.T) {
	// Create temporary test environment
	testDir := t.TempDir()
	kindDir := filepath.Join(testDir, "metrics")
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create kind directory: %v", err)
	}

	// Create CAS instance
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, "test_audit_aggregation_metric", casQueue)

	// Create an object
	objectID := "TAM-003"
	originalData := []byte(`id: TAM-003
kind: test_audit_aggregation_metric
title: Test Object
status: exploring
schema_version: "` + objects.DefaultSchemaVersion + `"
collection_count: 1
first_seen: "2030-01-05T00:00:00Z"
last_seen: "2030-01-05T00:00:00Z"`)

	if err := cas.Create(objectID, originalData); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Wait for index updates to be processed
	if err := storage.FlushListingIndexForKind("test_audit_aggregation_metric"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Get the hash and file path
	hash, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get hash: %v", err)
	}
	hashFile := filepath.Join(kindDir, hash+".yaml")

	// Delete the hash file (simulating tampering)
	if err := fileutil.Remove(hashFile); err != nil {
		t.Fatalf("Failed to delete hash file: %v", err)
	}

	// Attempt to read - should fail because file doesn't exist
	_, err = cas.Read(objectID)
	if err == nil {
		t.Error("Expected error when reading deleted file, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "failed to read hash file") && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("Expected file not found error, got: %v", err)
	}
}

// TestCAS_TamperDetection_YAMLCorruption tests that YAML corruption is detected
func TestCAS_TamperDetection_YAMLCorruption(t *testing.T) {
	// Create temporary test environment
	testDir := t.TempDir()
	kindDir := filepath.Join(testDir, "metrics")
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create kind directory: %v", err)
	}

	// Create CAS instance
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, "test_audit_aggregation_metric", casQueue)

	// Create an object
	objectID := "TAM-004"
	originalData := []byte(`id: TAM-004
kind: test_audit_aggregation_metric
title: Test Object
status: exploring
schema_version: "` + objects.DefaultSchemaVersion + `"
collection_count: 1
first_seen: "2030-01-05T00:00:00Z"
last_seen: "2030-01-05T00:00:00Z"`)

	if err := cas.Create(objectID, originalData); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Wait for index updates to be processed
	if err := storage.FlushListingIndexForKind("test_audit_aggregation_metric"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Get the hash and file path
	hash, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get hash: %v", err)
	}
	hashFile := filepath.Join(kindDir, hash+".yaml")

	// Corrupt the YAML (but keep same hash by modifying in a way that changes hash)
	// Actually, we need to corrupt it in a way that changes the hash
	// Let's add invalid YAML that would change the hash
	corruptedData := []byte(`id: TAM-004
kind: test_audit_aggregation_metric
title: Test Object
status: exploring
schema_version: "` + objects.DefaultSchemaVersion + `"
collection_count: 1
first_seen: "2030-01-05T00:00:00Z"
last_seen: "2030-01-05T00:00:00Z"
invalid: [unclosed bracket`)

	if err := fileutil.WriteFile(hashFile, corruptedData, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to corrupt file: %v", err)
	}

	// Attempt to read - should detect hash mismatch (content changed)
	_, err = cas.Read(objectID)
	if err == nil {
		t.Error("Expected error when reading corrupted file, got nil")
	}
	// Should detect either hash mismatch or YAML parse error
	if err != nil && !strings.Contains(err.Error(), "hash mismatch") && !strings.Contains(err.Error(), "failed to parse YAML") {
		t.Errorf("Expected hash mismatch or YAML parse error, got: %v", err)
	}
}
