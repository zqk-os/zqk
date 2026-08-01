package validation

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestCheckPrecondition_PriorityPlanActiveWithBackrefs(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()
	obj := map[string]any{
		objects.FieldKeyID:              "ITEM-EXAMPLE",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PLAN-EXAMPLE",
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
	}
	opts := &ValidationOptions{
		ObjectStatusLookup: func(id string) (string, error) {
			return objects.ObjectStatusActive, nil
		},
		ObjectLookup: func(id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyID:                id,
				objects.FieldKeyStatus:            objects.ObjectStatusActive,
				objects.FieldKeyRelatedObjectRefs: []any{"ITEM-EXAMPLE"},
				objects.FieldKeyBacklogItemRefs:   "ITEM-EXAMPLE",
			}, nil
		},
	}
	pc := "priority_plan_ref target must be in active status"
	if !gv.checkPrecondition(pc, obj, opts) {
		t.Fatalf("expected precondition to pass when PRI status is active (even with child backrefs)")
	}
	sc := objects.GetGlobalStatusChecker()
	t.Logf("IsPreliminary(priority_plan, active)=%v", sc.IsPreliminary(objects.KindPriorityPlan, objects.ObjectStatusActive))
}

func TestIsRefActive_RealPRIKindInference(t *testing.T) {
	t.Parallel()
	id := "PLAN-EXAMPLE"
	kind := GetIDValidator().InferKindFromID(id)
	t.Logf("inferred kind=%q", kind)
	sc := objects.GetGlobalStatusChecker()
	for _, st := range []string{"active", "grooming", "prioritizing"} {
		t.Logf("status=%s prelim=%v system=%v archive=%v active=%v", st,
			sc.IsPreliminary(kind, st), sc.IsSystem(kind, st), sc.IsArchive(kind, st), sc.IsActive(kind, st))
	}
}
