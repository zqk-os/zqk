package objects

import (
	"testing"
)

func TestFieldDiscovery_Extended(t *testing.T) {
	ResetGlobalFieldRegistryForTesting()
	defer ResetGlobalFieldRegistryForTesting()

	reg := GetGlobalFieldRegistry()
	if reg == nil {
		t.Fatal("expected non-nil FieldRegistry")
	}

	stamp := reg.CacheStamp()
	_ = stamp

	if !reg.TryReloadFromFindSpecsDir() {
		t.Log("specs dir not found or could not reload from findSpecsDir")
	}

	// MapAny helper functions
	anyMap := map[interface{}]interface{}{
		"is_active":  true,
		"is_deleted": false,
		"count":      int(42),
		"count64":    int64(99),
		"title":      "Some Title",
	}

	if !getBoolFromMapAny(anyMap, "is_active") {
		t.Error("expected true for is_active")
	}
	if getBoolFromMapAny(anyMap, "is_deleted") {
		t.Error("expected false for is_deleted")
	}
	if getBoolFromMapAny(anyMap, "nonexistent") {
		t.Error("expected false for nonexistent key")
	}

	if n := getIntFromMapAny(anyMap, "count"); n != 42 {
		t.Errorf("getIntFromMapAny count = %d, want 42", n)
	}
	if n := getIntFromMapAny(anyMap, "count64"); n != 99 {
		t.Errorf("getIntFromMapAny count64 = %d, want 99", n)
	}
	if n := getIntFromMapAny(anyMap, "missing"); n != 0 {
		t.Errorf("getIntFromMapAny missing = %d, want 0", n)
	}

	if s := getStringFromMapAny(anyMap, "title"); s != "Some Title" {
		t.Errorf("getStringFromMapAny title = %q, want 'Some Title'", s)
	}
	if s := getStringFromMapAny(anyMap, "missing"); s != "" {
		t.Errorf("getStringFromMapAny missing = %q, want empty", s)
	}

	// extractEnumValues with invalid/valid inputs
	var enumDst []string
	extractEnumValues(nil, &enumDst)
	extractEnumValues("not_a_slice", &enumDst)
	extractEnumValues([]any{"val1", 123, "val2"}, &enumDst)
	if len(enumDst) != 2 || enumDst[0] != "val1" || enumDst[1] != "val2" {
		t.Errorf("extractEnumValues failed: %v", enumDst)
	}
	extractEnumValues([]any{"a"}, nil) // nil dst should not panic

	// extractFieldInfo with map[interface{}]interface{} validation and checklist
	fieldDefAny := map[string]any{
		FieldKeyType:    "enum",
		"semantic_type": "status_enum",
		"traits":        []any{"readable", "writable"},
		"validation": map[interface{}]interface{}{
			"required": true,
			"enum":     []any{"opt1", "opt2"},
		},
		"checklist": map[interface{}]interface{}{
			"lifecycle":    "mutable",
			"storage_role": "primary",
			"purpose":      "field description",
			"criticality":  "composition",
		},
	}
	info := reg.extractFieldInfo("my_field", fieldDefAny, "backlog_item", false)
	if !info.Required || info.Lifecycle != "mutable" || info.StorageRole != "primary" || info.Description != "field description" {
		t.Errorf("extractFieldInfo with interface map mismatch: %+v", info)
	}
	if len(info.EnumValues) != 2 || info.EnumValues[0] != "opt1" {
		t.Errorf("EnumValues mismatch: %v", info.EnumValues)
	}

	// extractFieldInfo with origin_lifecycle and minCount in validation
	fieldDefMinCount := map[string]any{
		FieldKeyType: "list",
		"validation": map[interface{}]interface{}{
			"minCount": 1,
		},
		"checklist": map[interface{}]interface{}{
			"origin_lifecycle": "immutable",
		},
	}
	infoMin := reg.extractFieldInfo("list_field", fieldDefMinCount, "goal", false)
	if !infoMin.Required || infoMin.Lifecycle != "immutable" {
		t.Errorf("infoMin expected required and immutable: %+v", infoMin)
	}
}
