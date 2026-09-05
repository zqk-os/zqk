package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestFormatKindFieldsSegregatedTable(t *testing.T) {
	t.Parallel()
	kf := &objects.KindFields{
		Kind: "test_kind",
		CommonFields: []objects.FieldInfo{
			{Name: "id", Type: "string", Required: true},
			{Name: "kind", Type: "string"},
		},
		SpecializedFields: []objects.FieldInfo{
			{Name: "title", Type: "string", Description: "A title"},
		},
	}
	out := FormatKindFieldsSegregatedTable(kf)
	s := string(out)
	if !strings.Contains(s, "Fields for 'test_kind':") {
		t.Errorf("expected title, got: %s", s)
	}
	if !strings.Contains(s, CommonFieldsHeader) {
		t.Errorf("expected common header, got: %s", s)
	}
	if !strings.Contains(s, SpecializedFieldsHeader) {
		t.Errorf("expected specialized header, got: %s", s)
	}
	if !strings.Contains(s, "id") || !strings.Contains(s, "kind") || !strings.Contains(s, "title") {
		t.Errorf("expected field names, got: %s", s)
	}
}

func TestFormatKindFieldsSegregatedJSON(t *testing.T) {
	t.Parallel()
	kf := &objects.KindFields{
		Kind: "backlog_item",
		CommonFields: []objects.FieldInfo{
			{Name: "id", Type: "string"},
		},
		SpecializedFields: []objects.FieldInfo{
			{Name: "status", Type: "string"},
		},
	}
	out, err := FormatKindFieldsSegregatedJSON(kf)
	if err != nil {
		t.Fatalf("FormatKindFieldsSegregatedJSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m[objects.FieldKeyKind] != "backlog_item" {
		t.Errorf("kind = %v", m[objects.FieldKeyKind])
	}
	if _, ok := m["common_fields"]; !ok {
		t.Error("missing common_fields")
	}
	if _, ok := m["specialized_fields"]; !ok {
		t.Error("missing specialized_fields")
	}
}

func TestKindFieldsSegregatedData(t *testing.T) {
	t.Parallel()
	kf := &objects.KindFields{
		Kind:              "x",
		CommonFields:      []objects.FieldInfo{{Name: "a"}},
		SpecializedFields: []objects.FieldInfo{{Name: "b"}},
	}
	data := KindFieldsSegregatedData(kf)
	if data[objects.FieldKeyKind] != "x" {
		t.Errorf("kind = %v", data[objects.FieldKeyKind])
	}
	common, _ := data["common_fields"].([]map[string]any)
	if len(common) != 1 || common[0][objects.FieldKeyName] != "a" {
		t.Errorf("common_fields = %v", common)
	}
	spec, _ := data["specialized_fields"].([]map[string]any)
	if len(spec) != 1 || spec[0][objects.FieldKeyName] != "b" {
		t.Errorf("specialized_fields = %v", spec)
	}
}
