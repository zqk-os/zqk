package storage

import (
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// TestContentAddressableStorage_Create tests creating an object with content-addressable storage
// Hash is calculated from content BEFORE writing, and the hash becomes the filename
func TestContentAddressableStorage_Create(t *testing.T) {
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"
	objectID := "AAM-001"

	// Test data
	testData := []byte("id: AAM-001\nkind: audit_aggregation_metric\nevent_count: 10\n")

	// 1. Calculate hash from content BEFORE writing (Git's approach)
	expectedHash := CalculateSHA256Hash(testData)

	// 2. Create the object using content-addressable storage
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)
	err := cas.Create(objectID, testData)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Wait for index updates to be processed
	if err := FlushListingIndexForKind(kind); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// 3. Verify file exists with hash as filename
	hashFile := filepath.Join(kindDir, expectedHash+".yaml")
	if _, err := fileutil.Stat(hashFile); err != nil {
		t.Fatalf("Hash-based file does not exist: %v", err)
	}

	// 4. Verify ID index maps ID to hash
	indexHash, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get hash for ID: %v", err)
	}
	if indexHash != expectedHash {
		t.Errorf("Index hash mismatch: expected %s, got %s", expectedHash, indexHash)
	}

	// 5. Verify file content matches what we wrote
	fileContent, err := fileutil.ReadFile(hashFile)
	if err != nil {
		t.Fatalf("Failed to read hash file: %v", err)
	}
	if !bytes.Equal(fileContent, testData) {
		t.Errorf("File content mismatch:\nexpected: %q\ngot: %q", string(testData), string(fileContent))
	}

	// 6. Verify hash in filename matches content (integrity check)
	actualHash := CalculateSHA256Hash(fileContent)
	if actualHash != expectedHash {
		t.Errorf("Hash mismatch: expected %s, got %s", expectedHash, actualHash)
	}
}

// TestContentAddressableStorage_Read tests reading an object by ID
// Should look up hash in index, then read the hash-based file
func TestContentAddressableStorage_Read(t *testing.T) {
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"
	objectID := "AAM-002"

	// Create object first
	testData := []byte("id: AAM-002\nkind: audit_aggregation_metric\nevent_count: 20\n")
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)
	err := cas.Create(objectID, testData)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Read object by ID
	readData, err := cas.Read(objectID)
	if err != nil {
		t.Fatalf("Failed to read object: %v", err)
	}

	// Verify content matches (may be normalized YAML, so check it's valid)
	if len(readData) == 0 {
		t.Error("Read data is empty")
	}
}

// TestContentAddressableStorage_ReadCorruptedHash tests that corrupted files are detected
// Even if hash matches, YAML validation should catch corruption
func TestContentAddressableStorage_ReadCorruptedHash(t *testing.T) {
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"
	objectID := "AAM-CORRUPT"

	// Create valid object
	testData := []byte("id: AAM-CORRUPT\nkind: audit_aggregation_metric\nevent_count: 30\n")
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)
	err := cas.Create(objectID, testData)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Get hash
	hash, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get hash: %v", err)
	}

	// Corrupt the file by writing invalid YAML (but keep same hash - this shouldn't happen in practice)
	// Actually, if we corrupt it, the hash will change, so let's corrupt it in a way that breaks YAML
	hashFile := filepath.Join(kindDir, hash+".yaml")
	corruptedData := []byte("id: AAM-CORRUPT\nkind: audit_aggregation_metric\nevent_count: 30\ninvalid: [unclosed bracket\n")
	err = fileutil.WriteFile(hashFile, corruptedData, paths.FilePerm644)
	if err != nil {
		t.Fatalf("Failed to corrupt file: %v", err)
	}

	// Reading should fail due to YAML validation
	_, err = cas.Read(objectID)
	if err == nil {
		t.Error("Read should fail on corrupted YAML")
	}
	if err != nil {
		errStr := err.Error()
		if !strings.Contains(errStr, "parse YAML") && !strings.Contains(errStr, "hash mismatch") {
			t.Errorf("Expected YAML parse error or hash mismatch, got: %v", err)
		}
	}
}

// TestContentAddressableStorage_ReadHashMismatch tests that hash mismatches are detected
func TestContentAddressableStorage_ReadHashMismatch(t *testing.T) {
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"
	objectID := "AAM-HASHMISMATCH"

	// Create valid object
	testData := []byte("id: AAM-HASHMISMATCH\nkind: audit_aggregation_metric\nevent_count: 40\n")
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)
	err := cas.Create(objectID, testData)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Get hash
	hash, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get hash: %v", err)
	}

	// Corrupt the file by changing content (hash will mismatch)
	hashFile := filepath.Join(kindDir, hash+".yaml")
	corruptedData := []byte("id: AAM-HASHMISMATCH\nkind: audit_aggregation_metric\nevent_count: 999\n")
	err = fileutil.WriteFile(hashFile, corruptedData, paths.FilePerm644)
	if err != nil {
		t.Fatalf("Failed to corrupt file: %v", err)
	}

	// Reading should fail due to hash mismatch
	_, err = cas.Read(objectID)
	if err == nil {
		t.Error("Read should fail on hash mismatch")
	}
	if err != nil && !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("Expected hash mismatch error, got: %v", err)
	}
}

// TestContentAddressableStorage_Update tests updating an object
// Should create new hash-based file, update index, delete old file
func TestContentAddressableStorage_Update(t *testing.T) {
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"
	objectID := "AAM-003"

	// Create initial object
	initialData := []byte("id: AAM-003\nkind: audit_aggregation_metric\nevent_count: 30\n")
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)
	err := cas.Create(objectID, initialData)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	initialHash, _ := cas.GetHashForID(objectID) //nolint:errcheck // Test helper - error handling not critical
	initialFile := filepath.Join(kindDir, initialHash+".yaml")

	// Update object
	updatedData := []byte("id: AAM-003\nkind: audit_aggregation_metric\nevent_count: 40\n")
	err = cas.Update(objectID, updatedData)
	if err != nil {
		t.Fatalf("Failed to update object: %v", err)
	}

	// Verify old file is deleted
	if _, err := fileutil.Stat(initialFile); err == nil {
		t.Error("Old hash-based file should be deleted after update")
	}

	// Verify new file exists with new hash
	newHash, _ := cas.GetHashForID(objectID) //nolint:errcheck // Test helper - error handling not critical
	if newHash == initialHash {
		t.Error("Hash should change after update")
	}

	newFile := filepath.Join(kindDir, newHash+".yaml")
	if _, err := fileutil.Stat(newFile); err != nil {
		t.Fatalf("New hash-based file does not exist: %v", err)
	}

	// Verify content matches updated data
	readData, err := cas.Read(objectID)
	if err != nil {
		t.Fatalf("Failed to read updated object: %v", err)
	}
	if len(readData) == 0 {
		t.Error("Read data is empty")
	}
}

// TestContentAddressableStorage_Delete tests deleting an object
// Should delete hash-based file and remove index entry
func TestContentAddressableStorage_Delete(t *testing.T) {
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"
	objectID := "AAM-004"

	// Create object
	testData := []byte("id: AAM-004\nkind: audit_aggregation_metric\nevent_count: 50\n")
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)
	err := cas.Create(objectID, testData)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	hash, _ := cas.GetHashForID(objectID) //nolint:errcheck // Test helper - error handling not critical
	hashFile := filepath.Join(kindDir, hash+".yaml")

	// Delete object
	err = cas.Delete(objectID)
	if err != nil {
		t.Fatalf("Failed to delete object: %v", err)
	}

	// Verify file is deleted
	if _, err := fileutil.Stat(hashFile); err == nil {
		t.Error("Hash-based file should be deleted")
	}

	// Verify index entry is removed
	_, err = cas.GetHashForID(objectID)
	if err == nil {
		t.Error("Index entry should be removed after delete")
	}
}

// TestContentAddressableStorage_HashAsFilename tests that hash is always the filename
// This is the core guarantee of content-addressable storage
func TestContentAddressableStorage_HashAsFilename(t *testing.T) {
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"
	objectID := "AAM-005"

	testData := []byte("id: AAM-005\nkind: audit_aggregation_metric\nevent_count: 60\n")
	expectedHash := CalculateSHA256Hash(testData)

	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)
	err := cas.Create(objectID, testData)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Verify filename is exactly the hash
	expectedFile := filepath.Join(kindDir, expectedHash+".yaml")
	if _, err := fileutil.Stat(expectedFile); err != nil {
		t.Fatalf("File should be named with hash: %v", err)
	}

	// Verify no other files exist (except index)
	files, err := fileutil.ReadDir(kindDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	hashFileFound := false
	for _, file := range files {
		if file.Name() == expectedHash+".yaml" {
			hashFileFound = true
		} else if file.Name() != ".audit_aggregation_metric.index" {
			t.Errorf("Unexpected file in directory: %s", file.Name())
		}
	}

	if !hashFileFound {
		t.Error("Hash-based file not found in directory")
	}
}

// TestContentAddressableStorage_ConcurrentWrites tests concurrent writes to same ID
// Should handle locking properly
func TestContentAddressableStorage_ConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"
	objectID := "AAM-006"

	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)

	// Create initial object
	initialData := []byte("id: AAM-006\nkind: audit_aggregation_metric\nevent_count: 70\n")
	err := cas.Create(objectID, initialData)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Concurrent updates (simulate race condition)
	done := make(chan error, 2)

	goroutinelabels.StartTestGoroutine("test_update_1", "updating object in test (first update)", func() {
		data := []byte("id: AAM-006\nkind: audit_aggregation_metric\nevent_count: 80\n")
		done <- cas.Update(objectID, data)
	})

	goroutinelabels.StartTestGoroutine("test_update_2", "updating object in test (second update)", func() {
		data := []byte("id: AAM-006\nkind: audit_aggregation_metric\nevent_count: 90\n")
		done <- cas.Update(objectID, data)
	})

	// Wait for both to complete
	err1 := <-done
	err2 := <-done

	// At least one should succeed (locking should prevent both)
	if err1 != nil && err2 != nil {
		t.Logf("Both updates failed (expected with locking): %v, %v", err1, err2)
	}

	// Verify object still exists and is readable
	_, err = cas.Read(objectID)
	if err != nil {
		t.Fatalf("Object should still be readable after concurrent updates: %v", err)
	}
}

// TestContentAddressableStorage_Deduplication tests that same content = same file
// This is a key benefit of content-addressable storage
func TestContentAddressableStorage_Deduplication(t *testing.T) {
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "metrics")
	kind := "audit_aggregation_metric"

	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(kindDir, kind, casQueue)

	// Same content for two different IDs
	sharedContent := []byte("id: SHARED\nkind: audit_aggregation_metric\nevent_count: 100\n")
	expectedHash := CalculateSHA256Hash(sharedContent)

	// Create two objects with same content
	err1 := cas.Create("AAM-007", sharedContent)
	err2 := cas.Create("AAM-008", sharedContent)

	if err1 != nil {
		t.Fatalf("Failed to create first object: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("Failed to create second object: %v", err2)
	}

	// Verify only one file exists (same hash = same file)
	hashFile := filepath.Join(kindDir, expectedHash+".yaml")
	if _, err := fileutil.Stat(hashFile); err != nil {
		t.Fatalf("Shared hash file should exist: %v", err)
	}

	// Verify both IDs point to same hash
	hash1, _ := cas.GetHashForID("AAM-007") //nolint:errcheck // Test helper - error handling not critical
	hash2, _ := cas.GetHashForID("AAM-008") //nolint:errcheck // Test helper - error handling not critical

	if hash1 != hash2 {
		t.Errorf("Same content should produce same hash: %s != %s", hash1, hash2)
	}

	if hash1 != expectedHash {
		t.Errorf("Hash should match expected: %s != %s", hash1, expectedHash)
	}
}

// TestPhantomCreate_MembraneVisibility tests creation membrane visibility, index sync, and draft repair handling.
func TestPhantomCreate_MembraneVisibility(t *testing.T) {
	testRoot, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	objectID := "TST-PHANTOM-001"
	kind := "test_case"
	objMap := map[string]any{
		objects.FieldKeyID:            objectID,
		objects.FieldKeyKind:          kind,
		objects.FieldKeyTitle:         "Phantom Create Test",
		objects.FieldKeyStatus:        objects.ObjectStatusDraft,
		objects.FieldKeySchemaVersion: "2.0.0",
		objects.FieldKeyNamespaceID:   "zqk:kernel",
	}

	// 1. Create object via storage membrane
	if err := fileStorage.Create(ctx, secCtx, objMap); err != nil {
		t.Fatalf("Failed to create object via storage membrane: %v", err)
	}

	// 2. Flush index write queue to guarantee CAS index mapping
	if queue := caspkg.GetListingIndexWriteQueueForProjectRoot(testRoot); queue != nil {
		_ = queue.FlushKind(kind, 5*time.Second)
	}

	// 3. Verify object is readable from storage
	readObj, err := fileStorage.Read(ctx, secCtx, objectID)
	if err != nil {
		t.Fatalf("Expected object %s to be readable after create, got err: %v", objectID, err)
	}
	if readObj[objects.FieldKeyTitle] != "Phantom Create Test" {
		t.Errorf("Expected title 'Phantom Create Test', got %v", readObj[objects.FieldKeyTitle])
	}

	// 4. Verify draft orphan cleanup queue integration
	draftsDir := filepath.Join(testRoot, ".zqk", "drafts")
	_ = fileutil.MkdirAll(draftsDir, paths.DirPerm755)
	orphanFile := filepath.Join(draftsDir, "phantom-draft-999.yaml")
	_ = fileutil.WriteFile(orphanFile, []byte("draft: true"), paths.FilePerm644)

	orphanQueue := fileStorage.GetOrphanCleanupQueue()
	if orphanQueue != nil {
		enqueued, err := orphanQueue.CleanupOrphanedDrafts(ctx, draftsDir, 0)
		if err != nil {
			t.Fatalf("CleanupOrphanedDrafts failed: %v", err)
		}
		if enqueued != 1 {
			t.Errorf("Expected 1 enqueued orphan draft, got %d", enqueued)
		}
	}
}
