package objects

import (
	"testing"
)

func TestLoaderOrderError_Error(t *testing.T) {
	t.Parallel()
	err := &LoaderOrderError{
		Message: "spec not loaded",
		Step:    "instance_loading",
	}

	expected := "loading order violation at instance_loading: spec not loaded"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestNewLoadingOrder(t *testing.T) {
	t.Parallel()
	lo := NewLoadingOrder()
	if lo == nil {
		t.Fatal("NewLoadingOrder() returned nil")
	}
	if lo.specsLoaded == nil {
		t.Fatal("specsLoaded map is nil")
	}
	if len(lo.specsLoaded) != 0 {
		t.Errorf("Expected empty specsLoaded map, got %d entries", len(lo.specsLoaded))
	}
}

func TestLoadingOrder_MarkSpecLoaded(t *testing.T) {
	t.Parallel()
	lo := NewLoadingOrder()

	lo.MarkSpecLoaded("criteria")
	if !lo.specsLoaded["criteria"] {
		t.Error("criteria should be marked as loaded")
	}

	lo.MarkSpecLoaded("base_object")
	if !lo.specsLoaded["base_object"] {
		t.Error("base_object should be marked as loaded")
	}

	if len(lo.specsLoaded) != 2 {
		t.Errorf("Expected 2 loaded specs, got %d", len(lo.specsLoaded))
	}
}

func TestLoadingOrder_RequireSpecLoaded(t *testing.T) {
	t.Parallel()
	lo := NewLoadingOrder()

	// Test with unloaded spec
	err := lo.RequireSpecLoaded("criteria")
	if err == nil {
		t.Fatal("Expected error for unloaded spec")
	}

	loaderErr, ok := err.(*LoaderOrderError)
	if !ok {
		t.Fatalf("Expected LoaderOrderError, got %T", err)
	}

	if loaderErr.Step != "instance_loading" {
		t.Errorf("Expected step 'instance_loading', got %q", loaderErr.Step)
	}

	// Test with loaded spec
	lo.MarkSpecLoaded("criteria")
	err = lo.RequireSpecLoaded("criteria")
	if err != nil {
		t.Errorf("Expected no error for loaded spec, got %v", err)
	}
}

func TestGetRequiredSpecs(t *testing.T) {
	t.Parallel()
	specs := GetRequiredSpecs()

	expectedSpecs := []string{
		"criteria",
		"base_object",
		"auditable",
		"extensible_object",
	}

	if len(specs) != len(expectedSpecs) {
		t.Errorf("Expected %d specs, got %d", len(expectedSpecs), len(specs))
	}

	specMap := make(map[string]bool)
	for _, spec := range specs {
		specMap[spec] = true
	}

	for _, expected := range expectedSpecs {
		if !specMap[expected] {
			t.Errorf("Expected spec %q not found in result", expected)
		}
	}
}
