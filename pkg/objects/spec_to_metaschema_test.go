package objects

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/dna"
)

func TestSpecToMetaSchema(t *testing.T) {
	kernelCritical := true
	spec := &Spec{
		Ontology:      "account",
		SchemaVersion: "2.0.0",
		Extends:       "base_object",
		StorageProfile: "cas_entity",
		KernelCritical: &kernelCritical,
		Traits:         []string{"auditable", "streamable"},
		Fields: map[string]any{
			"code": map[string]any{
				"type": "string",
				"validation": map[string]any{
					"required":   true,
					"min_length": 3,
					"max_length": 20,
					"pattern":    "^[A-Z]{3}-\\d{2}$",
				},
				"description": "Unique account code",
			},
			"tier": map[string]any{
				"type": "enum",
				"validation": map[string]any{
					"required": true,
					"enum":     []any{"P0", "P1", "P2", "P3"},
				},
			},
		},
	}

	meta := spec.ToMetaSchema()
	if meta == nil {
		t.Fatal("expected non-nil MetaSchema")
	}

	if meta.TargetKind != "account" {
		t.Errorf("expected target kind 'account', got %q", meta.TargetKind)
	}
	if !meta.KernelCritical {
		t.Errorf("expected KernelCritical true")
	}
	if !meta.HasTrait(dna.TraitAuditable) {
		t.Errorf("expected auditable trait")
	}
	if !meta.HasTrait(dna.TraitStreamable) {
		t.Errorf("expected streamable trait")
	}

	// Verify field rules structural shape
	codeRule, ok := meta.Fields["code"]
	if !ok {
		t.Fatal("expected 'code' field spec")
	}
	if !codeRule.Required || codeRule.MinLength != 3 || codeRule.MaxLength != 20 || codeRule.Pattern != "^[A-Z]{3}-\\d{2}$" {
		t.Errorf("code rule shape mismatch: %+v", codeRule)
	}

	// Verify vetting
	valid := map[string]any{
		"code": "ACC-01",
		"tier": "P0",
	}
	if err := meta.VetObject(valid); err != nil {
		t.Fatalf("expected valid object to pass MetaSchema vetting: %v", err)
	}

	invalid := map[string]any{
		"code": "bad",
		"tier": "P0",
	}
	if err := meta.VetObject(invalid); err == nil {
		t.Errorf("expected pattern error on invalid code, got nil")
	}
}
