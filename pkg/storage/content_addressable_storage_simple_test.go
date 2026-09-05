package storage

import (
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/storage/filecas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"testing"
)

// TestContentAddressableStorage_Simple is a minimal test to debug hanging
func TestContentAddressableStorage_Simple(t *testing.T) {
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"
	objectID := "AAM-001"

	t.Logf("Creating CAS with kindDir=%s, kind=%s", kindDir, kind)
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)

	testData := []byte("id: AAM-001\nkind: audit_aggregation_metric\nevent_count: 10\n")
	expectedHash := CalculateSHA256Hash(testData)
	t.Logf("Expected hash: %s", expectedHash)

	t.Log("Calling Create...")
	err := cas.Create(objectID, testData)
	if err != nil {
		t.Fatalf("Failed to create: %v", err)
	}
	t.Log("Create succeeded")

	// Check if file exists
	hashFile := filepath.Join(kindDir, expectedHash+".yaml")
	if _, err := fileutil.Stat(hashFile); err != nil {
		t.Fatalf("File does not exist: %v", err)
	}
	t.Log("File exists")

	// Check index
	indexHash, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get hash: %v", err)
	}
	t.Logf("Index hash: %s", indexHash)
}

// TestIDIndex_OldestIDs verifies OldestIDs returns IDs sorted by created_at when CreatedAt is populated.
func TestIDIndex_OldestIDs(t *testing.T) {
	dir := t.TempDir()
	idx := &filecas.IDIndex{
		Version:  "1",
		Kind:     "audit_event",
		Mappings: map[string]string{"A": "h1", "B": "h2", "C": "h3"},
		CreatedAt: map[string]string{
			"A": "2030-02-01T10:00:00Z",
			"B": "2030-02-01T09:00:00Z",
			"C": "2030-02-01T11:00:00Z",
		},
		FilePath: filepath.Join(dir, ".audit_event.index"),
	}
	// Oldest first: B (09), A (10), C (11)
	got := idx.OldestIDs(2)
	if len(got) != 2 {
		t.Fatalf("OldestIDs(2) len = %d, want 2", len(got))
	}
	if got[0] != "B" || got[1] != "A" {
		t.Errorf("OldestIDs(2) = %v, want [B A]", got)
	}
	// Empty CreatedAt returns nil
	idx.CreatedAt = nil
	if idx.OldestIDs(5) != nil {
		t.Error("OldestIDs with nil CreatedAt should return nil")
	}
}
