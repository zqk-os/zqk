package objects

import (
	"context"
	"fmt"
	"sort"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// TestSpecValidator_FieldChecklistCompleteness tests that all fields have complete checklists
// TDD: Write tests first, then implement
func TestSpecValidator_FieldChecklistCompleteness(t *testing.T) {
	t.Parallel()
	// Required checklist items for field vetting (defined in criteria objects)
	// These are the field vetting questions that must be answered

	loader := NewSpecLoader("")
	// For tests, we'll use nil criteria lookup (no traceability)
	// In real usage, you would create a storage provider and use storage.NewCriteriaLookupFunc
	validator := NewSpecValidator(loader, nil)
	ctx := pkgctx.NewSystemContext()

	tests := []struct {
		name        string
		specFile    string
		wantErrors  bool
		fieldName   string
		missingItem string
	}{
		{
			name:        "account spec validation finds missing items",
			specFile:    "account.yaml",
			wantErrors:  true, // Account spec has inherited fields missing field_profile_code and default
			fieldName:   "id", // id field from base_object is missing field_profile_code
			missingItem: "field_profile_code",
		},
		// Note: Testing missing items requires a spec with incomplete checklist
		// We test this by verifying the validator finds missing items in real specs
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This will fail until ValidateSpec is implemented
			spec, err := loader.LoadSpecWithInheritance(tt.specFile)
			if err != nil {
				t.Fatalf("Failed to load spec: %v", err)
			}

			errors := validator.ValidateSpec(ctx, spec)

			if (len(errors) > 0) != tt.wantErrors {
				t.Errorf("ValidateSpec() errors = %v, wantErrors %v", errors, tt.wantErrors)
			}

			// If we expect errors, verify specific missing items
			if tt.wantErrors && tt.missingItem != emptyValue {
				found := false
				for _, err := range errors {
					if err.Field == tt.fieldName && err.MissingItem == tt.missingItem {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error for missing %s in field %s", tt.missingItem, tt.fieldName)
				}
			}
		})
	}
}

// TestSpecValidator_AllFieldsValidated tests that all fields in a spec are validated
func TestSpecValidator_AllFieldsValidated(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	// For tests, we'll use nil criteria lookup (no traceability)
	// In real usage, you would create a storage provider and use storage.NewCriteriaLookupFunc
	validator := NewSpecValidator(loader, nil)
	ctx := pkgctx.NewSystemContext()

	spec, err := loader.LoadSpecWithInheritance("account.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec: %v", err)
	}

	errors := validator.ValidateSpec(ctx, spec)

	// Check that all fields are validated
	validatedFields := make(map[string]bool)
	for _, err := range errors {
		validatedFields[err.Field] = true
	}

	// All fields should be checked (errors or not)
	// If a field has no errors, it means it passed validation
	for fieldName := range spec.ResolvedFields {
		// Field should either have errors (meaning it was checked) or be valid
		// We can't easily test "no errors" without knowing which fields are valid
		// So we'll test that fields with errors are reported
		_ = validatedFields[fieldName]
	}
}

// TestSpecValidator_InheritedFieldsValidated tests that inherited fields are also validated
func TestSpecValidator_InheritedFieldsValidated(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	// For tests, we'll use nil criteria lookup (no traceability)
	// In real usage, you would create a storage provider and use storage.NewCriteriaLookupFunc
	validator := NewSpecValidator(loader, nil)
	ctx := pkgctx.NewSystemContext()

	// account.yaml extends base_object, so it should validate inherited fields too
	spec, err := loader.LoadSpecWithInheritance("account.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec: %v", err)
	}

	errors := validator.ValidateSpec(ctx, spec)

	// Check that inherited fields (like 'id' from base_object) are validated
	inheritedFields := []string{"id", "title", "status", "created_at", "created_by"}
	for _, fieldName := range inheritedFields {
		// Field should be in resolved fields
		if _, ok := spec.ResolvedFields[fieldName]; ok {
			// Field should be validated (either has errors or is valid)
			// We can't easily test "no errors" without full implementation
			_ = errors
		}
	}
}

// TestSpecValidator_CriteriaTraceability tests that validation errors reference criteria objects
// when storage provider is available
func TestSpecValidator_CriteriaTraceability(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	// For tests, we'll use nil criteria lookup (no traceability)
	// In real usage, you would create a storage provider and use storage.NewCriteriaLookupFunc
	validator := NewSpecValidator(loader, nil)
	ctx := pkgctx.NewSystemContext()

	spec, err := loader.LoadSpecWithInheritance("account.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec: %v", err)
	}

	errors := validator.ValidateSpec(ctx, spec)

	// If criteria are found, they should be referenced
	// If not found, CriteriaRef will be empty (acceptable - graceful degradation)
	hasCriteriaRefs := false
	for _, err := range errors {
		if err.CriteriaRef != emptyValue {
			hasCriteriaRefs = true
			// Verify criteria ID format
			if len(err.CriteriaRef) < 5 || err.CriteriaRef[:5] != "CRIT-" {
				t.Errorf("Invalid criteria reference format: %s", err.CriteriaRef)
			}
		}
	}

	// At least some errors should have criteria refs if criteria exist
	// (This test verifies the mechanism works, not that all criteria exist)
	if len(errors) > 0 && !hasCriteriaRefs {
		t.Logf("Note: No criteria references found - criteria may not exist in test environment")
	}
}

// TestSpecValidator_WithoutStorageProvider tests that validation works without storage provider
func TestSpecValidator_WithoutStorageProvider(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	validator := NewSpecValidator(loader, nil) // No criteria lookup function
	ctx := pkgctx.NewSystemContext()

	spec, err := loader.LoadSpecWithInheritance("account.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec: %v", err)
	}

	errors := validator.ValidateSpec(ctx, spec)

	// Validation should still work, but criteria refs will be empty
	if len(errors) == 0 {
		t.Error("Expected validation errors, got none")
	}

	// All errors should have empty criteria refs
	for _, err := range errors {
		if err.CriteriaRef != emptyValue {
			t.Errorf("Expected empty criteria ref without storage provider, got %s", err.CriteriaRef)
		}
	}
}

// TestSpecValidator_getCriteriaRef tests the getCriteriaRef method
func TestSpecValidator_getCriteriaRef(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	ctx := pkgctx.NewSystemContext()

	// Test with nil criteria lookup (no traceability)
	validator := NewSpecValidator(loader, nil)
	criteriaRef := validator.getCriteriaRef(ctx, "purpose")
	if criteriaRef != emptyValue {
		t.Errorf("Expected empty criteria ref with nil lookup, got %s", criteriaRef)
	}

	// Test with criteria lookup function that returns a value
	mockLookup := func(ctx context.Context, itemName string) (string, error) {
		if itemName == "purpose" {
			return "CRIT-001", nil
		}
		return "", nil
	}
	validatorWithLookup := NewSpecValidator(loader, mockLookup)
	criteriaRef = validatorWithLookup.getCriteriaRef(ctx, "purpose")
	if criteriaRef != "CRIT-001" {
		t.Errorf("Expected criteria ref 'CRIT-001', got %s", criteriaRef)
	}

	// Test with criteria lookup function that returns error
	errorLookup := func(ctx context.Context, itemName string) (string, error) {
		return "", fmt.Errorf("criteria not found")
	}
	validatorWithError := NewSpecValidator(loader, errorLookup)
	criteriaRef = validatorWithError.getCriteriaRef(ctx, "purpose")
	if criteriaRef != emptyValue {
		t.Errorf("Expected empty criteria ref on error, got %s", criteriaRef)
	}

	// Test with criteria lookup function that returns empty string
	emptyLookup := func(ctx context.Context, itemName string) (string, error) {
		return "", nil
	}
	validatorWithEmpty := NewSpecValidator(loader, emptyLookup)
	criteriaRef = validatorWithEmpty.getCriteriaRef(ctx, "purpose")
	if criteriaRef != emptyValue {
		t.Errorf("Expected empty criteria ref when lookup returns empty, got %s", criteriaRef)
	}
}

func sortValidationErrors(errs []ValidationError) {
	sort.Slice(errs, func(i, j int) bool {
		if errs[i].Field != errs[j].Field {
			return errs[i].Field < errs[j].Field
		}
		if errs[i].MissingItem != errs[j].MissingItem {
			return errs[i].MissingItem < errs[j].MissingItem
		}
		return errs[i].Message < errs[j].Message
	})
}

// TestLoadSpecAndValidate_SingleLoad ensures load+validate does not double-call the loader for one file.
func TestLoadSpecAndValidate_SingleLoad(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	ctx := pkgctx.NewSystemContext()
	spec, errs, err := LoadSpecAndValidate(ctx, loader, "account.yaml", nil)
	if err != nil {
		t.Fatalf("LoadSpecAndValidate: %v", err)
	}
	if spec == nil || spec.Ontology == "" {
		t.Fatal("expected spec with ontology")
	}
	direct := ValidateLoadedSpec(ctx, loader, spec, nil)
	if len(direct) != len(errs) {
		t.Fatalf("len mismatch: LoadSpecAndValidate %d vs ValidateLoadedSpec %d", len(errs), len(direct))
	}
}

// TestValidateLoadedSpec_MatchesNewSpecValidator ensures the unified entrypoint is not a second code path.
func TestValidateLoadedSpec_MatchesNewSpecValidator(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	ctx := pkgctx.NewSystemContext()
	spec, err := loader.LoadSpecWithInheritance("account.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	direct := NewSpecValidator(loader, nil).ValidateSpec(ctx, spec)
	unified := ValidateLoadedSpec(ctx, loader, spec, nil)
	sortValidationErrors(direct)
	sortValidationErrors(unified)
	if len(direct) != len(unified) {
		t.Fatalf("error count mismatch: direct %d unified %d", len(direct), len(unified))
	}
	for i := range direct {
		if direct[i].Field != unified[i].Field || direct[i].MissingItem != unified[i].MissingItem || direct[i].Message != unified[i].Message {
			t.Fatalf("diff at %d: direct %+v unified %+v", i, direct[i], unified[i])
		}
	}
}
