package objects

import (
	"reflect"
	"testing"
)

func TestCoerceToStringList_preservesObjectStages(t *testing.T) {
	t.Parallel()
	in := []any{map[string]any{FieldKeyID: "bind_seats", "executor": "bind_seats"}}
	got := CoerceToStringList(in)
	gotList, ok := got.([]any)
	if !ok {
		t.Fatalf("got %T %#v, want []any of maps", got, got)
	}
	if len(gotList) != 1 {
		t.Fatalf("len=%d", len(gotList))
	}
	m, ok := gotList[0].(map[string]any)
	if !ok || m["executor"] != "bind_seats" {
		t.Fatalf("stage=%#v", gotList[0])
	}
}

func TestCoerceToStringList_stringBecomesList(t *testing.T) {
	t.Parallel()
	got := CoerceToStringList("GOAL-1")
	want := []string{"GOAL-1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestCoerceToStringList_newlineConcatBecomesList(t *testing.T) {
	t.Parallel()
	got := CoerceToStringList("GOAL-1\n\nGOAL-2")
	want := []string{"GOAL-1", "GOAL-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestCoerceMutationFields_goalRefsString(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyKind:     KindCriteria,
		FieldKeyGoalRefs: "GOAL-x",
	}
	CoerceMutationFields(KindCriteria, obj)
	got, ok := obj[FieldKeyGoalRefs].([]string)
	if !ok || !reflect.DeepEqual(got, []string{"GOAL-x"}) {
		t.Fatalf("goal_refs=%#v", obj[FieldKeyGoalRefs])
	}
}

func TestCoerceMutationFields_criteriaTypeAliasesCategory(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyKind: KindCriteria,
		FieldKeyType: "acceptance",
	}
	CoerceMutationFields(KindCriteria, obj)
	if got, _ := obj[FieldKeyCategory].(string); got != "acceptance" {
		t.Fatalf("category=%q", got)
	}
	if _, ok := obj[FieldKeyType]; ok {
		t.Fatal("type alias should be dropped after remap")
	}
}

func TestCoerceMutationFields_criteriaTypeKeyAliasesCategory(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		FieldKeyKind:         KindCriteria,
		FieldKeyCriteriaType: "performance",
	}
	CoerceMutationFields(KindCriteria, obj)
	if got, _ := obj[FieldKeyCategory].(string); got != "performance" {
		t.Fatalf("category=%q", got)
	}
	if _, ok := obj[FieldKeyCriteriaType]; ok {
		t.Fatal("criteria_type alias should be dropped after remap")
	}
}

func TestRequireCriteriaCategory(t *testing.T) {
	t.Parallel()
	if err := RequireCriteriaCategory(KindBacklogItem, map[string]any{}); err != nil {
		t.Fatalf("non-criteria: %v", err)
	}
	if err := RequireCriteriaCategory(KindCriteria, map[string]any{FieldKeyCategory: "acceptance"}); err != nil {
		t.Fatalf("categorized: %v", err)
	}
	if err := RequireCriteriaCategory(KindCriteria, map[string]any{}); err == nil {
		t.Fatal("expected empty category to fail")
	}
}

func TestAppendRefList_appendsOntoExistingSlice(t *testing.T) {
	t.Parallel()
	got := AppendRefList([]string{"GOAL-1"}, "GOAL-2")
	want := []string{"GOAL-1", "GOAL-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestFieldLooksLikeRefList(t *testing.T) {
	t.Parallel()
	if !FieldLooksLikeRefList(FieldKeyGoalRefs, nil) {
		t.Fatal("goal_refs should look like a ref list")
	}
	if FieldLooksLikeRefList(FieldKeyTitle, "hello") {
		t.Fatal("title should not look like a ref list")
	}
	if !FieldLooksLikeRefList("tags", []any{"a"}) {
		t.Fatal("existing slice should look like a list")
	}
}

func TestCoercePersonaTitleAndName(t *testing.T) {
	t.Parallel()

	// 1. Persona with title only keeps title and name is absent
	obj1 := map[string]any{
		FieldKeyKind:  KindPersona,
		FieldKeyTitle: "Platform Engineer",
	}
	CoerceMutationFields(KindPersona, obj1)
	if obj1["name"] != nil {
		t.Errorf("expected name to be absent, got %v", obj1["name"])
	}

	// 2. Persona with default/placeholder "required" name gets name pruned
	obj2 := map[string]any{
		FieldKeyKind:  KindPersona,
		FieldKeyTitle: "Security Auditor",
		"name":        "required",
	}
	CoerceMutationFields(KindPersona, obj2)
	if obj2["name"] != nil {
		t.Errorf("expected name to be pruned, got %v", obj2["name"])
	}

	// 3. Persona with legacy name only gets title populated and name pruned
	obj3 := map[string]any{
		FieldKeyKind: KindPersona,
		"name":       "Architect",
	}
	CoerceMutationFields(KindPersona, obj3)
	if obj3[FieldKeyTitle] != "Architect" {
		t.Errorf("expected title to be populated from legacy name, got %v", obj3[FieldKeyTitle])
	}
	if obj3["name"] != nil {
		t.Errorf("expected name to be pruned, got %v", obj3["name"])
	}
}

// TestCoerceMutationFields_commitRefsScoping verifies BLI-MESH-MCP-SPEC-001 and BLI-MESH-SWARM-ENVELOPE-001:
// commit_refs is only coerced to commit_hashes for kinds supporting commit_hashes (backlog_item),
// and is cleanly pruned from criteria without polluting the criteria schema with unknown fields.
func TestCoerceMutationFields_commitRefsScoping(t *testing.T) {
	t.Parallel()

	// Criteria with commit_refs: commit_refs pruned, commit_hashes NOT created
	crit := map[string]any{
		FieldKeyKind:  KindCriteria,
		FieldKeyID:    "CRIT-TEST-001",
		"commit_refs": []any{"0feed3bcf6"},
	}
	CoerceMutationFields(KindCriteria, crit)
	if _, hasRefs := crit["commit_refs"]; hasRefs {
		t.Error("expected commit_refs to be deleted from criteria")
	}
	if _, hasHashes := crit[FieldKeyCommitHashes]; hasHashes {
		t.Error("expected commit_hashes NOT to be injected into criteria")
	}

	// BacklogItem with commit_refs: converted to commit_hashes
	bli := map[string]any{
		FieldKeyKind:  KindBacklogItem,
		FieldKeyID:    "BLI-TEST-001",
		"commit_refs": []any{"0feed3bcf6"},
	}
	CoerceMutationFields(KindBacklogItem, bli)
	if _, hasRefs := bli["commit_refs"]; hasRefs {
		t.Error("expected commit_refs to be deleted from backlog_item")
	}
	hashes, ok := asStringList(bli[FieldKeyCommitHashes])
	if !ok || len(hashes) != 1 || hashes[0] != "0feed3bcf6" {
		t.Errorf("expected commit_hashes to be populated on backlog_item, got %#v", bli[FieldKeyCommitHashes])
	}
}
