package utility

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func setupTestStorageProvider(t *testing.T, projectRoot string) storage.ObjectStorageProvider {
	t.Helper()

	factory := storage.NewTestingFactory()
	anyStorage, err := factory.CreateFileStorage(projectRoot)
	if err != nil {
		t.Fatalf("Failed to create test file storage: %v", err)
	}

	provider, ok := anyStorage.(storage.ObjectStorageProvider)
	if !ok {
		t.Fatalf("Expected file storage provider type, got %T", anyStorage)
	}

	testkit.RegisterStorageTestCleanup(t, projectRoot, anyStorage)

	return provider
}

// TestBuildDependencyGraphAndSort_BasicOrdering tests that objects are sorted by dependencies
func TestBuildDependencyGraphAndSort_BasicOrdering(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Create objects with dependencies: goal depends on workstream, workstream depends on account
	objSlice := []map[string]any{
		{
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyID:            "GOAL-001",
			objects.FieldKeyTitle:         "Test Goal",
			objects.FieldKeyWorkstreamRef: "WS-001",
		},
		{
			objects.FieldKeyKind:     "workstream",
			objects.FieldKeyID:       "WS-001",
			objects.FieldKeyTitle:    "Test Workstream",
			objects.FieldKeyOwnerRef: "ACC-TEST",
		},
		{
			objects.FieldKeyKind:     "account",
			objects.FieldKeyID:       "ACC-TEST",
			objects.FieldKeyTitle:    "Test Account",
			objects.FieldKeyUsername: "test",
		},
	}

	layers, err := builder.buildDependencyGraphAndSort(objSlice)
	if err != nil {
		t.Fatalf("buildDependencyGraphAndSort failed: %v", err)
	}

	// Flatten layers to verify order
	var sorted []map[string]any
	for _, layer := range layers {
		sorted = append(sorted, layer...)
	}

	// Verify order: account should come first, then workstream, then goal
	if len(sorted) != 3 {
		t.Fatalf("Expected 3 objects, got %d", len(sorted))
	}

	// Account should be first (no dependencies)
	if id, ok := sorted[0][objects.FieldKeyID].(string); !ok || id != "ACC-TEST" {
		t.Errorf("Expected account to be first, got %v", sorted[0][objects.FieldKeyID])
	}

	// Workstream should be second (depends on account)
	if id, ok := sorted[1][objects.FieldKeyID].(string); !ok || id != "WS-001" {
		t.Errorf("Expected workstream to be second, got %v", sorted[1][objects.FieldKeyID])
	}

	// Goal should be third (depends on workstream)
	if id, ok := sorted[2][objects.FieldKeyID].(string); !ok || id != "GOAL-001" {
		t.Errorf("Expected goal to be third, got %v", sorted[2][objects.FieldKeyID])
	}
}

// TestBuildDependencyGraphAndSort_ForwardReferences tests that forward references within data file are allowed
func TestBuildDependencyGraphAndSort_ForwardReferences(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Create objects with forward reference: goal references workstream that comes later in the list
	objSlice := []map[string]any{
		{
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyID:            "GOAL-001",
			objects.FieldKeyTitle:         "Test Goal",
			objects.FieldKeyWorkstreamRef: "WS-001", // Forward reference
		},
		{
			objects.FieldKeyKind:  "workstream",
			objects.FieldKeyID:    "WS-001",
			objects.FieldKeyTitle: "Test Workstream",
		},
	}

	layers, err := builder.buildDependencyGraphAndSort(objSlice)
	if err != nil {
		t.Fatalf("buildDependencyGraphAndSort failed: %v", err)
	}

	// Flatten layers to verify order
	var sorted []map[string]any
	for _, layer := range layers {
		sorted = append(sorted, layer...)
	}

	// Verify order: workstream should come first (no dependencies), then goal
	if len(sorted) != 2 {
		t.Fatalf("Expected 2 objects, got %d", len(sorted))
	}

	// Workstream should be first (no dependencies)
	if id, ok := sorted[0][objects.FieldKeyID].(string); !ok || id != "WS-001" {
		t.Errorf("Expected workstream to be first, got %v", sorted[0][objects.FieldKeyID])
	}

	// Goal should be second (depends on workstream)
	if id, ok := sorted[1][objects.FieldKeyID].(string); !ok || id != "GOAL-001" {
		t.Errorf("Expected goal to be second, got %v", sorted[1][objects.FieldKeyID])
	}
}

// TestBuildDependencyGraphAndSort_ComplexChain tests a complex dependency chain
func TestBuildDependencyGraphAndSort_ComplexChain(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Create complex chain: account -> workstream -> goal -> milestone -> backlog_item
	objSlice := []map[string]any{
		{
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyID:            "BLI-001",
			objects.FieldKeyTitle:         "Test Backlog Item",
			objects.FieldKeyMilestoneRefs: []string{"MIL-001"},
		},
		{
			objects.FieldKeyKind:  "milestone",
			objects.FieldKeyID:    "MIL-001",
			objects.FieldKeyTitle: "Test Milestone",
			"goal_ref":            "GOAL-001",
		},
		{
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyID:            "GOAL-001",
			objects.FieldKeyTitle:         "Test Goal",
			objects.FieldKeyWorkstreamRef: "WS-001",
		},
		{
			objects.FieldKeyKind:     "workstream",
			objects.FieldKeyID:       "WS-001",
			objects.FieldKeyTitle:    "Test Workstream",
			objects.FieldKeyOwnerRef: "ACC-TEST",
		},
		{
			objects.FieldKeyKind:     "account",
			objects.FieldKeyID:       "ACC-TEST",
			objects.FieldKeyTitle:    "Test Account",
			objects.FieldKeyUsername: "test",
		},
	}

	layers, err := builder.buildDependencyGraphAndSort(objSlice)
	if err != nil {
		t.Fatalf("buildDependencyGraphAndSort failed: %v", err)
	}

	// Flatten layers to verify order
	var sorted []map[string]any
	for _, layer := range layers {
		sorted = append(sorted, layer...)
	}

	// Verify order: account -> workstream -> goal -> milestone -> backlog_item
	expectedOrder := []string{"ACC-TEST", "WS-001", "GOAL-001", "MIL-001", "BLI-001"}
	if len(sorted) != len(expectedOrder) {
		t.Fatalf("Expected %d objects, got %d", len(expectedOrder), len(sorted))
	}

	for i, expectedID := range expectedOrder {
		if id, ok := sorted[i][objects.FieldKeyID].(string); !ok || id != expectedID {
			t.Errorf("Position %d: expected %s, got %v", i, expectedID, sorted[i][objects.FieldKeyID])
		}
	}
}

// TestBuildDependencyGraphAndSort_ObjectsWithoutIDs tests that objects without IDs are handled
func TestBuildDependencyGraphAndSort_ObjectsWithoutIDs(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Create objects: one with ID, one without
	objSlice := []map[string]any{
		{
			objects.FieldKeyKind:  "account",
			objects.FieldKeyID:    "ACC-TEST",
			objects.FieldKeyTitle: "Test Account",
		},
		{
			objects.FieldKeyKind: "workstream",
			// No ID - should be handled gracefully
			objects.FieldKeyTitle: "Test Workstream",
		},
	}

	layers, err := builder.buildDependencyGraphAndSort(objSlice)
	if err != nil {
		t.Fatalf("buildDependencyGraphAndSort failed: %v", err)
	}

	// Flatten layers
	var sorted []map[string]any
	for _, layer := range layers {
		sorted = append(sorted, layer...)
	}

	// Should return both objects
	if len(sorted) != 2 {
		t.Fatalf("Expected 2 objects, got %d", len(sorted))
	}

	// Object with ID should be first (can be sorted)
	// Object without ID should be included (will be created with auto-generated ID)
	foundAccount := false
	foundWorkstream := false
	for _, obj := range sorted {
		if id, ok := obj[objects.FieldKeyID].(string); ok && id == "ACC-TEST" {
			foundAccount = true
		}
		if kind, ok := obj[objects.FieldKeyKind].(string); ok && kind == "workstream" {
			foundWorkstream = true
		}
	}

	if !foundAccount {
		t.Error("Expected to find account object")
	}
	if !foundWorkstream {
		t.Error("Expected to find workstream object without ID")
	}
}

// TestBuildDependencyGraphAndSort_AccountUsernameMapping tests account username to ID mapping
func TestBuildDependencyGraphAndSort_AccountUsernameMapping(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Create account with username (should auto-generate account:username ID)
	// and workstream that references it
	objSlice := []map[string]any{
		{
			objects.FieldKeyKind:     "workstream",
			objects.FieldKeyID:       "WS-001",
			objects.FieldKeyTitle:    "Test Workstream",
			objects.FieldKeyOwnerRef: "ACC-TEST", // References account by username format
		},
		{
			objects.FieldKeyKind:     "account",
			objects.FieldKeyTitle:    "Test Account",
			objects.FieldKeyUsername: "test", // No ID, but has username
		},
	}

	layers, err := builder.buildDependencyGraphAndSort(objSlice)
	if err != nil {
		t.Fatalf("buildDependencyGraphAndSort failed: %v", err)
	}

	// Flatten layers
	var sorted []map[string]any
	for _, layer := range layers {
		sorted = append(sorted, layer...)
	}

	// Verify order: account should come first
	if len(sorted) != 2 {
		t.Fatalf("Expected 2 objects, got %d", len(sorted))
	}

	// Account should be first
	accountObj := sorted[0]
	if kind, ok := accountObj[objects.FieldKeyKind].(string); !ok || kind != "account" {
		t.Errorf("Expected account to be first, got %v", accountObj[objects.FieldKeyKind])
	}

	// Account should have ID auto-generated from username
	if id, ok := accountObj[objects.FieldKeyID].(string); !ok || id != "ACC-TEST" {
		t.Errorf("Expected account ID to be 'ACC-TEST', got %v", accountObj[objects.FieldKeyID])
	}

	// Workstream should be second
	if id, ok := sorted[1][objects.FieldKeyID].(string); !ok || id != "WS-001" {
		t.Errorf("Expected workstream to be second, got %v", sorted[1][objects.FieldKeyID])
	}
}

// TestBuildDependencyGraphAndSort_MultipleReferences tests objects with multiple references
func TestBuildDependencyGraphAndSort_MultipleReferences(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Create backlog item with multiple milestone references
	objSlice := []map[string]any{
		{
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyID:            "BLI-001",
			objects.FieldKeyTitle:         "Test Backlog Item",
			objects.FieldKeyMilestoneRefs: []string{"MIL-001", "MIL-002"},
		},
		{
			objects.FieldKeyKind:  "milestone",
			objects.FieldKeyID:    "MIL-001",
			objects.FieldKeyTitle: "Milestone 1",
		},
		{
			objects.FieldKeyKind:  "milestone",
			objects.FieldKeyID:    "MIL-002",
			objects.FieldKeyTitle: "Milestone 2",
		},
	}

	layers, err := builder.buildDependencyGraphAndSort(objSlice)
	if err != nil {
		t.Fatalf("buildDependencyGraphAndSort failed: %v", err)
	}

	// Flatten layers
	var sorted []map[string]any
	for _, layer := range layers {
		sorted = append(sorted, layer...)
	}

	// Verify order: milestones should come before backlog item
	if len(sorted) != 3 {
		t.Fatalf("Expected 3 objects, got %d", len(sorted))
	}

	// First two should be milestones (order doesn't matter, but both should be before backlog item)
	milestoneCount := 0
	backlogItemIndex := -1
	for i, obj := range sorted {
		if kind, ok := obj[objects.FieldKeyKind].(string); ok && kind == "milestone" {
			milestoneCount++
		}
		if id, ok := obj[objects.FieldKeyID].(string); ok && id == "BLI-001" {
			backlogItemIndex = i
		}
	}

	if milestoneCount != 2 {
		t.Errorf("Expected 2 milestones, found %d", milestoneCount)
	}

	if backlogItemIndex == -1 {
		t.Error("Expected to find backlog item")
	}

	// Backlog item should be last (depends on both milestones)
	if backlogItemIndex != 2 {
		t.Errorf("Expected backlog item to be last, but it's at index %d", backlogItemIndex)
	}
}

// TestBuildDependencyGraphAndSort_CircularDependency tests that circular dependencies are handled gracefully
func TestBuildDependencyGraphAndSort_CircularDependency(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Create circular dependency: A depends on B, B depends on A
	objSlice := []map[string]any{
		{
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyID:            "GOAL-A",
			objects.FieldKeyTitle:         "Goal A",
			objects.FieldKeyWorkstreamRef: "WS-B",
		},
		{
			objects.FieldKeyKind:  "workstream",
			objects.FieldKeyID:    "WS-B",
			objects.FieldKeyTitle: "Workstream B",
			"goal_ref":            "GOAL-A", // Circular reference
		},
	}

	// Should not error, but may not produce perfect ordering
	layers, err := builder.buildDependencyGraphAndSort(objSlice)
	if err != nil {
		t.Fatalf("buildDependencyGraphAndSort should handle circular dependencies gracefully, got error: %v", err)
	}

	// Flatten layers
	var sorted []map[string]any
	for _, layer := range layers {
		sorted = append(sorted, layer...)
	}

	// Should return both objects (even if order isn't perfect)
	if len(sorted) != 2 {
		t.Fatalf("Expected 2 objects, got %d", len(sorted))
	}

	// Both objects should be present
	foundGoal := false
	foundWorkstream := false
	for _, obj := range sorted {
		if id, ok := obj[objects.FieldKeyID].(string); ok && id == "GOAL-A" {
			foundGoal = true
		}
		if id, ok := obj[objects.FieldKeyID].(string); ok && id == "WS-B" {
			foundWorkstream = true
		}
	}

	if !foundGoal {
		t.Error("Expected to find GOAL-A")
	}
	if !foundWorkstream {
		t.Error("Expected to find WS-B")
	}
}

// TestBuildDependencyGraphAndSort_ExternalReferences tests that external references (not in data file) are ignored
func TestBuildDependencyGraphAndSort_ExternalReferences(t *testing.T) {
	// Use MkdirTemp + cleanupStorageTestRoot (same as scenario_builder_waitgroup_panic_test): t.TempDir
	// plus RemoveAll in cleanup fights the framework's own temp removal.
	tmpDir, err := fileutil.MkdirTemp("", "zqk-external-refs-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageFactory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()
	cleanupStorageTestRoot(t, tmpDir, storageProvider)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	// Create object that references external object (not in data file)
	objSlice := []map[string]any{
		{
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyID:            "GOAL-001",
			objects.FieldKeyTitle:         "Test Goal",
			objects.FieldKeyWorkstreamRef: "WS-EXTERNAL", // External reference (not in data file)
		},
	}

	layers, err := builder.buildDependencyGraphAndSort(objSlice)
	if err != nil {
		t.Fatalf("buildDependencyGraphAndSort failed: %v", err)
	}

	// Flatten layers
	var sorted []map[string]any
	for _, layer := range layers {
		sorted = append(sorted, layer...)
	}

	// Should return the object (external references don't affect sorting)
	if len(sorted) != 1 {
		t.Fatalf("Expected 1 object, got %d", len(sorted))
	}

	if id, ok := sorted[0][objects.FieldKeyID].(string); !ok || id != "GOAL-001" {
		t.Errorf("Expected GOAL-001, got %v", sorted[0][objects.FieldKeyID])
	}
}

// TestReferenceExistsMemoized tests the memoized reference cache
func TestReferenceExistsMemoized(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	ctx := pkgctx.NewSystemContext()
	idStream := make(map[string]string)
	idStreamMu := &sync.Mutex{}
	referenceCache := make(map[string]bool)
	referenceCacheMu := &sync.Mutex{}

	// Pre-populate cache with existing object
	referenceCache["ACC-TEST"] = true

	// Test 1: Check ID stream (should return true)
	idStream["WS-001"] = "WS-001"
	if !builder.referenceExistsMemoized(ctx, "WS-001", idStream, idStreamMu, referenceCache, referenceCacheMu) {
		t.Error("Expected reference to exist in ID stream")
	}

	// Test 2: Check reference cache (should return true)
	if !builder.referenceExistsMemoized(ctx, "ACC-TEST", idStream, idStreamMu, referenceCache, referenceCacheMu) {
		t.Error("Expected reference to exist in cache")
	}

	// Test 3: Check non-existent reference (should return false and cache negative result)
	if builder.referenceExistsMemoized(ctx, "NON-EXISTENT", idStream, idStreamMu, referenceCache, referenceCacheMu) {
		t.Error("Expected reference to not exist")
	}

	// Verify negative result was cached
	referenceCacheMu.Lock()
	cached, exists := referenceCache["NON-EXISTENT"]
	referenceCacheMu.Unlock()
	if !exists {
		t.Error("Expected negative result to be cached")
	}
	if cached {
		t.Error("Expected cached value to be false")
	}
}

// TestVerifyReferencedObjectsExistMemoized tests reference validation with memoized cache
func TestVerifyReferencedObjectsExistMemoized(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	storageProvider := setupTestStorageProvider(t, tmpDir)

	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	idStream := make(map[string]string)
	idStreamMu := &sync.Mutex{}
	referenceCache := make(map[string]bool)
	referenceCacheMu := &sync.Mutex{}

	// Pre-populate cache with existing references
	referenceCache["WS-001"] = true
	referenceCache["ACC-TEST"] = true

	// Test 1: Object with valid references (should pass)
	objWithValidRefs := map[string]any{
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyID:            "GOAL-001",
		objects.FieldKeyTitle:         "Test Goal",
		objects.FieldKeyWorkstreamRef: "WS-001",
		objects.FieldKeyOwnerRef:      "ACC-TEST",
	}

	err1 := builder.verifyReferencedObjectsExistMemoized(ctx, objWithValidRefs, "goal", idStream, idStreamMu, referenceCache, referenceCacheMu)
	if err1 != nil {
		t.Errorf("Expected no error for valid references, got: %v", err1)
	}

	// Test 2: Object with missing reference (should fail)
	// Use a reference that's not in cache and won't exist in storage
	objWithMissingRef := map[string]any{
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyID:            "GOAL-002",
		objects.FieldKeyTitle:         "Test Goal 2",
		objects.FieldKeyWorkstreamRef: "WS-MISSING-12345", // Use a clearly non-existent ID
	}

	err2 := builder.verifyReferencedObjectsExistMemoized(ctx, objWithMissingRef, "goal", idStream, idStreamMu, referenceCache, referenceCacheMu)
	if err2 == nil {
		t.Error("Expected error for missing reference")
	}
	if err2 != nil {
		// Verify it's an error type
		if !reflect.TypeOf(err2).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			t.Error("Expected error type")
		}
	}
}
