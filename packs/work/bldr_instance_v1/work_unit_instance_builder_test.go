package bldr_instance_v1_test

import (
	"testing"

	"github.com/zqk-os/zqk/packs/work/bldr_instance_v1"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestWorkUnitInstanceBuilder(t *testing.T) {
	t.Parallel()

	builder := bldr_instance_v1.NewWorkUnitInstanceBuilder(objects.DefaultSchemaVersion)
	if builder == nil {
		t.Fatal("expected non-nil WorkUnitInstanceBuilder")
	}
	if builder.GetKind() != "work_unit" {
		t.Errorf("expected kind 'work_unit', got %q", builder.GetKind())
	}

	builder.ID("WKU-001").
		Title("Test Work Unit").
		Urn("urn:zqk:wku:001").
		Version(1)

	inst, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if inst == nil {
		t.Fatal("expected non-nil instance map")
	}

	if inst[objects.FieldKeyID] != "WKU-001" {
		t.Errorf("expected id 'WKU-001', got %v", inst[objects.FieldKeyID])
	}
	if inst[objects.FieldKeyKind] != "work_unit" {
		t.Errorf("expected kind 'work_unit', got %v", inst[objects.FieldKeyKind])
	}
}
