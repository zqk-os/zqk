package objects

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestSpecValidator_DisplayLength_ListableFields(t *testing.T) {
	t.Parallel()
	// Helper to create a minimal valid spec with checklist
	createMinimalSpec := func(fieldDef map[string]any) *Spec {
		// Add minimal checklist to field
		fieldDef["checklist"] = map[string]any{
			FieldKeyPurpose:         "test field",
			"system_usage":          []string{"testing"},
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
		}
		return &Spec{
			ResolvedTraits: []string{"listable", "readable"},
			ResolvedFields: map[string]any{
				"test_field": fieldDef,
			},
		}
	}

	tests := []struct {
		name          string
		spec          *Spec
		wantErrors    int
		errorContains string
	}{
		{
			name: "listable field with display_length passes",
			spec: createMinimalSpec(map[string]any{
				"traits": []any{"listable"},
				"validation": map[string]any{
					"display_length": 30,
				},
			}),
			wantErrors: 0,
		},
		{
			name: "listable field without display_length fails",
			spec: createMinimalSpec(map[string]any{
				"traits": []any{"listable"},
				"validation": map[string]any{
					"required": true,
				},
			}),
			wantErrors:    1,
			errorContains: "missing display_length constraint",
		},
		{
			name: "listable field with invalid display_length type fails",
			spec: createMinimalSpec(map[string]any{
				"traits": []any{"listable"},
				"validation": map[string]any{
					"display_length": "30", // string instead of int
				},
			}),
			wantErrors:    1,
			errorContains: "invalid display_length type",
		},
		{
			name: "listable field with zero display_length fails",
			spec: createMinimalSpec(map[string]any{
				"traits": []any{"listable"},
				"validation": map[string]any{
					"display_length": 0,
				},
			}),
			wantErrors:    1,
			errorContains: "must be positive",
		},
		{
			name: "listable field with negative display_length fails",
			spec: createMinimalSpec(map[string]any{
				"traits": []any{"listable"},
				"validation": map[string]any{
					"display_length": -5,
				},
			}),
			wantErrors:    1,
			errorContains: "must be positive",
		},
		{
			name: "non-listable field without display_length passes",
			spec: createMinimalSpec(map[string]any{
				"traits": []any{"readable"},
				"validation": map[string]any{
					"required": true,
				},
			}),
			wantErrors: 0,
		},
		{
			name: "filterable field with display_length passes",
			spec: createMinimalSpec(map[string]any{
				"traits": []any{"filterable"},
				"validation": map[string]any{
					"display_length": 20,
				},
			}),
			wantErrors: 0,
		},
		{
			name: "filterable field without display_length fails",
			spec: createMinimalSpec(map[string]any{
				"traits": []any{"filterable"},
				"validation": map[string]any{
					"required": true,
				},
			}),
			wantErrors:    1,
			errorContains: "missing display_length constraint",
		},
		{
			name: "sortable field with display_length passes",
			spec: createMinimalSpec(map[string]any{
				"traits": []any{"sortable"},
				"validation": map[string]any{
					"display_length": 25,
				},
			}),
			wantErrors: 0,
		},
		{
			name: "sortable field without display_length fails",
			spec: createMinimalSpec(map[string]any{
				"traits": []any{"sortable"},
				"validation": map[string]any{
					"required": true,
				},
			}),
			wantErrors:    1,
			errorContains: "missing display_length constraint",
		},
		{
			name: "field with multiple display traits (listable and filterable) with display_length passes",
			spec: createMinimalSpec(map[string]any{
				"traits": []any{"listable", "filterable", "sortable"},
				"validation": map[string]any{
					"display_length": 30,
				},
			}),
			wantErrors: 0,
		},
		{
			name: "listable field with float64 display_length passes",
			spec: createMinimalSpec(map[string]any{
				"traits": []any{"listable"},
				"validation": map[string]any{
					"display_length": float64(30), // YAML numbers can be float64
				},
			}),
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewSpecLoader("")
			validator := NewSpecValidator(loader, nil)

			errors := validator.ValidateSpec(pkgctx.NewSystemContext(), tt.spec)

			// Filter to only display_length related errors
			displayLengthErrors := []ValidationError{}
			for _, err := range errors {
				if err.MissingItem == "display_length" || contains(err.Message, "display_length") {
					displayLengthErrors = append(displayLengthErrors, err)
				}
			}

			if len(displayLengthErrors) != tt.wantErrors {
				t.Errorf("ValidateSpec() returned %d display_length errors, want %d", len(displayLengthErrors), tt.wantErrors)
				for _, err := range displayLengthErrors {
					t.Logf("  Display Length Error: %s", err.Message)
				}
				// Also log all errors for debugging
				if len(errors) > len(displayLengthErrors) {
					t.Logf("  Other errors (%d):", len(errors)-len(displayLengthErrors))
					for _, err := range errors {
						if err.MissingItem != "display_length" && !contains(err.Message, "display_length") {
							t.Logf("    %s: %s", err.Field, err.Message)
						}
					}
				}
			}

			if tt.errorContains != emptyValue {
				found := false
				for _, err := range displayLengthErrors {
					if contains(err.Message, tt.errorContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected display_length error containing '%s', got errors: %v", tt.errorContains, displayLengthErrors)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
