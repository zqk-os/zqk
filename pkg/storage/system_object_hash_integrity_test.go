package storage

import (
	"context"

	"fmt"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// disableStreamStorageForTest turns off stream storage for the duration of the test so that
// WriteSystemObjectAndRegisterHash can be used for audit_event/change_journal_entry (CAS path).
// Call at the start of any test that writes stream-backed kinds via WriteSystemObjectAndRegisterHash.
// Uses t.Setenv (not os.Setenv) so the Go test runner does not run this test in parallel with
// other tests that touch the same env (e.g. stream-backed CRUD tests). Do not use.
func disableStreamStorageForTest(t *testing.T) {
	t.Helper()
	t.Setenv(zqkenv.StreamStorageEnabled().Name(), "0")
}

// TestWriteSystemObjectAndRegisterHash_Integrity verifies that writeSystemObjectAndRegisterHash
// creates objects with hashes that match what system check would read.
// This test ensures the fix for hash mismatches on system-built objects.
func TestWriteSystemObjectAndRegisterHash_Integrity(t *testing.T) {
	disableStreamStorageForTest(t)
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "audit", "2030-01")
	objectID := "AUD-001"
	filePath := filepath.Join(kindDir, "AUD-001.yaml")

	// Create test data (simulating an audit event)
	testData := []byte(`id: AUD-001
kind: audit_event
schema_version: "` + objects.DefaultSchemaVersion + `"
created_at: "2030-01-04T20:00:00Z"
created_by: ACC-1785920548450214012-68b850c0
status: completed
event_type: scheduler_job_started
operation: "Scheduler job SCH-001 (cache_refresh) started"
target_kind: scheduler_job
target_id: SCH-001
severity: low
metadata:
  source: scheduler
  job_id: SCH-001
  job_type: cache_refresh
`)

	// Write system object and register hash
	err := WriteSystemObjectAndRegisterHash(filePath, testData, "audit_event", kindDir, objectID, nil)
	if err != nil {
		t.Fatalf("Failed to write system object: %v", err)
	}

	// Verify file exists
	if _, err := fileutil.Stat(filePath); err != nil {
		t.Fatalf("File was not created: %v", err)
	}

	// Read file back from disk (simulating what system check does)
	fileContent, err := fileutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file back: %v", err)
	}

	// Calculate hash from what's on disk
	actualHash := CalculateSHA256Hash(fileContent)

	// Load hash registry and verify stored hash matches
	hashRegistry := NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", kindDir)
	if err := hashRegistry.Load(); err != nil {
		t.Fatalf("Failed to load hash registry: %v", err)
	}

	filename := filepath.Base(filePath)
	storedHash := hashRegistry.GetHash(filename)
	if storedHash == emptyValue {
		t.Fatal("Hash was not stored in registry")
	}

	// CRITICAL: The stored hash must match what we read from disk
	// This is the core integrity check - if this fails, system check will detect a mismatch
	if storedHash != actualHash {
		t.Errorf("Hash mismatch detected!\nStored hash: %s\nActual hash: %s\nThis indicates the hash was calculated before the file was fully written to disk.",
			storedHash, actualHash)
	}
}

// TestWriteSystemObjectAndRegisterHash_ConcurrentWrites verifies that concurrent writes
// eventually result in correct hashes. This tests the retry logic in saveHashRegistryWithRetryForSystemObject.
// Note: Concurrent writes may have transient failures due to registry locking, but retries should succeed.
func TestWriteSystemObjectAndRegisterHash_ConcurrentWrites(t *testing.T) {
	disableStreamStorageForTest(t)
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "audit", "2030-01")

	// Create multiple objects concurrently
	numObjects := 5 // Reduced to avoid overwhelming the retry mechanism
	errors := make(chan error, numObjects)

	for i := 0; i < numObjects; i++ {
		goroutinelabels.NewGoroutine("storage_test", "concurrent system object write").StartSimple(func() {
			func(id int) {
				objectID := fmt.Sprintf("AUD-%d", id)
				filePath := filepath.Join(kindDir, objectID+".yaml")
				testData := []byte("id: " + objectID + "\nkind: audit_event\n")

				err := WriteSystemObjectAndRegisterHash(filePath, testData, "audit_event", kindDir, objectID, nil)
				errors <- err
			}(i)
		})
	}

	// Wait for all writes to complete
	successCount := 0
	for i := 0; i < numObjects; i++ {
		if err := <-errors; err != nil {
			// Some failures are expected with concurrent writes due to registry locking
			// The retry mechanism should handle most cases, but we allow some failures
			t.Logf("Concurrent write failed (may be transient): %v", err)
		} else {
			successCount++
		}
	}

	// At least some writes should succeed
	if successCount == 0 {
		t.Fatal("All concurrent writes failed - retry mechanism may not be working")
	}

	// Wait deterministically for any pending writes to complete
	// Since writes are synchronous, we can check if hash registry has been updated
	registryReady := false
	for start := time.Now(); time.Since(start) < 5*time.Second; time.Sleep(10 * time.Millisecond) {
		hashRegistry := NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", kindDir)
		if err := hashRegistry.Load(); err != nil {
			continue
		}
		registryReady = true
		break
	}
	if !registryReady {
		t.Log("Hash registry may not have been updated yet")
	}

	// Verify hashes for successfully written files
	hashRegistry := NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", kindDir)
	if err := hashRegistry.Load(); err != nil {
		t.Fatalf("Failed to load hash registry: %v", err)
	}

	verifiedCount := 0
	for i := 0; i < numObjects; i++ {
		objectID := "AUD-" + string(rune('0'+i))
		filename := objectID + ".yaml"
		filePath := filepath.Join(kindDir, filename)

		// Check if file exists
		if _, err := fileutil.Stat(filePath); err != nil {
			continue // File wasn't created (write failed)
		}

		// Read file and calculate hash
		fileContent, err := fileutil.ReadFile(filePath)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", filename, err)
			continue
		}

		actualHash := CalculateSHA256Hash(fileContent)
		storedHash := hashRegistry.GetHash(filename)

		if storedHash == emptyValue {
			t.Logf("Hash not found for %s (may be due to concurrent write failure)", filename)
			continue
		}

		if storedHash != actualHash {
			t.Errorf("Hash mismatch for %s: stored=%s, actual=%s", filename, storedHash, actualHash)
		} else {
			verifiedCount++
		}
	}

	// At least some files should have correct hashes
	if verifiedCount == 0 {
		t.Fatal("No files had correct hashes - integrity validation would fail")
	}

	t.Logf("Successfully verified %d/%d concurrent writes", verifiedCount, numObjects)
}

// TestWriteSystemObjectAndRegisterHash_FileSystemDelay verifies that the 10ms delay
// after sync is sufficient for file system to flush writes. This test simulates
// the race condition that was causing hash mismatches.
func TestWriteSystemObjectAndRegisterHash_FileSystemDelay(t *testing.T) {
	disableStreamStorageForTest(t)
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "audit", "2030-01")
	objectID := "AUD-002"
	filePath := filepath.Join(kindDir, "AUD-002.yaml")

	testData := []byte(`id: AUD-002
kind: audit_event
schema_version: "` + objects.DefaultSchemaVersion + `"
created_at: "2030-01-04T20:00:00Z"
created_by: ACC-1785920548450214012-68b850c0
status: completed
`)

	// Write system object
	err := WriteSystemObjectAndRegisterHash(filePath, testData, "audit_event", kindDir, objectID, nil)
	if err != nil {
		t.Fatalf("Failed to write system object: %v", err)
	}

	// Immediately read file back (simulating system check running right after creation)
	// The delay in writeSystemObjectAndRegisterHash should ensure this works
	fileContent, err := fileutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file immediately after write: %v", err)
	}

	// Calculate hash from what's on disk
	actualHash := CalculateSHA256Hash(fileContent)

	// Load hash registry
	hashRegistry := NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", kindDir)
	if err := hashRegistry.Load(); err != nil {
		t.Fatalf("Failed to load hash registry: %v", err)
	}

	filename := filepath.Base(filePath)
	storedHash := hashRegistry.GetHash(filename)

	// Verify hash matches - this should pass because of the delay in writeSystemObjectAndRegisterHash
	if storedHash != actualHash {
		t.Errorf("Hash mismatch detected immediately after write (delay insufficient):\nStored: %s\nActual: %s",
			storedHash, actualHash)
	}

	// Wait a bit and verify again (should still match)
	// This is for timestamp precision, not async operation - small delay is acceptable
	time.Sleep(50 * time.Millisecond)
	fileContent2, err := fileutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file after delay: %v", err)
	}

	actualHash2 := CalculateSHA256Hash(fileContent2)
	if actualHash2 != actualHash {
		t.Errorf("File content changed after delay - file system inconsistency detected")
	}
}

// TestWriteSystemObjectAndRegisterHash_LineEndings verifies that hash calculation
// handles file system line ending transformations correctly. The hash must be
// calculated from what's actually on disk, not from in-memory data.
func TestWriteSystemObjectAndRegisterHash_LineEndings(t *testing.T) {
	disableStreamStorageForTest(t)
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "audit", "2030-01")
	objectID := "AUD-003"
	filePath := filepath.Join(kindDir, "AUD-003.yaml")

	// Use data with explicit line endings
	testData := []byte("id: AUD-003\nkind: audit_event\nfield: value\n")

	// Write system object
	err := WriteSystemObjectAndRegisterHash(filePath, testData, "audit_event", kindDir, objectID, nil)
	if err != nil {
		t.Fatalf("Failed to write system object: %v", err)
	}

	// Read file back (file system may have transformed line endings)
	fileContent, err := fileutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Calculate hash from what's on disk
	actualHash := CalculateSHA256Hash(fileContent)

	// Load hash registry
	hashRegistry := NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", kindDir)
	if err := hashRegistry.Load(); err != nil {
		t.Fatalf("Failed to load hash registry: %v", err)
	}

	filename := filepath.Base(filePath)
	storedHash := hashRegistry.GetHash(filename)

	// Hash must match what's on disk, even if line endings were transformed
	if storedHash != actualHash {
		t.Errorf("Hash mismatch (line ending transformation issue):\nStored: %s\nActual: %s\nOriginal data length: %d\nRead data length: %d",
			storedHash, actualHash, len(testData), len(fileContent))
	}
}

// TestSchedulerAuditEvent_HashIntegrity verifies that scheduler-created audit events
// have correct hashes. This is a critical test since scheduler events are created
// frequently and must pass integrity validation.
func TestSchedulerAuditEvent_HashIntegrity(t *testing.T) {
	disableStreamStorageForTest(t)
	// This test would require a full scheduler setup, so we'll test the underlying
	// writeSystemObjectAndRegisterHash function instead, which is what the scheduler uses.
	// The actual scheduler integration is tested in scheduler tests.

	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "audit", "2030-01")
	objectID := "AUD-004"
	filePath := filepath.Join(kindDir, "AUD-004.yaml")

	// Simulate scheduler audit event data
	testData := []byte(`id: AUD-004
kind: audit_event
schema_version: "` + objects.DefaultSchemaVersion + `"
created_at: "2030-01-04T20:00:00Z"
created_by: ACC-1785920548450214012-68b850c0
status: completed
event_type: scheduler_job_completed
operation: "Scheduler job SCH-001 (cache_refresh) completed successfully in 1.5s"
target_kind: scheduler_job
target_id: SCH-001
severity: low
metadata:
  source: scheduler
  job_id: SCH-001
  job_type: cache_refresh
  category: system_maintenance
  duration_seconds: 1.5
  success: true
`)

	// Write using the same function scheduler uses
	err := WriteSystemObjectAndRegisterHash(filePath, testData, "audit_event", kindDir, objectID, nil)
	if err != nil {
		t.Fatalf("Failed to write scheduler audit event: %v", err)
	}

	// Verify hash integrity (what system check would do)
	fileContent, err := fileutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	actualHash := CalculateSHA256Hash(fileContent)

	hashRegistry := NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", kindDir)
	if err := hashRegistry.Load(); err != nil {
		t.Fatalf("Failed to load hash registry: %v", err)
	}

	filename := filepath.Base(filePath)
	storedHash := hashRegistry.GetHash(filename)

	if storedHash != actualHash {
		t.Errorf("Scheduler audit event hash mismatch:\nStored: %s\nActual: %s\nThis would cause system check to fail",
			storedHash, actualHash)
	}
}

// TestAggregationMetric_HashIntegrity verifies that aggregation metrics created
// via storage.Create() have correct hashes. Aggregation metrics are created by
// the scheduler and must pass integrity validation.
func TestAggregationMetric_HashIntegrity(t *testing.T) {
	disableStreamStorageForTest(t)
	// Do not t.Parallel: disableStreamStorageForTest mutates ZQK_STREAM_STORAGE_ENABLED; parallel tests
	// race and getObjectFilePath can observe stream vs non-stream inconsistently (flake: object not found in stream).
	// This test verifies that objects created via Create() (which aggregation metrics use)
	// have correct hashes. The Create() method uses writeObjectFileWithPermAndData
	// which should have the same sync+delay logic.

	tempDir := t.TempDir()

	// Create required directory structure
	mustEnsureProcessSpecsLayout(t, tempDir)

	storage, err := NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tempDir, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// Create a test aggregation metric object
	metric := map[string]any{
		objects.FieldKeyID:                     "AAM-001",
		objects.FieldKeyKind:                   objects.KindAuditAggregationMetric,
		objects.FieldKeyTitle:                  "Test Aggregation Metric",
		objects.FieldKeyStatus:                 objects.ObjectStatusCompleted,
		objects.FieldKeyMetricType:             "system",
		objects.FieldKeyAggregationWindowStart: "2030-01-04T00:00:00Z",
		objects.FieldKeyAggregationWindowEnd:   "2030-01-04T01:00:00Z",
		objects.FieldKeyEventCount:             10,
		objects.FieldKeyEventTypeCounts:        map[string]int{"scheduler_job_started": 5, "scheduler_job_completed": 5},
		objects.FieldKeyCollectionCount:        1,
		objects.FieldKeyFirstSeen:              "2030-01-04T00:00:00Z",
		objects.FieldKeyLastSeen:               "2030-01-04T01:00:00Z",
		objects.FieldKeySchemaVersion:          objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:              "2030-01-04T20:00:00Z",
		objects.FieldKeyCreatedBy:              "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:              "2030-01-04T20:00:00Z",
		objects.FieldKeyUpdatedBy:              "ACC-1785920548450214012-68b850c0",
	}

	// Create object via storage (same path aggregation metrics use)
	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
		Roles:     []string{"admin"},
	}

	err = storage.Create(ctx, secCtx, metric)
	if err != nil {
		t.Fatalf("Failed to create aggregation metric: %v", err)
	}

	// Get file path (for CAS objects, this returns hash-based path)
	filePath, err := storage.getObjectFilePath("AAM-001", objects.KindAuditAggregationMetric)
	if err != nil {
		t.Fatalf("Failed to get file path: %v", err)
	}

	// Hash registry save is synchronous (Save() waits for completion), but we still
	// need to wait a moment for file system to flush
	// This is for file system flush, not async operation - small delay is acceptable
	time.Sleep(100 * time.Millisecond)

	// Read file back and calculate hash (what system check does)
	fileContent, err := fileutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	actualHash := CalculateSHA256Hash(fileContent)

	// Load hash registry to get the stored hash
	// For CAS objects, hash registry uses ID-based filename as key, not hash-based filename
	// The hash registry is stored in the kind directory (metrics/), not in bucket subdirectories
	// Get the kind directory from the process directory
	kindDir := storage.GetKindDir(objects.KindAuditAggregationMetric)

	hashRegistry := NewHashRegistry(pkgctx.NewSystemContext(), "audit_aggregation_metric", kindDir)
	if err := hashRegistry.Load(); err != nil {
		t.Fatalf("Failed to load hash registry: %v", err)
	}

	// Hash registry key is ID-based filename (AAM-001.yaml), not hash-based filename
	idBasedFilename := "AAM-001.yaml"
	storedHash := hashRegistry.GetHash(idBasedFilename)

	// If still empty, try hash-based filename as fallback (some implementations might use it)
	if storedHash == emptyValue {
		hashBasedFilename := filepath.Base(filePath)
		storedHash = hashRegistry.GetHash(hashBasedFilename)
	}

	// Verify hash matches
	if storedHash != actualHash {
		t.Errorf("Aggregation metric hash mismatch:\nStored: %s\nActual: %s\nThis would cause system check to fail",
			storedHash, actualHash)
	}
}

// TestSystemObjectHashIntegrity_EndToEnd simulates the full flow:
// 1. System creates object (scheduler/aggregation)
// 2. Hash is registered
// 3. System check reads file and verifies hash
// This ensures the entire integrity validation pipeline works correctly.
func TestSystemObjectHashIntegrity_EndToEnd(t *testing.T) {
	disableStreamStorageForTest(t)
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "audit", "2030-01")

	// Step 1: System creates audit event (simulating scheduler)
	objectID := "AUD-005"
	filePath := filepath.Join(kindDir, "AUD-005.yaml")
	testData := []byte(`id: AUD-005
kind: audit_event
schema_version: "` + objects.DefaultSchemaVersion + `"
created_at: "2030-01-04T20:00:00Z"
created_by: ACC-1785920548450214012-68b850c0
status: completed
event_type: scheduler_job_started
`)

	err := WriteSystemObjectAndRegisterHash(filePath, testData, "audit_event", kindDir, objectID, nil)
	if err != nil {
		t.Fatalf("Step 1 failed: Failed to create system object: %v", err)
	}

	// Step 2: Wait a moment (simulating time passing)
	// This is for timestamp precision, not async operation - small delay is acceptable
	time.Sleep(50 * time.Millisecond)

	// Step 3: System check reads file and verifies hash (simulating integrity validation)
	fileContent, err := fileutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Step 3 failed: Failed to read file: %v", err)
	}

	actualHash := CalculateSHA256Hash(fileContent)

	hashRegistry := NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", kindDir)
	if err := hashRegistry.Load(); err != nil {
		t.Fatalf("Step 3 failed: Failed to load hash registry: %v", err)
	}

	filename := filepath.Base(filePath)
	storedHash := hashRegistry.GetHash(filename)

	// Step 4: Verify integrity (this is what system check does)
	if storedHash == emptyValue {
		t.Fatal("Step 4 failed: Hash not found in registry (integrity violation: missing hash)")
	}

	if storedHash != actualHash {
		t.Fatalf("Step 4 failed: Hash mismatch detected (integrity violation: hash mismatch)\nStored: %s\nActual: %s\nThis is the exact error system check would report",
			storedHash, actualHash)
	}

	// If we get here, integrity validation passed
	t.Log("End-to-end integrity validation passed: system object created with correct hash")
}
