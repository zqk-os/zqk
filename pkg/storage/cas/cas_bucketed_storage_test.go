package cas_test

import (
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/validation"
)

// registerCASTestStorageCleanup registers the two-phase teardown these CAS tests need. Cleanups run
// LIFO, so Shutdown lands first (closing write-behind and index writers), then the storage teardown
// pipeline, then t.TempDir removal — open writers otherwise leave the temp tree non-empty.
func registerCASTestStorageCleanup(t *testing.T, fileStorage *storage.FileObjectStorage) {
	t.Helper()
	if cleanup := fileStorage.GetTestCleanup(); cleanup != nil {
		t.Cleanup(cleanup)
	}
	t.Cleanup(func() { _ = fileStorage.Shutdown(context.Background()) })
}

// TestCAS_BucketedStorage_AuditEvent tests CAS with bucketed storage (base_metric in YYYY-MM subdirectories)
func TestCAS_BucketedStorage_AuditEvent(t *testing.T) {
	storage.DisableStreamStorageForTest(t)
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-bucketed-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)
	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	registerCASTestStorageCleanup(t, fileStorage)
	baseAuditDir := fileStorage.GetKindDir("base_metric")

	// Verify CAS is enabled for base_metric
	if !fileStorage.UsesContentAddressableStorage("base_metric") {
		t.Fatalf("CAS should be enabled for base_metric when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}

	// Create metrics events in different months to test bucketing
	now := time.Now().UTC()
	months := []string{
		now.Format("2006-01"),
		now.AddDate(0, -1, 0).Format("2006-01"), // Last month
		now.AddDate(0, -2, 0).Format("2006-01"), // 2 months ago
	}

	metricsEvents := []map[string]any{
		{
			objects.FieldKeyID:              "BAS-BUCKET-001",
			objects.FieldKeyKind:            "base_metric",
			objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:       now.Format(time.RFC3339),
			objects.FieldKeyCreatedBy:       "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:       now.Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:       "ACC-SYSTEM",
			objects.FieldKeyOriginSystem:    validation.DefaultOriginSystem,
			objects.FieldKeyOriginProject:   validation.DefaultOriginProject,
			objects.FieldKeyStatus:          objects.ObjectStatusImplemented,
			objects.FieldKeyEventType:       "test_event",
			objects.FieldKeyOperation:       "Test metrics event 1",
			objects.FieldKeySeverity:        "low",
			objects.FieldKeyMetricType:      "system",
			objects.FieldKeyFirstSeen:       now.Format(time.RFC3339),
			objects.FieldKeyLastSeen:        now.Format(time.RFC3339),
			objects.FieldKeyCollectionCount: 1,
		},
		{
			objects.FieldKeyID:              "BAS-BUCKET-002",
			objects.FieldKeyKind:            "base_metric",
			objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:       now.AddDate(0, -1, 0).Format(time.RFC3339),
			objects.FieldKeyCreatedBy:       "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:       now.AddDate(0, -1, 0).Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:       "ACC-SYSTEM",
			objects.FieldKeyOriginSystem:    validation.DefaultOriginSystem,
			objects.FieldKeyOriginProject:   validation.DefaultOriginProject,
			objects.FieldKeyStatus:          objects.ObjectStatusImplemented,
			objects.FieldKeyEventType:       "test_event",
			objects.FieldKeyOperation:       "Test metrics event 2",
			objects.FieldKeySeverity:        "low",
			objects.FieldKeyMetricType:      "system",
			objects.FieldKeyFirstSeen:       now.AddDate(0, -1, 0).Format(time.RFC3339),
			objects.FieldKeyLastSeen:        now.AddDate(0, -1, 0).Format(time.RFC3339),
			objects.FieldKeyCollectionCount: 1,
		},
		{
			objects.FieldKeyID:              "BAS-BUCKET-003",
			objects.FieldKeyKind:            "base_metric",
			objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:       now.AddDate(0, -2, 0).Format(time.RFC3339),
			objects.FieldKeyCreatedBy:       "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:       now.AddDate(0, -2, 0).Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:       "ACC-SYSTEM",
			objects.FieldKeyOriginSystem:    validation.DefaultOriginSystem,
			objects.FieldKeyOriginProject:   validation.DefaultOriginProject,
			objects.FieldKeyStatus:          objects.ObjectStatusImplemented,
			objects.FieldKeyEventType:       "test_event",
			objects.FieldKeyOperation:       "Test metrics event 3",
			objects.FieldKeySeverity:        "low",
			objects.FieldKeyMetricType:      "system",
			objects.FieldKeyFirstSeen:       now.AddDate(0, -2, 0).Format(time.RFC3339),
			objects.FieldKeyLastSeen:        now.AddDate(0, -2, 0).Format(time.RFC3339),
			objects.FieldKeyCollectionCount: 1,
		},
	}

	// Create metrics events using storage.WriteSystemObjectAndRegisterHash (what scheduler uses)
	for i, event := range metricsEvents {
		data, err := storage.FormatMultiLineYAML(event)
		if err != nil {
			t.Fatalf("Failed to format metrics event %d: %v", i+1, err)
		}

		// Determine metrics directory (bucketed by month)
		month := months[i]
		bucketAuditDir := filepath.Join(baseAuditDir, month)
		if err := fileutil.MkdirAll(bucketAuditDir, paths.DirPerm755); err != nil {
			t.Fatalf("Failed to create metrics directory %s: %v", month, err)
		}

		metricsFilePath := filepath.Join(bucketAuditDir, event[objects.FieldKeyID].(string)+".yaml")
		err = storage.WriteSystemObjectAndRegisterHash(metricsFilePath, data, "base_metric", baseAuditDir, event[objects.FieldKeyID].(string), fileStorage)
		if err != nil {
			t.Fatalf("Failed to create metrics event %s: %v", event[objects.FieldKeyID], err)
		}
	}

	// Wait for index updates to be processed (async batching)
	writeQueue := caspkg.GetGlobalListingIndexWriteQueue()
	if err := writeQueue.FlushKind("base_metric", 5*time.Second); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Verify all events are in CAS
	cas, err := fileStorage.GetContentAddressableStorage("base_metric")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	for _, event := range metricsEvents {
		eventID := event[objects.FieldKeyID].(string)
		hash, err := cas.GetHashForID(eventID)
		if err != nil {
			t.Errorf("Audit event %s should exist in CAS index: %v", eventID, err)
			continue
		}

		// Verify we can read it back
		readObj, err := fileStorage.Read(ctx, secCtx, eventID)
		if err != nil {
			t.Errorf("Failed to read metrics event %s: %v", eventID, err)
			continue
		}

		if readObj[objects.FieldKeyID] != eventID {
			t.Errorf("Expected ID %s, got %v", eventID, readObj[objects.FieldKeyID])
		}

		// Verify getObjectFilePath returns correct path (should handle bucketing)
		filePath, err := fileStorage.GetObjectFilePath(eventID, "base_metric")
		if err != nil {
			t.Errorf("Failed to get file path for %s: %v", eventID, err)
			continue
		}

		// For CAS objects, path should be hash-based
		fileName := filepath.Base(filePath)
		if len(fileName) < 68 || !strings.HasSuffix(fileName, ".yaml") {
			t.Errorf("Expected hash-based filename for CAS object, got %s", fileName)
		}

		// Verify filename matches hash
		expectedHashFile := hash + ".yaml"
		if fileName != expectedHashFile {
			t.Errorf("Expected filename %s, got %s", expectedHashFile, fileName)
		}

		// Verify file exists in the correct month directory
		// Check if it's in any of the month directories
		foundInMonth := false
		for _, month := range months {
			monthDir := filepath.Join(baseAuditDir, month)
			if strings.HasPrefix(filePath, monthDir) {
				foundInMonth = true
				break
			}
		}
		if !foundInMonth {
			t.Errorf("File path %s should be in one of the month directories", filePath)
		}

		// Verify file actually exists
		if _, err := fileutil.Stat(filePath); err != nil {
			t.Errorf("CAS file should exist at %s: %v", filePath, err)
		}
	}

	// Test List operation with bucketed storage
	storageCtx := pkgctx.GetStorageContext()
	filter := storage.ListFilter{
		Kind: "base_metric",
	}

	result, err := fileStorage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("Failed to list metrics events: %v", err)
	}

	// Should find all events
	found := make(map[string]bool)
	for _, obj := range result.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if strings.HasPrefix(id, "BAS-BUCKET-") {
			found[id] = true
		}
	}

	for _, event := range metricsEvents {
		eventID := event[objects.FieldKeyID].(string)
		if !found[eventID] {
			t.Errorf("Audit event %s not found in list results", eventID)
		}
	}
}

// TestCAS_BucketedStorage_OnTheFlyCreation tests that objects are created in correct buckets on-the-fly
func TestCAS_BucketedStorage_OnTheFlyCreation(t *testing.T) {
	storage.DisableStreamStorageForTest(t)
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-bucketed-onthefly-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)
	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	registerCASTestStorageCleanup(t, fileStorage)
	baseAuditDir := fileStorage.GetKindDir("base_metric")

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}

	// Create metrics events with different created_at dates to test automatic bucketing
	now := time.Now().UTC()
	testCases := []struct {
		id            string
		createdAt     time.Time
		expectedMonth string
	}{
		{
			id:            "BAS-ONTHEFLY-001",
			createdAt:     now,
			expectedMonth: now.Format("2006-01"),
		},
		{
			id:            "BAS-ONTHEFLY-002",
			createdAt:     now.AddDate(0, -1, 0),
			expectedMonth: now.AddDate(0, -1, 0).Format("2006-01"),
		},
		{
			id:            "BAS-ONTHEFLY-003",
			createdAt:     now.AddDate(0, 1, 0), // Future month
			expectedMonth: now.AddDate(0, 1, 0).Format("2006-01"),
		},
	}

	for _, tc := range testCases {
		event := map[string]any{
			objects.FieldKeyID:              tc.id,
			objects.FieldKeyKind:            "base_metric",
			objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:       tc.createdAt.Format(time.RFC3339),
			objects.FieldKeyCreatedBy:       "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:       tc.createdAt.Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:       "ACC-SYSTEM",
			objects.FieldKeyOriginSystem:    validation.DefaultOriginSystem,
			objects.FieldKeyOriginProject:   validation.DefaultOriginProject,
			objects.FieldKeyStatus:          objects.ObjectStatusImplemented,
			objects.FieldKeyEventType:       "test_event",
			objects.FieldKeyOperation:       "On-the-fly test event",
			objects.FieldKeySeverity:        "low",
			objects.FieldKeyMetricType:      "system",
			objects.FieldKeyFirstSeen:       tc.createdAt.Format(time.RFC3339),
			objects.FieldKeyLastSeen:        tc.createdAt.Format(time.RFC3339),
			objects.FieldKeyCollectionCount: 1,
		}

		// Format and write using storage.WriteSystemObjectAndRegisterHash
		data, err := storage.FormatMultiLineYAML(event)
		if err != nil {
			t.Fatalf("Failed to format metrics event %s: %v", tc.id, err)
		}

		// Determine metrics directory based on created_at (bucketed by month)
		bucketAuditDir := filepath.Join(baseAuditDir, tc.expectedMonth)
		metricsFilePath := filepath.Join(bucketAuditDir, tc.id+".yaml")

		err = storage.WriteSystemObjectAndRegisterHash(metricsFilePath, data, "base_metric", baseAuditDir, tc.id, fileStorage)
		if err != nil {
			t.Fatalf("Failed to create metrics event %s: %v", tc.id, err)
		}
	}

	// Wait for index updates to be processed (async batching)
	writeQueue := caspkg.GetGlobalListingIndexWriteQueue()
	if err := writeQueue.FlushKind("base_metric", 5*time.Second); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Verify all objects are in CAS
	cas, err := fileStorage.GetContentAddressableStorage("base_metric")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	for _, tc := range testCases {
		hash, err := cas.GetHashForID(tc.id)
		if err != nil {
			t.Errorf("Audit event %s should exist in CAS index: %v", tc.id, err)
			continue
		}

		// Verify getObjectFilePath finds it in the correct bucket
		filePath, err := fileStorage.GetObjectFilePath(tc.id, "base_metric")
		if err != nil {
			t.Errorf("Failed to get file path for %s: %v", tc.id, err)
			continue
		}

		// Verify path is in the correct month directory
		expectedDir := filepath.Join(baseAuditDir, tc.expectedMonth)
		if !strings.HasPrefix(filePath, expectedDir) {
			t.Errorf("Expected file path to be in %s, got %s", expectedDir, filePath)
		}

		// Verify filename is hash-based
		fileName := filepath.Base(filePath)
		expectedHashFile := hash + ".yaml"
		if fileName != expectedHashFile {
			t.Errorf("Expected filename %s, got %s", expectedHashFile, fileName)
		}

		// Verify file exists
		if _, err := fileutil.Stat(filePath); err != nil {
			t.Errorf("CAS file should exist at %s: %v", filePath, err)
		}

		// Verify we can read it back
		readObj, err := fileStorage.Read(ctx, secCtx, tc.id)
		if err != nil {
			t.Errorf("Failed to read metrics event %s: %v", tc.id, err)
			continue
		}

		if readObj[objects.FieldKeyID] != tc.id {
			t.Errorf("Expected ID %s, got %v", tc.id, readObj[objects.FieldKeyID])
		}
	}
}

// TestCAS_BucketedStorage_Update tests that updates work correctly with bucketed CAS objects
func TestCAS_BucketedStorage_Update(t *testing.T) {
	storage.DisableStreamStorageForTest(t)
	// Do not t.Parallel: DisableStreamStorageForTest mutates ZQK_STREAM_STORAGE_ENABLED; parallel
	// tests would race and base_metric updates could follow the stream-backed path.
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-bucketed-update-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	registerCASTestStorageCleanup(t, fileStorage)

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}

	// Create an metrics event
	now := time.Now().UTC()
	month := now.Format("2006-01")
	event := map[string]any{
		objects.FieldKeyID:              "BAS-001",
		objects.FieldKeyKind:            "base_metric",
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:       now.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:       "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:       now.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:       "ACC-SYSTEM",
		objects.FieldKeyOriginSystem:    validation.DefaultOriginSystem,
		objects.FieldKeyOriginProject:   validation.DefaultOriginProject,
		objects.FieldKeyStatus:          objects.ObjectStatusImplemented,
		objects.FieldKeyTitle:           "Bucketed CAS update metric",
		objects.FieldKeyMetricType:      "system",
		objects.FieldKeyFirstSeen:       now.Format(time.RFC3339),
		objects.FieldKeyLastSeen:        now.Format(time.RFC3339),
		objects.FieldKeyCollectionCount: 1,
	}

	// Create using storage.WriteSystemObjectAndRegisterHash
	data, err := storage.FormatMultiLineYAML(event)
	if err != nil {
		t.Fatalf("Failed to format metrics event: %v", err)
	}

	baseAuditDir := fileStorage.GetKindDir("base_metric")
	bucketAuditDir := filepath.Join(baseAuditDir, month)
	if err := fileutil.MkdirAll(bucketAuditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create metrics directory: %v", err)
	}

	metricsFilePath := filepath.Join(bucketAuditDir, "BAS-001.yaml")
	err = storage.WriteSystemObjectAndRegisterHash(metricsFilePath, data, "base_metric", baseAuditDir, "BAS-001", fileStorage)
	if err != nil {
		t.Fatalf("Failed to create metrics event: %v", err)
	}

	// Wait for index updates to be processed
	if err := storage.FlushListingIndexForKind("base_metric"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Get initial hash
	cas, err := fileStorage.GetContentAddressableStorage("base_metric")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	initialHash, err := cas.GetHashForID("BAS-001")
	if err != nil {
		t.Fatalf("Failed to get initial hash: %v", err)
	}

	// Update the event
	updates := map[string]any{
		objects.FieldKeyCollectionCount: 2,
	}

	err = fileStorage.Update(ctx, secCtx, "BAS-001", updates)
	if err != nil {
		t.Fatalf("Failed to update metrics event: %v", err)
	}

	// Wait for index updates to be processed
	if err := storage.FlushListingIndexForKind("base_metric"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Verify hash changed
	updatedHash, err := cas.GetHashForID("BAS-001")
	if err != nil {
		t.Fatalf("Failed to get updated hash: %v", err)
	}

	if initialHash == updatedHash {
		t.Errorf("Hash should have changed after update: %s", initialHash)
	}

	// Verify content is updated
	readObj, err := fileStorage.Read(ctx, secCtx, "BAS-001")
	if err != nil {
		t.Fatalf("Failed to read updated metrics event: %v", err)
	}

	count, _ := readObj[objects.FieldKeyCollectionCount].(int)
	if count != 2 {
		t.Errorf("Expected collection_count 2, got %v", readObj[objects.FieldKeyCollectionCount])
	}

	// Verify file path still works
	filePath, err := fileStorage.GetObjectFilePath("BAS-001", "base_metric")
	if err != nil {
		t.Fatalf("Failed to get file path: %v", err)
	}

	// Should be hash-based and in correct month directory
	fileName := filepath.Base(filePath)
	expectedHashFile := updatedHash + ".yaml"
	if fileName != expectedHashFile {
		t.Errorf("Expected filename %s, got %s", expectedHashFile, fileName)
	}

	if !strings.HasPrefix(filePath, bucketAuditDir) {
		t.Errorf("File path should be in month directory %s, got %s", bucketAuditDir, filePath)
	}

	storage.FlushAllOrFail(t, fileStorage.GetProjectRoot())
}
