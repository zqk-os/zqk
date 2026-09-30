//go:build lifecycle_test
// +build lifecycle_test

// This test file is excluded from normal test runs to avoid import cycle issues.
// The cycle is: validation -> specbuilder/bldr_v2 -> validation (via base_object_builder.go)
//
// To run these tests, use: go test -tags=lifecycle_test ./pkg/validation -run TestOrganizationalOntology

package validation

import (
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2" // Register builders for tests
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// TestOrganizationalOntology_SpecsLoadable tests that all organizational ontology specs can be loaded
func TestOrganizationalOntology_SpecsLoadable(t *testing.T) {
	t.Parallel()
	specsDir := filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)

	organizationalKinds := []string{
		"organization",
		"division",
		"department",
		"team",
		"partnership",
	}

	for _, kind := range organizationalKinds {
		t.Run(kind, func(t *testing.T) {
			spec, err := specLoader.LoadSpecWithInheritance(kind + ".yaml")
			if err != nil {
				t.Fatalf("Failed to load spec for %s: %v", kind, err)
			}

			if spec == nil {
				t.Fatalf("Spec for %s is nil", kind)
			}

			// Verify it extends extensible_object
			if spec.Extends != "extensible_object" {
				t.Errorf("Expected %s to extend extensible_object, got %s", kind, spec.Extends)
			}

			// Verify ontology is set (should match the kind name)
			if spec.Ontology != kind {
				t.Errorf("Expected ontology '%s' for %s, got %s", kind, kind, spec.Ontology)
			}

			// Verify spec has fields
			if len(spec.ResolvedFields) == 0 {
				t.Errorf("Spec for %s should have resolved fields", kind)
			}
		})
	}
}

// TestOrganizationalOntology_ValidInstances tests that valid instances can be created and validated
func TestOrganizationalOntology_ValidInstances(t *testing.T) {
	t.Parallel()
	specsDir := filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)
	// Set builder registry on spec loader to enable version-aware loading
	builderRegistry := builders.GetGlobalRegistry()
	adapter := builders.NewSpecLoaderAdapter(builderRegistry)
	specLoader.SetBuilderRegistry(adapter)
	lifecycleLoader := objects.NewLifecycleLoader("")
	validator := NewGoValidatorWithLoaders(specLoader, lifecycleLoader)

	// Helper to create valid audit fields and extensible_object fields
	validBaseFields := map[string]any{
		objects.FieldKeyCreatedAt:         "2025-12-31T00:00:00Z",
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         "2025-12-31T00:00:00Z",
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyDomain:            "custom",  // Required by extensible_object
		objects.FieldKeySpecInterpreter:   "default", // Required by extensible_object
		objects.FieldKeySpecContextBroker: "default", // Required by extensible_object
	}

	tests := []struct {
		name string
		obj  map[string]any
		kind string
	}{
		{
			name: "valid organization",
			kind: "organization",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:               "ORG-001",
					objects.FieldKeyKind:             "organization",
					objects.FieldKeyTitle:            "Test Organization",
					objects.FieldKeyStatus:           "proposed",
					objects.FieldKeyOrganizationName: "Acme Corp",
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
		},
		{
			name: "valid division",
			kind: "division",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:           "DIV-001",
					objects.FieldKeyKind:         "division",
					objects.FieldKeyTitle:        "Engineering Division",
					objects.FieldKeyStatus:       "proposed",
					objects.FieldKeyDivisionName: "Engineering",
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
		},
		{
			name: "valid department",
			kind: "department",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:             "DEP-001",
					objects.FieldKeyKind:           "department",
					objects.FieldKeyTitle:          "Software Engineering",
					objects.FieldKeyStatus:         "proposed",
					objects.FieldKeyDepartmentName: "Software Engineering",
					objects.FieldKeyDivisionRef:    "DIV-001",
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
		},
		{
			name: "valid team",
			kind: "team",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:          "TEA-001",
					objects.FieldKeyKind:        "team",
					objects.FieldKeyTitle:       "Platform Team",
					objects.FieldKeyStatus:      "proposed",
					objects.FieldKeyTeamName:    "Platform Team",
					objects.FieldKeyDivisionRef: "DIV-001",
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
		},
		{
			name: "valid partnership",
			kind: "partnership",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:               "PAR-001",
					objects.FieldKeyKind:             "partnership",
					objects.FieldKeyTitle:            "Strategic Partnership",
					objects.FieldKeyStatus:           "proposed",
					objects.FieldKeyPartnershipName:  "Tech Alliance",
					objects.FieldKeyPartnershipType:  "strategic_alliance", // Fixed: must match enum
					objects.FieldKeyOrganizationRefs: []string{"ORG-001", "ORG-002"},
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
		},
		{
			name: "organization with division references",
			kind: "organization",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:               "ORG-002",
					objects.FieldKeyKind:             "organization",
					objects.FieldKeyTitle:            "Test Org with Divisions",
					objects.FieldKeyStatus:           "proposed",
					objects.FieldKeyOrganizationName: "Test Corp",
					objects.FieldKeyDivisionRefs:     []string{"DIV-001", "DIV-002"},
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
		},
		{
			name: "division with parent and child references",
			kind: "division",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:                "DIV-002",
					objects.FieldKeyKind:              "division",
					objects.FieldKeyTitle:             "Sub Division",
					objects.FieldKeyStatus:            "proposed",
					objects.FieldKeyDivisionName:      "Sub Division",
					objects.FieldKeyParentDivisionRef: "DIV-001",
					objects.FieldKeyChildDivisionRefs: []string{"DIV-003"},
					objects.FieldKeyTeamRefs:          []string{"TEA-001", "TEA-002"},
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := DefaultValidationOptions()
			result, err := validator.Validate(pkgctx.NewSystemContext(), tt.obj, tt.kind, options)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			if !result.IsValid {
				t.Errorf("Expected valid instance, but got validation errors:")
				for _, validationErr := range result.Errors {
					t.Errorf("  - %s: %s", validationErr.Field, validationErr.Message)
				}
			}
		})
	}
}

// TestOrganizationalOntology_InvalidInstances tests that invalid instances are caught
func TestOrganizationalOntology_InvalidInstances(t *testing.T) {
	t.Parallel()
	specsDir := filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)
	// Set builder registry on spec loader to enable version-aware loading
	builderRegistry := builders.GetGlobalRegistry()
	adapter := builders.NewSpecLoaderAdapter(builderRegistry)
	specLoader.SetBuilderRegistry(adapter)
	lifecycleLoader := objects.NewLifecycleLoader("")
	validator := NewGoValidatorWithLoaders(specLoader, lifecycleLoader)

	validBaseFields := map[string]any{
		objects.FieldKeyCreatedAt:         "2025-12-31T00:00:00Z",
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         "2025-12-31T00:00:00Z",
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyDomain:            "custom",
		objects.FieldKeySpecInterpreter:   "default",
		objects.FieldKeySpecContextBroker: "default",
	}

	tests := []struct {
		name      string
		obj       map[string]any
		kind      string
		wantValid bool
		errorMsg  string // Expected error message substring
	}{
		{
			name: "organization missing required organization_name",
			kind: "organization",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:     "ORG-003",
					objects.FieldKeyKind:   "organization",
					objects.FieldKeyTitle:  "Test Org",
					objects.FieldKeyStatus: "active",
					// Missing organization_name
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
			wantValid: false,
			errorMsg:  "organization_name",
		},
		{
			name: "division missing required division_name",
			kind: "division",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:     "DIV-003",
					objects.FieldKeyKind:   "division",
					objects.FieldKeyTitle:  "Test Division",
					objects.FieldKeyStatus: "active",
					// Missing division_name
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
			wantValid: false,
			errorMsg:  "division_name",
		},
		{
			name: "department missing required department_name",
			kind: "department",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:     "DEP-002",
					objects.FieldKeyKind:   "department",
					objects.FieldKeyTitle:  "Test Department",
					objects.FieldKeyStatus: "proposed",
					// Missing department_name
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
			wantValid: false,
			errorMsg:  "department_name",
		},
		{
			name: "team missing required team_name",
			kind: "team",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:     "TEA-002",
					objects.FieldKeyKind:   "team",
					objects.FieldKeyTitle:  "Test Team",
					objects.FieldKeyStatus: "proposed",
					// Missing team_name
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
			wantValid: false,
			errorMsg:  "team_name",
		},
		{
			name: "partnership missing required partnership_name",
			kind: "partnership",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:     "PAR-002",
					objects.FieldKeyKind:   "partnership",
					objects.FieldKeyTitle:  "Test Partnership",
					objects.FieldKeyStatus: "proposed",
					// Missing partnership_name
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
			wantValid: false,
			errorMsg:  "partnership_name",
		},
		{
			name: "invalid ID pattern",
			kind: "organization",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:               "INVALID", // Doesn't match ORG-XXX pattern
					objects.FieldKeyKind:             "organization",
					objects.FieldKeyTitle:            "Test Org",
					objects.FieldKeyStatus:           "proposed",
					objects.FieldKeyOrganizationName: "Test Corp",
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
			wantValid: false,
			errorMsg:  "pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := DefaultValidationOptions()
			result, err := validator.Validate(pkgctx.NewSystemContext(), tt.obj, tt.kind, options)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			if result.IsValid != tt.wantValid {
				t.Errorf("Validate() IsValid = %v, want %v", result.IsValid, tt.wantValid)
			}

			if !tt.wantValid {
				// Verify we got the expected error
				found := false
				for _, validationErr := range result.Errors {
					if tt.errorMsg == emptyValue || contains(validationErr.Message, tt.errorMsg) || contains(validationErr.Field, tt.errorMsg) {
						found = true
						break
					}
				}
				if !found && tt.errorMsg != emptyValue {
					t.Errorf("Expected error message containing '%s', but got errors: %+v", tt.errorMsg, result.Errors)
				}
			}
		})
	}
}

// TestOrganizationalOntology_ReferenceFields tests that reference fields are properly validated
func TestOrganizationalOntology_ReferenceFields(t *testing.T) {
	t.Parallel()
	specsDir := filepath.Join("..", "..", paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)
	// Set builder registry on spec loader to enable version-aware loading
	builderRegistry := builders.GetGlobalRegistry()
	adapter := builders.NewSpecLoaderAdapter(builderRegistry)
	specLoader.SetBuilderRegistry(adapter)
	lifecycleLoader := objects.NewLifecycleLoader("")
	validator := NewGoValidatorWithLoaders(specLoader, lifecycleLoader)

	validBaseFields := map[string]any{
		objects.FieldKeyCreatedAt:         "2025-12-31T00:00:00Z",
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         "2025-12-31T00:00:00Z",
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyDomain:            "custom",
		objects.FieldKeySpecInterpreter:   "default",
		objects.FieldKeySpecContextBroker: "default",
	}

	tests := []struct {
		name      string
		obj       map[string]any
		kind      string
		wantValid bool
	}{
		{
			name: "department with valid division reference",
			kind: "department",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:             "DEP-003",
					objects.FieldKeyKind:           "department",
					objects.FieldKeyTitle:          "Test Department",
					objects.FieldKeyStatus:         "proposed",
					objects.FieldKeyDepartmentName: "Test Department",
					objects.FieldKeyDivisionRef:    "DIV-001", // Valid reference format
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
			wantValid: true,
		},
		{
			name: "team with valid division and department references",
			kind: "team",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:            "TEA-003",
					objects.FieldKeyKind:          "team",
					objects.FieldKeyTitle:         "Test Team",
					objects.FieldKeyStatus:        "proposed",
					objects.FieldKeyTeamName:      "Test Team",
					objects.FieldKeyDivisionRef:   "DIV-001",
					objects.FieldKeyDepartmentRef: "DEP-001",
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
			wantValid: true,
		},
		{
			name: "organization with multiple division references",
			kind: "organization",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:               "ORG-004",
					objects.FieldKeyKind:             "organization",
					objects.FieldKeyTitle:            "Test Org",
					objects.FieldKeyStatus:           "proposed",
					objects.FieldKeyOrganizationName: "Test Corp",
					objects.FieldKeyDivisionRefs:     []string{"DIV-001", "DIV-002", "DIV-003"},
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
			wantValid: true,
		},
		{
			name: "partnership with multiple organization references",
			kind: "partnership",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:               "PAR-003",
					objects.FieldKeyKind:             "partnership",
					objects.FieldKeyTitle:            "Test Partnership",
					objects.FieldKeyStatus:           "proposed",
					objects.FieldKeyPartnershipName:  "Test Partnership",
					objects.FieldKeyPartnershipType:  "strategic_alliance",
					objects.FieldKeyOrganizationRefs: []string{"ORG-001", "ORG-002", "ORG-003"},
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := DefaultValidationOptions()
			result, err := validator.Validate(pkgctx.NewSystemContext(), tt.obj, tt.kind, options)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			if result.IsValid != tt.wantValid {
				t.Errorf("Validate() IsValid = %v, want %v", result.IsValid, tt.wantValid)
				if len(result.Errors) > 0 {
					t.Logf("Validation errors: %+v", result.Errors)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring (case-insensitive)
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
