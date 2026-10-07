package cli

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
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

func TestParseFieldFlagsWithAppend_StringAndNonString(t *testing.T) {
	t.Parallel()
	logger := logging.NewEventLogger(pkgctx.NewSystemContext())
	fp := NewFieldParser(logger)

	// Append to existing string
	got, err := fp.ParseFieldFlagsWithAppend(
		[]string{"description+=Line two"},
		map[string]any{"description": "Line one"},
	)
	if err != nil {
		t.Fatalf("ParseFieldFlagsWithAppend failed: %v", err)
	}
	if got["description"] != "Line one\n\nLine two" {
		t.Errorf("expected multiline description, got: %q", got["description"])
	}

	// Append to empty/nil currentObj
	got, err = fp.ParseFieldFlagsWithAppend(
		[]string{"description+=Solo line"},
		nil,
	)
	if err != nil {
		t.Fatalf("ParseFieldFlagsWithAppend nil failed: %v", err)
	}
	if got["description"] != "Solo line" {
		t.Errorf("expected 'Solo line', got: %q", got["description"])
	}

	// Append to non-string existing value
	got, err = fp.ParseFieldFlagsWithAppend(
		[]string{"notes+=Additional"},
		map[string]any{"notes": 12345},
	)
	if err != nil {
		t.Fatalf("ParseFieldFlagsWithAppend non-string failed: %v", err)
	}
	if got["notes"] != "12345\n\nAdditional" {
		t.Errorf("expected converted multiline string, got: %q", got["notes"])
	}
}

// TestDataLoader_FieldFlags_ConsistentWithUpdate verifies BLI-TDE-VALIDATION-CREATE-FIELDS-001
// and TDE-1787604878092294000-debe585a: DataLoader must parse array/json fields via FieldParser
// so create and update handle types identically without storing unrepairable stringified literals.
func TestDataLoader_FieldFlags_ConsistentWithUpdate(t *testing.T) {
	t.Parallel()
	logger := logging.NewEventLogger(pkgctx.NewSystemContext())
	dl := NewDataLoader(logger)

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringP("file", "f", "", "")
	cmd.Flags().StringP("data", "d", "", "")
	cmd.Flags().StringArray("field", nil, "")

	if err := cmd.Flags().Set("field", `tags=["kernel","validation"]`); err != nil {
		t.Fatalf("Set field flag: %v", err)
	}
	data, _, err := dl.LoadData(cmd, nil)
	if err != nil {
		t.Fatalf("LoadData: %v", err)
	}
	tags, ok := data["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Fatalf("expected tags to be []any of len 2, got %T: %v", data["tags"], data["tags"])
	}
}

func TestParseFieldFlag_AllSeparatorsAndErrors(t *testing.T) {
	t.Parallel()
	logger := logging.NewEventLogger(pkgctx.NewSystemContext())
	fp := NewFieldParser(logger)

	// Equal
	k, v, isApp, err := fp.ParseFieldFlag("title=Hello")
	if err != nil || k != "title" || v != "Hello" || isApp {
		t.Errorf("expected title=Hello, got %s=%s, isApp=%v, err=%v", k, v, isApp, err)
	}

	// Colon
	k, v, isApp, err = fp.ParseFieldFlag("kind:goal")
	if err != nil || k != "kind" || v != "goal" || isApp {
		t.Errorf("expected kind:goal, got %s=%s, isApp=%v, err=%v", k, v, isApp, err)
	}

	// Append
	k, v, isApp, err = fp.ParseFieldFlag("desc+=extra line")
	if err != nil || k != "desc" || v != "extra line" || !isApp {
		t.Errorf("expected desc+=extra line, got %s=%s, isApp=%v, err=%v", k, v, isApp, err)
	}

	// Invalid format (no separator)
	_, _, _, err = fp.ParseFieldFlag("invalid_string_no_separator")
	if err == nil {
		t.Errorf("expected error for invalid field format")
	}
}
