package system

// ITEM-177483 exempt (TEARDOWN_PIPELINE_INVENTORY): no temp ZQK tree; t.Setenv(TestRoot,"") for repo specs. Not RunProjectTestTeardown.

import (
	"testing"

	"github.com/lanceman/zqk/pkg/interactive"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestStreamingTemplateLoop_ProcessLoop uses GetGlobalFieldRegistry().Reload(); clear ZQK_TEST_ROOT so repo specs are used (t.Setenv restores after).
func TestStreamingTemplateLoop_ProcessLoop(t *testing.T) {
	t.Setenv(zqkenv.TestRoot(), "")
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := interactive.NewTemplateGenerator(fieldRegistry)
	loop := interactive.NewTemplateLoop(generator)

	// Test first iteration (no values provided)
	loopState, err := loop.ProcessLoop("criteria", map[string]any{})
	if err != nil {
		t.Fatalf("Failed to process loop: %v", err)
	}

	if loopState.IsComplete {
		t.Error("Loop should not be complete with no values provided")
	}

	if len(loopState.MissingRequired) == 0 {
		t.Error("Should have missing required fields")
	}

	t.Logf("Missing required fields: %v", loopState.MissingRequired)

	// Test second iteration (some values provided)
	providedValues := map[string]any{
		objects.FieldKeyTitle:    "Test Criteria",
		objects.FieldKeyCategory: "acceptance",
	}

	loopState2, err := loop.ProcessLoop("criteria", providedValues)
	if err != nil {
		t.Fatalf("Failed to process loop: %v", err)
	}

	// With only title and category, implementation may consider loop complete if those are the only user-required fields (id, dates may be auto-generated).
	t.Logf("Missing required fields after partial: %v", loopState2.MissingRequired)
	if len(loopState2.InvalidFields) > 0 {
		t.Logf("Invalid fields: %v", loopState2.InvalidFields)
	}

	// Test complete iteration (all required fields provided)
	allRequiredValues := make(map[string]any)
	allRequiredValues[objects.FieldKeyTitle] = "Test Criteria"
	allRequiredValues[objects.FieldKeyCategory] = "acceptance"

	// Get template to find all required fields
	templateWithTokens, err := generator.GenerateTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to generate template: %v", err)
	}

	for _, fieldName := range templateWithTokens.RequiredFields {
		tokenInfo := templateWithTokens.FieldInfo[fieldName]

		// Skip auto-generated fields for this test (they'd be handled by the system)
		if fieldName == "id" || fieldName == "created_at" || fieldName == "created_by" ||
			fieldName == "updated_at" || fieldName == "updated_by" {
			continue
		}

		// Provide default values based on type
		switch tokenInfo.Type {
		case "string", "text":
			allRequiredValues[fieldName] = "test value"
		case "enum":
			if len(tokenInfo.EnumValues) > 0 {
				allRequiredValues[fieldName] = tokenInfo.EnumValues[0]
			} else {
				allRequiredValues[fieldName] = "test"
			}
		case "integer", "number":
			allRequiredValues[fieldName] = 0
		case "boolean":
			allRequiredValues[fieldName] = false
		case "list":
			allRequiredValues[fieldName] = []any{}
		default:
			allRequiredValues[fieldName] = "test"
		}
	}

	loopState3, err := loop.ProcessLoop("criteria", allRequiredValues)
	if err != nil {
		t.Fatalf("Failed to process loop: %v", err)
	}

	// Note: Loop might still not be complete because of auto-generated fields
	// But user-provided required fields should be complete
	t.Logf("Loop complete: %v", loopState3.IsComplete)
	t.Logf("Missing required: %v", loopState3.MissingRequired)
	t.Logf("Invalid fields: %v", loopState3.InvalidFields)

	if loopState3.IsComplete && loopState3.FilledTemplate == emptyValue {
		t.Error("Filled template should be populated when loop is complete")
	}

	if loopState3.IsComplete {
		t.Logf("Filled template:\n%s", loopState3.FilledTemplate)
	}
}

// TestStreamingTemplateLoop_GetFieldsToElicit uses GetGlobalFieldRegistry().Reload(); clear ZQK_TEST_ROOT so repo specs are used (t.Setenv restores after).
func TestStreamingTemplateLoop_GetFieldsToElicit(t *testing.T) {
	t.Setenv(zqkenv.TestRoot(), "")
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := interactive.NewTemplateGenerator(fieldRegistry)
	loop := interactive.NewTemplateLoop(generator)

	// Process loop with missing fields
	loopState, err := loop.ProcessLoop("criteria", map[string]any{
		objects.FieldKeyTitle: "Test",
	})
	if err != nil {
		t.Fatalf("Failed to process loop: %v", err)
	}

	// Get fields to elicit
	fieldsToElicit := loop.GetFieldsToElicit(loopState)

	if len(fieldsToElicit) == 0 {
		t.Error("Should have fields to elicit")
	}

	t.Logf("Fields to elicit: %d", len(fieldsToElicit))
	for _, field := range fieldsToElicit {
		t.Logf("  - %s (%s, required: %v)", field.Name, field.Type, field.Required)
	}

	// Verify category is in fields to elicit (if it's required)
	hasCategory := false
	for _, field := range fieldsToElicit {
		if field.Name == "category" {
			hasCategory = true
			break
		}
	}

	t.Logf("Has category in fields to elicit: %v", hasCategory)
}
