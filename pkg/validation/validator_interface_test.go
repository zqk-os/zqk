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

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2" // Register builders for tests
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

func TestValidatorRegistry(t *testing.T) {
	t.Parallel()
	registry := NewValidatorRegistry()

	// Test default validator
	validator := registry.Get("")
	if validator == nil {
		t.Fatal("Default validator should not be nil")
	}
	if validator.Name() != "go" {
		t.Errorf("Expected default validator name 'go', got '%s'", validator.Name())
	}

	// Test getting by name
	validator = registry.Get("go")
	if validator == nil {
		t.Fatal("Go validator should not be nil")
	}

	// Test getting non-existent validator (should return default)
	validator = registry.Get("nonexistent")
	if validator == nil {
		t.Fatal("Should return default validator for nonexistent name")
	}
	if validator.Name() != "go" {
		t.Errorf("Expected default validator for nonexistent name, got '%s'", validator.Name())
	}
}

func TestGoValidator_Name(t *testing.T) {
	t.Parallel()
	validator := NewGoValidator()
	if validator.Name() != "go" {
		t.Errorf("Expected validator name 'go', got '%s'", validator.Name())
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
				t.Errorf("SupportsFeature(%s) = %v, want %v", tt.feature, got, tt.want)
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
		objects.FieldKeyCreatedAt:     "2025-12-25T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     "2025-12-25T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
	}

	options := DefaultValidationOptions()
	result, err := validator.Validate(pkgctx.NewSystemContext(), obj, "backlog_item", options)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	// Should be valid (or have warnings only)
	if result == nil {
		t.Fatal("Validation result should not be nil")
	}
}

func TestValidationOptions_Default(t *testing.T) {
	t.Parallel()
	options := DefaultValidationOptions()
	if options == nil {
		t.Fatal("DefaultValidationOptions() should not return nil")
	}
	if !options.ValidateLifecycle {
		t.Error("Default should have ValidateLifecycle = true")
	}
	if !options.ValidateSemanticTypes {
		t.Error("Default should have ValidateSemanticTypes = true")
	}
	if options.StrictMode {
		t.Error("Default should have StrictMode = false")
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
		t.Fatal("Registered validator should not be nil")
	}
	if validator.Name() != "custom" {
		t.Errorf("Expected validator name 'custom', got '%s'", validator.Name())
	}

	// Registering again should overwrite
	anotherValidator := &mockValidator{name: "custom2"}
	registry.Register("custom", anotherValidator)
	validator = registry.Get("custom")
	if validator.Name() != "custom2" {
		t.Errorf("Expected validator name 'custom2' after re-register, got '%s'", validator.Name())
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
		t.Fatal("Default validator should not be nil")
	}
	if validator.Name() != "custom" {
		t.Errorf("Expected default validator name 'custom', got '%s'", validator.Name())
	}

	// Setting default to non-existent validator should not change default
	registry.SetDefault("nonexistent")
	validator = registry.Get("")
	if validator.Name() != "custom" {
		t.Errorf("Expected default validator to remain 'custom', got '%s'", validator.Name())
	}
}

// TestValidatorRegistry_List tests the List function
func TestValidatorRegistry_List(t *testing.T) {
	t.Parallel()
	registry := NewValidatorRegistry()

	// Initially should have "go" and "default"
	names := registry.List()
	if len(names) < 2 {
		t.Errorf("Expected at least 2 validators, got %d", len(names))
	}

	// Register additional validators
	customValidator := &mockValidator{name: "custom"}
	registry.Register("custom", customValidator)
	anotherValidator := &mockValidator{name: "another"}
	registry.Register("another", anotherValidator)

	// List should include all registered validators
	names = registry.List()
	if len(names) < 4 {
		t.Errorf("Expected at least 4 validators, got %d", len(names))
	}

	// Check that all registered validators are in the list
	nameMap := make(map[string]bool)
	for _, name := range names {
		nameMap[name] = true
	}

	if !nameMap["go"] {
		t.Error("List() should include 'go' validator")
	}
	if !nameMap["default"] {
		t.Error("List() should include 'default' validator")
	}
	if !nameMap["custom"] {
		t.Error("List() should include 'custom' validator")
	}
	if !nameMap["another"] {
		t.Error("List() should include 'another' validator")
	}
}

// TestGetGlobalRegistry tests the GetGlobalRegistry function
func TestGetGlobalRegistry(t *testing.T) {
	t.Parallel()
	registry1 := GetGlobalRegistry()
	registry2 := GetGlobalRegistry()

	// Should return the same instance
	if registry1 != registry2 {
		t.Error("GetGlobalRegistry() should return the same instance")
	}

	// Should have default validators
	validator := registry1.Get("go")
	if validator == nil {
		t.Fatal("Global registry should have 'go' validator")
	}
	if validator.Name() != "go" {
		t.Errorf("Expected validator name 'go', got '%s'", validator.Name())
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
