package objects

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

// TestSpecDependencyGraph_CycleDetection tests cycle detection with a mock cycle
func TestSpecDependencyGraph_CycleDetection(t *testing.T) {
	t.Parallel()
	// Create a temporary directory with specs that form a cycle
	tmpDir := t.TempDir()

	// Create spec A that extends B
	specA := `ontology: spec_a
extends: spec_b
`
	// Create spec B that extends C
	specB := `ontology: spec_b
extends: spec_c
`
	// Create spec C that extends A (cycle!)
	specC := `ontology: spec_c
extends: spec_a
`

	if err := os.WriteFile(filepath.Join(tmpDir, "spec_a.yaml"), []byte(specA), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create spec_a.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "spec_b.yaml"), []byte(specB), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create spec_b.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "spec_c.yaml"), []byte(specC), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create spec_c.yaml: %v", err)
	}

	loader := NewSpecLoader(tmpDir)
	graph := NewSpecDependencyGraph(loader)

	err := graph.BuildGraph()
	if err != nil {
		t.Fatalf("BuildGraph() error = %v", err)
	}

	cycles := graph.DetectCycles()
	if len(cycles) == 0 {
		t.Error("Expected to detect a cycle, but none were found")
	}

	// Verify the cycle contains all three specs
	if len(cycles) > 0 {
		cycle := cycles[0]
		if len(cycle.Specs) < 3 {
			t.Errorf("Expected cycle to contain at least 3 specs, got %d", len(cycle.Specs))
		}
		// Verify all three specs are in the cycle
		specsInCycle := make(map[string]bool)
		for _, spec := range cycle.Specs {
			specsInCycle[spec] = true
		}
		if !specsInCycle["spec_a"] || !specsInCycle["spec_b"] || !specsInCycle["spec_c"] {
			t.Errorf("Cycle should contain spec_a, spec_b, and spec_c, got %v", cycle.Specs)
		}
	}

	// Topological sort should fail with cycles
	_, err = graph.TopologicalSort()
	if err == nil {
		t.Error("Expected TopologicalSort() to fail with cycles, but it succeeded")
	}
}
