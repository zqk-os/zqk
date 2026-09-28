package bldr_instance_v1_test

import (
	"testing"

	"github.com/zqk-os/zqk/packs/work/bldr_instance_v1"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestBacklogItemInstanceBuilder(t *testing.T) {
	t.Parallel()

	builder := bldr_instance_v1.NewBacklogItemInstanceBuilder(objects.DefaultSchemaVersion)
	if builder == nil {
		t.Fatal("expected non-nil BacklogItemInstanceBuilder")
	}
	if builder.GetKind() != "backlog_item" {
		t.Errorf("expected kind 'backlog_item', got %q", builder.GetKind())
	}
	if builder.GetSchemaVersion() != objects.DefaultSchemaVersion {
		t.Errorf("expected schema version %q, got %q", objects.DefaultSchemaVersion, builder.GetSchemaVersion())
	}

	builder.ID("BLI-TEST-001").
		Title("Test Backlog Item").
		Urn("urn:zqk:bli:test-001").
		Version(1).
		WorkstreamRefs([]string{"WS-TEST-001"})

	inst, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if inst == nil {
		t.Fatal("expected non-nil instance map")
	}

	if inst[objects.FieldKeyID] != "BLI-TEST-001" {
		t.Errorf("expected id 'BLI-TEST-001', got %v", inst[objects.FieldKeyID])
	}
	if inst[objects.FieldKeyTitle] != "Test Backlog Item" {
		t.Errorf("expected title 'Test Backlog Item', got %v", inst[objects.FieldKeyTitle])
	}
	if inst[objects.FieldKeyKind] != "backlog_item" {
		t.Errorf("expected kind 'backlog_item', got %v", inst[objects.FieldKeyKind])
	}
}
