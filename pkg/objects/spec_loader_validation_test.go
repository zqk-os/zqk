package objects

import (
	"testing"
)

// TestSpecLoader_ValidateDependencyGraph tests that SpecLoader can validate the dependency graph
func TestSpecLoader_ValidateDependencyGraph(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")

	errors := loader.ValidateDependencyGraph()
	if len(errors) > 0 {
		t.Errorf("ValidateDependencyGraph() found errors: %v", errors)
	}
}

// TestSpecLoader_GetLoadOrder tests that SpecLoader can get specs in load order
func TestSpecLoader_GetLoadOrder(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")

	specs, err := loader.GetLoadOrder()
	if err != nil {
		t.Fatalf("GetLoadOrder() error = %v", err)
	}

	if len(specs) == 0 {
		t.Error("Expected to get specs in load order, got 0")
	}

	// Verify auditable comes before base_object
	auditableIndex := -1
	baseObjectIndex := -1
	for i, spec := range specs {
		if spec.Ontology == "auditable" {
			auditableIndex = i
		}
		if spec.Ontology == "base_object" {
			baseObjectIndex = i
		}
	}

	if auditableIndex == -1 {
		t.Error("Expected to find 'auditable' in load order")
	}
	if baseObjectIndex == -1 {
		t.Error("Expected to find 'base_object' in load order")
	}
	if auditableIndex >= baseObjectIndex {
		t.Errorf("Expected 'auditable' (%d) to come before 'base_object' (%d)", auditableIndex, baseObjectIndex)
	}
}
