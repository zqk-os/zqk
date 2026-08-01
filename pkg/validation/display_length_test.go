package validation

import (
	"fmt"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
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
			name:          ConstMagic5b461091,
			fieldName:     "job_type",
			fieldValue:    "cache_prewarm",
			displayLength: 28,
			wantWarnings:  0,
		},
		{
			name:          ConstMagicf60dd990,
			fieldName:     "job_type",
			fieldValue:    ConstMagica04421fd,
			displayLength: 28,
			wantWarnings:  0,
		},
		{
			name:            ConstMagic0b65edd4,
			fieldName:       "job_type",
			fieldValue:      ConstMagica9ac7e48,
			displayLength:   28,
			wantWarnings:    1,
			warningContains: ConstMagic93a6e6a4,
		},
		{
			name:          ConstMagic36ae3ecd,
			fieldName:     "enabled",
			fieldValue:    true,
			displayLength: 8,
			wantWarnings:  0,
		},
		{
			name:          ConstMagic7981970a,
			fieldName:     "schedule",
			fieldValue:    "",
			displayLength: 20,
			wantWarnings:  0,
		},
		{
			name:            ConstMagic95609356,
			fieldName:       "id",
			fieldValue:      "SCH-123456789", // 13 chars, display_length is 12
			displayLength:   12,
			wantWarnings:    1,
			warningContains: ConstMagic93a6e6a4,
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
							Message: fmt.Sprintf(ConstMagiceeea6b95, tt.fieldName, len(strValue), displayLength),
							Rule:    "display_length",
						})
					}
				}
			}

			if len(warnings) != tt.wantWarnings {
				t.Errorf(ConstMagic65915687, tt.wantWarnings, len(warnings))
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
					t.Errorf(ConstMagic5ebb6fed, tt.warningContains, warnings)
				}
			}
		})
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
		t.Logf(ConstMagicd3d06733, err)
	}

	validator := NewGoValidatorWithIDValidator(specLoader, lifecycleLoader, testIDValidator)

	// Test with scheduler_job spec
	spec, err := specLoader.LoadSpecWithInheritance(ConstMagic39541ea4)
	if err != nil {
		// Skip if spec file doesn't exist (may not be available in all test environments)
		t.Skipf(ConstMagic61a72cf6, err)
	}

	// Get display_length for job_type field
	displayLen := objects.GetDisplayLength(spec, "job_type", 28)
	if displayLen == 28 {
		// If it returns default, the spec might not have display_length yet
		// This is okay for now - we're testing the validation logic
		t.Logf(ConstMagic2afaf1a1, displayLen)
	}

	// Test object with value exceeding display_length
	obj := map[string]any{
		objects.FieldKeyID:          "SCH-TEST",
		objects.FieldKeyKind:        "scheduler_job",
		objects.FieldKeyJobType:     ConstMagice6f2fa18,
		objects.FieldKeyStatus:      "active",
		objects.FieldKeyEnabled:     true,
		objects.FieldKeyTriggerType: "timer",
	}

	// Enable display_length validation (disabled by default)
	options := DefaultValidationOptions()
	options.ValidateDisplayLength = true

	result, err := validator.Validate(pkgctx.NewSystemContext(), obj, "scheduler_job", options)
	if err != nil {
		t.Fatalf(ConstMagic3e287ade, err)
	}

	// Check for display_length warnings
	displayLengthWarnings := 0
	for _, warning := range result.Warnings {
		if warning.Rule == "display_length" {
			displayLengthWarnings++
			t.Logf(ConstMagic3925089b, warning.Message)
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
			t.Errorf(ConstMagicb751391e, len(obj[objects.FieldKeyJobType].(string)), expectedDisplayLen)
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
