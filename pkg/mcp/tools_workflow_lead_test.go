package mcp

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestExtractWhatsNextLeadPlan(t *testing.T) {
	t.Parallel()
	plan := map[string]any{objects.FieldKeyID: "PRI-LEAD"}
	got, found, err := extractWhatsNextLeadPlan(map[string]any{
		workflowKeyPriorityPlan: plan,
		"active_plans": []any{
			map[string]any{objects.FieldKeyID: "PRI-GROOM"},
		},
	})
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	m, ok := got.(map[string]any)
	if !ok || m[objects.FieldKeyID] != "PRI-LEAD" {
		t.Fatalf("got %#v", got)
	}

	nested, found, err := extractWhatsNextLeadPlan(map[string]any{
		"data": map[string]any{workflowKeyPriorityPlan: plan},
	})
	if err != nil || !found {
		t.Fatalf("nested found=%v err=%v", found, err)
	}
	if nested.(map[string]any)[objects.FieldKeyID] != "PRI-LEAD" {
		t.Fatalf("nested %#v", nested)
	}

	_, found, err = extractWhatsNextLeadPlan(map[string]any{workflowKeyObjects: []any{plan}})
	if err != nil {
		t.Fatalf("objects-only: %v", err)
	}
	if found {
		t.Fatal("must not treat objects[0] as the Gantt lead")
	}
}
