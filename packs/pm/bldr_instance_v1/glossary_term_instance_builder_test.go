package bldr_instance_v1_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/packs/pm/bldr_instance_v1"
)

func TestGlossaryTermInstanceBuilder(t *testing.T) {
	t.Parallel()

	builder := bldr_instance_v1.NewGlossaryTermInstanceBuilder(objects.DefaultSchemaVersion)
	if builder == nil {
		t.Fatal("expected non-nil GlossaryTermInstanceBuilder")
	}
	if builder.GetKind() != "glossary_term" {
		t.Errorf("expected kind 'glossary_term', got %q", builder.GetKind())
	}

	builder.ID("GLO-001").
		Title("Knowledge Kernel").
		Definition("The centralized immutable graph for process coordination.").
		Version(1)

	inst, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if inst == nil {
		t.Fatal("expected non-nil instance map")
	}

	if inst[objects.FieldKeyID] != "GLO-001" {
		t.Errorf("expected id 'GLO-001', got %v", inst[objects.FieldKeyID])
	}
	if inst[objects.FieldKeyKind] != "glossary_term" {
		t.Errorf("expected kind 'glossary_term', got %v", inst[objects.FieldKeyKind])
	}
}
