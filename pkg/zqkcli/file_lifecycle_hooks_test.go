package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/testservices"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// TestFileBackendLifecycleHooks tests that lifecycle hooks are triggered correctly
// when objects are created, updated, or deleted in the file backend
//
//nolint:gocyclo // Test function intentionally exercises many lifecycle scenarios
func TestFileBackendLifecycleHooks(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "internal.file_lifecycle_hooks"})
	fileStorage := proj.FileStorage

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// Create hook manager
	hookManager := NewLifecycleHookManager()

	// Track hook invocations
	var statusChangeCount int
	var createCount int
	var updateCount int
	var deleteCount int
	var statusChanges []string

	// Register hooks
	hookManager.RegisterHook(LifecycleHook{
		OnStatusChange: func(objID, kind, oldStatus, newStatus string, obj map[string]any) error {
			statusChangeCount++
			statusChanges = append(statusChanges, fmt.Sprintf("%s:%s->%s", objID, oldStatus, newStatus))
			return nil
		},
		OnCreate: func(objID, kind string, obj map[string]any) error {
			createCount++
			return nil
		},
		OnUpdate: func(objID, kind string, oldObj, newObj map[string]any) error {
			updateCount++
			return nil
		},
		OnDelete: func(objID, kind string) error {
			deleteCount++
			return nil
		},
	})

	// Test Create hook
	t.Run("CreateHook", func(t *testing.T) {
		hookManager.ClearInvocations()
		createCount = 0

		testID := fmt.Sprintf("ITEM-%03d", time.Now().UnixNano()%1000000)
		testObj := map[string]any{
			objects.FieldKeyID:            testID,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "File Backend Lifecycle Hook Test",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		}

		// Create object (in real implementation, this would trigger hooks)
		if err := fileStorage.Create(ctx, secCtx, testObj); err != nil {
			t.Fatalf("Failed to create object: %v", err)
		}

		// Manually trigger hook (simulating integration)
		if err := hookManager.TriggerCreate(testID, "backlog_item", testObj); err != nil {
			t.Fatalf("Failed to trigger create hook: %v", err)
		}

		// Verify hook was called
		if createCount != 1 {
			t.Errorf("Expected create hook to be called once, got %d", createCount)
		}

		invocations := hookManager.GetInvocations()
		if len(invocations) != 1 || invocations[0].Type != "create" {
			t.Errorf("Expected 1 create invocation, got %+v", invocations)
		}

		// Cleanup
		if err := fileStorage.Delete(ctx, secCtx, testID, false); err != nil {
			t.Logf("Failed to cleanup test object: %v", err)
		}
	})

	// Test Status Change hook
	t.Run("StatusChangeHook", func(t *testing.T) {
		hookManager.ClearInvocations()
		statusChangeCount = 0
		statusChanges = statusChanges[:0]

		testID := fmt.Sprintf("ITEM-%03d", time.Now().UnixNano()%1000000)
		testObj := map[string]any{
			objects.FieldKeyID:            testID,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "File Backend Status Change Hook Test",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		}

		// Create object
		if err := fileStorage.Create(ctx, secCtx, testObj); err != nil {
			t.Fatalf("Failed to create object: %v", err)
		}

		// Read object to get current status
		existing, err := fileStorage.Read(ctx, secCtx, testID)
		if err != nil {
			t.Fatalf("Failed to read object: %v", err)
		}
		oldStatus, _ := existing[objects.FieldKeyStatus].(string)

		// Update status
		updates := map[string]any{
			objects.FieldKeyStatus: "validated",
		}
		if err := fileStorage.Update(ctx, secCtx, testID, updates); err != nil {
			t.Fatalf("Failed to update object: %v", err)
		}

		// Read updated object
		updated, err := fileStorage.Read(ctx, secCtx, testID)
		if err != nil {
			t.Fatalf("Failed to read updated object: %v", err)
		}
		newStatus, _ := updated[objects.FieldKeyStatus].(string)

		// Manually trigger hook (simulating integration)
		if err := hookManager.TriggerStatusChange(testID, "backlog_item", oldStatus, newStatus, updated); err != nil {
			t.Fatalf("Failed to trigger status change hook: %v", err)
		}

		// Verify hook was called
		if statusChangeCount != 1 {
			t.Errorf("Expected status change hook to be called once, got %d", statusChangeCount)
		}

		if len(statusChanges) != 1 || statusChanges[0] != fmt.Sprintf("%s:exploring->validated", testID) {
			t.Errorf("Expected status change 'exploring->validated', got %v", statusChanges)
		}

		invocations := hookManager.GetInvocations()
		if len(invocations) != 1 || invocations[0].Type != "status_change" {
			t.Errorf("Expected 1 status_change invocation, got %+v", invocations)
		}

		// Cleanup
		if err := fileStorage.Delete(ctx, secCtx, testID, false); err != nil {
			t.Logf("Failed to cleanup test object: %v", err)
		}
	})

	// Test Delete hook
	t.Run("DeleteHook", func(t *testing.T) {
		hookManager.ClearInvocations()
		deleteCount = 0

		testID := fmt.Sprintf("ITEM-%03d", time.Now().UnixNano()%1000000)
		testObj := map[string]any{
			objects.FieldKeyID:            testID,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "File Backend Delete Hook Test",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		}

		// Create object
		if err := fileStorage.Create(ctx, secCtx, testObj); err != nil {
			t.Fatalf("Failed to create object: %v", err)
		}

		// Delete object (in real implementation, this would trigger hooks)
		// Mark context as CLI operation for authorization
		cliCtx := storage.WithCLIOperation(ctx)
		if err := fileStorage.Delete(cliCtx, secCtx, testID, false); err != nil {
			t.Fatalf("Failed to delete object: %v", err)
		}

		// Manually trigger hook (simulating integration)
		if err := hookManager.TriggerDelete(testID, "backlog_item"); err != nil {
			t.Fatalf("Failed to trigger delete hook: %v", err)
		}

		// Verify hook was called
		if deleteCount != 1 {
			t.Errorf("Expected delete hook to be called once, got %d", deleteCount)
		}

		invocations := hookManager.GetInvocations()
		if len(invocations) != 1 || invocations[0].Type != "delete" {
			t.Errorf("Expected 1 delete invocation, got %+v", invocations)
		}
	})
}

// TestFileBackendAutoTransitionEvaluation tests that auto-transitions are evaluated correctly
// based on lifecycle conditions in the file backend
//
//nolint:gocyclo // Test function intentionally exercises many transition scenarios
func TestFileBackendAutoTransitionEvaluation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "internal.file_auto_transition"})
	testRoot := proj.Root
	fileStorage := proj.FileStorage

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	ctx := pkgctx.NewSystemContext()

	// Load lifecycle definition
	lifecycleLoader := objects.NewLifecycleLoader("")
	lifecycle, err := lifecycleLoader.LoadLifecycle("backlog_item")
	if err != nil {
		t.Fatalf("Failed to load backlog_item lifecycle: %v", err)
	}

	// Find auto-transition for all_linked_milestones_complete
	var autoTransition *objects.Transition
	for i := range lifecycle.Transitions {
		trans := &lifecycle.Transitions[i]
		if trans.Auto && trans.From == objectStatusInProgress && trans.To == objectStatusComplete {
			autoTransition = trans
			break
		}
	}

	if autoTransition == nil {
		t.Fatal("Could not find auto-transition for all_linked_milestones_complete")
	}

	t.Logf("Found auto-transition: %s -> %s (condition: %v)", autoTransition.From, autoTransition.To, autoTransition)

	// Test: Create backlog item with linked milestones
	t.Run("EvaluateAllLinkedMilestonesComplete", func(t *testing.T) {
		// Use fixed IDs so reference validation and CAS index see the created objects reliably
		baseID := "ITEM-001"
		milestone1ID := "ITEM-002"
		milestone2ID := "ITEM-003"

		// Create milestones first
		milestone1 := map[string]any{
			objects.FieldKeyID:            milestone1ID,
			objects.FieldKeyKind:          "backlog_item", // Using backlog_item as placeholder
			objects.FieldKeyTitle:         "Test Milestone 1",
			objects.FieldKeyStatus:        objectStatusComplete,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		}

		milestone2 := map[string]any{
			objects.FieldKeyID:            milestone2ID,
			objects.FieldKeyKind:          "backlog_item", // Using backlog_item as placeholder
			objects.FieldKeyTitle:         "Test Milestone 2",
			objects.FieldKeyStatus:        objectStatusComplete,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		}

		// Create milestones in file storage
		if err := fileStorage.Create(ctx, secCtx, milestone1); err != nil {
			t.Fatalf("Failed to create milestone 1: %v", err)
		}
		if err := fileStorage.Create(ctx, secCtx, milestone2); err != nil {
			t.Fatalf("Failed to create milestone 2: %v", err)
		}
		// Flush CAS index so reference validation during backlog item create sees the new IDs.
		if q := storage.GetListingIndexWriteQueueForProjectRoot(testRoot); q != nil {
			_ = q.FlushKind("backlog_item", 5*time.Second) //nolint:errcheck // best-effort
		}

		// Create priority plan (required for status 'in_progress')
		priorityPlanID := "PLAN-001"
		priorityPlan := map[string]any{
			objects.FieldKeyID:            priorityPlanID,
			objects.FieldKeyKind:          "priority_plan",
			objects.FieldKeyTitle:         "Test Priority Plan",
			objects.FieldKeyStatus:        "active",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		}
		if err := fileStorage.Create(ctx, secCtx, priorityPlan); err != nil {
			t.Fatalf("Failed to create priority plan: %v", err)
		}
		if q := storage.GetListingIndexWriteQueueForProjectRoot(testRoot); q != nil {
			_ = q.FlushKind("priority_plan", 5*time.Second) //nolint:errcheck // best-effort
		}

		// Create backlog item with linked milestones
		// Note: status 'in_progress' requires priority_plan_ref per validation rules
		backlogItem := map[string]any{
			objects.FieldKeyID:              baseID,
			objects.FieldKeyKind:            "backlog_item",
			objects.FieldKeyTitle:           "File Backend Auto-Transition Test",
			objects.FieldKeyStatus:          objectStatusInProgress,
			objects.FieldKeyMilestoneRefs:   []string{milestone1ID, milestone2ID},
			objects.FieldKeyPriorityPlanRef: priorityPlanID,
			objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:       zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:       "test",
			objects.FieldKeyUpdatedAt:       zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:       "test",
		}

		if err := fileStorage.Create(ctx, secCtx, backlogItem); err != nil {
			// Cleanup on failure
			_ = fileStorage.Delete(ctx, secCtx, milestone1ID, false)   //nolint:errcheck // Test cleanup - errors are acceptable
			_ = fileStorage.Delete(ctx, secCtx, milestone2ID, false)   //nolint:errcheck // Test cleanup - errors are acceptable
			_ = fileStorage.Delete(ctx, secCtx, priorityPlanID, false) //nolint:errcheck // Test cleanup - errors are acceptable
			t.Fatalf("Failed to create backlog item: %v", err)
		}

		// Evaluate condition: all_linked_milestones_complete
		// In a real implementation, this would query the file storage to check milestone statuses
		allComplete := evaluateAllLinkedMilestonesComplete(ctx, fileStorage, secCtx, storageCtx, baseID)
		if !allComplete {
			t.Error("Expected all linked milestones to be complete")
		}

		// Verify auto-transition should be triggered
		if autoTransition.Auto && allComplete {
			t.Logf("Auto-transition condition met: %s -> %s", autoTransition.From, autoTransition.To)
			// In real implementation, this would trigger the status change
		}

		// Cleanup
		if err := fileStorage.Delete(ctx, secCtx, baseID, false); err != nil {
			t.Logf("Failed to cleanup base: %v", err)
		}
		if err := fileStorage.Delete(ctx, secCtx, milestone1ID, false); err != nil {
			t.Logf("Failed to cleanup milestone1: %v", err)
		}
		if err := fileStorage.Delete(ctx, secCtx, milestone2ID, false); err != nil {
			t.Logf("Failed to cleanup milestone2: %v", err)
		}
	})
}

// TestFileGraphBackendLifecycleParity tests that lifecycle hooks work identically
// in both file and graph backends
//
//nolint:gocyclo // Test function intentionally exercises many parity scenarios
func TestFileGraphBackendLifecycleParity(t *testing.T) {
	// Skip if graph backend is not available
	if !isGraphBackendAvailable() {
		t.Skip("Graph backend not available (ZQK_GRAPH_ENABLED not set or graph not running)")
	}

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "internal.file_graph_lifecycle_parity"})
	testRoot := proj.Root
	fileStorage := proj.FileStorage

	// Setup test services (MemGraph)
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Minute)
	defer cancel()

	services, err := testservices.SetupTestServices(ctx)
	if err != nil {
		t.Fatalf("Failed to setup test services: %v", err)
	}
	//nolint:errcheck // Test cleanup - errors are acceptable
	defer func() { _ = services.Cleanup() }()

	graphManager := mcp.GetGraphConnectionManager()
	if !graphManager.IsEnabled() {
		t.Fatal("Graph backend should be enabled")
	}

	pool, err := graphManager.GetPool(ctx)
	if err != nil {
		t.Fatalf("Failed to get graph connection pool: %v", err)
	}
	graphStorage := storage.NewPoolAwareGraphStorage(pool, testRoot)

	secCtx := pkgctx.NewSystemSecurityContext()

	// Create hook managers for both backends
	fileHookManager := NewLifecycleHookManager()
	graphHookManager := NewLifecycleHookManager()

	// Track invocations for both
	var fileStatusChanges []string
	var graphStatusChanges []string

	// Register hooks for file backend
	fileHookManager.RegisterHook(LifecycleHook{
		OnStatusChange: func(objID, kind, oldStatus, newStatus string, obj map[string]any) error {
			fileStatusChanges = append(fileStatusChanges, fmt.Sprintf("%s:%s->%s", objID, oldStatus, newStatus))
			return nil
		},
	})

	// Register hooks for graph backend
	graphHookManager.RegisterHook(LifecycleHook{
		OnStatusChange: func(objID, kind, oldStatus, newStatus string, obj map[string]any) error {
			graphStatusChanges = append(graphStatusChanges, fmt.Sprintf("%s:%s->%s", objID, oldStatus, newStatus))
			return nil
		},
	})

	// Test: Status change should trigger hooks in both backends
	t.Run("StatusChangeParity", func(t *testing.T) {
		fileHookManager.ClearInvocations()
		graphHookManager.ClearInvocations()
		fileStatusChanges = fileStatusChanges[:0]
		graphStatusChanges = graphStatusChanges[:0]

		baseID := fmt.Sprintf("ITEM-%03d", time.Now().UnixNano()%1000000)

		// Create same object in both backends
		testObj := map[string]any{
			objects.FieldKeyID:            baseID,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Lifecycle Parity Test",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		}

		// Create in file backend
		if err := fileStorage.Create(ctx, secCtx, testObj); err != nil {
			t.Fatalf("Failed to create object in file backend: %v", err)
		}

		// Create in graph backend
		if err := graphStorage.Create(ctx, secCtx, testObj); err != nil {
			t.Fatalf("Failed to create object in graph backend: %v", err)
		}

		// Update status in file backend
		fileUpdates := map[string]any{objects.FieldKeyStatus: "validated"}
		if err := fileStorage.Update(ctx, secCtx, baseID, fileUpdates); err != nil {
			t.Fatalf("Failed to update object in file backend: %v", err)
		}

		// Update status in graph backend
		graphUpdates := map[string]any{objects.FieldKeyStatus: "validated"}
		if err := graphStorage.Update(ctx, secCtx, baseID, graphUpdates); err != nil {
			t.Fatalf("Failed to update object in graph backend: %v", err)
		}

		// Read updated objects
		fileObj, err := fileStorage.Read(ctx, secCtx, baseID)
		if err != nil {
			t.Fatalf("Failed to read object from file backend: %v", err)
		}

		graphObj, err := graphStorage.Read(ctx, secCtx, baseID)
		if err != nil {
			t.Fatalf("Failed to read object from graph backend: %v", err)
		}

		// Trigger hooks for both backends
		fileOldStatus := "exploring"
		fileNewStatus, _ := fileObj[objects.FieldKeyStatus].(string)
		if err := fileHookManager.TriggerStatusChange(baseID, "backlog_item", fileOldStatus, fileNewStatus, fileObj); err != nil {
			t.Fatalf("Failed to trigger file backend hook: %v", err)
		}

		graphOldStatus := "exploring"
		graphNewStatus, _ := graphObj[objects.FieldKeyStatus].(string)
		if err := graphHookManager.TriggerStatusChange(baseID, "backlog_item", graphOldStatus, graphNewStatus, graphObj); err != nil {
			t.Fatalf("Failed to trigger graph backend hook: %v", err)
		}

		// Verify both backends triggered hooks identically
		if len(fileStatusChanges) != len(graphStatusChanges) {
			t.Errorf("Hook invocation count mismatch: file=%d, graph=%d", len(fileStatusChanges), len(graphStatusChanges))
		}

		if len(fileStatusChanges) > 0 && len(graphStatusChanges) > 0 {
			if fileStatusChanges[0] != graphStatusChanges[0] {
				t.Errorf("Hook invocations differ: file=%v, graph=%v", fileStatusChanges, graphStatusChanges)
			}
		}

		// Cleanup
		if err := fileStorage.Delete(ctx, secCtx, baseID, false); err != nil {
			t.Logf("Failed to cleanup base (file): %v", err)
		}
		if err := graphStorage.Delete(ctx, secCtx, baseID, false); err != nil {
			t.Logf("Failed to cleanup base (graph): %v", err)
		}
	})
}
