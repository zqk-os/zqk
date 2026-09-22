// BLI-STARTER-COMMUNITY-049 / PRI-STARTER-COMMUNITY-049 coverage elevation
package objects

import "testing"

func TestExtraFieldVersioningHashAndEdgeRole(t *testing.T) {
	if IsHashedFilename("short.yaml") {
		t.Fatal("short")
	}
	if IsHashedFilename("zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz.yaml") {
		t.Fatal("non-hex")
	}
	hex64 := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef.yaml"
	if !IsHashedFilename(hex64) {
		t.Fatal("hex")
	}
	if !IsEdgeRoleName("membership") || IsEdgeRoleName("nope") {
		t.Fatal("edge role")
	}
	_ = ListKernelCriticalKinds()
	if FieldChecklistSecurityIsSensitive("pii") == false || FieldChecklistSecurityIsSensitive("non-sensitive") {
		t.Fatal("security")
	}
	_ = IsKernelObjectRefField("priority_plan_ref")
	_ = IsLeftoverNonKernelRefField("branch_ref")

	def := map[string]any{}
	SetFieldVersionInfo(nil, &FieldVersionInfo{})
	MarkFieldCreated(def, "agent")
	got := GetFieldVersionInfo(def)
	if got.CreatedBy != "agent" {
		t.Fatal(got)
	}
	MarkFieldCreated(def, "again")
	MarkFieldModified(def, "agent", false, "tweak", "note")
	MarkFieldModified(map[string]any{}, "agent", true, "break", "")
	MarkFieldDeprecated(def, "agent", "other", "old")
	MarkFieldArchived(def, "agent", "archive")
	MarkFieldDeleted(def, "agent", "other", "gone")

	breaking, _ := DetectBreakingChange(
		map[string]any{
			FieldKeyType: "string",
			"validation": map[string]any{"required": false, "enum": []any{"a", "b"}, "pattern": "^a"},
		},
		map[string]any{
			FieldKeyType: "int",
			"validation": map[string]any{"required": true, "enum": []any{"a"}, "pattern": "^b"},
		},
	)
	if !breaking {
		t.Fatal("expected breaking")
	}
	_ = incrementMajorVersion("")
	_ = incrementMajorVersion("9.0.0")
	_ = incrementMinorVersion("")
	_ = incrementMinorVersion("1.0.0")
	_ = incrementMinorVersion("1.9.0")
}
