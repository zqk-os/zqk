package storage

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestReferenceStringMatchesObjectID(t *testing.T) {
	if !ReferenceStringMatchesObjectID("ITEM-1", "ITEM-1") {
		t.Fatal("bare id")
	}
	if !ReferenceStringMatchesObjectID("backlog_item:ITEM-1", "ITEM-1") {
		t.Fatal("kind prefix")
	}
	if ReferenceStringMatchesObjectID("ITEM-2", "ITEM-1") {
		t.Fatal("mismatch")
	}
}

func TestStripReferenceFieldsRemovingID(t *testing.T) {
	obj := map[string]any{
		objects.FieldKeyID:           "REQ-1",
		objects.FieldKeyKind:         "requirement",
		objects.FieldKeyCriteriaRefs: []any{"CRIT-1", "CRIT-2"},
		"strategic_plan_ref":         "STRAT-1",
		objects.FieldKeyCommitRefs:   []any{"abc123"},
	}
	updates := StripReferenceFieldsRemovingID(obj, "CRIT-1")
	if updates == nil {
		t.Fatal("expected updates")
	}
	refs, _ := updates[objects.FieldKeyCriteriaRefs].([]any)
	if len(refs) != 1 || refs[0] != "CRIT-2" {
		t.Fatalf("criteria_refs: %#v", updates[objects.FieldKeyCriteriaRefs])
	}
	if _, ok := updates["strategic_plan_ref"]; ok {
		t.Fatalf("unexpected strategic_plan change: %#v", updates["strategic_plan_ref"])
	}
	if _, ok := updates[objects.FieldKeyCommitRefs]; ok {
		t.Fatal("commit_refs must not be stripped")
	}
}
