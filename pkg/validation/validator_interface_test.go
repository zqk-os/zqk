//go:build lifecycle_test
// +build lifecycle_test

// This test file is excluded from normal test runs to avoid import cycle issues.
// The cycle is: validation -> specbuilder/bldr_v2 -> validation (via base_object_builder.go)
//
// To run these tests, use: go test -tags=lifecycle_test ./pkg/validation -run TestValidator

package validation

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2" // Register builders for tests
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

func TestValidatorRegistry(t *testing.T) {
	t.Parallel()
	registry := NewValidatorRegistry()

	// Test default validator
	validator := registry.Get("")
	if validator == nil {
		t.Fatal(ConstMagiccbb2f856)
	}
	if validator.Name() != "go" {
		t.Errorf(ConstMagic5e73ca8d, validator.Name())
	}

	// Test getting by name
	validator = registry.Get("go")
	if validator == nil {
		t.Fatal(ConstMagic374cc900)
	}

	// Test getting non-existent validator (should return default)
	validator = registry.Get("nonexistent")
	if validator == nil {
		t.Fatal(ConstMagicefdf843e)
	}
	if validator.Name() != "go" {
		t.Errorf(ConstMagic27caea8e, validator.Name())
	}
}

func TestGoValidator_Name(t *testing.T) {
	t.Parallel()
	validator := NewGoValidator()
	if validator.Name() != "go" {
		t.Errorf(ConstMagic3c38595a, validator.Name())
	}
}

func TestGoValidator_SupportsFeature(t *testing.T) {
	t.Parallel()
	validator := NewGoValidator()

	tests := []struct {
		feature string
		want    bool
	}{
		{"lifecycle", true},
		{"semantic_types", true},
		{"custom_rules", false},
		{"shacl_export", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.feature, func(t *testing.T) {
			got := validator.SupportsFeature(tt.feature)
			if got != tt.want {
				t.Errorf(ConstMagic734eefcd, tt.feature, got, tt.want)
			}
		})
	}
}

func TestGoValidator_Validate(t *testing.T) {
	t.Parallel()
	// Create spec loader with builder registry configured
	specLoader := objects.NewSpecLoader("")
	builderRegistry := builders.GetGlobalRegistry()
	adapter := builders.NewSpecLoaderAdapter(builderRegistry)
	specLoader.SetBuilderRegistry(adapter)

	// Create validator with configured spec loader
	validator := NewGoValidatorWithLoaders(specLoader, nil)

	// Test with valid object
	obj := map[string]any{
		objects.FieldKeyID:            "BLI-001",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Test Item",
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     ConstMagiccd842f93,
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     ConstMagiccd842f93,
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
	}

	options := DefaultValidationOptions()
	result, err := validator.Validate(pkgctx.NewSystemContext(), obj, "backlog_item", options)
	if err != nil {
		t.Fatalf(ConstMagic6762e2c0, err)
	}

	// Should be valid (or have warnings only)
	if result == nil {
		t.Fatal(ConstMagic8428c61d)
	}
}

func TestValidationOptions_Default(t *testing.T) {
	t.Parallel()
	options := DefaultValidationOptions()
	if options == nil {
		t.Fatal(ConstMagicf16c4f98)
	}
	if !options.ValidateLifecycle {
		t.Error(ConstMagic7f073b98)
	}
	if !options.ValidateSemanticTypes {
		t.Error(ConstMagicea3b0e49)
	}
	if options.StrictMode {
		t.Error(ConstMagicaf2ebb3e)
	}
}

// TestValidatorRegistry_Register tests the Register function
func TestValidatorRegistry_Register(t *testing.T) {
	t.Parallel()
	registry := NewValidatorRegistry()

	// Create a custom validator
	customValidator := &mockValidator{name: "custom"}
	registry.Register("custom", customValidator)

	// Should be able to retrieve it
	validator := registry.Get("custom")
	if validator == nil {
		t.Fatal(ConstMagice24758a9)
	}
	if validator.Name() != "custom" {
		t.Errorf(ConstMagic9b5c6493, validator.Name())
	}

	// Registering again should overwrite
	anotherValidator := &mockValidator{name: "custom2"}
	registry.Register("custom", anotherValidator)
	validator = registry.Get("custom")
	if validator.Name() != "custom2" {
		t.Errorf(ConstMagic7ab24cc0, validator.Name())
	}
}

// TestValidatorRegistry_SetDefault tests the SetDefault function
func TestValidatorRegistry_SetDefault(t *testing.T) {
	t.Parallel()
	registry := NewValidatorRegistry()

	// Register a custom validator
	customValidator := &mockValidator{name: "custom"}
	registry.Register("custom", customValidator)

	// Set it as default
	registry.SetDefault("custom")

	// Getting with empty name should return custom validator
	validator := registry.Get("")
	if validator == nil {
		t.Fatal(ConstMagiccbb2f856)
	}
	if validator.Name() != "custom" {
		t.Errorf(ConstMagic4f4315ea, validator.Name())
	}

	// Setting default to non-existent validator should not change default
	registry.SetDefault("nonexistent")
	validator = registry.Get("")
	if validator.Name() != "custom" {
		t.Errorf(ConstMagic4f73589c, validator.Name())
	}
}

// TestValidatorRegistry_List tests the List function
func TestValidatorRegistry_List(t *testing.T) {
	t.Parallel()
	registry := NewValidatorRegistry()

	// Initially should have "go" and "default"
	names := registry.List()
	if len(names) < 2 {
		t.Errorf(ConstMagicbab2b469, len(names))
	}

	// Register additional validators
	customValidator := &mockValidator{name: "custom"}
	registry.Register("custom", customValidator)
	anotherValidator := &mockValidator{name: "another"}
	registry.Register("another", anotherValidator)

	// List should include all registered validators
	names = registry.List()
	if len(names) < 4 {
		t.Errorf(ConstMagic4bc685e8, len(names))
	}

	// Check that all registered validators are in the list
	nameMap := make(map[string]bool)
	for _, name := range names {
		nameMap[name] = true
	}

	if !nameMap["go"] {
		t.Error(ConstMagic61c85e86)
	}
	if !nameMap["default"] {
		t.Error(ConstMagic109ae71f)
	}
	if !nameMap["custom"] {
		t.Error(ConstMagic5b56e601)
	}
	if !nameMap["another"] {
		t.Error(ConstMagic96b34c12)
	}
}

// TestGetGlobalRegistry tests the GetGlobalRegistry function
func TestGetGlobalRegistry(t *testing.T) {
	t.Parallel()
	registry1 := GetGlobalRegistry()
	registry2 := GetGlobalRegistry()

	// Should return the same instance
	if registry1 != registry2 {
		t.Error(ConstMagic0e94f572)
	}

	// Should have default validators
	validator := registry1.Get("go")
	if validator == nil {
		t.Fatal(ConstMagicd0766367)
	}
	if validator.Name() != "go" {
		t.Errorf(ConstMagic3c38595a, validator.Name())
	}
}

// mockValidator is a test implementation of the Validator interface
type mockValidator struct {
	name string
}

func (m *mockValidator) Validate(ctx context.Context, obj map[string]any, kind string, options *ValidationOptions) (*ValidationResult, error) {
	return &ValidationResult{IsValid: true}, nil
}

func (m *mockValidator) Name() string {
	return m.name
}

func (m *mockValidator) SupportsFeature(feature string) bool {
	return false
}
