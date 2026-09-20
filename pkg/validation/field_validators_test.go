package validation

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestResolveStringForPatternValidation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		fieldValue any
		wantStr    string
		wantOK     bool
	}{
		{"string value", "BLI-001", "BLI-001", true},
		{"empty string", "", "", true},
		{"time.Time", now, "2026-03-05T12:00:00Z", true},
		{"nil", nil, "", false},
		{"int small", 42, "", false},
		{"float non-Unix", 3.14, "", false},
		{"slice", []string{"a"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStr, gotOK := ResolveStringForPatternValidation(tt.fieldValue)
			if gotStr != tt.wantStr || gotOK != tt.wantOK {
				t.Errorf(ConstMagicd7604f57, tt.fieldValue, gotStr, gotOK, tt.wantStr, tt.wantOK)
			}
		})
	}
}

func TestResolveStringForPatternValidationWithField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldName string
		fieldVal  any
		wantStr   string
		wantOK    bool
	}{
		{"schema_version float 2.0", "schema_version", 2.0, objects.DefaultSchemaVersion, true},
		{"schema_version float 1.5", "schema_version", 1.5, "1.5.0", true},
		{"schema_version string", "schema_version", objects.DefaultSchemaVersion, objects.DefaultSchemaVersion, true},
		{"schema_version string 2.0", "schema_version", "2.0", objects.DefaultSchemaVersion, true},
		{"schema_version string 2", "schema_version", "2", objects.DefaultSchemaVersion, true},
		{"created_at float Unix", "created_at", 1735689600.0, "2025-01-01T00:00:00Z", true},
		{"created_at int64 Unix", "created_at", int64(1735689600), "2025-01-01T00:00:00Z", true},
		{"created_at string Unix seconds", "created_at", "1735689600", "2025-01-01T00:00:00Z", true},
		{"created_at string Unix millis", "created_at", "1735689600000", "2025-01-01T00:00:00Z", true},
		{"created_at string Unix seconds .0", "created_at", "1735689600.0", "2025-01-01T00:00:00Z", true},
		{"other field float 2.0 not Unix", "other", 2.0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStr, gotOK := ResolveStringForPatternValidationWithField(tt.fieldName, tt.fieldVal)
			if gotStr != tt.wantStr || gotOK != tt.wantOK {
				t.Errorf(ConstMagic02e907b6, tt.fieldName, tt.fieldVal, gotStr, gotOK, tt.wantStr, tt.wantOK)
			}
		})
	}
}

func TestValidateRequiredField_Description(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldVal  any
		wantRule  string
		wantValid bool
	}{
		{"empty description", "", "required", false},
		{"short description < 10 chars", "short", "minLength", false},
		{"placeholder required", "required", "placeholder", false},
		{"placeholder todo", "TODO", "placeholder", false},
		{"placeholder tbd", "tbd", "placeholder", false},
		{"valid description", "Substantive narrative description that exceeds minimum length", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequiredField("description", tt.fieldVal, true, false)
			if tt.wantValid {
				if err != nil {
					t.Errorf("expected valid, got error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error with rule %s, got nil", tt.wantRule)
				} else if err.Rule != tt.wantRule {
					t.Errorf("got rule %s, want %s", err.Rule, tt.wantRule)
				}
			}
		})
	}
}
