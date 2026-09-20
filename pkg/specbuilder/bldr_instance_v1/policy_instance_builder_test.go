package bldr_instance_v1_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
)

func TestPolicyInstanceBuilder(t *testing.T) {
	t.Parallel()

	builder := bldr_instance_v1.NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)
	if builder == nil {
		t.Fatal("expected non-nil PolicyInstanceBuilder")
	}
	if builder.GetKind() != "policy" {
		t.Errorf("expected kind 'policy', got %q", builder.GetKind())
	}

	builder.ID("POL-TEST-001").
		Title("Test Policy").
		Urn("urn:zqk:pol:test-001").
		Version("1.0.0")

	inst, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if inst == nil {
		t.Fatal("expected non-nil instance map")
	}

	if inst[objects.FieldKeyID] != "POL-TEST-001" {
		t.Errorf("expected id 'POL-TEST-001', got %v", inst[objects.FieldKeyID])
	}
	if inst[objects.FieldKeyTitle] != "Test Policy" {
		t.Errorf("expected title 'Test Policy', got %v", inst[objects.FieldKeyTitle])
	}
	if inst[objects.FieldKeyKind] != "policy" {
		t.Errorf("expected kind 'policy', got %v", inst[objects.FieldKeyKind])
	}
}
