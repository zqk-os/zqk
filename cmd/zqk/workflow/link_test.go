package workflow

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestExtractTargetCriteria(t *testing.T) {
	// Test direct criteria
	critObj := map[string]any{
		objects.FieldKeyKind: objects.KindCriteria,
	}
	id, err := extractTargetCriteria("CRIT-1", critObj)
	if err != nil || id != "CRIT-1" {
		t.Fatalf("expected CRIT-1, got %s, err: %v", id, err)
	}

	// Test requirement with criteria refs
	reqObj := map[string]any{
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-2"},
	}
	id, err = extractTargetCriteria("REQ-1", reqObj)
	if err != nil || id != "CRIT-2" {
		t.Fatalf("expected CRIT-2, got %s, err: %v", id, err)
	}

	// Test requirement without criteria refs
	reqEmptyObj := map[string]any{
		objects.FieldKeyKind: objects.KindRequirement,
	}
	_, err = extractTargetCriteria("REQ-2", reqEmptyObj)
	if err == nil {
		t.Fatal("expected error for requirement without criteria refs")
	}
}
