package bldr_instance_v1

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestGlossaryTermInstanceBuilder_Build(t *testing.T) {
	t.Parallel()
	builder := NewGlossaryTermInstanceBuilder(objects.DefaultSchemaVersion)

	builder.SetID("GLS-001")
	builder.SetField("title", "New system object")
	builder.SetDefinition("An object kind created or managed by the system (e.g. metrics, audit events), not by direct user creation in docs/architecture/.")
	builder.SetContextScope("operational")
	builder.SetCategory("concept")
	builder.SetStatus("active")
	instance, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if instance["id"] != "GLS-001" {
		t.Errorf("id = %v, want GLS-001", instance["id"])
	}
	if instance["kind"] != "glossary_term" {
		t.Errorf("kind = %v, want glossary_term", instance["kind"])
	}
	if instance["definition"] == nil || instance["definition"] == emptyValue {
		t.Error("definition required and must be set")
	}
	if instance["status"] != "active" {
		t.Errorf("status = %v, want active", instance["status"])
	}
}
