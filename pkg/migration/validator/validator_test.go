package validator

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestValidateReferences(t *testing.T) {
	t.Parallel()
	validator := NewValidator()

	// Test with all references resolved
	resolvedRefs := map[string]bool{
		"MIL-001":  true,
		"GOAL-001": true,
		"MIL-002":  true,
	}

	errors := validator.ValidateReferences(resolvedRefs)
	if len(errors) != 0 {
		t.Errorf("Expected 0 errors for all resolved refs, got %d", len(errors))
	}

	// Test with unresolved references
	unresolvedRefs := map[string]bool{
		"MIL-001":  true,
		"MIL-999":  false, // Unresolved
		"GOAL-001": true,
		"GOAL-999": false, // Unresolved
	}

	errors = validator.ValidateReferences(unresolvedRefs)
	if len(errors) != 2 {
		t.Errorf("Expected 2 errors for unresolved refs, got %d", len(errors))
	}
}

func TestValidateRequiredFields(t *testing.T) {
	t.Parallel()
	validator := NewValidator()

	// Test with all required fields
	properties := map[string]any{
		objects.FieldKeyID:    "BLI-001",
		objects.FieldKeyKind:  "backlog_item",
		objects.FieldKeyTitle: "Test Item",
	}

	errors := validator.ValidateRequiredFields(properties, []string{"id", "kind", "title"})
	if len(errors) != 0 {
		t.Errorf("Expected 0 errors, got %d", len(errors))
	}

	// Test with missing required fields
	properties = map[string]any{
		objects.FieldKeyID: "BLI-001",
		// Missing "kind" and "title"
	}

	errors = validator.ValidateRequiredFields(properties, []string{"id", "kind", "title"})
	if len(errors) != 2 {
		t.Errorf("Expected 2 errors for missing fields, got %d", len(errors))
	}
}

func TestDetectOrphans(t *testing.T) {
	t.Parallel()
	validator := NewValidator()

	// Test with no orphans (all entities have relationships)
	entityIDs := []string{"BLI-001", "BLI-002", "BLI-003"}
	relationships := map[string][]string{
		"BLI-001": {"MIL-001"},
		"BLI-002": {"MIL-001"},
		"BLI-003": {"GOAL-001"},
	}

	orphans := validator.DetectOrphans(entityIDs, relationships)
	if len(orphans) != 0 {
		t.Errorf("Expected 0 orphans, got %d", len(orphans))
	}

	// Test with orphans
	relationships = map[string][]string{
		"BLI-001": {"MIL-001"},
		"BLI-002": {"MIL-001"},
		// BLI-003 has no relationships
	}

	orphans = validator.DetectOrphans(entityIDs, relationships)
	if len(orphans) != 1 {
		t.Errorf("Expected 1 orphan, got %d", len(orphans))
	}

	if orphans[0] != "BLI-003" {
		t.Errorf("Expected orphan 'BLI-003', got '%s'", orphans[0])
	}
}

func TestValidateEdgeTypes(t *testing.T) {
	t.Parallel()
	validator := NewValidator()

	// Define valid edge types
	validEdgeTypes := []string{
		"IMPLEMENTS",
		"BELONGS_TO",
		"ACHIEVES",
		"HAS_SOURCE",
		"DEFINES",
	}

	// Test with valid edge types
	edgeTypes := []string{"IMPLEMENTS", "BELONGS_TO", "HAS_SOURCE"}
	errors := validator.ValidateEdgeTypes(edgeTypes, validEdgeTypes)
	if len(errors) != 0 {
		t.Errorf("Expected 0 errors for valid edge types, got %d", len(errors))
	}

	// Test with invalid edge types
	edgeTypes = []string{"IMPLEMENTS", "INVALID_EDGE", "HAS_SOURCE"}
	errors = validator.ValidateEdgeTypes(edgeTypes, validEdgeTypes)
	if len(errors) != 1 {
		t.Errorf("Expected 1 error for invalid edge type, got %d", len(errors))
	}
}

func TestCheckDuplicateIDs(t *testing.T) {
	t.Parallel()
	validator := NewValidator()

	// Test with no duplicates
	ids := []string{"BLI-001", "BLI-002", "BLI-003"}
	duplicates := validator.CheckDuplicateIDs(ids)
	if len(duplicates) != 0 {
		t.Errorf("Expected 0 duplicates, got %d", len(duplicates))
	}

	// Test with duplicates
	ids = []string{"BLI-001", "BLI-002", "BLI-001", "BLI-003", "BLI-002"}
	duplicates = validator.CheckDuplicateIDs(ids)
	if len(duplicates) != 2 {
		t.Errorf("Expected 2 duplicates, got %d", len(duplicates))
	}
}
