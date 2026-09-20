package objects

import (
	"testing"
)

// TestCoercePersona_prunesName documents the prune-and-harmonize contract:
// title is the single canonical identity field (inherited from base_object).
// The legacy persona.name property is migrated into title when title is empty,
// then name is always deleted from the persisted object.
func TestCoercePersona_prunesName(t *testing.T) {
	t.Parallel()

	// 1. Title only → name must NOT appear (pruned / absent).
	obj1 := map[string]any{
		FieldKeyKind:  KindPersona,
		FieldKeyTitle: "Platform Engineer",
	}
	CoerceMutationFields(KindPersona, obj1)
	if got := obj1["name"]; got != nil {
		t.Errorf("title-only: expected name pruned (absent), got %v", got)
	}
	if got := obj1[FieldKeyTitle]; got != "Platform Engineer" {
		t.Errorf("title-only: title changed to %v", got)
	}

	// 2. Legacy name only → title populated from name, name deleted.
	obj2 := map[string]any{
		FieldKeyKind: KindPersona,
		"name":       "Architect",
	}
	CoerceMutationFields(KindPersona, obj2)
	if got := obj2[FieldKeyTitle]; got != "Architect" {
		t.Errorf("name-only: expected title=%q, got %v", "Architect", got)
	}
	if _, present := obj2["name"]; present {
		t.Errorf("name-only: name should be pruned after harmonize, got %v", obj2["name"])
	}

	// 3. Both name and title present → title wins, name pruned.
	obj3 := map[string]any{
		FieldKeyKind:  KindPersona,
		FieldKeyTitle: "Security Auditor",
		"name":        "LegacyName",
	}
	CoerceMutationFields(KindPersona, obj3)
	if got := obj3[FieldKeyTitle]; got != "Security Auditor" {
		t.Errorf("both: title should stay %q, got %v", "Security Auditor", got)
	}
	if _, present := obj3["name"]; present {
		t.Errorf("both: name should be pruned, got %v", obj3["name"])
	}

	// 4. Placeholder name ("required") → treated as empty, pruned.
	obj4 := map[string]any{
		FieldKeyKind:  KindPersona,
		FieldKeyTitle: "Ops Lead",
		"name":        "required",
	}
	CoerceMutationFields(KindPersona, obj4)
	if got := obj4[FieldKeyTitle]; got != "Ops Lead" {
		t.Errorf("placeholder-name: title should stay %q, got %v", "Ops Lead", got)
	}
	if _, present := obj4["name"]; present {
		t.Errorf("placeholder-name: name should be pruned, got %v", obj4["name"])
	}

	// 5. Placeholder name ("required") with empty title → title stays empty,
	//    name pruned (no spurious backfill from a placeholder).
	obj5 := map[string]any{
		FieldKeyKind: KindPersona,
		"name":       "required",
	}
	CoerceMutationFields(KindPersona, obj5)
	if _, present := obj5["name"]; present {
		t.Errorf("placeholder-name+empty-title: name should be pruned, got %v", obj5["name"])
	}
}
