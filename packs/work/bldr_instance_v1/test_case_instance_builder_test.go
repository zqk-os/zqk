package bldr_instance_v1_test

import (
	"testing"

	"github.com/zqk-os/zqk/packs/work/bldr_instance_v1"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestTestCaseInstanceBuilder(t *testing.T) {
	t.Parallel()

	builder := bldr_instance_v1.NewTestCaseInstanceBuilder(objects.DefaultSchemaVersion)
	if builder == nil {
		t.Fatal("expected non-nil TestCaseInstanceBuilder")
	}
	if builder.GetKind() != "test_case" {
		t.Errorf("expected kind 'test_case', got %q", builder.GetKind())
	}

	builder.ID("TST-001").
		Title("Test Case 1").
		Urn("urn:zqk:tst:001").
		Version(1).
		CriteriaRefs([]string{"CRIT-001"})

	inst, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if inst == nil {
		t.Fatal("expected non-nil instance map")
	}

	if inst[objects.FieldKeyID] != "TST-001" {
		t.Errorf("expected id 'TST-001', got %v", inst[objects.FieldKeyID])
	}
	if inst[objects.FieldKeyTitle] != "Test Case 1" {
		t.Errorf("expected title 'Test Case 1', got %v", inst[objects.FieldKeyTitle])
	}
	if inst[objects.FieldKeyKind] != "test_case" {
		t.Errorf("expected kind 'test_case', got %v", inst[objects.FieldKeyKind])
	}
}
