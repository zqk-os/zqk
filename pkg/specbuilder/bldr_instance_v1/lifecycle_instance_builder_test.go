package bldr_instance_v1_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
)

func TestLifecycleInstanceBuilder(t *testing.T) {
	t.Parallel()

	builder := bldr_instance_v1.NewLifecycleInstanceBuilder(objects.DefaultSchemaVersion)
	if builder == nil {
		t.Fatal("expected non-nil LifecycleInstanceBuilder")
	}
	if builder.GetKind() != "lifecycle" {
		t.Errorf("expected kind 'lifecycle', got %q", builder.GetKind())
	}

	builder.ID("LFC-001").
		Title("Standard Task Lifecycle").
		Urn("urn:zqk:lfc:001").
		Version(1)

	inst, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if inst == nil {
		t.Fatal("expected non-nil instance map")
	}

	if inst[objects.FieldKeyID] != "LFC-001" {
		t.Errorf("expected id 'LFC-001', got %v", inst[objects.FieldKeyID])
	}
	if inst[objects.FieldKeyKind] != "lifecycle" {
		t.Errorf("expected kind 'lifecycle', got %v", inst[objects.FieldKeyKind])
	}
}
