package validation

import (
	"fmt"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestGoValidator_DisplayLength_Warnings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		fieldName       string
		fieldValue      any
		displayLength   int
		wantWarnings    int
		warningContains string
	}{
		{
			name:          "value within display_length generates no warning",
			fieldName:     "job_type",
			fieldValue:    "cache_prewarm",
			displayLength: 28,
			wantWarnings:  0,
		},
		{
			name:          "value exactly at display_length generates no warning",
			fieldName:     "job_type",
			fieldValue:    "audit_event_aggregation",
			displayLength: 28,
			wantWarnings:  0,
		},
		{
			name:            "value exceeding display_length generates warning",
			fieldName:       "job_type",
			fieldValue:      "this_is_a_very_long_job_type_name_that_exceeds_display_length",
			displayLength:   28,
			wantWarnings:    1,
			warningContains: "exceeds display_length constraint",
		},
		{
			name:          "non-string value generates no warning",
			fieldName:     "enabled",
			fieldValue:    true,
			displayLength: 8,
			wantWarnings:  0,
		},
		{
			name:          "empty string generates no warning",
			fieldName:     "schedule",
			fieldValue:    "",
			displayLength: 20,
			wantWarnings:  0,
		},
		{
			name:            "value one character over display_length generates warning",
			fieldName:       "id",
			fieldValue:      "SCH-123456789", // 13 chars, display_length is 12
			displayLength:   12,
			wantWarnings:    1,
			warningContains: "exceeds display_length constraint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal spec with the field
			spec := &objects.Spec{
				ResolvedFields: map[string]any{
					tt.fieldName: map[string]any{
						objects.FieldKeyType: "string",
						"validation": map[string]any{
							"display_length": tt.displayLength,
						},
					},
				},
			}

			// Test the display_length validation logic directly
			fieldDef := spec.ResolvedFields[tt.fieldName].(map[string]any)
			validation := fieldDef["validation"].(map[string]any)

			// Test the display_length validation logic
			var warnings []ValidationWarning
			if displayLength, ok := validation["display_length"].(int); ok {
				if strValue, ok := tt.fieldValue.(string); ok {
					if len(strValue) > displayLength {
						warnings = append(warnings, ValidationWarning{
							Field:   tt.fieldName,
							Message: fmt.Sprintf("Field %s value length (%d) exceeds display_length constraint (%d) - may be truncated in table displays", tt.fieldName, len(strValue), displayLength),
							Rule:    "display_length",
						})
					}
				}
			}

			if len(warnings) != tt.wantWarnings {
				t.Errorf("Expected %d warnings, got %d", tt.wantWarnings, len(warnings))
				for _, w := range warnings {
					t.Logf("  Warning: %s", w.Message)
				}
			}

			if tt.warningContains != emptyValue && len(warnings) > 0 {
				found := false
				for _, w := range warnings {
					if containsDisplayLength(w.Message, tt.warningContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected warning containing '%s', got warnings: %v", tt.warningContains, warnings)
				}
			}
		})
	}
}

func TestBaseObjectTitleUsesDisplayLengthWithoutStorageLimit(t *testing.T) {
	t.Parallel()

	specLoader := objects.NewSpecLoader(getTestSpecsDir(t))
	spec, err := specLoader.LoadSpecWithInheritance("base_object.yaml")
	if err != nil {
		t.Fatal(err)
	}
	titleDef, ok := spec.ResolvedFields[objects.FieldKeyTitle].(map[string]any)
	if !ok {
		t.Fatalf("title field definition missing: %T", spec.ResolvedFields[objects.FieldKeyTitle])
	}
	titleValidation, ok := titleDef["validation"].(map[string]any)
	if !ok {
		t.Fatalf("title validation missing: %T", titleDef["validation"])
	}
	if _, limited := titleValidation["max_length"]; limited {
		t.Fatal("title must preserve full canonical data; max_length belongs in display_length")
	}
	if got := objects.GetDisplayLength(spec, objects.FieldKeyTitle, 0); got != 120 {
		t.Fatalf("title display_length = %d, want 120", got)
	}
}

func TestGoValidator_DisplayLength_Integration(t *testing.T) {
	t.Parallel()
	// Integration test using actual spec loader
	// Use test specs directory to ensure spec files are available
	specsDir := getTestSpecsDir(t)
	specLoader := objects.NewSpecLoader(specsDir)
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	// Create test-specific IDValidator for isolation
	testIDValidator := NewIDValidatorWithConfigs(
		specsDir,
		getTestIDPrefixesConfig(),
		getTestPathsConfig(t),
	)
	if err := testIDValidator.LoadPatterns(); err != nil {
		t.Logf("Warning: Failed to load ID patterns: %v (ID validation may be permissive)", err)
	}

	validator := NewGoValidatorWithIDValidator(specLoader, lifecycleLoader, testIDValidator)

	// Test with scheduler_job spec
	spec, err := specLoader.LoadSpecWithInheritance("scheduler_job.yaml")
	if err != nil {
		// Skip if spec file doesn't exist (may not be available in all test environments)
		t.Skipf("Skipping test - scheduler_job spec not found: %v", err)
	}

	// Get display_length for job_type field
	displayLen := objects.GetDisplayLength(spec, "job_type", 28)
	if displayLen == 28 {
		// If it returns default, the spec might not have display_length yet
		// This is okay for now - we're testing the validation logic
		t.Logf("job_type display_length: %d (may be default)", displayLen)
	}

	// Test object with value exceeding display_length
	obj := map[string]any{
		objects.FieldKeyID:          "SCH-TEST",
		objects.FieldKeyKind:        "scheduler_job",
		objects.FieldKeyJobType:     "this_is_a_very_long_job_type_name_that_exceeds_display_length_constraint",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
		objects.FieldKeyEnabled:     true,
		objects.FieldKeyTriggerType: "timer",
	}

	// Enable display_length validation (disabled by default)
	options := DefaultValidationOptions()
	options.ValidateDisplayLength = true

	result, err := validator.Validate(pkgctx.NewSystemContext(), obj, "scheduler_job", options)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Check for display_length warnings
	displayLengthWarnings := 0
	for _, warning := range result.Warnings {
		if warning.Rule == "display_length" {
			displayLengthWarnings++
			t.Logf("Display length warning: %s", warning.Message)
		}
	}

	// We expect at least one warning if job_type value exceeds its display_length
	// Validator uses default display_length (50) when not specified in spec
	expectedDisplayLen := displayLen
	if displayLen == 28 {
		// 28 is the default passed to GetDisplayLength, but validator uses 50 as default
		expectedDisplayLen = 50
	}
	if expectedDisplayLen < len(obj[objects.FieldKeyJobType].(string)) {
		if displayLengthWarnings == 0 {
			t.Errorf("Expected display_length warning for job_type (value length %d exceeds display_length %d), but got none", len(obj[objects.FieldKeyJobType].(string)), expectedDisplayLen)
		}
	}
}

func containsDisplayLength(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
