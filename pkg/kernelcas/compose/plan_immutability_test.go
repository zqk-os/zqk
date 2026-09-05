package compose

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestEvalRefusePlanStatus_BLIInProgressRequiresExecutionFacingPlan(t *testing.T) {
	cfg := map[string]any{
		"plan_field":          objects.FieldKeyPriorityPlanRef,
		"when_object_status":  []any{objects.ObjectStatusInProgress},
		"require_plan_status": []any{objects.ObjectStatusActive, objects.ObjectStatusInProgress},
		"message_fmt":         "backlog_item cannot be in_progress because priority_plan %s is in '%s' status (must be 'active' or 'in_progress')",
	}
	lookup := func(id string) (map[string]any, error) {
		return map[string]any{
			objects.FieldKeyID:     id,
			objects.FieldKeyStatus: objects.ObjectStatusGrooming,
		}, nil
	}
	obj := map[string]any{
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef: "PRI-grooming",
	}
	errs := evalRefusePlanStatus(pkgctx.NewSystemContext(), obj, lookup, cfg)
	if len(errs) != 1 {
		t.Fatalf("want 1 err for grooming plan, got %#v", errs)
	}

	lookupActive := func(id string) (map[string]any, error) {
		return map[string]any{
			objects.FieldKeyID:     id,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		}, nil
	}
	if errs := evalRefusePlanStatus(pkgctx.NewSystemContext(), obj, lookupActive, cfg); len(errs) != 0 {
		t.Fatalf("active plan should allow: %#v", errs)
	}
}

func TestEvalRefusePlanStatus_NewLinkRefusesActivePlan(t *testing.T) {
	cfg := map[string]any{
		"plan_field":    objects.FieldKeyPriorityPlanRef,
		"refuse_when":   []any{objects.ObjectStatusActive, objects.ObjectStatusInProgress},
		"message_fmt":   "cannot assign priority_plan_ref %s: priority plan is '%s' (execution-facing / sealed)",
		"new_link_only": true,
	}
	lookupActive := func(id string) (map[string]any, error) {
		if id == "PRI-locked" {
			return map[string]any{
				objects.FieldKeyID:     id,
				objects.FieldKeyStatus: objects.ObjectStatusActive,
			}, nil
		}
		// Prior BLI had no plan — new link.
		return map[string]any{
			objects.FieldKeyID:              id,
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "",
		}, nil
	}
	obj := map[string]any{
		objects.FieldKeyID:              "BLI-new",
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef: "PRI-locked",
	}
	errs := evalRefusePlanStatus(pkgctx.NewSystemContext(), obj, lookupActive, cfg)
	if len(errs) != 1 {
		t.Fatalf("want 1 err for new link to active plan, got %#v", errs)
	}

	lookupSame := func(id string) (map[string]any, error) {
		if id == "PRI-locked" {
			return map[string]any{
				objects.FieldKeyID:     id,
				objects.FieldKeyStatus: objects.ObjectStatusActive,
			}, nil
		}
		return map[string]any{
			objects.FieldKeyID:              id,
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-locked",
		}, nil
	}
	if errs := evalRefusePlanStatus(pkgctx.NewSystemContext(), obj, lookupSame, cfg); len(errs) != 0 {
		t.Fatalf("same-plan update must allow: %#v", errs)
	}

	lookupGrooming := func(id string) (map[string]any, error) {
		if id == "PRI-groom" {
			return map[string]any{
				objects.FieldKeyID:     id,
				objects.FieldKeyStatus: objects.ObjectStatusGrooming,
			}, nil
		}
		return map[string]any{
			objects.FieldKeyID:              id,
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "",
		}, nil
	}
	objGroom := map[string]any{
		objects.FieldKeyID:              "BLI-new2",
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef: "PRI-groom",
	}
	if errs := evalRefusePlanStatus(pkgctx.NewSystemContext(), objGroom, lookupGrooming, cfg); len(errs) != 0 {
		t.Fatalf("new link to grooming must allow: %#v", errs)
	}
}

func TestEvalRefusePlanStatus_PlannedBLIRefusesCompletePlan(t *testing.T) {
	cfg := map[string]any{
		"plan_field":         objects.FieldKeyPriorityPlanRef,
		"when_object_status": []any{objects.ObjectStatusPlanned},
		"refuse_when":        []any{objects.ObjectStatusComplete},
		"message_fmt":        "backlog_item cannot stay planned on complete priority plan %s (status %s); reopen the plan or complete/move the item",
	}
	lookup := func(id string) (map[string]any, error) {
		return map[string]any{
			objects.FieldKeyID:     id,
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
		}, nil
	}
	obj := map[string]any{
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef: "PRI-done",
	}
	errs := evalRefusePlanStatus(pkgctx.NewSystemContext(), obj, lookup, cfg)
	if len(errs) != 1 {
		t.Fatalf("want 1 err for planned on complete plan, got %#v", errs)
	}
	lookupActive := func(id string) (map[string]any, error) {
		return map[string]any{
			objects.FieldKeyID:     id,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		}, nil
	}
	if errs := evalRefusePlanStatus(pkgctx.NewSystemContext(), obj, lookupActive, cfg); len(errs) != 0 {
		t.Fatalf("active plan should allow planned: %#v", errs)
	}
}

func TestEvalRefusePlanStatus_ArchivedBLIRequiresArchivedPlan(t *testing.T) {
	cfg := map[string]any{
		"plan_field":          objects.FieldKeyPriorityPlanRef,
		"when_object_status":  []any{objects.ObjectStatusArchived},
		"require_plan_status": []any{objects.ObjectStatusArchived},
		"message_fmt":         "backlog_item cannot be archived because priority_plan %s is in '%s' status (must be 'archived'; promote the plan)",
	}
	lookupComplete := func(id string) (map[string]any, error) {
		return map[string]any{
			objects.FieldKeyID:     id,
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
		}, nil
	}
	obj := map[string]any{
		objects.FieldKeyStatus:          objects.ObjectStatusArchived,
		objects.FieldKeyPriorityPlanRef: "PRI-live",
	}
	errs := evalRefusePlanStatus(pkgctx.NewSystemContext(), obj, lookupComplete, cfg)
	if len(errs) != 1 {
		t.Fatalf("want 1 err for archived BLI on complete plan, got %#v", errs)
	}
	lookupArchived := func(id string) (map[string]any, error) {
		return map[string]any{
			objects.FieldKeyID:     id,
			objects.FieldKeyStatus: objects.ObjectStatusArchived,
		}, nil
	}
	if errs := evalRefusePlanStatus(pkgctx.NewSystemContext(), obj, lookupArchived, cfg); len(errs) != 0 {
		t.Fatalf("archived plan should allow archived BLI: %#v", errs)
	}
	noPlan := map[string]any{objects.FieldKeyStatus: objects.ObjectStatusArchived}
	if errs := evalRefusePlanStatus(pkgctx.NewSystemContext(), noPlan, lookupComplete, cfg); len(errs) != 0 {
		t.Fatalf("draft-plane archived BLI with no plan must allow: %#v", errs)
	}
}
