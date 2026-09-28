package bldr_instance_v1_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/packs/work/bldr_instance_v1"
)

func TestWorkIntervalInstanceBuilder(t *testing.T) {
	t.Parallel()

	builder := bldr_instance_v1.NewWorkIntervalInstanceBuilder(objects.DefaultSchemaVersion)
	if builder == nil {
		t.Fatal("expected non-nil WorkIntervalInstanceBuilder")
	}
	if builder.GetKind() != "work_interval" {
		t.Errorf("expected kind 'work_interval', got %q", builder.GetKind())
	}

	builder.ID("WKI-001").
		Urn("urn:zqk:wki:001").
		Version(1)

	inst, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if inst == nil {
		t.Fatal("expected non-nil instance map")
	}

	if inst[objects.FieldKeyID] != "WKI-001" {
		t.Errorf("expected id 'WKI-001', got %v", inst[objects.FieldKeyID])
	}
	if inst[objects.FieldKeyKind] != "work_interval" {
		t.Errorf("expected kind 'work_interval', got %v", inst[objects.FieldKeyKind])
	}
}
