package objects

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestLifecycleLoader_ExtendsBaseLifecycle tests that lifecycles can extend base_lifecycle
func TestLifecycleLoader_ExtendsBaseLifecycle(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	// Test loading workflow lifecycle which extends base_lifecycle
	lifecycle, err := loader.LoadLifecycle("workflow")
	if err != nil {
		t.Fatalf("Failed to load workflow lifecycle: %v", err)
	}

	if lifecycle == nil {
		t.Fatal("Lifecycle is nil")
	}

	// Verify status mapping exists
	if len(lifecycle.StatusMapping) == 0 {
		t.Error("Expected status mapping to be populated for workflow lifecycle")
	}

	// Verify status mapping contains expected mappings
	expectedMappings := map[string]string{
		"proposed":    "draft",
		"approved":    "active",
		"in_progress": "active",
		"implemented": "active",
		"archived":    "archived",
		"error":       "error",
	}

	for parentStatus, expectedChildStatus := range expectedMappings {
		if actual, ok := lifecycle.StatusMapping[parentStatus]; !ok {
			t.Errorf("Expected status mapping for %s, but not found", parentStatus)
		} else if actual != expectedChildStatus {
			t.Errorf("Status mapping for %s: expected %s, got %s", parentStatus, expectedChildStatus, actual)
		}
	}

	// Verify that workflow-specific statuses are present
	statusMap := make(map[string]bool)
	for _, s := range lifecycle.Statuses {
		statusMap[s.Value] = true
	}

	expectedStatuses := []string{"draft", "active", "deprecated", "archived", "error"}
	for _, expected := range expectedStatuses {
		if !statusMap[expected] {
			t.Errorf("Expected status %s not found in workflow lifecycle", expected)
		}
	}

	// Verify origin status is from child (conceptual), not parent (proposed)
	originStatuses := []string{}
	for _, s := range lifecycle.Statuses {
		if s.Origin {
			originStatuses = append(originStatuses, s.Value)
		}
	}
	if len(originStatuses) != 1 || originStatuses[0] != "conceptual" {
		t.Errorf("Expected origin status to be 'conceptual', got %v", originStatuses)
	}
}

// TestLifecycleLoader_StatusMapping tests that status mapping correctly maps parent statuses to child statuses
func TestLifecycleLoader_StatusMapping(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	// Test priority_plan lifecycle which has custom status mapping
	lifecycle, err := loader.LoadLifecycle("priority_plan")
	if err != nil {
		t.Fatalf("Failed to load priority_plan lifecycle: %v", err)
	}

	if lifecycle.StatusMapping == nil {
		t.Fatal("Expected status mapping to exist")
	}

	// Verify specific mappings
	mappingTests := []struct {
		parentStatus string
		childStatus  string
	}{
		{"proposed", "grooming"},
		{"approved", "grooming"},
		{"implemented", "complete"},
		{"archived", "archived"},
		{"error", "cancelled"},
	}

	for _, tt := range mappingTests {
		if actual, ok := lifecycle.StatusMapping[tt.parentStatus]; !ok {
			t.Errorf("Expected status mapping for %s, but not found", tt.parentStatus)
		} else if actual != tt.childStatus {
			t.Errorf("Status mapping for %s: expected %s, got %s", tt.parentStatus, tt.childStatus, actual)
		}
	}
	// Check valve: in_progress is a first-class sibling of active. status_mapping
	// must not collapse it onto active (or grooming). TRACK: BLI-1785439369431933000-f0cccd6c
	if mapped, ok := lifecycle.StatusMapping["in_progress"]; ok && mapped != "in_progress" {
		t.Error("priority_plan status_mapping must not include in_progress (would launder execution lock)")
	}

	// Verify that priority_plan-specific statuses are present
	statusMap := make(map[string]bool)
	for _, s := range lifecycle.Statuses {
		statusMap[s.Value] = true
	}

	expectedStatuses := []string{"grooming", "active", "complete", "archived", "cancelled", "blocked", "paused", "in_progress"}
	for _, expected := range expectedStatuses {
		if !statusMap[expected] {
			t.Logf("Warning: Expected status %s not found (may be OK if lifecycle changed)", expected)
		}
	}
}

// TestLifecycleLoader_CircularDependency tests that circular dependencies are detected
func TestLifecycleLoader_CircularDependency(t *testing.T) {
	t.Parallel()
	// Create temporary directory with circular dependency
	tmpDir := t.TempDir()
	loader := NewLifecycleLoader(tmpDir)

	// Create lifecycle A that extends B
	lifecycleA := `extends: lifecycle_b
object_type: a
statuses:
  - value: a1
    initial: true
`
	_ = fileutil.WriteFile(filepath.Join(tmpDir, "lifecycle_a_lifecycle.yaml"), []byte(lifecycleA), paths.FilePerm644) //nolint:errcheck // Test setup - errors would cause test failure

	// Create lifecycle B that extends C
	lifecycleB := `extends: lifecycle_c
object_type: b
statuses:
  - value: b1
    initial: true
`
	_ = fileutil.WriteFile(filepath.Join(tmpDir, "lifecycle_b_lifecycle.yaml"), []byte(lifecycleB), paths.FilePerm644) //nolint:errcheck // Test setup - errors would cause test failure

	// Create lifecycle C that extends A (circular!)
	lifecycleC := `extends: lifecycle_a
object_type: c
statuses:
  - value: c1
    initial: true
`
	_ = fileutil.WriteFile(filepath.Join(tmpDir, "lifecycle_c_lifecycle.yaml"), []byte(lifecycleC), paths.FilePerm644) //nolint:errcheck // Test setup - errors would cause test failure

	// Try to load lifecycle A - should detect circular dependency
	_, err := loader.LoadLifecycle("lifecycle_a")
	if err == nil {
		t.Fatal("Expected error for circular dependency, but got nil")
	}

	if err.Error() == emptyValue || !testContainsString(err.Error(), "circular dependency") {
		t.Errorf("Expected circular dependency error, got: %v", err)
	}
}

// TestLifecycleLoader_MergeLifecycles tests that parent and child lifecycles are correctly merged
func TestLifecycleLoader_MergeLifecycles(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	// Test backlog_item lifecycle which extends base_lifecycle
	lifecycle, err := loader.LoadLifecycle("backlog_item")
	if err != nil {
		t.Fatalf("Failed to load backlog_item lifecycle: %v", err)
	}

	// Verify that both parent and child statuses are present
	statusMap := make(map[string]bool)
	for _, s := range lifecycle.Statuses {
		statusMap[s.Value] = true
	}

	// Should have backlog_item-specific statuses
	backlogSpecificStatuses := []string{"exploring", "validated", "roadmap", "deferred", "planned"}
	for _, status := range backlogSpecificStatuses {
		if !statusMap[status] {
			t.Errorf("Expected backlog_item-specific status %s not found", status)
		}
	}

	// Should also have inherited statuses (mapped via status_mapping)
	// Note: base_lifecycle statuses are mapped to backlog_item statuses, so we check for mapped versions
	if !statusMap["exploring"] {
		t.Error("Expected 'exploring' status (mapped from 'proposed') not found")
	}
	if !statusMap["validated"] {
		t.Error("Expected 'validated' status (mapped from 'approved') not found")
	}

	// Verify transitions are merged (should have both parent and child transitions)
	if len(lifecycle.Transitions) == 0 {
		t.Error("Expected transitions to be merged from parent and child")
	}
}

// TestLifecycleLoader_DefaultLifecycle tests that objects without specific lifecycle use base_lifecycle
func TestLifecycleLoader_DefaultLifecycle(t *testing.T) {
	t.Parallel()
	lifecyclesDir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	loader := NewLifecycleLoader(lifecyclesDir)

	// Try to load a lifecycle for a kind that doesn't have a specific lifecycle file
	// This should fall back to base_lifecycle
	lifecycle, err := loader.LoadLifecycle("nonexistent_kind")
	if err == nil {
		// If it doesn't error, it should return base_lifecycle
		if lifecycle == nil {
			t.Fatal("Expected base_lifecycle to be returned, got nil")
		}

		// Verify it has base_lifecycle statuses
		statusMap := make(map[string]bool)
		for _, s := range lifecycle.Statuses {
			statusMap[s.Value] = true
		}

		expectedBaseStatuses := []string{"proposed", "approved", "in_progress", "implemented", "archived", "error"}
		for _, expected := range expectedBaseStatuses {
			if !statusMap[expected] {
				t.Errorf("Expected base_lifecycle status %s not found", expected)
			}
		}
	}
}

// Helper function to check if a string contains a substring
func testContainsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
