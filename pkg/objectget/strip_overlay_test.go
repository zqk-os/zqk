package objectget

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestIsReferenceResolverOverlayFieldKey(t *testing.T) {
	cases := map[string]bool{
		"reference_resolver_overlay_applied":      true,
		"reference_resolver_overlay_capped":       true,
		"resolved_goal_refs":                      true,
		objects.FieldKeyResolvedRelatedObjectRefs: true,
		"resolved_priority_plan_ref":              true,
		objects.FieldKeyResolvedAt:                false,
		objects.FieldKeyResolvedBy:                false,
		objects.FieldKeyGoalRefs:                  false,
		objects.FieldKeyTitle:                     false,
	}
	for k, want := range cases {
		if got := IsReferenceResolverOverlayFieldKey(k); got != want {
			t.Fatalf("%s: got %v want %v", k, got, want)
		}
	}
}

func TestStripReferenceResolverOverlayFields(t *testing.T) {
	obj := map[string]any{
		objects.FieldKeyID:                   "REQ-1",
		objects.FieldKeyGoalRefs:             []string{"GOAL-1"},
		"resolved_goal_refs":                 []any{map[string]any{objects.FieldKeyID: "GOAL-1"}},
		"reference_resolver_overlay_applied": true,
		objects.FieldKeyResolvedAt:           "2026-01-01T00:00:00Z",
	}
	n := StripReferenceResolverOverlayFields(obj)
	if n != 2 {
		t.Fatalf("stripped %d, want 2", n)
	}
	if _, ok := obj["resolved_goal_refs"]; ok {
		t.Fatal("resolved_goal_refs should be gone")
	}
	if _, ok := obj["reference_resolver_overlay_applied"]; ok {
		t.Fatal("overlay flag should be gone")
	}
	if obj[objects.FieldKeyResolvedAt] != "2026-01-01T00:00:00Z" {
		t.Fatal("resolved_at must remain (risk_blocker durable field)")
	}
	if obj[objects.FieldKeyGoalRefs] == nil {
		t.Fatal("goal_refs must remain")
	}

	// Unset sentinels must survive so polluted CAS can be scrubbed.
	scrub := map[string]any{
		"resolved_goal_refs":                 storage.FieldUnset,
		"reference_resolver_overlay_applied": storage.FieldUnset,
		objects.FieldKeyTitle:                "keep",
	}
	if n := StripReferenceResolverOverlayFields(scrub); n != 0 {
		t.Fatalf("must not strip FieldUnset overlays, removed %d", n)
	}
	if !storage.IsFieldUnset(scrub["resolved_goal_refs"]) {
		t.Fatal("unset sentinel lost")
	}
}
