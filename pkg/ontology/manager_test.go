package ontology_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/ontology"
)

func TestOntologyManager_RegisterAndValidate(t *testing.T) {
	mgr := ontology.NewManager()

	o := ontology.Ontology{
		ID:      "core-1.0",
		Version: "1.0",
		Classes: map[string]ontology.Class{
			"Person": {
				Name: "Person",
				Properties: map[string]ontology.Property{
					objects.FieldKeyName: {Name: "name", Type: "string", Required: true},
					"age":                {Name: "age", Type: "int", Required: false},
				},
			},
		},
	}

	if err := mgr.Register(o); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Valid instance
	validInstance := map[string]any{objects.FieldKeyName: "Alice", "age": 30}
	if err := mgr.ValidateInstance("Person", validInstance); err != nil {
		t.Errorf("Expected valid instance to pass, got: %v", err)
	}

	// Invalid instance (missing required 'name')
	invalidInstance := map[string]any{"age": 30}
	if err := mgr.ValidateInstance("Person", invalidInstance); err != ontology.ErrInvalidInstance {
		t.Errorf("Expected ErrInvalidInstance, got: %v", err)
	}
}
