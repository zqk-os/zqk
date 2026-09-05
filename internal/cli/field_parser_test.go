package cli

import (
	"reflect"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestParseFieldValue(t *testing.T) {
	t.Parallel()
	logger := logging.NewEventLogger(pkgctx.NewSystemContext())
	fp := NewFieldParser(logger)

	tests := []struct {
		name  string
		value string
		want  any
	}{
		{"empty", "", ""},
		{"plain string", "hello", "hello"},
		{"string with spaces", "hello world", "hello world"},
		{"number", "42", float64(42)},
		{"float", "3.14", float64(3.14)},
		{"bool true", "true", true},
		{"bool false", "false", false},
		{"json list of strings", `["a","b","c"]`, []any{"a", "b", "c"}},
		{"json array numbers", "[1, 2, 3]", []any{float64(1), float64(2), float64(3)}},
		{"json array", `["x","y"]`, []any{"x", "y"}},
		{"json object", `{"k":"v"}`, map[string]any{"k": "v"}},
		{"json nested", `{"a":1,"b":[2,3]}`, map[string]any{"a": float64(1), "b": []any{float64(2), float64(3)}}},
		{"backward compat non-json string", "no=equals", "no=equals"},
		{"backward compat single word", "active", "active"},
		{"prose with colon not a map", "Session closed: done", "Session closed: done"},
		{"json string with colon", `"Session closed: done"`, "Session closed: done"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fp.ParseFieldValue(tt.value)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFieldValue(%q) = %v (%T), want %v (%T)", tt.value, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestParseFieldFlags_complexValues(t *testing.T) {
	t.Parallel()
	logger := logging.NewEventLogger(pkgctx.NewSystemContext())
	fp := NewFieldParser(logger)

	got, err := fp.ParseFieldFlags([]string{
		"title=Simple",
		`meta.tags=["a","b"]`,
		"count=2",
	})
	if err != nil {
		t.Fatalf("ParseFieldFlags: %v", err)
	}
	if g, ok := got[objects.FieldKeyTitle].(string); !ok || g != "Simple" {
		t.Errorf("title = %v, want Simple", got[objects.FieldKeyTitle])
	}
	if g, ok := got["meta.tags"]; !ok {
		t.Errorf("missing meta.tags")
	} else if sl, ok := g.([]any); !ok || len(sl) != 2 {
		t.Errorf("meta.tags = %v, want [a b]", g)
	}
	if g, ok := got["count"].(float64); !ok || g != 2 {
		t.Errorf("count = %v, want float64(2)", got["count"])
	}
}

func TestParseFieldFlags_goalRefsBareStringBecomesList(t *testing.T) {
	t.Parallel()
	logger := logging.NewEventLogger(pkgctx.NewSystemContext())
	fp := NewFieldParser(logger)
	got, err := fp.ParseFieldFlags([]string{"goal_refs=GOAL-x"})
	if err != nil {
		t.Fatalf("ParseFieldFlags: %v", err)
	}
	want := []string{"GOAL-x"}
	if !reflect.DeepEqual(got[objects.FieldKeyGoalRefs], want) {
		t.Fatalf("goal_refs=%#v want %#v", got[objects.FieldKeyGoalRefs], want)
	}
}

func TestParseFieldFlagsWithAppend_goalRefsAppendsList(t *testing.T) {
	t.Parallel()
	logger := logging.NewEventLogger(pkgctx.NewSystemContext())
	fp := NewFieldParser(logger)
	got, err := fp.ParseFieldFlagsWithAppend(
		[]string{"goal_refs+=GOAL-2"},
		map[string]any{objects.FieldKeyGoalRefs: []string{"GOAL-1"}},
	)
	if err != nil {
		t.Fatalf("ParseFieldFlagsWithAppend: %v", err)
	}
	want := []string{"GOAL-1", "GOAL-2"}
	if !reflect.DeepEqual(got[objects.FieldKeyGoalRefs], want) {
		t.Fatalf("goal_refs=%#v want %#v", got[objects.FieldKeyGoalRefs], want)
	}
}
