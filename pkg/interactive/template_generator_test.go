package interactive

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestGenerateTemplateWithTokens(t *testing.T) {
	t.Parallel()
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.Reload(); err != nil {
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := NewTemplateGenerator(fieldRegistry)

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

	tokens, err := DetectTokensInTemplate(template)
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

	result, err := ReplaceTokensInTemplate(template, fieldValues)
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
	t.Parallel()
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.Reload(); err != nil {
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := NewTemplateGenerator(fieldRegistry)
	templateWithTokens, err := generator.GenerateTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to generate template: %v", err)
	}

	// Test with missing required fields
	providedValues := map[string]any{
		objects.FieldKeyTitle: "Test Title",
		// Missing category and other required fields
	}

	missingRequired, invalidFields := ValidateFieldCompleteness(templateWithTokens, providedValues)

	if len(missingRequired) == 0 {
		t.Error("Should detect missing required fields")
	}

	// Category might be required - check if it's in required fields
	if len(missingRequired) > 0 {
		t.Logf("Missing required fields: %v", missingRequired)
	}

	if len(invalidFields) > 0 {
		t.Logf("Invalid fields: %v", invalidFields)
	}

	// Test with all required fields provided
	allRequiredValues := make(map[string]any)
	for _, fieldName := range templateWithTokens.RequiredFields {
		tokenInfo := templateWithTokens.FieldInfo[fieldName]
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

	missingRequired2, invalidFields2 := ValidateFieldCompleteness(templateWithTokens, allRequiredValues)

	if len(missingRequired2) > 0 {
		t.Errorf("Should not have missing required fields when all provided: %v", missingRequired2)
	}

	if len(invalidFields2) > 0 {
		t.Logf("Invalid fields (may be expected for default values): %v", invalidFields2)
	}
}

func TestValidateFieldType(t *testing.T) {
	t.Parallel()
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.Reload(); err != nil {
		t.Fatalf("Failed to reload field registry: %v", err)
	}

	generator := NewTemplateGenerator(fieldRegistry)
	templateWithTokens, err := generator.GenerateTemplateWithTokens("criteria")
	if err != nil {
		t.Fatalf("Failed to generate template: %v", err)
	}

	// Test enum validation
	if categoryInfo := templateWithTokens.FieldInfo[objects.FieldKeyCategory]; categoryInfo != nil && categoryInfo.Type == "enum" {
		// Valid enum value
		err := validateFieldType("category", "acceptance", categoryInfo)
		if err != nil {
			t.Errorf("Valid enum value should not error: %v", err)
		}

		// Invalid enum value
		err = validateFieldType("category", "invalid_value", categoryInfo)
		if err == nil {
			t.Error("Invalid enum value should error")
		}

		// Wrong type
		err = validateFieldType("category", 123, categoryInfo)
		if err == nil {
			t.Error("Wrong type for enum should error")
		}
	}

	// Test string validation
	if titleInfo := templateWithTokens.FieldInfo[objects.FieldKeyTitle]; titleInfo != nil {
		err := validateFieldType("title", "test string", titleInfo)
		if err != nil {
			t.Errorf("Valid string value should not error: %v", err)
		}

		err = validateFieldType("title", 123, titleInfo)
		if err == nil && titleInfo.Type == "string" {
			t.Error("Wrong type for string should error")
		}
	}
}
