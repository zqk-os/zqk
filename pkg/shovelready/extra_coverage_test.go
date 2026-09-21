package shovelready

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestIsReady(t *testing.T) {
	t.Parallel()

	bli := readyBLI()
	if !IsReady(bli) {
		t.Fatal("expected readyBLI to be ready")
	}

	delete(bli, objects.FieldKeyCriteriaRefs)
	if IsReady(bli) {
		t.Fatal("expected bli without criteria to not be ready")
	}
}

func TestHasEstimatedScope_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		bli  map[string]any
		want bool
	}{
		{
			bli:  map[string]any{"priority": "p0-urgent"},
			want: true,
		},
		{
			bli:  map[string]any{"priority_tier": "P1-high"},
			want: true,
		},
		{
			bli:  map[string]any{"scope": "p2-mid"},
			want: true,
		},
		{
			bli:  map[string]any{"size": "p3-low"},
			want: true,
		},
		{
			bli:  map[string]any{"size": "critical"},
			want: true,
		},
		{
			bli:  map[string]any{"size": "invalid_scope_value"},
			want: false,
		},
		{
			bli:  map[string]any{"size": 123},
			want: false,
		},
	}

	for _, tt := range tests {
		got := hasEstimatedScope(tt.bli)
		if got != tt.want {
			t.Fatalf("hasEstimatedScope(%v) = %v, want %v", tt.bli, got, tt.want)
		}
	}
}

func TestHasLaneAssignment_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		bli  map[string]any
		want bool
	}{
		{
			bli:  map[string]any{objects.FieldKeyPersonaRef: "PER-1"},
			want: true,
		},
		{
			bli:  map[string]any{objects.FieldKeyAssigneePersonaRef: "PER-2"},
			want: true,
		},
		{
			bli:  map[string]any{objects.FieldKeyStakeholderType: "owner"},
			want: true,
		},
		{
			bli:  map[string]any{objects.FieldKeyStakeholders: "developer"},
			want: true,
		},
		{
			bli:  map[string]any{objects.FieldKeyStakeholders: []string{"dev1"}},
			want: true,
		},
		{
			bli:  map[string]any{objects.FieldKeyStakeholders: []any{"dev2"}},
			want: true,
		},
		{
			bli:  map[string]any{objects.FieldKeyStakeholders: 123},
			want: false,
		},
		{
			bli:  map[string]any{},
			want: false,
		},
	}

	for _, tt := range tests {
		got := hasLaneAssignment(tt.bli)
		if got != tt.want {
			t.Fatalf("hasLaneAssignment(%v) = %v, want %v", tt.bli, got, tt.want)
		}
	}
}

func TestHasNonEmptyRefList_EdgeCases(t *testing.T) {
	t.Parallel()

	obj := map[string]any{
		"nil_key":    nil,
		"str_empty":  "   ",
		"str_val":    "REF-1",
		"list_any":   []any{"", "REF-2"},
		"list_empty": []any{"   ", ""},
		"list_str":   []string{"", "REF-3"},
		"invalid":    123,
	}

	if hasNonEmptyRefList(obj, "nil_key") {
		t.Fatal("expected nil_key to be false")
	}
	if hasNonEmptyRefList(obj, "str_empty") {
		t.Fatal("expected str_empty to be false")
	}
	if !hasNonEmptyRefList(obj, "str_val") {
		t.Fatal("expected str_val to be true")
	}
	if !hasNonEmptyRefList(obj, "list_any") {
		t.Fatal("expected list_any to be true")
	}
	if hasNonEmptyRefList(obj, "list_empty") {
		t.Fatal("expected list_empty to be false")
	}
	if !hasNonEmptyRefList(obj, "list_str") {
		t.Fatal("expected list_str to be true")
	}
	if hasNonEmptyRefList(obj, "invalid") {
		t.Fatal("expected invalid to be false")
	}
}
