package validation

import (
	"context"
	"testing"
)

type dummyTestValidator struct {
	name string
}

func (d *dummyTestValidator) Validate(ctx context.Context, obj map[string]any, kind string, options *ValidationOptions) (*ValidationResult, error) {
	return &ValidationResult{IsValid: true}, nil
}

func (d *dummyTestValidator) SupportsFeature(feature string) bool {
	return feature == "test_feat"
}

func (d *dummyTestValidator) Name() string {
	return d.name
}

func TestValidatorRegistry_SetDefaultAndList(t *testing.T) {
	vr := NewValidatorRegistry()
	v1 := &dummyTestValidator{name: "val1"}
	v2 := &dummyTestValidator{name: "val2"}

	vr.Register("val1", v1)
	vr.Register("val2", v2)

	list := vr.List()
	if len(list) < 2 {
		t.Fatalf("expected at least 2 validators, got %d", len(list))
	}

	vr.SetDefault("val2")
	def := vr.Get("")
	if def == nil || def.Name() != "val2" {
		t.Errorf("Get('') = %v, want val2", def)
	}

	// Unknown validator for SetDefault should be ignored
	vr.SetDefault("nonexistent")
	def = vr.Get("")
	if def == nil || def.Name() != "val2" {
		t.Errorf("expected default to remain val2, got %v", def)
	}

	// Get unknown validator returns default
	unk := vr.Get("does_not_exist")
	if unk == nil || unk.Name() != "val2" {
		t.Errorf("expected fallback to default, got %v", unk)
	}

	// Nil validator in interface
	var nilVal *dummyTestValidator
	vr.Register("nil_val", nilVal)
	if got := vr.Get("nil_val"); got != nil {
		t.Errorf("expected nil for typed nil validator, got %v", got)
	}

	// Default options
	opts := DefaultValidationOptions()
	if !opts.ValidateLifecycle || !opts.ValidateSemanticTypes {
		t.Errorf("expected default validation options to be enabled")
	}

	// Global registry
	g := GetGlobalRegistry()
	if g == nil {
		t.Fatal("expected global registry to be non-nil")
	}
}
