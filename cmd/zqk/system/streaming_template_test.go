package system

// BLI-177483 exempt (TEARDOWN_PIPELINE_INVENTORY): no temp ZQK tree; t.Setenv(TestRoot,"") for repo specs. Not RunProjectTestTeardown.

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/interactive"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestGenerateTemplateWithTokens uses GetGlobalFieldRegistry().Reload() which finds specs via findSpecsDir().
// Clear ZQK_TEST_ROOT so repo specs are used; t.Setenv restores the prior value after the test.
func TestGenerateTemplateWithTokens(t *testing.T) {
	t.Setenv(zqkenv.TestRoot().Name(), "")
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := interactive.NewTemplateGenerator(fieldRegistry)

	// Test with criteria (has relatively few fields)
	templateWithTokens, err := generator.GenerateTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to generate template: %v", err)
	}

	// Verify template contains tokens
	if !strings.Contains(templateWithTokens.Template, "{title}") {
		t.Error("Template should contain {title} token")
	}

	if !strings.Contains(templateWithTokens.Template, "{category}") {
		t.Error("Template should contain {category} token")
	}

	// Verify required fields are identified
	if len(templateWithTokens.RequiredFields) == 0 {
		t.Error("Should have at least one required field")
	}

	// Verify title is in required fields (criteria should require title)
	hasTitle := false
	for _, fieldName := range templateWithTokens.RequiredFields {
		if fieldName == "title" {
			hasTitle = true
			break
		}
	}
	if !hasTitle {
		t.Error("Title should be in required fields for criteria")
	}

	// Verify field info is populated
	if templateWithTokens.FieldInfo[objects.FieldKeyTitle] == nil {
		t.Error("Field info for 'title' should be populated")
	}

	titleInfo := templateWithTokens.FieldInfo[objects.FieldKeyTitle]
	if titleInfo.Type != "string" && titleInfo.Type != "text" {
		t.Errorf("Title should be string or text type, got %s", titleInfo.Type)
	}

	// Verify category field info (should be enum for criteria)
	if categoryInfo := templateWithTokens.FieldInfo[objects.FieldKeyCategory]; categoryInfo != nil {
		if categoryInfo.Type != "enum" {
			t.Errorf("Category should be enum type for criteria, got %s", categoryInfo.Type)
		}
		if len(categoryInfo.EnumValues) == 0 {
			t.Error("Category enum should have values")
		}
	}

	t.Logf("Generated template:\n%s", templateWithTokens.Template)
	t.Logf("Required fields: %v", templateWithTokens.RequiredFields)
	t.Logf("Tokens: %v", templateWithTokens.Tokens)
}

func TestDetectTokensInTemplate(t *testing.T) {
	t.Parallel()
	template := `kind: criteria
title: {title}
category: {category}
description: {description}
status: {status}`

	tokens, err := interactive.DetectTokensInTemplate(template)
	if err != nil {
		t.Fatalf("Failed to detect tokens: %v", err)
	}

	expectedTokens := []string{"title", "category", "description", "status"}
	if len(tokens) != len(expectedTokens) {
		t.Errorf("Expected %d tokens, got %d: %v", len(expectedTokens), len(tokens), tokens)
	}

	tokenMap := make(map[string]bool)
	for _, token := range tokens {
		tokenMap[token] = true
	}

	for _, expected := range expectedTokens {
		if !tokenMap[expected] {
			t.Errorf("Expected token %q not found", expected)
		}
	}
}

func TestReplaceTokensInTemplate(t *testing.T) {
	t.Parallel()
	template := `kind: criteria
title: {title}
category: {category}
description: {description}`

	fieldValues := map[string]any{
		objects.FieldKeyTitle:       "Test Criteria",
		objects.FieldKeyCategory:    "acceptance",
		objects.FieldKeyDescription: "A test criteria",
	}

	result, err := interactive.ReplaceTokensInTemplate(template, fieldValues)
	if err != nil {
		t.Fatalf("Failed to replace tokens: %v", err)
	}

	if strings.Contains(result, "{title}") {
		t.Error("Template should not contain {title} token after replacement")
	}

	if !strings.Contains(result, `"Test Criteria"`) {
		t.Error("Template should contain replaced title value")
	}

	if !strings.Contains(result, `"acceptance"`) {
		t.Error("Template should contain replaced category value")
	}

	t.Logf("Replaced template:\n%s", result)
}

func TestValidateFieldCompleteness(t *testing.T) {
	t.Cleanup(ensureRepoSpecsForFieldRegistry(t))
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		if strings.Contains(err.Error(), "could not find specs directory") {
			t.Skipf("Specs directory not available (e.g. under bundler): %v", err)
		}
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := interactive.NewTemplateGenerator(fieldRegistry)
	templateWithTokens, err := generator.GenerateTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to generate template: %v", err)
	}

	// Test with missing required fields
	providedValues := map[string]any{
		objects.FieldKeyTitle: "Test Title",
		// Missing category and other required fields
	}

	missingRequired, invalidFields := interactive.ValidateFieldCompleteness(templateWithTokens, providedValues)

	if len(missingRequired) == 0 {
		t.Error("Should detect missing required fields")
	}

	if len(missingRequired) > 0 {
		t.Logf("Missing required fields: %v", missingRequired)
	}

	if len(invalidFields) > 0 {
		t.Logf("Invalid fields: %v", invalidFields)
	}

	// Build values for all fields the template reports as required
	allRequiredValues := make(map[string]any)
	for _, fieldName := range templateWithTokens.RequiredFields {
		tokenInfo := templateWithTokens.FieldInfo[fieldName]
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
	// Also fill any field the validator reported as missing (spec may require more than template tokens)
	for _, fieldName := range missingRequired {
		if _, set := allRequiredValues[fieldName]; set {
			continue
		}
		if tokenInfo, ok := templateWithTokens.FieldInfo[fieldName]; ok && len(tokenInfo.EnumValues) > 0 {
			allRequiredValues[fieldName] = tokenInfo.EnumValues[0]
		} else {
			allRequiredValues[fieldName] = "test value"
		}
	}

	missingRequired2, invalidFields2 := interactive.ValidateFieldCompleteness(templateWithTokens, allRequiredValues)

	if len(missingRequired2) > 0 {
		t.Errorf("Should not have missing required fields when all provided: %v", missingRequired2)
	}

	if len(invalidFields2) > 0 {
		t.Logf("Invalid fields (may be expected for default values): %v", invalidFields2)
	}
}

// TestValidateFieldType tests field type validation
// NOTE: validateFieldType is a private function in pkg/interactive, so this test
// validates the behavior indirectly through the public API (ValidateFieldCompleteness)
func TestValidateFieldType(t *testing.T) {
	bindFieldRegistryToModuleSpecs(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := interactive.NewTemplateGenerator(fieldRegistry)
	templateWithTokens, err := generator.GenerateTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to generate template: %v", err)
	}

	// Test enum validation through ValidateFieldCompleteness
	if categoryInfo := templateWithTokens.FieldInfo[objects.FieldKeyCategory]; categoryInfo != nil && categoryInfo.Type == "enum" {
		// Valid enum value
		providedValues := map[string]any{objects.FieldKeyCategory: "acceptance"}
		_, invalidFields := interactive.ValidateFieldCompleteness(templateWithTokens, providedValues)
		if len(invalidFields) > 0 {
			t.Errorf("Valid enum value should not error: %v", invalidFields)
		}

		// Invalid enum value
		providedValuesInvalid := map[string]any{objects.FieldKeyCategory: "invalid_value"}
		_, invalidFields2 := interactive.ValidateFieldCompleteness(templateWithTokens, providedValuesInvalid)
		if len(invalidFields2) == 0 {
			t.Error("Invalid enum value should error")
		}

		// Wrong type
		providedValuesWrongType := map[string]any{objects.FieldKeyCategory: 123}
		_, invalidFields3 := interactive.ValidateFieldCompleteness(templateWithTokens, providedValuesWrongType)
		if len(invalidFields3) == 0 {
			t.Error("Wrong type for enum should error")
		}
	}

	// Test string validation through ValidateFieldCompleteness
	if titleInfo := templateWithTokens.FieldInfo[objects.FieldKeyTitle]; titleInfo != nil {
		providedValues := map[string]any{objects.FieldKeyTitle: "test string"}
		_, invalidFields := interactive.ValidateFieldCompleteness(templateWithTokens, providedValues)
		if len(invalidFields) > 0 {
			t.Errorf("Valid string value should not error: %v", invalidFields)
		}

		providedValuesWrongType := map[string]any{objects.FieldKeyTitle: 123}
		_, invalidFields2 := interactive.ValidateFieldCompleteness(templateWithTokens, providedValuesWrongType)
		if titleInfo.Type == "string" && len(invalidFields2) == 0 {
			t.Error("Wrong type for string should error")
		}
	}
}
