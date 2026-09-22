package objects

import (
	"testing"
)

func TestSystemFieldsRegistry_Extended(t *testing.T) {
	reg := GetGlobalSystemFieldsRegistry()
	if reg == nil {
		t.Fatal("expected non-nil SystemFieldsRegistry")
	}

	// isSystemGeneratedFieldDef unit checks
	if !isSystemGeneratedFieldDef(map[string]any{FieldKeyPermissions: "r-x"}) {
		t.Error("permissions r-x should be system generated")
	}
	if !isSystemGeneratedFieldDef(map[string]any{"checklist": map[string]any{FieldKeyAuthority: "Automation agent"}}) {
		t.Error("checklist authority containing automation should be system generated")
	}
	if isSystemGeneratedFieldDef(map[string]any{FieldKeyPermissions: "rw-"}) {
		t.Error("permissions rw- should not be system generated")
	}

	// Test cached operations
	fields, err := reg.GetSystemGeneratedFields()
	if err != nil {
		t.Fatalf("GetSystemGeneratedFields failed: %v", err)
	}
	if fields == nil {
		t.Fatal("expected non-nil fields map")
	}

	isSys, err := reg.IsSystemGeneratedField("created_at")
	if err != nil {
		t.Fatalf("IsSystemGeneratedField failed: %v", err)
	}
	_ = isSys

	if err := reg.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
}
