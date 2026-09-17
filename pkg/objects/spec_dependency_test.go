package objects

import (
	"testing"
)

// TestSpecDependencyGraph_BuildGraph tests building the dependency graph
func TestSpecDependencyGraph_BuildGraph(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	graph := NewSpecDependencyGraph(loader)

	err := graph.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error = %v", err)
	}

	// Verify we found specs
	if len(graph.specs) == 0 {
		t.Error("Expected to find specs, got 0")
	}

	// Verify auditable is in the graph (root spec)
	if _, ok := graph.specs["auditable"]; !ok {
		t.Error("Expected to find 'auditable' spec in graph")
	}

	// Verify base_object is in the graph
	if _, ok := graph.specs["base_object"]; !ok {
		t.Error("Expected to find 'base_object' spec in graph")
	}
}

// TestSpecDependencyGraph_DetectCycles tests cycle detection
func TestSpecDependencyGraph_DetectCycles(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	graph := NewSpecDependencyGraph(loader)

	err := graph.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error = %v", err)
	}

	cycles := graph.DetectCycles()
	if len(cycles) > 0 {
		t.Errorf("Detected cycles in spec dependency graph: %v", cycles)
	}
}

// TestSpecDependencyGraph_TopologicalSort tests topological sorting
func TestSpecDependencyGraph_TopologicalSort(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	graph := NewSpecDependencyGraph(loader)

	err := graph.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error = %v", err)
	}

	order, err := graph.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	// Verify auditable comes before base_object
	auditableIndex := -1
	baseObjectIndex := -1
	for i, spec := range order {
		if spec.Ontology == "auditable" {
			auditableIndex = i
		}
		if spec.Ontology == "base_object" {
			baseObjectIndex = i
		}
	}

	if auditableIndex == -1 {
		t.Error("Expected to find 'auditable' in topological sort")
	}
	if baseObjectIndex == -1 {
		t.Error("Expected to find 'base_object' in topological sort")
	}
	if auditableIndex >= baseObjectIndex {
		t.Errorf("Expected 'auditable' (%d) to come before 'base_object' (%d)", auditableIndex, baseObjectIndex)
	}

	// Verify base_object comes before criteria
	criteriaIndex := -1
	for i, spec := range order {
		if spec.Ontology == "criteria" {
			criteriaIndex = i
		}
	}

	if criteriaIndex == -1 {
		t.Error("Expected to find 'criteria' in topological sort")
	}
	if baseObjectIndex >= criteriaIndex {
		t.Errorf("Expected 'base_object' (%d) to come before 'criteria' (%d)", baseObjectIndex, criteriaIndex)
	}
}

// TestSpecDependencyGraph_TopologicalSort_Deterministic verifies BLI-SPEC-ORDER-VERIF-001:
// topological sort on spec loading order produces 100% deterministic output across runs.
func TestSpecDependencyGraph_TopologicalSort_Deterministic(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	graph := NewSpecDependencyGraph(loader)

	if err := graph.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph() error = %v", err)
	}

	firstOrder, err := graph.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}
	firstOntologies := make([]string, len(firstOrder))
	for i, s := range firstOrder {
		firstOntologies[i] = s.Ontology
	}

	for run := 0; run < 20; run++ {
		nextOrder, err := graph.TopologicalSort()
		if err != nil {
			t.Fatalf("run %d: TopologicalSort() error = %v", run, err)
		}
		if len(nextOrder) != len(firstOntologies) {
			t.Fatalf("run %d: length mismatch %d != %d", run, len(nextOrder), len(firstOntologies))
		}
		for i, s := range nextOrder {
			if s.Ontology != firstOntologies[i] {
				t.Fatalf("run %d: non-deterministic order at index %d: %q != %q", run, i, s.Ontology, firstOntologies[i])
			}
		}
	}
}

// TestSpecDependencyGraph_MissingParent tests detection of missing parent specs
func TestSpecDependencyGraph_MissingParent(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	graph := NewSpecDependencyGraph(loader)

	err := graph.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error = %v", err)
	}

	missing := graph.FindMissingParents()
	if len(missing) > 0 {
		t.Errorf("Found specs with missing parents: %v", missing)
	}
}

// TestSpecDependencyGraph_ValidateGraph tests full graph validation
func TestSpecDependencyGraph_ValidateGraph(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	graph := NewSpecDependencyGraph(loader)

	err := graph.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error = %v", err)
	}

	errors := graph.ValidateGraph()
	if len(errors) > 0 {
		t.Errorf("Graph validation found errors: %v", errors)
	}
}

// TestGraphValidationError_LogFields tests the LogFields method
func TestGraphValidationError_LogFields(t *testing.T) {
	t.Parallel()
	err := &GraphValidationError{
		Type:    "cycle",
		Message: "Circular dependency detected",
		Specs:   []string{"spec1", "spec2"},
	}

	fields := err.LogFields()

	// LogFields should return alternating key-value pairs
	if len(fields) != 6 {
		t.Errorf("Expected 6 fields (3 pairs), got %d", len(fields))
	}

	// Verify structure: key, value, key, value, ...
	expectedFields := map[string]any{
		FieldKeyType: "cycle",
		"message":    "Circular dependency detected",
		"specs":      []string{"spec1", "spec2"},
	}

	for i := 0; i < len(fields); i += 2 {
		if i+1 >= len(fields) {
			t.Fatal("Fields should be in key-value pairs")
		}
		key, ok := fields[i].(string)
		if !ok {
			t.Errorf("Expected string key at index %d, got %T", i, fields[i])
			continue
		}
		value := fields[i+1]
		expectedValue, ok := expectedFields[key]
		if !ok {
			t.Errorf("Unexpected key in fields: %s", key)
			continue
		}
		if key == "specs" {
			// Compare slices
			expectedSlice := expectedValue.([]string)
			actualSlice, ok := value.([]string)
			if !ok {
				t.Errorf("Expected []string for specs, got %T", value)
				continue
			}
			if len(actualSlice) != len(expectedSlice) {
				t.Errorf("Expected %d specs, got %d", len(expectedSlice), len(actualSlice))
				continue
			}
			for j, expected := range expectedSlice {
				if actualSlice[j] != expected {
					t.Errorf("Expected spec[%d] = %q, got %q", j, expected, actualSlice[j])
				}
			}
		} else if value != expectedValue {
			t.Errorf("Expected %s = %v, got %v", key, expectedValue, value)
		}
	}
}

func TestSpecDependencyGraph_SpecCacheRevisionTracking(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	graph := NewSpecDependencyGraph(loader)
	if err := graph.BuildGraph(); err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	built := graph.BuiltAtSpecCacheRevision()
	if built != loader.SpecCacheRevision() {
		t.Fatalf("BuiltAtSpecCacheRevision %d want same as loader %d", built, loader.SpecCacheRevision())
	}
	if graph.LoaderSpecCacheInvalidatedSinceBuild() {
		t.Fatal("expected not invalidated immediately after build")
	}
	loader.ClearCache()
	if !graph.LoaderSpecCacheInvalidatedSinceBuild() {
		t.Fatal("expected invalidated after ClearCache")
	}
	if graph.BuiltAtSpecCacheRevision() != built {
		t.Fatal("BuiltAtSpecCacheRevision should not change when loader invalidates")
	}
}
