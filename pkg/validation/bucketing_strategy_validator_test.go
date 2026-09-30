package validation

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestValidateBucketingStrategy_ArchiveStrategy tests archive strategy validation
func TestValidateBucketingStrategy_ArchiveStrategy(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		strategy      map[string]any
		expectedError bool
		errorField    string
	}{
		{
			name: "valid simple archive strategy",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: true,
					"archive_after":         "720h",
					"archive_tier":          "warm",
				},
			},
			expectedError: false,
		},
		{
			name: "valid multi-tier archive strategy",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: true,
					"tier_progression": []any{
						map[string]any{
							objects.FieldKeyTier: "warm",
							"duration":           "720h",
						},
						map[string]any{
							objects.FieldKeyTier: "cold",
							"duration":           "8760h",
							"compression":        true,
						},
						map[string]any{
							objects.FieldKeyTier: "iced",
							"duration":           "0",
							"compression":        true,
							"encryption":         true,
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "enabled archive strategy without archive_after or tier_progression",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: true,
				},
			},
			expectedError: true,
			errorField:    "archive_strategy.archive_after",
		},
		{
			name: "invalid archive_after duration format",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: true,
					"archive_after":         "invalid",
				},
			},
			expectedError: true,
			errorField:    "archive_strategy.archive_after",
		},
		{
			name: "invalid tier in tier_progression",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: true,
					"tier_progression": []any{
						map[string]any{
							objects.FieldKeyTier: "invalid_tier",
							"duration":           "720h",
						},
					},
				},
			},
			expectedError: true,
			errorField:    "archive_strategy.tier_progression[0].tier",
		},
		{
			name: "missing duration in tier_progression",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: true,
					"tier_progression": []any{
						map[string]any{
							objects.FieldKeyTier: "warm",
						},
					},
				},
			},
			expectedError: true,
			errorField:    "archive_strategy.tier_progression[0].duration",
		},
		{
			name: "archive strategy disabled (no validation needed)",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
				objects.FieldKeyArchiveStrategy: map[string]any{
					objects.FieldKeyEnabled: false,
				},
			},
			expectedError: false,
		},
		{
			name: "no archive strategy (optional field)",
			strategy: map[string]any{
				objects.FieldKeyStrategyType: "chronological",
				objects.FieldKeyField:        "created_at",
				objects.FieldKeyFormat:       "2006-01",
				objects.FieldKeyEnabled:      true,
				objects.FieldKeyAppliesTo:    []any{"audit_event"},
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errors := ValidateBucketingStrategy(tc.strategy)

			if tc.expectedError {
				if len(errors) == 0 {
					t.Errorf("Expected validation error for %s, got none", tc.name)
				} else {
					// Check if error is for the expected field
					found := false
					for _, err := range errors {
						if err.Field == tc.errorField {
							found = true
							break
						}
					}
					if !found && tc.errorField != emptyValue {
						t.Errorf("Expected error for field %s, got errors: %v", tc.errorField, errors)
					}
				}
			} else {
				// Filter out non-archive errors for this test
				archiveErrors := []ValidationError{}
				for _, err := range errors {
					if containsArchiveField(err.Field, "archive_strategy") {
						archiveErrors = append(archiveErrors, err)
					}
				}
				if len(archiveErrors) > 0 {
					t.Errorf("Expected no archive strategy validation errors, got: %v", archiveErrors)
				}
			}
		})
	}
}

// contains checks if a string contains a substring (helper for test)
func containsArchiveField(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsArchiveFieldMiddle(s, substr)))
}

func containsArchiveFieldMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
