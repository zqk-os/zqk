package storage

import (
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/storage/filecas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"testing"
)

// TestContentAddressableStorage_GetFilePathForID tests getting file path from ID
// This is what utilities like getObjectFilePath would use
func TestContentAddressableStorage_GetFilePathForID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"
	objectID := "AAM-001"

	testData := []byte("id: AAM-001\nkind: audit_aggregation_metric\nevent_count: 10\n")
	expectedHash := CalculateSHA256Hash(testData)

	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)
	err := cas.Create(objectID, testData)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Get file path from ID
	filePath, err := cas.GetFilePathForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get file path: %v", err)
	}

	// Verify path is correct
	expectedPath := filepath.Join(kindDir, expectedHash+".yaml")
	if filePath != expectedPath {
		t.Errorf("File path mismatch: expected %s, got %s", expectedPath, filePath)
	}

	// Verify file exists at that path
	if _, err := fileutil.Stat(filePath); err != nil {
		t.Fatalf("File does not exist at path: %v", err)
	}
}

// TestContentAddressableStorage_ListIDs tests listing all IDs from index
// This is much faster than directory scanning
func TestContentAddressableStorage_ListIDs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"

	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)

	// Create multiple objects
	ids := []string{"AAM-001", "AAM-002", "AAM-003"}
	for _, id := range ids {
		data := []byte("id: " + id + "\nkind: audit_aggregation_metric\nevent_count: 10\n")
		err := cas.Create(id, data)
		if err != nil {
			t.Fatalf("Failed to create object %s: %v", id, err)
		}
	}

	// List all IDs
	listedIDs, err := cas.ListIDs()
	if err != nil {
		t.Fatalf("Failed to list IDs: %v", err)
	}

	// Verify all IDs are present
	if len(listedIDs) != len(ids) {
		t.Errorf("ID count mismatch: expected %d, got %d", len(ids), len(listedIDs))
	}

	idMap := make(map[string]bool)
	for _, id := range listedIDs {
		idMap[id] = true
	}

	for _, expectedID := range ids {
		if !idMap[expectedID] {
			t.Errorf("ID %s not found in list", expectedID)
		}
	}
}

// TestContentAddressableStorage_GetAllMappings tests getting all ID -> hash mappings
func TestContentAddressableStorage_GetAllMappings(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"

	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)

	// Create multiple objects
	testObjects := map[string][]byte{
		"AAM-001": []byte("id: AAM-001\nkind: audit_aggregation_metric\nevent_count: 10\n"),
		"AAM-002": []byte("id: AAM-002\nkind: audit_aggregation_metric\nevent_count: 20\n"),
	}

	expectedHashes := make(map[string]string)
	for id, data := range testObjects {
		hash := CalculateSHA256Hash(data)
		expectedHashes[id] = hash
		err := cas.Create(id, data)
		if err != nil {
			t.Fatalf("Failed to create object %s: %v", id, err)
		}
	}

	// Get all mappings
	mappings, err := cas.GetAllMappings()
	if err != nil {
		t.Fatalf("Failed to get mappings: %v", err)
	}

	// Verify mappings
	if len(mappings) != len(testObjects) {
		t.Errorf("Mapping count mismatch: expected %d, got %d", len(testObjects), len(mappings))
	}

	for id, expectedHash := range expectedHashes {
		actualHash, exists := mappings[id]
		if !exists {
			t.Errorf("ID %s not found in mappings", id)
			continue
		}
		if actualHash != expectedHash {
			t.Errorf("Hash mismatch for %s: expected %s, got %s", id, expectedHash, actualHash)
		}
	}
}
