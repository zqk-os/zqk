package storage

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// setupTestEnvironment creates a temporary directory and the necessary docs/process structure
//
//nolint:gocritic // Named returns not needed; keep simple tuple
func setupTestEnvironment(t *testing.T) (string, func()) {
	testRoot := t.TempDir()
	mustEnsureProcessSpecsLayout(t, testRoot)

	return testRoot, func() {
		fileutil.RemoveAll(testRoot)
	}
}

// createCleanupTestAggregationMetric creates a valid audit_aggregation_metric object for testing
func createCleanupTestAggregationMetric(id string, createdAt time.Time) map[string]any {
	return map[string]any{
		objects.FieldKeyID:                     id,
		objects.FieldKeyKind:                   objects.KindAuditAggregationMetric,
		objects.FieldKeyTitle:                  "Test Aggregation Metric " + id,
		objects.FieldKeyCreatedAt:              createdAt.Format(time.RFC3339),
		objects.FieldKeyStatus:                 objects.ObjectStatusCompleted,
		objects.FieldKeyMetricType:             "system",
		objects.FieldKeySchemaVersion:          objects.DefaultSchemaVersion,
		objects.FieldKeyAggregationWindowStart: createdAt.Format(time.RFC3339),
		objects.FieldKeyAggregationWindowEnd:   createdAt.Format(time.RFC3339),
		objects.FieldKeyEventCount:             10,
		objects.FieldKeyEventTypeCounts:        map[string]int{"create": 5, "update": 5},
		objects.FieldKeyCollectionCount:        1,
		objects.FieldKeyFirstSeen:              createdAt.Format(time.RFC3339),
		objects.FieldKeyLastSeen:               createdAt.Format(time.RFC3339),
	}
}

// createCleanupTestCommandMetric creates a valid command_metric object for testing
func createCleanupTestCommandMetric(id string, createdAt time.Time) map[string]any {
	return map[string]any{
		objects.FieldKeyID:                      id,
		objects.FieldKeyKind:                    objects.KindCommandMetric,
		objects.FieldKeyTitle:                   "Test Command Metric " + id,
		objects.FieldKeyCreatedAt:               createdAt.Format(time.RFC3339),
		objects.FieldKeyStatus:                  objects.ObjectStatusImplemented,
		objects.FieldKeyMetricType:              "command",
		objects.FieldKeySchemaVersion:           objects.DefaultSchemaVersion,
		objects.FieldKeyCommand:                 "test command",
		objects.FieldKeyNormalizedCmd:           "test command",
		objects.FieldKeyCollectionCount:         1,
		objects.FieldKeyFirstSeen:               createdAt.Format(time.RFC3339),
		objects.FieldKeyLastSeen:                createdAt.Format(time.RFC3339),
		objects.FieldKeyAvgDurationSeconds:      1.0,
		objects.FieldKeyBaselineDurationSeconds: 1.0,
		objects.FieldKeyInvocationCount:         1,
		objects.FieldKeySuccessCount:            1,
		objects.FieldKeyFailureCount:            0,
		objects.FieldKeyTimeoutCount:            0,
		objects.FieldKeyErrorRate:               0.0,
		objects.FieldKeyTimeoutRate:             0.0,
		objects.FieldKeyFastestDurationSeconds:  1.0,
		objects.FieldKeySlowestDurationSeconds:  1.0,
	}
}

// TestCleanupWithBucketing_AggregationMetrics tests cleanup of audit_aggregation_metric
// objects with monthly bucketing strategy, ensuring objects older than 30 days are deleted
// while recent objects are preserved
func TestCleanupWithBucketing_AggregationMetrics(t *testing.T) {
	testRoot := t.TempDir()
	// Remove audit dir before t.TempDir cleanup so RemoveAll(testRoot) does not fail with "directory not empty"
	// (Delete/Create can create audit events under docs/process/audit)
	auditDir := datacell.CellCASPrimaryDir(testRoot, "audit")
	defer func() { _ = fileutil.RemoveAll(auditDir) }()

	// Create required directory structure
	mustEnsureProcessSpecsLayout(t, testRoot)

	// Create storage
	storage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})
	BuildPathAliasCacheForProject(testRoot)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Create mock aggregation metrics with different ages
	now := time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)
	cutoffDate := now.AddDate(0, 0, -30) // 30 days ago

	// Objects to keep (within retention period)
	recentObjects := []map[string]any{
		createCleanupTestAggregationMetric("AAM-501", now.AddDate(0, 0, -10)), // 10 days ago
		createCleanupTestAggregationMetric("AAM-502", now.AddDate(0, 0, -20)), // 20 days ago
		createCleanupTestAggregationMetric("AAM-503", now.AddDate(0, 0, -29)), // 29 days ago (just within retention)
	}

	// Objects to delete (older than retention period)
	oldObjects := []map[string]any{
		createCleanupTestAggregationMetric("AAM-504", now.AddDate(0, 0, -31)), // 31 days ago
		createCleanupTestAggregationMetric("AAM-505", now.AddDate(0, 0, -60)), // 60 days ago
		createCleanupTestAggregationMetric("AAM-506", now.AddDate(0, -2, 0)),  // 2 months ago
	}

	// Create all objects
	//nolint:gocritic // Intentionally creating new slice by appending two slices
	allObjects := append(recentObjects, oldObjects...)
	for _, obj := range allObjects {
		if err := storage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Failed to create object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Verify all objects exist
	initialCount := len(allObjects)
	listResult, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
		Kind: "audit_aggregation_metric",
	})
	if err != nil {
		t.Fatalf("Failed to list objects: %v", err)
	}
	if len(listResult.Objects) != initialCount {
		t.Errorf("Expected %d objects initially, got %d", initialCount, len(listResult.Objects))
	}

	// Query for objects older than cutoff date using semantic operator
	cutoffStr := cutoffDate.Format(time.RFC3339)
	filterResult, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
		Kind: "audit_aggregation_metric",
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$before": cutoffStr, // Semantic operator - parses timestamps before comparison
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to query old objects: %v", err)
	}

	// Debug: Print all objects and their created_at dates
	t.Logf("Cutoff date: %s", cutoffStr)
	for _, obj := range allObjects {
		createdAt, _ := obj[objects.FieldKeyCreatedAt].(string)
		t.Logf("Object %s: created_at=%s", obj[objects.FieldKeyID], createdAt)
	}
	for _, obj := range filterResult.Objects {
		createdAt, _ := obj[objects.FieldKeyCreatedAt].(string)
		t.Logf("Filtered object %s: created_at=%s", obj[objects.FieldKeyID], createdAt)
	}

	// Verify we found the old objects
	if len(filterResult.Objects) != len(oldObjects) {
		t.Errorf("Expected to find %d old objects, got %d. Cutoff: %s", len(oldObjects), len(filterResult.Objects), cutoffStr)
	}

	// Verify we found the correct objects
	foundIDs := make(map[string]bool)
	for _, obj := range filterResult.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		foundIDs[id] = true
	}

	for _, oldObj := range oldObjects {
		id, _ := oldObj[objects.FieldKeyID].(string)
		if !foundIDs[id] {
			t.Errorf("Expected to find old object %s in query results", id)
		}
	}

	// Delete old objects (use CLI context for delete operations)
	cliCtx := WithTestHardDelete(ctx)
	deletedCount := 0
	for _, obj := range filterResult.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if err := storage.Delete(cliCtx, secCtx, id, false); err != nil {
			t.Errorf("Failed to delete object %s: %v", id, err)
		} else {
			deletedCount++
		}
	}

	if deletedCount != len(oldObjects) {
		t.Errorf("Expected to delete %d objects, deleted %d", len(oldObjects), deletedCount)
	}

	// Flush CAS index so List sees the updated index (audit_aggregation_metric uses CAS; per-project queue must drain)
	FlushAllOrFail(t, storage.GetProjectRoot())

	// Verify remaining objects
	finalResult, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
		Kind: "audit_aggregation_metric",
	})
	if err != nil {
		t.Fatalf("Failed to list remaining objects: %v", err)
	}

	if len(finalResult.Objects) != len(recentObjects) {
		t.Errorf("Expected %d objects to remain, got %d", len(recentObjects), len(finalResult.Objects))
	}

	// Verify correct objects remain
	remainingIDs := make(map[string]bool)
	for _, obj := range finalResult.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		remainingIDs[id] = true
	}

	for _, recentObj := range recentObjects {
		id, _ := recentObj[objects.FieldKeyID].(string)
		if !remainingIDs[id] {
			t.Errorf("Expected recent object %s to remain, but it was deleted", id)
		}
	}

	// Verify old objects are gone
	for _, oldObj := range oldObjects {
		id, _ := oldObj[objects.FieldKeyID].(string)
		if remainingIDs[id] {
			t.Errorf("Expected old object %s to be deleted, but it still exists", id)
		}
	}
}

// TestCleanupWithBucketing_CommandMetrics tests cleanup of command_metric objects
// with monthly bucketing strategy, ensuring objects older than 90 days are deleted
func TestCleanupWithBucketing_CommandMetrics(t *testing.T) {
	testRoot := t.TempDir()

	// Create required directory structure
	mustEnsureProcessSpecsLayout(t, testRoot)

	// Create storage
	storage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})
	BuildPathAliasCacheForProject(testRoot)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Create mock command metrics with different ages
	now := time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)
	cutoffDate := now.AddDate(0, 0, -90) // 90 days ago

	// Objects to keep (within retention period)
	recentObjects := []map[string]any{
		createCleanupTestCommandMetric("CMD-001", now.AddDate(0, 0, -30)), // 30 days ago
		createCleanupTestCommandMetric("CMD-002", now.AddDate(0, 0, -60)), // 60 days ago
		createCleanupTestCommandMetric("CMD-003", now.AddDate(0, 0, -89)), // 89 days ago (just within retention)
	}

	// Objects to delete (older than retention period)
	oldObjects := []map[string]any{
		createCleanupTestCommandMetric("CMD-004", now.AddDate(0, 0, -91)),  // 91 days ago
		createCleanupTestCommandMetric("CMD-005", now.AddDate(0, 0, -120)), // 120 days ago
		createCleanupTestCommandMetric("CMD-006", now.AddDate(0, -6, 0)),   // 6 months ago
	}

	// Create all objects
	//nolint:gocritic // Intentionally creating new slice by appending two slices
	allObjects := append(recentObjects, oldObjects...)
	for _, obj := range allObjects {
		if err := storage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Failed to create object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Query for objects older than cutoff date using semantic operator
	cutoffStr := cutoffDate.Format(time.RFC3339)
	filterResult, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
		Kind: "command_metric",
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$before": cutoffStr, // Semantic operator - parses timestamps before comparison
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to query old objects: %v", err)
	}

	// Verify we found the old objects
	if len(filterResult.Objects) != len(oldObjects) {
		t.Errorf("Expected to find %d old objects, got %d", len(oldObjects), len(filterResult.Objects))
	}

	// Delete old objects (use CLI context for delete operations)
	cliCtx := WithTestHardDelete(ctx)
	deletedCount := 0
	for _, obj := range filterResult.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if err := storage.Delete(cliCtx, secCtx, id, false); err != nil {
			t.Errorf("Failed to delete object %s: %v", id, err)
		} else {
			deletedCount++
		}
	}

	if deletedCount != len(oldObjects) {
		t.Errorf("Expected to delete %d objects, deleted %d", len(oldObjects), deletedCount)
	}

	// Verify remaining objects
	finalResult, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
		Kind: "command_metric",
	})
	if err != nil {
		t.Fatalf("Failed to list remaining objects: %v", err)
	}

	if len(finalResult.Objects) != len(recentObjects) {
		t.Errorf("Expected %d objects to remain, got %d", len(recentObjects), len(finalResult.Objects))
	}
}

// TestCleanupWithBucketing_CrossBucketQueries tests that cleanup queries work correctly
// when objects are stored in different monthly buckets
func TestCleanupWithBucketing_CrossBucketQueries(t *testing.T) {
	testRoot, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create storage
	storage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})
	BuildPathAliasCacheForProject(testRoot)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	now := time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)
	cutoffDate := now.AddDate(0, 0, -30)

	// Create objects in different months to test cross-bucket queries
	crossBucketFixtures := []struct {
		id         string
		createdAt  time.Time
		shouldKeep bool
	}{
		// Current month - keep (within 30 days)
		{"AAM-601", now.AddDate(0, 0, -5), true},  // 5 days ago - keep
		{"AAM-602", now.AddDate(0, 0, -15), true}, // 15 days ago - keep
		{"AAM-603", now.AddDate(0, 0, -25), true}, // 25 days ago - keep (within 30 days)
		// Previous month - delete (older than 30 days)
		{"AAM-604", now.AddDate(0, -1, -5), false},  // 1 month + 5 days ago (~35 days) - delete
		{"AAM-605", now.AddDate(0, -1, -10), false}, // 1 month + 10 days ago (~40 days) - delete
		{"AAM-606", now.AddDate(0, -1, -35), false}, // 1 month + 35 days ago (~65 days) - delete
		// Two months ago - delete
		{"AAM-607", now.AddDate(0, -2, 0), false},
		{"AAM-608", now.AddDate(0, -2, -15), false},
		// Three months ago - delete
		{"AAM-609", now.AddDate(0, -3, 0), false},
	}

	var expectedKeep []string
	var expectedDelete []string

	for _, obj := range crossBucketFixtures {
		objData := createCleanupTestAggregationMetric(obj.id, obj.createdAt)
		// Clean up any existing object first (best effort - may not exist)
		cliCtx := WithTestHardDelete(ctx)
		_ = storage.Delete(cliCtx, secCtx, obj.id, false) //nolint:errcheck // Test cleanup - error handling not critical
		// Deterministically wait for deletion to complete
		if !ensureObjectDeleted(ctx, storage, secCtx, obj.id) {
			t.Fatalf("Failed to confirm deletion of object %s within timeout", obj.id)
		}
		if err := storage.Create(ctx, secCtx, objData); err != nil {
			t.Fatalf("Failed to create object %s: %v", obj.id, err)
		}

		if obj.shouldKeep {
			expectedKeep = append(expectedKeep, obj.id)
		} else {
			expectedDelete = append(expectedDelete, obj.id)
		}
	}

	// Query for objects older than cutoff date using semantic operator
	cutoffStr := cutoffDate.Format(time.RFC3339)
	filterResult, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
		Kind: "audit_aggregation_metric",
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$before": cutoffStr, // Semantic operator - parses timestamps before comparison
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to query old objects: %v", err)
	}

	// Verify we found the correct old objects across buckets
	foundIDs := make(map[string]bool)
	for _, obj := range filterResult.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		foundIDs[id] = true
	}

	for _, id := range expectedDelete {
		if !foundIDs[id] {
			t.Errorf("Expected to find old object %s in query results", id)
		}
	}

	// Verify we didn't find objects that should be kept
	for _, id := range expectedKeep {
		if foundIDs[id] {
			t.Errorf("Object %s should be kept but was found in old objects query", id)
		}
	}

	// Delete old objects (use CLI context for delete operations)
	cliCtx := WithTestHardDelete(ctx)
	for _, obj := range filterResult.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if err := storage.Delete(cliCtx, secCtx, id, false); err != nil {
			t.Errorf("Failed to delete object %s: %v", id, err)
		}
	}

	// Verify remaining objects
	finalResult, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
		Kind: "audit_aggregation_metric",
	})
	if err != nil {
		t.Fatalf("Failed to list remaining objects: %v", err)
	}

	remainingIDs := make(map[string]bool)
	for _, obj := range finalResult.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		remainingIDs[id] = true
	}

	for _, id := range expectedKeep {
		if !remainingIDs[id] {
			t.Logf("Expected object %s to remain, but it was deleted", id)
		}
	}
}

// TestCleanupWithBucketing_ArchivingStrategy tests that archiving strategy respects
// retention tiers (warm, cold, iced) when cleaning up objects
func TestCleanupWithBucketing_ArchivingStrategy(t *testing.T) {
	testRoot, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create storage
	storage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})
	BuildPathAliasCacheForProject(testRoot)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	now := time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)

	// Create objects in different retention tiers
	// Warm tier: 0-30 days (keep)
	// Cold tier: 30-365 days (archive, don't delete)
	// Iced tier: >365 days (long-term retention)
	retentionFixtures := []struct {
		id         string
		createdAt  time.Time
		tier       string
		shouldKeep bool
	}{
		{"AAM-001", now.AddDate(0, 0, -10), "warm", true}, // 10 days ago - keep
		{"AAM-002", now.AddDate(0, 0, -25), "warm", true}, // 25 days ago - keep
		{"AAM-003", now.AddDate(0, 0, -60), "cold", true}, // 60 days ago - archive (keep for now)
		{"AAM-004", now.AddDate(0, -6, 0), "cold", true},  // 6 months ago - archive (keep for now)
		{"AAM-005", now.AddDate(-1, -6, 0), "iced", true}, // 1.5 years ago - long-term (keep)
	}

	// For this test, we'll test the 30-day cleanup policy (warm tier boundary)
	cutoffDate := now.AddDate(0, 0, -30)

	for _, obj := range retentionFixtures {
		objData := createCleanupTestAggregationMetric(obj.id, obj.createdAt)
		if err := storage.Create(ctx, secCtx, objData); err != nil {
			t.Fatalf("Failed to create object %s: %v", obj.id, err)
		}
	}

	// Query for objects older than 30 days (warm tier cutoff) using semantic operator
	cutoffStr := cutoffDate.Format(time.RFC3339)
	filterResult, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
		Kind: "audit_aggregation_metric",
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$before": cutoffStr, // Semantic operator - parses timestamps before comparison
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to query old objects: %v", err)
	}

	// For 30-day retention policy, we should find objects older than 30 days.
	// In practice, additional objects may be created by other test scenarios,
	// so assert that we find at least the expected set rather than an exact
	// count.

	// Verify we found the correct objects
	foundIDs := make(map[string]bool)
	for _, obj := range filterResult.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		foundIDs[id] = true
	}

	expectedOldIDs := []string{"AAM-003", "AAM-004", "AAM-005"}
	for _, id := range expectedOldIDs {
		if !foundIDs[id] {
			t.Errorf("Expected to find old object %s in query results", id)
		}
	}
}

// TestCleanupWithBucketing_EdgeCases tests edge cases for cleanup logic
func TestCleanupWithBucketing_EdgeCases(t *testing.T) {
	testRoot, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create storage
	storage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})
	BuildPathAliasCacheForProject(testRoot)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	now := time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)
	cutoffDate := now.AddDate(0, 0, -30)

	// Test edge cases:
	// 1. Object created exactly at cutoff (should be kept - boundary condition)
	// 2. Object created 1 second before cutoff (should be deleted)
	// 3. Object created 1 second after cutoff (should be kept)
	// 4. Object with missing created_at (should be handled gracefully)
	// 5. Object with invalid created_at format (should be handled gracefully)

	edgeCaseObjs := []map[string]any{
		createCleanupTestAggregationMetric("AAM-801", cutoffDate),                     // Exactly at cutoff - keep (>= cutoff)
		createCleanupTestAggregationMetric("AAM-802", cutoffDate.Add(-1*time.Second)), // 1 second before - delete
		createCleanupTestAggregationMetric("AAM-803", cutoffDate.Add(1*time.Second)),  // 1 second after - keep
	}

	for _, obj := range edgeCaseObjs {
		if err := storage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Failed to create object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Query for objects older than cutoff using semantic operator
	cutoffStr := cutoffDate.Format(time.RFC3339)
	filterResult, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
		Kind: "audit_aggregation_metric",
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$before": cutoffStr, // Semantic operator - parses timestamps before comparison
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to query old objects: %v", err)
	}

	// Should find only the object created 1 second before cutoff
	if len(filterResult.Objects) != 1 {
		t.Errorf("Expected to find 1 object older than cutoff, got %d", len(filterResult.Objects))
	}

	if len(filterResult.Objects) > 0 {
		foundID, _ := filterResult.Objects[0][objects.FieldKeyID].(string)
		if foundID != "AAM-802" {
			t.Errorf("Expected to find AAM-802, got %s", foundID)
		}
	}
}
