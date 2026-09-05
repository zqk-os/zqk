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
