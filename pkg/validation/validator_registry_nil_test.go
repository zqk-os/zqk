package validation

import "testing"

func TestValidatorRegistry_Get_normalizesTypedNil(t *testing.T) {
	t.Parallel()
	registry := NewValidatorRegistry()
	var nilGo *GoValidator
	registry.Register("nilholder", nilGo)
	if v := registry.Get("nilholder"); v != nil {
		t.Fatalf("expected nil Validator, got %T %#v", v, v)
	}
}
