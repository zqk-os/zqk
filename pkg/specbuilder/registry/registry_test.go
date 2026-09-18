package registry

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// MockBuilder simulates a spec builder for testing purposes.
type MockBuilder struct{}

func (m *MockBuilder) GetOntology() string { return "testOntology" }
func (m *MockBuilder) GetVersion() string  { return "v1_0_0" }
func (m *MockBuilder) Build() *objects.Spec {
	return &objects.Spec{
		Fields: map[string]any{
			"testField": map[string]any{objects.FieldKeyName: "testField", objects.FieldKeyType: "string", "field_profile_code": "TST-001", objects.FieldKeyDescription: "Test field"},
		},
	}
}

func TestGenerateRegistry(t *testing.T) {
	// Register the mock builder
	builders.GetGlobalRegistry().Register(&MockBuilder{})

	registry, err := GenerateRegistry()
	if err != nil {
		t.Fatalf("failed to generate registry: %v", err)
	}

	if _, ok := registry.Fields["TST-001"]; !ok {
		t.Fatal("registry does not contain TST-001")
	}
}

func TestLoadRegistry(t *testing.T) {
	registry := &FieldRegistry{
		Fields: map[string]FieldMetadata{
			"TST-001": {Name: "testField", Type: "string", ProfileCode: "TST-001", Description: "Test field"},
		},
	}

	path := "load_test_registry.json"
	defer fileutil.Remove(path)

	if err := SaveRegistry(path, registry); err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	loaded, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("failed to load registry: %v", err)
	}

	if loaded.Fields["TST-001"].Name != "testField" {
		t.Fatalf("expected testField, got %s", loaded.Fields["TST-001"].Name)
	}
}
