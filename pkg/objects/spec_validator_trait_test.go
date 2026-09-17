package objects

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestSpecValidator_ValidateTraits(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	validator := NewSpecValidator(loader, nil)

	// Load a spec that should have valid traits
	spec, err := loader.LoadSpecWithInheritance("account.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec: %v", err)
	}

	// Validate the spec (should include trait validation)
	errors := validator.ValidateSpec(pkgctx.NewSystemContext(), spec)

	// Check for trait validation errors
	traitErrors := 0
	for _, err := range errors {
		if err.Field == "traits" {
			traitErrors++
			t.Logf("Trait validation error: %s", err.Message)
		}
	}

	// Account spec should have valid traits (inherited from base_object)
	if traitErrors > 0 {
		t.Errorf("Expected no trait validation errors for account spec, got %d", traitErrors)
		for _, err := range errors {
			if err.Field == "traits" {
				t.Errorf("  Error: %s", err.Message)
			}
		}
	}
}

func TestSpecValidator_ValidateFieldTraits(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	validator := NewSpecValidator(loader, nil)

	// Load a spec with field-level traits
	spec, err := loader.LoadSpecWithInheritance("account.yaml")
	if err != nil {
		t.Fatalf("Failed to load spec: %v", err)
	}

	// Validate the spec
	errors := validator.ValidateSpec(pkgctx.NewSystemContext(), spec)

	// Check for field trait validation errors
	fieldTraitErrors := 0
	for _, err := range errors {
		if err.Message != emptyValue && (err.Message[:5] == "Field" && err.Message[6:11] == "trait") {
			fieldTraitErrors++
			t.Logf("Field trait validation error: %s", err.Message)
		}
	}

	// Account spec fields should have valid traits
	if fieldTraitErrors > 0 {
		t.Errorf("Expected no field trait validation errors for account spec, got %d", fieldTraitErrors)
	}
}

func TestSpecValidator_InvalidTrait(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	validator := NewSpecValidator(loader, nil)

	// Create a spec with an invalid trait
	spec := &Spec{
		Ontology:       "test",
		ResolvedTraits: []string{"invalid_trait", "listable"},
		ResolvedFields: make(map[string]any),
	}

	errors := validator.ValidateSpec(pkgctx.NewSystemContext(), spec)

	// Should have an error for invalid trait
	foundInvalid := false
	for _, err := range errors {
		if err.Field == "traits" && err.Message != emptyValue && err.Message[:6] == "Trait " {
			foundInvalid = true
			t.Logf("Found trait validation error: %s", err.Message)
			break
		}
	}

	if !foundInvalid {
		t.Error("Expected validation error for invalid trait, but none found")
	}
}

func TestSpecValidator_AuthoredRedundantIncludes(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	validator := NewSpecValidator(loader, nil)

	spec := &Spec{
		Ontology:       "qa_success_bad",
		Traits:         []string{"base_object_traits", "base_auditable_traits"},
		ResolvedTraits: []string{"base_object_traits", "base_auditable_traits"},
		ResolvedFields: make(map[string]any),
	}
	errors := validator.ValidateSpec(pkgctx.NewSystemContext(), spec)
	found := false
	for _, err := range errors {
		if err.Field == "traits" && err.MissingItem == TraitValidationCategoryRedundantInclude {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected authored redundant_include on traits, got %+v", errors)
	}
}

func TestSpecValidator_InheritedResolvedTraitsNotRedundantInclude(t *testing.T) {
	t.Parallel()
	loader := NewSpecLoader("")
	validator := NewSpecValidator(loader, nil)
	spec, err := loader.LoadSpecWithInheritance("account.yaml")
	if err != nil {
		t.Fatalf("load account: %v", err)
	}
	errors := validator.ValidateSpec(pkgctx.NewSystemContext(), spec)
	for _, err := range errors {
		if err.Field == "traits" && err.MissingItem == TraitValidationCategoryRedundantInclude {
			t.Fatalf("account authored traits should not be redundant_include: %s (ResolvedTraits=%v Traits=%v)", err.Message, spec.ResolvedTraits, spec.Traits)
		}
	}
}
