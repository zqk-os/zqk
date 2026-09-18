package objects

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// TestDisplayLength_Regression_SpecValidation tests that spec validation
// for display_length doesn't regress
func TestDisplayLength_Regression_SpecValidation(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	validator := NewSpecValidator(loader, nil)
	ctx := pkgctx.NewSystemContext()

	tests := []struct {
		name          string
		spec          *Spec
		wantErrors    int
		errorContains string
		description   string
	}{
		{
			name: "listable field with valid display_length passes",
			spec: &Spec{
				ResolvedTraits: []string{"listable"},
				ResolvedFields: map[string]any{
					"test_field": map[string]any{
						"traits": []any{"listable"},
						"checklist": map[string]any{
							FieldKeyPurpose:         "test",
							"system_usage":          []string{"test"},
							"criticality":           "composition",
							"cardinality":           "one",
							"lifecycle":             "immutable",
							FieldKeyAuthority:       "automation",
							"validation":            "test",
							FieldKeyDependencies:    "none",
							"default":               "",
							"observability":         "yes",
							"security":              "non-sensitive",
							FieldKeyAutomationHooks: "none",
							"field_profile_code":    "TEST-001",
						},
						"validation": map[string]any{
							"display_length": 30,
						},
					},
				},
			},
			wantErrors:  0,
			description: "Listable field with valid display_length should pass validation",
		},
		{
			name: "listable field without display_length fails",
			spec: &Spec{
				ResolvedTraits: []string{"listable"},
				ResolvedFields: map[string]any{
					"test_field": map[string]any{
						"traits": []any{"listable"},
						"checklist": map[string]any{
							FieldKeyPurpose:         "test",
							"system_usage":          []string{"test"},
							"criticality":           "composition",
							"cardinality":           "one",
							"lifecycle":             "immutable",
							FieldKeyAuthority:       "automation",
							"validation":            "test",
							FieldKeyDependencies:    "none",
							"default":               "",
							"observability":         "yes",
							"security":              "non-sensitive",
							FieldKeyAutomationHooks: "none",
							"field_profile_code":    "TEST-001",
						},
						"validation": map[string]any{
							"required": true,
						},
					},
				},
			},
			wantErrors:    1,
			errorContains: "missing display_length constraint",
			description:   "Listable field without display_length should fail validation",
		},
		{
			name: "non-listable field without display_length passes",
			spec: &Spec{
				ResolvedTraits: []string{"readable"},
				ResolvedFields: map[string]any{
					"test_field": map[string]any{
						"traits": []any{"readable"},
						"checklist": map[string]any{
							FieldKeyPurpose:         "test",
							"system_usage":          []string{"test"},
							"criticality":           "composition",
							"cardinality":           "one",
							"lifecycle":             "immutable",
							FieldKeyAuthority:       "automation",
							"validation":            "test",
							FieldKeyDependencies:    "none",
							"default":               "",
							"observability":         "yes",
							"security":              "non-sensitive",
							FieldKeyAutomationHooks: "none",
							"field_profile_code":    "TEST-001",
						},
						"validation": map[string]any{
							"required": true,
						},
					},
				},
			},
			wantErrors:  0,
			description: "Non-listable field without display_length should pass (not required)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateSpec(ctx, tt.spec)

			// Filter to only display_length related errors
			displayLengthErrors := []ValidationError{}
			for _, err := range errors {
				if err.MissingItem == "display_length" || contains(err.Message, "display_length") {
					displayLengthErrors = append(displayLengthErrors, err)
				}
			}

			if len(displayLengthErrors) != tt.wantErrors {
				t.Errorf("ValidateSpec() returned %d display_length errors, want %d (%s)",
					len(displayLengthErrors), tt.wantErrors, tt.description)
				for _, err := range displayLengthErrors {
					t.Logf("  Error: %s", err.Message)
				}
			}

			if tt.errorContains != emptyValue {
				found := false
				for _, err := range displayLengthErrors {
					if containsDisplayLength(err.Message, tt.errorContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected display_length error containing '%s' (%s), got errors: %v",
						tt.errorContains, tt.description, displayLengthErrors)
				}
			}
		})
	}
}

// TestDisplayLength_Regression_GetDisplayLength tests that GetDisplayLength
// doesn't regress with various spec configurations
func TestDisplayLength_Regression_GetDisplayLength(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		spec        *Spec
		fieldName   string
		defaultLen  int
		want        int
		description string
	}{
		{
			name: "spec with display_length as int",
			spec: &Spec{
				ResolvedFields: map[string]any{
					FieldKeyID: map[string]any{
						"validation": map[string]any{
							"display_length": 12,
						},
					},
				},
			},
			fieldName:   "id",
			defaultLen:  20,
			want:        12,
			description: "Should return display_length from spec when present",
		},
		{
			name: "spec with display_length as float64 (YAML parsing)",
			spec: &Spec{
				ResolvedFields: map[string]any{
					FieldKeyID: map[string]any{
						"validation": map[string]any{
							"display_length": float64(12),
						},
					},
				},
			},
			fieldName:   "id",
			defaultLen:  20,
			want:        12,
			description: "Should handle float64 display_length (YAML number parsing)",
		},
		{
			name: "spec without display_length uses default",
			spec: &Spec{
				ResolvedFields: map[string]any{
					FieldKeyID: map[string]any{
						"validation": map[string]any{
							"required": true,
						},
					},
				},
			},
			fieldName:   "id",
			defaultLen:  20,
			want:        20,
			description: "Should use default when display_length not in spec",
		},
		{
			name:        "nil spec uses default",
			spec:        nil,
			fieldName:   "id",
			defaultLen:  20,
			want:        20,
			description: "Should use default when spec is nil",
		},
		{
			name: "field not in spec uses default",
			spec: &Spec{
				ResolvedFields: map[string]any{
					"other_field": map[string]any{},
				},
			},
			fieldName:   "id",
			defaultLen:  20,
			want:        20,
			description: "Should use default when field not in spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDisplayLength(tt.spec, tt.fieldName, tt.defaultLen)
			if got != tt.want {
				t.Errorf("GetDisplayLength() = %v, want %v (%s)", got, tt.want, tt.description)
			}
		})
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
