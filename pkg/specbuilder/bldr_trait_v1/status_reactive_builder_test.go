package bldr_trait_v1

import "testing"

func TestStatusReactiveBuilderIsObjectLevelListenerAdmission(t *testing.T) {
	t.Parallel()
	trait := NewStatusReactiveBuilder().Build()
	if trait.Name != "status_reactive" {
		t.Fatalf("name=%q want status_reactive", trait.Name)
	}
	if !trait.ObjectLevel {
		t.Fatal("status_reactive must be object-level (listener admission), not a field trait")
	}
	if trait.FieldLevel {
		t.Fatal("status_reactive must not be field-level")
	}
}
