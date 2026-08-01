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

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2" // Register builders for tests
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
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
				t.Fatalf(ConstMagicaca29340, kind, err)
			}

			if spec == nil {
				t.Fatalf(ConstMagicc2e1cfca, kind)
			}

			// Verify it extends extensible_object
			if spec.Extends != ConstMagic41fa5b86 {
				t.Errorf(ConstMagicac31e02c, kind, spec.Extends)
			}

			// Verify ontology is set (should match the kind name)
			if spec.Ontology != kind {
				t.Errorf(ConstMagicbb87926a, kind, kind, spec.Ontology)
			}

			// Verify spec has fields
			if len(spec.ResolvedFields) == 0 {
				t.Errorf(ConstMagicad5a4899, kind)
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
		objects.FieldKeyCreatedAt:         ConstMagic0206afbb,
		objects.FieldKeyCreatedBy:         "account:test",
		objects.FieldKeyUpdatedAt:         ConstMagic0206afbb,
		objects.FieldKeyUpdatedBy:         "account:test",
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
			name: ConstMagic1436c43d,
			kind: "organization",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:               "ORG-001",
					objects.FieldKeyKind:             "organization",
					objects.FieldKeyTitle:            ConstMagice2e5cc46,
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
					objects.FieldKeyTitle:        ConstMagice9a1bdea,
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
			name: ConstMagice8cd6577,
			kind: "department",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:             "DEP-001",
					objects.FieldKeyKind:           "department",
					objects.FieldKeyTitle:          ConstMagic607c0574,
					objects.FieldKeyStatus:         "proposed",
					objects.FieldKeyDepartmentName: ConstMagic607c0574,
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
			name: ConstMagicb55f3bac,
			kind: "partnership",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:               "PAR-001",
					objects.FieldKeyKind:             "partnership",
					objects.FieldKeyTitle:            ConstMagic9dcf8fce,
					objects.FieldKeyStatus:           "proposed",
					objects.FieldKeyPartnershipName:  "Tech Alliance",
					objects.FieldKeyPartnershipType:  ConstMagic10155eec, // Fixed: must match enum
					objects.FieldKeyOrganizationRefs: []string{"ORG-001", "ORG-002"},
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
		},
		{
			name: ConstMagic9798e3ce,
			kind: "organization",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:               "ORG-002",
					objects.FieldKeyKind:             "organization",
					objects.FieldKeyTitle:            ConstMagic3bbdce06,
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
			name: ConstMagicdbd14688,
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
				t.Fatalf(ConstMagic6762e2c0, err)
			}

			if !result.IsValid {
				t.Errorf(ConstMagice324e829)
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
		objects.FieldKeyCreatedAt:         ConstMagic0206afbb,
		objects.FieldKeyCreatedBy:         "account:test",
		objects.FieldKeyUpdatedAt:         ConstMagic0206afbb,
		objects.FieldKeyUpdatedBy:         "account:test",
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
			name: ConstMagic075e5ba3,
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
			errorMsg:  ConstMagicd71a8daf,
		},
		{
			name: ConstMagicd5d1f80d,
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
			name: ConstMagic8231a72e,
			kind: "department",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:     "DEP-002",
					objects.FieldKeyKind:   "department",
					objects.FieldKeyTitle:  "Test Department",
					objects.FieldKeyStatus: "active",
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
			name: ConstMagicc95e323a,
			kind: "team",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:     "TEA-002",
					objects.FieldKeyKind:   "team",
					objects.FieldKeyTitle:  "Test Team",
					objects.FieldKeyStatus: "active",
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
			name: ConstMagic5679d804,
			kind: "partnership",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:     "PAR-002",
					objects.FieldKeyKind:   "partnership",
					objects.FieldKeyTitle:  ConstMagicd309ab32,
					objects.FieldKeyStatus: "active",
					// Missing partnership_name
				}
				for k, v := range validBaseFields {
					obj[k] = v
				}
				return obj
			}(),
			wantValid: false,
			errorMsg:  ConstMagic502c58b6,
		},
		{
			name: ConstMagiccebc294f,
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
				t.Fatalf(ConstMagic6762e2c0, err)
			}

			if result.IsValid != tt.wantValid {
				t.Errorf(ConstMagicf5ed72f6, result.IsValid, tt.wantValid)
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
					t.Errorf(ConstMagic187a2cdc, tt.errorMsg, result.Errors)
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
		objects.FieldKeyCreatedAt:         ConstMagic0206afbb,
		objects.FieldKeyCreatedBy:         "account:test",
		objects.FieldKeyUpdatedAt:         ConstMagic0206afbb,
		objects.FieldKeyUpdatedBy:         "account:test",
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
			name: ConstMagic07dffd64,
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
			name: ConstMagic8a6bc9fb,
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
			name: ConstMagica1f9d947,
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
			name: ConstMagic74c3b2e4,
			kind: "partnership",
			obj: func() map[string]any {
				obj := map[string]any{
					objects.FieldKeyID:               "PAR-003",
					objects.FieldKeyKind:             "partnership",
					objects.FieldKeyTitle:            ConstMagicd309ab32,
					objects.FieldKeyStatus:           "proposed",
					objects.FieldKeyPartnershipName:  ConstMagicd309ab32,
					objects.FieldKeyPartnershipType:  ConstMagic10155eec,
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
				t.Fatalf(ConstMagic6762e2c0, err)
			}

			if result.IsValid != tt.wantValid {
				t.Errorf(ConstMagicf5ed72f6, result.IsValid, tt.wantValid)
				if len(result.Errors) > 0 {
					t.Logf(ConstMagic6ad67277, result.Errors)
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
