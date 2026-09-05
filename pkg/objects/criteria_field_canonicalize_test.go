package objects

import "testing"

func TestCanonicalizeCriteriaFieldKeys_TypeAlias(t *testing.T) {
	t.Parallel()
	obj := map[string]any{FieldKeyType: "acceptance"}
	CanonicalizeCriteriaFieldKeys(obj)
	if obj[FieldKeyCategory] != "acceptance" {
		t.Fatalf("category = %#v, want acceptance", obj[FieldKeyCategory])
	}
	if _, ok := obj[FieldKeyType]; ok {
		t.Fatal("type alias must be dropped after remap")
	}
}

func TestCanonicalizeCriteriaFieldKeys_CriteriaTypeAlias(t *testing.T) {
	t.Parallel()
	obj := map[string]any{FieldKeyCriteriaType: "functional"}
	CanonicalizeCriteriaFieldKeys(obj)
	if obj[FieldKeyCategory] != "functional" {
		t.Fatalf("category = %#v, want functional", obj[FieldKeyCategory])
	}
}

func TestCanonicalizeCriteriaFieldKeys_KeepsExistingCategory(t *testing.T) {
	t.Parallel()
	obj := map[string]any{FieldKeyCategory: "security", FieldKeyType: "acceptance"}
	CanonicalizeCriteriaFieldKeys(obj)
	if obj[FieldKeyCategory] != "security" {
		t.Fatalf("category = %#v, want existing security", obj[FieldKeyCategory])
	}
}

func TestCanonicalizeCriteriaFieldKeys_NoOpOnNil(t *testing.T) {
	t.Parallel()
	CanonicalizeCriteriaFieldKeys(nil)
}

func TestRejectCriteriaRequirementRelatedRefs(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyCategory:          "acceptance",
		FieldKeyRelatedObjectRefs: []any{"BLI-1", "REQ-COMMS-SEAT-WORKER-001"},
	}
	if err := RejectCriteriaRequirementRelatedRefs(obj); err == nil {
		t.Fatal("REQ-* on related_object_refs must fail closed")
	}
	ok := map[string]any{
		FieldKeyCategory:          "acceptance",
		FieldKeyRelatedObjectRefs: []any{"BLI-1", "POL-1"},
	}
	if err := RejectCriteriaRequirementRelatedRefs(ok); err != nil {
		t.Fatalf("BLI/POL related refs must pass: %v", err)
	}
	if err := RejectCriteriaRequirementRelatedRefs(nil); err != nil {
		t.Fatalf("nil-safe: %v", err)
	}
}
