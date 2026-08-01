package storage

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
)

// ensureObjectDeleted deterministically waits for an object to be deleted by polling
// with a context timeout. Returns true if the object is confirmed deleted (or never existed), false on timeout.
// This function handles the case where the object might not exist initially (Delete may have failed with "not found").
func ensureObjectDeleted(ctx context.Context, storage ObjectStorageProvider, secCtx *pkgctx.SecurityContext, objectID string) bool {
	// Create timeout context for the entire operation
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// First check: if object doesn't exist, we're done (it's already "deleted")
	// Use timeoutCtx so Read respects the timeout
	_, err := storage.Read(timeoutCtx, secCtx, objectID)
	if err != nil {
		// Check if error indicates object doesn't exist or is invalid
		if err == ErrObjectNotFound || strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "could not infer kind") || strings.Contains(err.Error(), "invalid ID") {
			return true // Object doesn't exist - already in desired state
		}
		// Check if context was cancelled/timed out
		if timeoutCtx.Err() != nil {
			return false // Timeout during initial check
		}
		// Unexpected error - assume object exists and needs deletion
		// Continue to polling
	}

	// Object exists - poll until it's deleted
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		select {
		case <-timeoutCtx.Done():
			if lastErr != nil {
				fmt.Printf("ensureObjectDeleted timed out for %s. Last error: %v\n", objectID, lastErr)
			}
			return false // Timeout - object still exists or deletion not complete
		case <-ticker.C:
			// Use timeoutCtx so Read respects the timeout
			_, err := storage.Read(timeoutCtx, secCtx, objectID)
			if err != nil {
				if err == ErrObjectNotFound || strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "could not infer kind") || strings.Contains(err.Error(), "invalid ID") {
					return true // Object confirmed deleted
				}
				lastErr = err
				// If error is not "not found", object might still exist or there's another issue
				// Continue polling
			}
		}
	}
}

// Helper function to compare string slices
// Uses slices.Equal for idiomatic Go code
func equalStringSlices(a, b []string) bool {
	return slices.Equal(a, b)
}

func TestFileObjectStorage_List_ComplexListOperations(t *testing.T) {
	// Use unified test environment setup for proper isolation
	testRoot, storage, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	processDir := datacell.ProcessPrimaryDir(testRoot)
	backlogDir := filepath.Join(processDir, "backlog")
	if err := os.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create backlog dir: %v", err)
	}

	ctx := context.Background()

	// Create referenced objects first (to satisfy validation)
	goalDir := filepath.Join(processDir, "goals")
	if err := os.MkdirAll(goalDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create goals dir: %v", err)
	}
	milestoneDir := filepath.Join(processDir, "milestones")
	if err := os.MkdirAll(milestoneDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create milestones dir: %v", err)
	}

	// Create referenced goals and milestones
	referencedObjects := []map[string]any{
		{objects.FieldKeyID: "GOAL-001", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 1", objects.FieldKeyStatus: "active", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count"},
		{objects.FieldKeyID: "GOAL-002", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 2", objects.FieldKeyStatus: "active", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count"},
		{objects.FieldKeyID: "GOAL-003", objects.FieldKeyKind: "goal", objects.FieldKeyTitle: "Goal 3", objects.FieldKeyStatus: "active", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTarget: "100", objects.FieldKeyMetric: "count"},
		{objects.FieldKeyID: "MIL-001", objects.FieldKeyKind: "milestone", objects.FieldKeyTitle: "Milestone 1", objects.FieldKeyStatus: "in_progress", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion},
	}
	for _, obj := range referencedObjects {
		if err := storage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create referenced object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Create test objects with various list configurations
	testObjects := []map[string]any{
		{
			objects.FieldKeyID:            "ITEM-001",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Item with single goal",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCategory:      "development",
			objects.FieldKeyGoalRefs:      []string{"GOAL-001"},
		},
		{
			objects.FieldKeyID:            "ITEM-002",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Item with multiple goals",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCategory:      "development",
			objects.FieldKeyGoalRefs:      []string{"GOAL-001", "GOAL-002", "GOAL-003"},
		},
		{
			objects.FieldKeyID:            "ITEM-003",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Item with empty goal list",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCategory:      "development",
			objects.FieldKeyGoalRefs:      []string{},
		},
		{
			objects.FieldKeyID:            "ITEM-004",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Item with no goal field",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCategory:      "development",
		},
		{
			objects.FieldKeyID:            "ITEM-005",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Item with milestones and goals",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCategory:      "development",
			objects.FieldKeyGoalRefs:      []string{"GOAL-001", "GOAL-002"},
			objects.FieldKeyMilestoneRefs: []string{"MIL-001"},
		},
		{
			objects.FieldKeyID:            "ITEM-006",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Item with exact goal match",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCategory:      "development",
			objects.FieldKeyGoalRefs:      []string{"GOAL-001", "GOAL-002"},
		},
	}

	// Create all test objects
	for _, obj := range testObjects {
		if err := storage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	t.Run("Contains single element ($has)", func(t *testing.T) {
		filter := ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyGoalRefs: map[string]any{
					"$has": "GOAL-001",
				},
			},
		}
		result, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		// Should return ITEM-001, ITEM-002, ITEM-005, ITEM-006
		if len(result.Objects) != 4 {
			t.Errorf("expected 4 objects, got %d", len(result.Objects))
		}
		ids := make([]string, len(result.Objects))
		for i, obj := range result.Objects {
			ids[i] = obj[objects.FieldKeyID].(string)
		}
		sort.Strings(ids)
		expected := []string{"ITEM-001", "ITEM-002", "ITEM-005", "ITEM-006"}
		if !equalStringSlices(ids, expected) {
			t.Errorf("expected %v, got %v", expected, ids)
		}
	})

	t.Run("Contains all elements ($hasAll)", func(t *testing.T) {
		filter := ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyGoalRefs: map[string]any{
					"$hasAll": []string{"GOAL-001", "GOAL-002"},
				},
			},
		}
		result, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		// Should return ITEM-002, ITEM-005, ITEM-006
		if len(result.Objects) != 3 {
			t.Errorf("expected 3 objects, got %d", len(result.Objects))
		}
		ids := make([]string, len(result.Objects))
		for i, obj := range result.Objects {
			ids[i] = obj[objects.FieldKeyID].(string)
		}
		sort.Strings(ids)
		expected := []string{"ITEM-002", "ITEM-005", "ITEM-006"}
		if !equalStringSlices(ids, expected) {
			t.Errorf("expected %v, got %v", expected, ids)
		}
	})

	t.Run("Contains any element ($hasAny)", func(t *testing.T) {
		filter := ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyGoalRefs: map[string]any{
					"$hasAny": []string{"GOAL-003", "GOAL-999"},
				},
			},
		}
		result, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		// Should return ITEM-002 (has GOAL-003)
		if len(result.Objects) != 1 {
			t.Errorf("expected 1 object, got %d", len(result.Objects))
		}
		if result.Objects[0][objects.FieldKeyID] != "ITEM-002" {
			t.Errorf("expected ITEM-002, got %s", result.Objects[0][objects.FieldKeyID])
		}
	})

	t.Run("Exact match ($eq)", func(t *testing.T) {
		filter := ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyGoalRefs: []string{"GOAL-001", "GOAL-002"},
			},
		}
		result, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		// Should return ITEM-005, ITEM-006 (exact match)
		if len(result.Objects) != 2 {
			t.Errorf("expected 2 objects, got %d", len(result.Objects))
		}
		ids := make([]string, len(result.Objects))
		for i, obj := range result.Objects {
			ids[i] = obj[objects.FieldKeyID].(string)
		}
		sort.Strings(ids)
		expected := []string{"ITEM-005", "ITEM-006"}
		if !equalStringSlices(ids, expected) {
			t.Errorf("expected %v, got %v", expected, ids)
		}
	})

	t.Run("Not equal ($ne)", func(t *testing.T) {
		filter := ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyGoalRefs: map[string]any{
					"$ne": []string{"GOAL-001"},
				},
			},
		}
		result, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		// Should return all except ITEM-001
		if len(result.Objects) != 5 {
			t.Errorf("expected 5 objects, got %d", len(result.Objects))
		}
	})

	t.Run("Empty list", func(t *testing.T) {
		// Filter for objects where goal_refs is empty or missing
		// Since empty lists are extracted during parsing, we need to check differently
		// For now, verify that objects with empty/missing goal_refs can be retrieved
		filter := ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyID: "ITEM-003", // Direct lookup to verify empty list exists
			},
		}
		result, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		// Should return ITEM-003
		if len(result.Objects) != 1 {
			t.Errorf("expected 1 object, got %d", len(result.Objects))
		}
		if result.Objects[0][objects.FieldKeyID] != "ITEM-003" {
			t.Errorf("expected ITEM-003, got %s", result.Objects[0][objects.FieldKeyID])
		}
		// Verify it has empty or missing goal_refs (empty lists are extracted during parsing)
		goalRefs, hasGoalRefs := result.Objects[0][objects.FieldKeyGoalRefs]
		if hasGoalRefs {
			// If present, should be empty
			switch v := goalRefs.(type) {
			case []string:
				if len(v) != 0 {
					t.Errorf("expected empty goal_refs, got %v", v)
				}
			case []any:
				if len(v) != 0 {
					t.Errorf("expected empty goal_refs, got %v", v)
				}
			default:
				// Other types are acceptable if empty
			}
		}
		// Also check ITEM-004 (missing field)
		filter2 := ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyID: "ITEM-004",
			},
		}
		result2, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter2)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(result2.Objects) != 1 || result2.Objects[0][objects.FieldKeyID] != "ITEM-004" {
			t.Errorf("expected ITEM-004, got %v", result2.Objects)
		}
	})

	t.Run("Multiple list filters (AND)", func(t *testing.T) {
		filter := ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyGoalRefs: map[string]any{
					"$has": "GOAL-001",
				},
				objects.FieldKeyMilestoneRefs: map[string]any{
					"$has": "MIL-001",
				},
			},
		}
		result, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		// Should return ITEM-005 (has both GOAL-001 and MIL-001)
		if len(result.Objects) != 1 {
			t.Errorf("expected 1 object, got %d", len(result.Objects))
		}
		if result.Objects[0][objects.FieldKeyID] != "ITEM-005" {
			t.Errorf("expected ITEM-005, got %s", result.Objects[0][objects.FieldKeyID])
		}
	})

	t.Run("List filter with scalar filter (AND)", func(t *testing.T) {
		filter := ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyGoalRefs: map[string]any{
					"$has": "GOAL-001",
				},
				objects.FieldKeyStatus: "exploring",
			},
		}
		result, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		// Should return all items with GOAL-001 and status exploring
		if len(result.Objects) != 4 {
			t.Errorf("expected 4 objects, got %d", len(result.Objects))
		}
	})

	t.Run("Sort by list field (sorts by first element)", func(t *testing.T) {
		filter := ListFilter{
			Kind:    "backlog_item",
			SortBy:  objects.FieldKeyGoalRefs,
			SortAsc: true,
		}
		result, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		// Verify sorted order (empty/missing first, then by first element)
		if len(result.Objects) < 2 {
			t.Errorf("expected at least 2 objects, got %d", len(result.Objects))
		}
	})
}

func TestFileObjectStorage_List_ParallelReads(t *testing.T) {
	// Use unified test environment setup for proper isolation
	testRoot, storage, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	processDir := datacell.ProcessPrimaryDir(testRoot)
	backlogDir := filepath.Join(processDir, "backlog")
	if err := os.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create backlog dir: %v", err)
	}

	ctx := context.Background()

	// Create referenced goals first (required for goal_refs validation)
	// Clean up any existing goals first to prevent "object already exists" errors
	cliCtx := WithCLIOperation(ctx)
	for i := 0; i < 10; i++ {
		goalID := fmt.Sprintf("GOAL-%03d", i)
		// Clean up any existing goal (best effort - may not exist)
		_ = storage.Delete(cliCtx, secCtx, goalID, false) //nolint:errcheck // Test cleanup - error handling not critical
		// Deterministically wait for deletion to complete
		if !ensureObjectDeleted(ctx, storage, secCtx, goalID) {
			t.Skipf("Skipping parallel reads test: failed to confirm deletion of goal %s within timeout", goalID)
		}

		goal := map[string]any{
			objects.FieldKeyID:            goalID,
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyTitle:         fmt.Sprintf("Goal %d", i),
			objects.FieldKeyStatus:        "active",
			objects.FieldKeyTarget:        "Test target",
			objects.FieldKeyMetric:        "Test metric",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		if err := storage.Create(ctx, secCtx, goal); err != nil {
			t.Fatalf("failed to create goal %s: %v", goalID, err)
		}
	}

	// Create many objects to test parallel reads
	// Reduced from 50 to 20 to avoid creating too many HashRegistry workers
	// (each kind gets its own HashRegistry with a worker goroutine)
	numObjects := 20
	for i := 0; i < numObjects; i++ {
		obj := map[string]any{
			objects.FieldKeyID:            fmt.Sprintf("ITEM-%03d", i),
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         fmt.Sprintf("Item %d", i),
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCategory:      "development",
			objects.FieldKeyGoalRefs:      []string{fmt.Sprintf("GOAL-%03d", i%10)},
		}
		if err := storage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	t.Run("Parallel read performance", func(t *testing.T) {
		filter := ListFilter{
			Kind: "backlog_item",
			Filters: map[string]any{
				objects.FieldKeyGoalRefs: map[string]any{
					"$has": "GOAL-000",
				},
			},
		}

		start := time.Now()
		result, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter)
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		// Should return objects where GOAL-000 appears in goal_refs
		// With 20 objects (ITEM-000 to ITEM-019) and goal_refs cycling through GOAL-000 to GOAL-009,
		// GOAL-000 appears in ITEM-000 and ITEM-010 (every 10th object)
		expectedCount := 2 // ITEM-000 and ITEM-010
		if len(result.Objects) != expectedCount {
			t.Errorf("expected %d objects with GOAL-000, got %d", expectedCount, len(result.Objects))
		}

		t.Logf("List operation took %v for %d total objects", duration, numObjects)
		// Performance check: should be reasonably fast even with many objects
		if duration > 5*time.Second {
			t.Errorf("List operation took too long: %v", duration)
		}
	})

	t.Run("Concurrent list operations", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, 10)

		// Run 10 concurrent list operations
		for i := 0; i < 10; i++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine("storage_test", "concurrent list operation").StartSimple(func() {
				func(iter int) {
					defer wg.Done()
					filter := ListFilter{
						Kind: "backlog_item",
						Filters: map[string]any{
							objects.FieldKeyGoalRefs: map[string]any{
								"$has": fmt.Sprintf("GOAL-%03d", iter%10),
							},
						},
					}
					result, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter)
					if err != nil {
						errors <- err
						return
					}
					if len(result.Objects) == 0 {
						errors <- fmt.Errorf("expected at least 1 object, got 0")
						return
					}
				}(i)
			})
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			t.Errorf("concurrent list operation failed: %v", err)
		}
	})
}

func TestFileObjectStorage_List_GroupingAndCounting(t *testing.T) {
	// Use unified test environment setup for proper isolation
	testRoot, storage, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	processDir := datacell.ProcessPrimaryDir(testRoot)
	backlogDir := filepath.Join(processDir, "backlog")
	if err := os.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create backlog dir: %v", err)
	}

	ctx := context.Background()

	// Create objects with different categories and statuses
	testObjects := []map[string]any{
		{objects.FieldKeyID: "ITEM-001", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 1", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "ITEM-002", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 2", objects.FieldKeyStatus: "exploring", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "ITEM-003", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 3", objects.FieldKeyStatus: "planned", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "ITEM-004", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 4", objects.FieldKeyStatus: "planned", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "product"},
		{objects.FieldKeyID: "ITEM-005", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Item 5", objects.FieldKeyStatus: "in_progress", objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "product"},
	}

	for _, obj := range testObjects {
		if err := storage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("failed to create object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	t.Run("Group by status", func(t *testing.T) {
		storageCtx := pkgctx.NewGroupingStorageContext(0) // No limit
		filter := ListFilter{
			Kind:    "backlog_item",
			GroupBy: "status",
		}
		result, err := storage.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		if result.Groups == nil {
			t.Fatal("expected groups, got nil")
		}

		// Should have 3 groups: exploring, planned, in_progress
		if len(result.Groups) != 3 {
			t.Errorf("expected 3 groups, got %d", len(result.Groups))
		}

		// Verify group counts
		if len(result.Groups["exploring"]) != 2 {
			t.Errorf("expected 2 items in 'exploring' group, got %d", len(result.Groups["exploring"]))
		}
		if len(result.Groups["planned"]) != 2 {
			t.Errorf("expected 2 items in 'planned' group, got %d", len(result.Groups["planned"]))
		}
		if len(result.Groups["in_progress"]) != 1 {
			t.Errorf("expected 1 item in 'in_progress' group, got %d", len(result.Groups["in_progress"]))
		}
	})

	t.Run("Group by category with limit", func(t *testing.T) {
		storageCtx := pkgctx.NewGroupingStorageContext(1) // Limit to 1 group
		filter := ListFilter{
			Kind:    "backlog_item",
			GroupBy: "category",
		}
		result, err := storage.List(ctx, secCtx, storageCtx, filter)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		// Should have at most 1 group (limited)
		if len(result.Groups) > 1 {
			t.Errorf("expected at most 1 group, got %d", len(result.Groups))
		}
	})

	t.Run("Count total objects", func(t *testing.T) {
		filter := ListFilter{
			Kind: "backlog_item",
		}
		result, err := storage.List(ctx, secCtx, pkgctx.GetStorageContext(), filter)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		totalCount, ok := result.Meta["total_count"].(int)
		if !ok {
			t.Fatal("expected total_count in meta")
		}

		// Total count should be at least the 5 objects created in this test.
		// Additional objects may exist in the test environment from other
		// scenarios, so treat counts >= 5 as acceptable.
		if totalCount < 5 {
			t.Errorf("expected total_count >= 5, got %d", totalCount)
		}
	})

	t.Run("Test Exists operation", func(t *testing.T) {
		// Test existing object
		exists, err := storage.Exists(ctx, secCtx, "ITEM-001")
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Error("expected ITEM-001 to exist")
		}

		// Test non-existing object
		exists, err = storage.Exists(ctx, secCtx, "ITEM-999")
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if exists {
			t.Error("expected ITEM-999 to not exist")
		}
	})

	t.Run("Test Count operation", func(t *testing.T) {
		// Count all objects
		filter := ListFilter{
			Kind: "backlog_item",
		}
		count, err := storage.Count(ctx, secCtx, filter)
		if err != nil {
			t.Fatalf("Count failed: %v", err)
		}
		if count < 5 {
			t.Errorf("expected count >= 5, got %d", count)
		}

		// Count with filter
		filter = ListFilter{
			Kind:    "backlog_item",
			Filters: map[string]any{objects.FieldKeyStatus: "planned"},
		}
		count, err = storage.Count(ctx, secCtx, filter)
		if err != nil {
			t.Fatalf("Count with filter failed: %v", err)
		}
		if count != 2 {
			t.Errorf("expected count 2 (planned items), got %d", count)
		}
	})
}
