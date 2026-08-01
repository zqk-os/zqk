package builders

import "testing"

func TestFieldBuilder(t *testing.T) {
	t.Parallel()
	// Test FieldBuilder with ChecklistBuilder
	checklist := NewChecklistBuilder().
		Authority("owner.").
		Purpose("Test field purpose").
		Security("non-sensitive.").
		Cardinality("one").
		Observability("yes.").
		Build()

	access := NewAccessBuilder().
		Requires("access:confidential").
		Build()

	validation := NewValidationBuilder().
		Required(true).
		MaxLength(80).
		Build()

	field := NewFieldBuilder("test_field", "string").
		WithChecklist(checklist).
		WithAccess(access).
		WithValidation(validation).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TEST-001").
		Build()

	// Verify structure
	if field["type"] != "string" {
		t.Errorf("Expected type 'string', got %v", field["type"])
	}

	if checklistMap, ok := field["checklist"].(map[string]any); ok {
		if checklistMap["authority"] != "owner." {
			t.Errorf("Expected authority 'owner.', got %v", checklistMap["authority"])
		}
		if checklistMap["security"] != "non-sensitive" {
			t.Errorf("Expected security normalized to 'non-sensitive', got %v", checklistMap["security"])
		}
		if checklistMap["observability"] != "yes" {
			t.Errorf("Expected observability normalized to 'yes', got %v", checklistMap["observability"])
		}
	} else {
		t.Error("Expected checklist map")
	}

	if validationMap, ok := field["validation"].(map[string]any); ok {
		if validationMap["required"] != true {
			t.Errorf("Expected required true, got %v", validationMap["required"])
		}
		if validationMap["max_length"] != 80 {
			t.Errorf("Expected max_length 80, got %v", validationMap["max_length"])
		}
	} else {
		t.Error("Expected validation map")
	}

	if traits, ok := field["traits"].([]any); ok {
		if len(traits) != 2 {
			t.Errorf("Expected 2 traits, got %d", len(traits))
		}
	} else {
		t.Error("Expected traits slice")
	}
}
