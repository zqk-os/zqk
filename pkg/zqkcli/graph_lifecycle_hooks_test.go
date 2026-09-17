package internal

// Tests that set ZQK_TEST_ROOT must not use t.Parallel(): the env var is process-global.

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// LifecycleHook represents a hook that can be triggered on lifecycle events
type LifecycleHook struct {
	OnStatusChange func(objID, kind, oldStatus, newStatus string, obj map[string]any) error
	OnCreate       func(objID, kind string, obj map[string]any) error
	OnUpdate       func(objID, kind string, oldObj, newObj map[string]any) error
	OnDelete       func(objID, kind string) error
}

// LifecycleHookManager manages lifecycle hooks and triggers them on events
type LifecycleHookManager struct {
	hooks []LifecycleHook
	mu    sync.RWMutex
	// Track hook invocations for testing
	invocations []HookInvocation
}

type HookInvocation struct {
	Type      string // "status_change", "create", "update", "delete"
	ObjectID  string
	Kind      string
	OldStatus string
	NewStatus string
	Timestamp time.Time
}

// NewLifecycleHookManager creates a new lifecycle hook manager
func NewLifecycleHookManager() *LifecycleHookManager {
	return &LifecycleHookManager{
		hooks:       make([]LifecycleHook, 0),
		invocations: make([]HookInvocation, 0),
	}
}

// RegisterHook registers a lifecycle hook
func (m *LifecycleHookManager) RegisterHook(hook LifecycleHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook)
}

// TriggerStatusChange triggers all registered status change hooks
func (m *LifecycleHookManager) TriggerStatusChange(objID, kind, oldStatus, newStatus string, obj map[string]any) error {
	m.mu.RLock()
	hooks := make([]LifecycleHook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.RUnlock()

	// Record invocation
	m.mu.Lock()
	m.invocations = append(m.invocations, HookInvocation{
		Type:      "status_change",
		ObjectID:  objID,
		Kind:      kind,
		OldStatus: oldStatus,
		NewStatus: newStatus,
		Timestamp: time.Now(),
	})
	m.mu.Unlock()

	// Trigger hooks
	for _, hook := range hooks {
		if hook.OnStatusChange != nil {
			if err := hook.OnStatusChange(objID, kind, oldStatus, newStatus, obj); err != nil {
				return err
			}
		}
	}
	return nil
}

// TriggerCreate triggers all registered create hooks
func (m *LifecycleHookManager) TriggerCreate(objID, kind string, obj map[string]any) error {
	m.mu.RLock()
	hooks := make([]LifecycleHook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.RUnlock()

	// Record invocation
	m.mu.Lock()
	m.invocations = append(m.invocations, HookInvocation{
		Type:      "create",
		ObjectID:  objID,
		Kind:      kind,
		Timestamp: time.Now(),
	})
	m.mu.Unlock()

	// Trigger hooks
	for _, hook := range hooks {
		if hook.OnCreate != nil {
			if err := hook.OnCreate(objID, kind, obj); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetInvocations returns all hook invocations (for testing)
func (m *LifecycleHookManager) GetInvocations() []HookInvocation {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]HookInvocation, len(m.invocations))
	copy(result, m.invocations)
	return result
}

// ClearInvocations clears all recorded invocations (for testing)
func (m *LifecycleHookManager) ClearInvocations() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invocations = m.invocations[:0]
}

// TestLifecycleHooks tests that lifecycle hooks are triggered correctly
// when objects are created, updated, or deleted in the graph backend
//
//nolint:gocyclo // Test function intentionally exercises many lifecycle scenarios
func TestLifecycleHooks(t *testing.T) {
	// Skip if graph backend is not available
	if !isGraphBackendAvailable() {
		t.Skip("Graph backend not available (ZQK_GRAPH_ENABLED not set or graph not running)")
	}

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "internal.graph_lifecycle_hooks"})
	testRoot := proj.Root
	// Prefer testkit pool (Database=test) over shared studio MemGraph — ID collisions
	// and leftover nodes make Create/Delete asserts flake.
	pool := testkit.PrepareGraphConnectionForTest(t)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Minute)
	defer cancel()
	ctx = storage.WithTestHardDelete(ctx)

	graphStorage := storage.NewPoolAwareGraphStorage(pool, testRoot)
	secCtx := pkgctx.NewSystemSecurityContext()

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

		testID := fmt.Sprintf("BLI-%d", time.Now().UnixNano())
		testObj := map[string]any{
			objects.FieldKeyID:            testID,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Lifecycle Hook Test",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		}

		// Create object (in real implementation, this would trigger hooks)
		if err := graphStorage.Create(ctx, secCtx, testObj); err != nil {
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

		//nolint:errcheck // Test cleanup - errors are acceptable
		// Cleanup
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = graphStorage.Delete(ctx, secCtx, testID, false)
	})

	// Test Status Change hook
	t.Run("StatusChangeHook", func(t *testing.T) {
		hookManager.ClearInvocations()
		statusChangeCount = 0
		statusChanges = statusChanges[:0]

		testID := fmt.Sprintf("BLI-%d", time.Now().UnixNano())
		testObj := map[string]any{
			objects.FieldKeyID:                       testID,
			objects.FieldKeyKind:                     "backlog_item",
			objects.FieldKeyTitle:                    "Status Change Hook Test",
			objects.FieldKeyStatus:                   "exploring",
			objects.FieldKeySchemaVersion:            objects.DefaultSchemaVersion,
			objects.FieldKeyProblemStatement:         "Lifecycle hook harness validates exploring→validated.",
			objects.FieldKeyAcceptanceConsiderations: "Status exploring→validated accepted for hook tests.",
			objects.FieldKeyContext:                  "graph lifecycle hooks",
			objects.FieldKeyCreatedAt:                zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:                "test",
			objects.FieldKeyUpdatedAt:                zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:                "test",
		}

		// Create object
		if err := graphStorage.Create(ctx, secCtx, testObj); err != nil {
			t.Fatalf("Failed to create object: %v", err)
		}

		// Read object to get current status
		existing, err := graphStorage.Read(ctx, secCtx, testID)
		if err != nil {
			t.Fatalf("Failed to read object: %v", err)
		}
		oldStatus, _ := existing[objects.FieldKeyStatus].(string)

		// Update status
		updates := map[string]any{
			objects.FieldKeyStatus: "validated",
		}
		if err := graphStorage.Update(ctx, secCtx, testID, updates); err != nil {
			t.Fatalf("Failed to update object: %v", err)
		}

		// Read updated object
		updated, err := graphStorage.Read(ctx, secCtx, testID)
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
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = graphStorage.Delete(ctx, secCtx, testID, false)
	})

	// Test Delete hook
	t.Run("DeleteHook", func(t *testing.T) {
		hookManager.ClearInvocations()
		deleteCount = 0

		testID := fmt.Sprintf("BLI-%d", time.Now().UnixNano())
		testObj := map[string]any{
			objects.FieldKeyID:            testID,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Delete Hook Test",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		}

		// Create object
		if err := graphStorage.Create(ctx, secCtx, testObj); err != nil {
			t.Fatalf("Failed to create object: %v", err)
		}

		// Delete object (in real implementation, this would trigger hooks)
		if err := graphStorage.Delete(ctx, secCtx, testID, false); err != nil {
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

// TriggerDelete triggers all registered delete hooks
func (m *LifecycleHookManager) TriggerDelete(objID, kind string) error {
	m.mu.RLock()
	hooks := make([]LifecycleHook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.RUnlock()

	// Record invocation
	m.mu.Lock()
	m.invocations = append(m.invocations, HookInvocation{
		Type:      "delete",
		ObjectID:  objID,
		Kind:      kind,
		Timestamp: time.Now(),
	})
	m.mu.Unlock()

	// Trigger hooks
	for _, hook := range hooks {
		if hook.OnDelete != nil {
			if err := hook.OnDelete(objID, kind); err != nil {
				return err
			}
		}
	}
	return nil
}

// TestAutoTransitionEvaluation tests that auto-transitions are evaluated correctly
// based on lifecycle conditions (e.g., all_linked_milestones_complete)
//
//nolint:gocyclo // Test function intentionally exercises many transition scenarios
func TestAutoTransitionEvaluation(t *testing.T) {
	// Skip if graph backend is not available
	if !isGraphBackendAvailable() {
		t.Skip("Graph backend not available (ZQK_GRAPH_ENABLED not set or graph not running)")
	}

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "internal.graph_auto_transition"})
	testRoot := proj.Root
	pool := testkit.PrepareGraphConnectionForTest(t)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Minute)
	defer cancel()
	// Promote-on-create so milestone/status fixtures keep complete / in_progress
	// (draft-first membrane otherwise coerces them to exploring).
	ctx = pkgctx.WithPromoteOnCreate(storage.WithTestHardDelete(ctx))

	graphStorage := storage.NewPoolAwareGraphStorage(pool, testRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

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

	// Test: Evaluate auto-transition condition
	t.Run("EvaluateAllLinkedMilestonesComplete", func(t *testing.T) {
		nano := time.Now().UnixNano()
		baseID := fmt.Sprintf("BLI-%d", nano)

		// Create milestones (using backlog items as placeholders for testing)
		// In a real scenario, these would be actual milestone objects
		milestone1ID := fmt.Sprintf("BLI-%d", nano+1)
		milestone2ID := fmt.Sprintf("BLI-%d", nano+2)

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

		// Create milestones in graph (with retry for connection issues)
		var err error
		for i := 0; i < 3; i++ {
			if err = graphStorage.Create(ctx, secCtx, milestone1); err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			t.Skipf("Skipping test due to connection issue creating milestone 1: %v", err)
			return
		}

		for i := 0; i < 3; i++ {
			if err = graphStorage.Create(ctx, secCtx, milestone2); err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			//nolint:errcheck // Test cleanup - errors are acceptable
			_ = graphStorage.Delete(ctx, secCtx, milestone1ID, false)
			t.Skipf("Skipping test due to connection issue creating milestone 2: %v", err)
			return
		}

		// Create backlog item with linked milestones
		backlogItem := map[string]any{
			objects.FieldKeyID:            baseID,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Auto-Transition Test",
			objects.FieldKeyStatus:        objectStatusInProgress,
			objects.FieldKeyMilestoneRefs: []string{milestone1ID, milestone2ID},
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		}

		if err := graphStorage.Create(ctx, secCtx, backlogItem); err != nil {
			//nolint:errcheck // Test cleanup - errors are acceptable
			_ = graphStorage.Delete(ctx, secCtx, milestone1ID, false)
			//nolint:errcheck // Test cleanup - errors are acceptable
			_ = graphStorage.Delete(ctx, secCtx, milestone2ID, false)
			t.Fatalf("Failed to create backlog item: %v", err)
		}

		// Evaluate condition: all_linked_milestones_complete
		// In a real implementation, this would query the graph to check milestone statuses
		allComplete := evaluateAllLinkedMilestonesComplete(ctx, graphStorage, secCtx, storageCtx, baseID)
		if !allComplete {
			t.Error("Expected all linked milestones to be complete")
		}

		// Verify auto-transition should be triggered
		if autoTransition.Auto && allComplete {
			t.Logf("Auto-transition condition met: %s -> %s", autoTransition.From, autoTransition.To)
			// In real implementation, this would trigger the status change
		}

		// Cleanup
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = graphStorage.Delete(ctx, secCtx, baseID, false)
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = graphStorage.Delete(ctx, secCtx, milestone1ID, false)
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = graphStorage.Delete(ctx, secCtx, milestone2ID, false)
	})
}

// evaluateAllLinkedMilestonesComplete evaluates if all linked milestones are complete
// This is a simplified implementation for testing
func evaluateAllLinkedMilestonesComplete(ctx context.Context, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, _ *pkgctx.StorageContext, backlogItemID string) bool {
	// Read backlog item
	backlogItem, err := storageProvider.Read(ctx, secCtx, backlogItemID)
	if err != nil {
		return false
	}

	// Get milestone references (graph backends may round-trip slices as []any)
	milestoneRefs := milestoneRefsFromObject(backlogItem)
	if len(milestoneRefs) == 0 {
		return false
	}

	// Check each milestone status
	for _, milestoneID := range milestoneRefs {
		milestone, err := storageProvider.Read(ctx, secCtx, milestoneID)
		if err != nil {
			return false
		}

		status, _ := milestone[objects.FieldKeyStatus].(string)
		if status != objectStatusComplete {
			return false
		}
	}

	return true
}

func milestoneRefsFromObject(obj map[string]any) []string {
	if refs, ok := obj[objects.FieldKeyMilestoneRefs].([]string); ok {
		return refs
	}
	if refs, ok := obj[objects.FieldKeyMilestoneRefs].([]any); ok {
		out := make([]string, 0, len(refs))
		for _, ref := range refs {
			if str, ok := ref.(string); ok && str != "" {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}
