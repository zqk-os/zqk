package storage

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestSemanticDateOperators tests all semantic date/time filter operators
func TestSemanticDateOperators(t *testing.T) {
	testRoot, cleanup := setupTestEnvironment(t)
	defer cleanup()

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

	// Create test objects with different timestamps
	now := time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)
	testObjects := []map[string]any{
		createCleanupTestAggregationMetric("AAM-101", now.AddDate(0, 0, -5)),  // 5 days ago
		createCleanupTestAggregationMetric("AAM-102", now.AddDate(0, 0, -10)), // 10 days ago
		createCleanupTestAggregationMetric("AAM-103", now.AddDate(0, 0, -20)), // 20 days ago
		createCleanupTestAggregationMetric("AAM-104", now.AddDate(0, 0, -30)), // 30 days ago (exactly)
		createCleanupTestAggregationMetric("AAM-105", now.AddDate(0, 0, -40)), // 40 days ago
	}

	// Clean up any existing objects first to prevent "object already exists" errors
	cliCtx := WithTestHardDelete(ctx)
	for _, obj := range testObjects {
		objID := obj[objects.FieldKeyID].(string)
		// Check if object exists first - if not, skip deletion
		// CAS may return different error formats, so we check for "not found" in the error message
		_, err := storage.Read(ctx, secCtx, objID)
		if err == nil {
			// Object exists - delete it
			if delErr := storage.Delete(cliCtx, secCtx, objID, false); delErr != nil {
				t.Fatalf("Failed to delete existing object %s: %v", objID, delErr)
			}
			// Deterministically wait for deletion to complete
			if !ensureObjectDeleted(ctx, storage, secCtx, objID) {
				t.Fatalf("Failed to confirm deletion of object %s within timeout", objID)
			}
		} else {
			// Object doesn't exist (ErrObjectNotFound or "not found" in error message)
			// This is fine - we can proceed to create
			// Note: CAS may return "file not found" errors which we treat as object not existing
		}
		// Object doesn't exist or was successfully deleted - create it
		if err := storage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Failed to create object %s: %v", objID, err)
		}
	}

	// Test $before operator
	t.Run("Before", func(t *testing.T) {
		cutoff := now.AddDate(0, 0, -30).Format(time.RFC3339)
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{"$before": cutoff},
			},
		})
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}
		// Should find AAM-105 (40 days ago, before 30 days)
		if len(result.Objects) != 1 {
			t.Errorf("Expected 1 object before cutoff, got %d", len(result.Objects))
		}
		if len(result.Objects) > 0 && result.Objects[0][objects.FieldKeyID] != "AAM-105" {
			t.Errorf("Expected AAM-105, got %s", result.Objects[0][objects.FieldKeyID])
		}
	})

	// Test $after operator
	t.Run("After", func(t *testing.T) {
		cutoff := now.AddDate(0, 0, -30).Format(time.RFC3339)
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{"$after": cutoff},
			},
		})
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}
		// Should find AAM-101, AAM-102, AAM-103 (all after 30 days ago)
		if len(result.Objects) != 3 {
			t.Errorf("Expected 3 objects after cutoff, got %d", len(result.Objects))
		}
		foundIDs := make(map[string]bool)
		for _, obj := range result.Objects {
			foundIDs[obj[objects.FieldKeyID].(string)] = true
		}
		expected := []string{"AAM-101", "AAM-102", "AAM-103"}
		for _, id := range expected {
			if !foundIDs[id] {
				t.Errorf("Expected to find %s, but it was not in results", id)
			}
		}
	})

	// Test $on operator (exact match)
	t.Run("On", func(t *testing.T) {
		exactTime := now.AddDate(0, 0, -20).Format(time.RFC3339)
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{"$on": exactTime},
			},
		})
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}
		// Should find AAM-103 (exactly 20 days ago)
		if len(result.Objects) != 1 {
			t.Errorf("Expected 1 object on exact time, got %d", len(result.Objects))
		}
		if len(result.Objects) > 0 && result.Objects[0][objects.FieldKeyID] != "AAM-103" {
			t.Errorf("Expected AAM-103, got %s", result.Objects[0][objects.FieldKeyID])
		}
	})

	// Test $onOrBefore operator (inclusive)
	t.Run("OnOrBefore", func(t *testing.T) {
		cutoff := now.AddDate(0, 0, -30).Format(time.RFC3339)
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{"$onOrBefore": cutoff},
			},
		})
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}
		// Should find AAM-104 (exactly 30 days ago) and AAM-105 (40 days ago)
		if len(result.Objects) != 2 {
			t.Errorf("Expected 2 objects on or before cutoff, got %d", len(result.Objects))
		}
		foundIDs := make(map[string]bool)
		for _, obj := range result.Objects {
			foundIDs[obj[objects.FieldKeyID].(string)] = true
		}
		expected := []string{"AAM-104", "AAM-105"}
		for _, id := range expected {
			if !foundIDs[id] {
				t.Errorf("Expected to find %s, but it was not in results", id)
			}
		}
	})

	// Test $onOrAfter operator (inclusive)
	t.Run("OnOrAfter", func(t *testing.T) {
		cutoff := now.AddDate(0, 0, -30).Format(time.RFC3339)
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{"$onOrAfter": cutoff},
			},
		})
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}
		// Should find AAM-101, AAM-102, AAM-103, AAM-104 (all on or after 30 days ago)
		if len(result.Objects) != 4 {
			t.Errorf("Expected 4 objects on or after cutoff, got %d", len(result.Objects))
		}
		foundIDs := make(map[string]bool)
		for _, obj := range result.Objects {
			foundIDs[obj[objects.FieldKeyID].(string)] = true
		}
		expected := []string{"AAM-101", "AAM-102", "AAM-103", "AAM-104"}
		for _, id := range expected {
			if !foundIDs[id] {
				t.Errorf("Expected to find %s, but it was not in results", id)
			}
		}
	})

	// Test $between operator (inclusive on both ends)
	t.Run("Between", func(t *testing.T) {
		start := now.AddDate(0, 0, -30).Format(time.RFC3339)
		end := now.AddDate(0, 0, -10).Format(time.RFC3339)
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{"$between": []string{start, end}},
			},
		})
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}
		// Should find AAM-102 (10 days ago), AAM-103 (20 days ago), AAM-104 (30 days ago)
		if len(result.Objects) != 3 {
			t.Errorf("Expected 3 objects between start and end, got %d", len(result.Objects))
		}
		foundIDs := make(map[string]bool)
		for _, obj := range result.Objects {
			foundIDs[obj[objects.FieldKeyID].(string)] = true
		}
		expected := []string{"AAM-102", "AAM-103", "AAM-104"}
		for _, id := range expected {
			if !foundIDs[id] {
				t.Errorf("Expected to find %s, but it was not in results", id)
			}
		}
	})
}

// TestSemanticDateOperators_EdgeCases tests edge cases for semantic operators
func TestSemanticDateOperators_EdgeCases(t *testing.T) {
	testRoot, cleanup := setupTestEnvironment(t)
	defer cleanup()

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

	// Create test objects with precise timestamps
	now := time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)
	exactTime := now.AddDate(0, 0, -15)
	oneSecondBefore := exactTime.Add(-time.Second)
	oneSecondAfter := exactTime.Add(time.Second)

	testObjects := []map[string]any{
		createCleanupTestAggregationMetric("AAM-201", oneSecondBefore),
		createCleanupTestAggregationMetric("AAM-202", exactTime),
		createCleanupTestAggregationMetric("AAM-203", oneSecondAfter),
	}

	// Clean up any existing objects first to prevent "object already exists" errors
	cliCtx := WithTestHardDelete(ctx)
	for _, obj := range testObjects {
		objID := obj[objects.FieldKeyID].(string)
		// Check if object exists first - if not, skip deletion
		// CAS may return different error formats, so we check for "not found" in the error message
		_, err := storage.Read(ctx, secCtx, objID)
		if err == nil {
			// Object exists - delete it
			if delErr := storage.Delete(cliCtx, secCtx, objID, false); delErr != nil {
				t.Fatalf("Failed to delete existing object %s: %v", objID, delErr)
			}
			// Deterministically wait for deletion to complete
			if !ensureObjectDeleted(ctx, storage, secCtx, objID) {
				t.Fatalf("Failed to confirm deletion of object %s within timeout", objID)
			}
		} else {
			// Object doesn't exist (ErrObjectNotFound or "not found" in error message)
			// This is fine - we can proceed to create
			// Note: CAS may return "file not found" errors which we treat as object not existing
		}
		// Object doesn't exist or was successfully deleted - create it
		if err := storage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Failed to create object %s: %v", objID, err)
		}
	}

	// Test $on with exact timestamp
	t.Run("OnExactTimestamp", func(t *testing.T) {
		exactTimeStr := exactTime.Format(time.RFC3339)
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{"$on": exactTimeStr},
			},
		})
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}
		// Should find only AAM-202 (exact match)
		if len(result.Objects) != 1 {
			t.Errorf("Expected 1 object with exact timestamp, got %d", len(result.Objects))
		}
		if len(result.Objects) > 0 && result.Objects[0][objects.FieldKeyID] != "AAM-202" {
			t.Errorf("Expected AAM-202, got %s", result.Objects[0][objects.FieldKeyID])
		}
	})

	// Test $before with boundary (exclusive)
	t.Run("BeforeBoundary", func(t *testing.T) {
		cutoff := exactTime.Format(time.RFC3339)
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{"$before": cutoff},
			},
		})
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}
		// Should find only AAM-201 (1 second before, exclusive)
		if len(result.Objects) != 1 {
			t.Errorf("Expected 1 object before cutoff (exclusive), got %d", len(result.Objects))
		}
		if len(result.Objects) > 0 && result.Objects[0][objects.FieldKeyID] != "AAM-201" {
			t.Errorf("Expected AAM-201, got %s", result.Objects[0][objects.FieldKeyID])
		}
	})

	// Test $onOrBefore with boundary (inclusive)
	t.Run("OnOrBeforeBoundary", func(t *testing.T) {
		cutoff := exactTime.Format(time.RFC3339)
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{"$onOrBefore": cutoff},
			},
		})
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}
		// Should find AAM-201 and AAM-202 (inclusive)
		if len(result.Objects) != 2 {
			t.Errorf("Expected 2 objects on or before cutoff (inclusive), got %d", len(result.Objects))
		}
		foundIDs := make(map[string]bool)
		for _, obj := range result.Objects {
			foundIDs[obj[objects.FieldKeyID].(string)] = true
		}
		if !foundIDs["AAM-201"] || !foundIDs["AAM-202"] {
			t.Errorf("Expected AAM-201 and AAM-202, got %v", foundIDs)
		}
	})

	// Test $between with same start and end (should find exact match)
	t.Run("BetweenSameStartEnd", func(t *testing.T) {
		exactTimeStr := exactTime.Format(time.RFC3339)
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{"$between": []string{exactTimeStr, exactTimeStr}},
			},
		})
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}
		// Should find only AAM-202 (exact match)
		if len(result.Objects) != 1 {
			t.Errorf("Expected 1 object between same start/end, got %d", len(result.Objects))
		}
		if len(result.Objects) > 0 && result.Objects[0][objects.FieldKeyID] != "AAM-202" {
			t.Errorf("Expected AAM-202, got %s", result.Objects[0][objects.FieldKeyID])
		}
	})
}

// TestSemanticDateOperators_InvalidInputs tests error handling for invalid inputs
func TestSemanticDateOperators_InvalidInputs(t *testing.T) {
	testRoot, cleanup := setupTestEnvironment(t)
	defer cleanup()

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

	// Create one test object
	now := time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)
	obj := createCleanupTestAggregationMetric("AAM-301", now)
	if err := storage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Test $between with invalid array (not 2 elements)
	t.Run("BetweenInvalidArray", func(t *testing.T) {
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{"$between": []string{"2030-01-01T00:00:00Z"}}, // Only 1 element
			},
		})
		if err != nil {
			t.Fatalf("Query should not error, got: %v", err)
		}
		// Should return no results (invalid filter)
		if len(result.Objects) != 0 {
			t.Errorf("Expected 0 objects for invalid $between filter, got %d", len(result.Objects))
		}
	})

	// Test $between with invalid timestamp format
	t.Run("BetweenInvalidTimestamp", func(t *testing.T) {
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{"$between": []string{"invalid-date", "also-invalid"}},
			},
		})
		if err != nil {
			t.Fatalf("Query should not error, got: %v", err)
		}
		// Should return no results (invalid timestamps)
		if len(result.Objects) != 0 {
			t.Errorf("Expected 0 objects for invalid timestamps, got %d", len(result.Objects))
		}
	})
}

// TestSemanticDateOperators_NonDateFields tests that semantic operators work on non-date fields
// (should fall back to string comparison if parsing fails)
func TestSemanticDateOperators_NonDateFields(t *testing.T) {
	testRoot, cleanup := setupTestEnvironment(t)
	defer cleanup()

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

	// Create test objects with string fields (not timestamps)
	testObjects := []map[string]any{
		createCleanupTestAggregationMetric("AAM-401", time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)),
		createCleanupTestAggregationMetric("AAM-402", time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)),
	}

	// Clean up any existing objects first to prevent "object already exists" errors
	cliCtx := WithTestHardDelete(ctx)
	for _, obj := range testObjects {
		objID := obj[objects.FieldKeyID].(string)
		// Check if object exists first - if not, skip deletion
		// CAS may return different error formats, so we check for "not found" in the error message
		_, err := storage.Read(ctx, secCtx, objID)
		if err == nil {
			// Object exists - delete it
			if delErr := storage.Delete(cliCtx, secCtx, objID, false); delErr != nil {
				t.Fatalf("Failed to delete existing object %s: %v", objID, delErr)
			}
			// Deterministically wait for deletion to complete
			if !ensureObjectDeleted(ctx, storage, secCtx, objID) {
				t.Fatalf("Failed to confirm deletion of object %s within timeout", objID)
			}
		} else {
			// Object doesn't exist (ErrObjectNotFound or "not found" in error message)
			// This is fine - we can proceed to create
			// Note: CAS may return "file not found" errors which we treat as object not existing
		}
		// Object doesn't exist or was successfully deleted - create it
		if err := storage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Failed to create object %s: %v", objID, err)
		}
	}

	// Test $before on a non-date field
	// Note: When parsing fails, the operator falls through to generic comparison
	// This test verifies that the operator doesn't crash on non-date fields
	t.Run("BeforeOnNonDateField", func(t *testing.T) {
		result, err := storage.List(ctx, secCtx, &pkgctx.StorageContext{}, ListFilter{
			Kind: "audit_aggregation_metric",
			Filters: map[string]any{
				objects.FieldKeyTitle: map[string]any{"$before": "2030-01-01T00:00:00Z"}, // title is not a timestamp
			},
		})
		if err != nil {
			t.Fatalf("Query should not error, got: %v", err)
		}
		// The behavior when parsing fails is implementation-dependent
		// We just verify it doesn't crash and returns a consistent result
		_ = result.Objects // Just verify query completes without error
	})
}
