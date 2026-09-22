package registry

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type ComplexMockBuilder struct{}

func (m *ComplexMockBuilder) GetOntology() string { return "complexOntology" }
func (m *ComplexMockBuilder) GetVersion() string  { return "v1_0_0" }
func (m *ComplexMockBuilder) Build() *objects.Spec {
	return &objects.Spec{
		Fields: map[string]any{
			// Invalid non-map field
			"invalidField": "not_a_map",
			// Field without profile code
			"noProfileCode": map[string]any{
				objects.FieldKeyName: "noProfileCode",
			},
			// Field with checklist fallback description
			"checklistField": map[string]any{
				"field_profile_code": "CHK-001",
				"checklist": map[string]any{
					objects.FieldKeyPurpose: "Checklist purpose description",
				},
			},
			// Field with duplicate profile code (same as mock builder TST-001)
			"duplicateField": map[string]any{
				"field_profile_code":        "TST-001",
				objects.FieldKeyDescription: "Duplicate profile code",
			},
		},
	}
}

func TestGenerateRegistry_EdgeCases(t *testing.T) {
	builders.GetGlobalRegistry().Register(&ComplexMockBuilder{})

	reg, err := GenerateRegistry()
	if err != nil {
		t.Fatalf("GenerateRegistry failed: %v", err)
	}

	chkField, ok := reg.Fields["CHK-001"]
	if !ok {
		t.Fatal("expected CHK-001 in registry")
	}
	if chkField.Description != "Checklist purpose description" {
		t.Fatalf("expected checklist purpose description, got %q", chkField.Description)
	}
}

func TestLoadRegistry_Errors(t *testing.T) {
	// Non-existent file error
	if _, err := LoadRegistry(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected error for missing file")
	}

	// Corrupt JSON error
	corruptPath := filepath.Join(t.TempDir(), "corrupt.json")
	if err := fileutil.WriteSecureFile(corruptPath, []byte("invalid json {{{")); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}
	if _, err := LoadRegistry(corruptPath); err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}
