package crud

import (
	"reflect"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestQueryBuilder_Basic(t *testing.T) {
	qb := NewQueryBuilder("backlog_item").
		IncludeFields("id", "title").
		Fields("status").
		AndFilter("priority", "high").
		Status("active").
		Limit(10).
		Offset(5).
		SortBy("title").
		GroupBy("priority")

	filter := qb.Build()

	if filter.Kind != "backlog_item" {
		t.Errorf("expected kind 'backlog_item', got %q", filter.Kind)
	}
	expectedFields := []string{"id", "title", "status"}
	if !reflect.DeepEqual(filter.Fields, expectedFields) {
		t.Errorf("expected fields %v, got %v", expectedFields, filter.Fields)
	}
	if filter.Filters["priority"] != "high" {
		t.Errorf("expected priority 'high', got %v", filter.Filters["priority"])
	}
	if filter.Filters[objects.FieldKeyStatus] != "active" {
		t.Errorf("expected status 'active', got %v", filter.Filters[objects.FieldKeyStatus])
	}
	if filter.Limit != 10 {
		t.Errorf("expected limit 10, got %d", filter.Limit)
	}
	if filter.Offset != 5 {
		t.Errorf("expected offset 5, got %d", filter.Offset)
	}
	if filter.SortBy != "title" || !filter.SortAsc {
		t.Errorf("expected sort by title asc, got %s asc=%v", filter.SortBy, filter.SortAsc)
	}
	if filter.GroupBy != "priority" {
		t.Errorf("expected group by priority, got %s", filter.GroupBy)
	}
}

func TestQueryBuilder_FilterOperations(t *testing.T) {
	t.Run("StatusIn and StatusNotIn", func(t *testing.T) {
		qb := NewQueryBuilder("goal").StatusIn("active", "in_progress")
		f := qb.Build()
		statMap, ok := f.Filters[objects.FieldKeyStatus].(map[string]any)
		if !ok || statMap["$in"] == nil {
			t.Fatalf("expected status $in map, got %v", f.Filters[objects.FieldKeyStatus])
		}

		qb2 := NewQueryBuilder("goal").StatusNotIn("complete", "archived")
		f2 := qb2.Build()
		statMap2, ok := f2.Filters[objects.FieldKeyStatus].(map[string]any)
		if !ok || statMap2["$nin"] == nil {
			t.Fatalf("expected status $nin map, got %v", f2.Filters[objects.FieldKeyStatus])
		}
	})

	t.Run("OrFilter and OrFilterBuilders", func(t *testing.T) {
		qb := NewQueryBuilder("backlog_item").
			OrFilter(
				map[string]any{"status": "in_progress"},
				map[string]any{"priority": "p0"},
			)
		f := qb.Build()
		orList, ok := f.Filters["$or"].([]map[string]any)
		if !ok || len(orList) != 2 {
			t.Fatalf("expected $or with 2 items, got %v", f.Filters["$or"])
		}

		// OrFilterBuilders
		b1 := NewQueryBuilder("").Status("planned")
		b2 := NewQueryBuilder("").Status("exploring")
		qbParent := NewQueryBuilder("task").OrFilterBuilders(b1, b2)
		fParent := qbParent.Build()
		orParent, ok := fParent.Filters["$or"].([]map[string]any)
		if !ok || len(orParent) != 2 {
			t.Fatalf("expected $or with 2 items from builders, got %v", fParent.Filters["$or"])
		}
	})

	t.Run("FilterOp and Id helpers", func(t *testing.T) {
		qb := NewQueryBuilder("audit_event").
			FilterOp("created_at", "$after", "2026-01-01T00:00:00Z").
			Id("AUD-100")
		f := qb.Build()
		if f.Filters[objects.FieldKeyID] != "AUD-100" {
			t.Errorf("expected id AUD-100, got %v", f.Filters[objects.FieldKeyID])
		}
		opMap, ok := f.Filters["created_at"].(map[string]any)
		if !ok || opMap["$after"] != "2026-01-01T00:00:00Z" {
			t.Errorf("expected $after filter, got %v", f.Filters["created_at"])
		}
	})

	t.Run("Clone and ToFilter", func(t *testing.T) {
		qb := NewQueryBuilder("milestone").
			IncludeFields("id", "title").
			Status("active")
		clone := qb.Clone().Status("complete")

		fOriginal := qb.ToFilter()
		fClone := clone.Build()

		if fOriginal.Filters[objects.FieldKeyStatus] != "active" {
			t.Errorf("original should still have status active, got %v", fOriginal.Filters[objects.FieldKeyStatus])
		}
		if fClone.Filters[objects.FieldKeyStatus] != "complete" {
			t.Errorf("clone should have status complete, got %v", fClone.Filters[objects.FieldKeyStatus])
		}
	})
}

func TestQueryFactory_Templates(t *testing.T) {
	qf := NewQueryFactory()

	t.Run("IdTitle", func(t *testing.T) {
		f := qf.IdTitle("goal").Build()
		if f.Kind != "goal" {
			t.Errorf("expected goal, got %s", f.Kind)
		}
		expected := []string{objects.FieldKeyID, objects.FieldKeyTitle}
		if !reflect.DeepEqual(f.Fields, expected) {
			t.Errorf("expected fields %v, got %v", expected, f.Fields)
		}
	})

	t.Run("IdTitleStatus", func(t *testing.T) {
		f := qf.IdTitleStatus("priority_plan").Build()
		expected := []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus}
		if !reflect.DeepEqual(f.Fields, expected) {
			t.Errorf("expected fields %v, got %v", expected, f.Fields)
		}
	})

	t.Run("IdTitleStatusActive", func(t *testing.T) {
		f := qf.IdTitleStatusActive("priority_plan").Build()
		expected := []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus}
		if !reflect.DeepEqual(f.Fields, expected) {
			t.Errorf("expected fields %v, got %v", expected, f.Fields)
		}
		if f.Filters[objects.FieldKeyStatus] != objects.ObjectStatusActive {
			t.Errorf("expected status=active, got %v", f.Filters[objects.FieldKeyStatus])
		}
	})

	t.Run("NotArchived", func(t *testing.T) {
		f := qf.NotArchived("backlog_item").Build()
		statMap, ok := f.Filters[objects.FieldKeyStatus].(map[string]any)
		if !ok || statMap["$ne"] != objects.ObjectStatusArchived {
			t.Errorf("expected status $ne archived, got %v", f.Filters[objects.FieldKeyStatus])
		}
	})

	t.Run("ActiveNotComplete", func(t *testing.T) {
		f := qf.ActiveNotComplete("priority_plan").Build()
		statMap, ok := f.Filters[objects.FieldKeyStatus].(map[string]any)
		if !ok || statMap["$nin"] == nil {
			t.Errorf("expected status $nin filter, got %v", f.Filters[objects.FieldKeyStatus])
		}
	})

	t.Run("ById", func(t *testing.T) {
		f := qf.ById("requirement", "REQ-123").Build()
		if f.Filters[objects.FieldKeyID] != "REQ-123" || f.Limit != 1 {
			t.Errorf("expected id=REQ-123 and limit=1, got filter=%v limit=%d", f.Filters, f.Limit)
		}
	})

	t.Run("Relationship Templates", func(t *testing.T) {
		fPlan := qf.ForPlan("backlog_item", "PRI-001").Build()
		if fPlan.Filters[objects.FieldKeyPriorityPlanRef] != "PRI-001" {
			t.Errorf("expected priority_plan_ref=PRI-001, got %v", fPlan.Filters[objects.FieldKeyPriorityPlanRef])
		}

		fPlanNA := qf.ForPlanNotArchived("backlog_item", "PRI-001").Build()
		if fPlanNA.Filters[objects.FieldKeyPriorityPlanRef] != "PRI-001" {
			t.Errorf("expected priority_plan_ref=PRI-001, got %v", fPlanNA.Filters[objects.FieldKeyPriorityPlanRef])
		}
		statMap, ok := fPlanNA.Filters[objects.FieldKeyStatus].(map[string]any)
		if !ok || statMap["$ne"] != objects.ObjectStatusArchived {
			t.Errorf("expected status $ne archived, got %v", fPlanNA.Filters[objects.FieldKeyStatus])
		}

		fPlanNT := qf.ForPlanNonTerminal("backlog_item", "PRI-001").Build()
		if fPlanNT.Filters[objects.FieldKeyPriorityPlanRef] != "PRI-001" {
			t.Errorf("expected priority_plan_ref=PRI-001, got %v", fPlanNT.Filters[objects.FieldKeyPriorityPlanRef])
		}
		statNin, ok := fPlanNT.Filters[objects.FieldKeyStatus].(map[string]any)
		if !ok || statNin["$nin"] == nil {
			t.Errorf("expected status $nin filter, got %v", fPlanNT.Filters[objects.FieldKeyStatus])
		}

		fMls := qf.ForMilestone("backlog_item", "MIL-001").Build()
		if fMls.Filters[objects.FieldKeyMilestoneRefs] != "MIL-001" {
			t.Errorf("expected milestone_refs=MIL-001, got %v", fMls.Filters[objects.FieldKeyMilestoneRefs])
		}

		fReq := qf.ForRequirement("criteria", "REQ-001").Build()
		if fReq.Filters[objects.FieldKeyRequirementRefs] != "REQ-001" {
			t.Errorf("expected requirement_refs=REQ-001, got %v", fReq.Filters[objects.FieldKeyRequirementRefs])
		}
	})

	t.Run("IdTitleStatusNotArchived", func(t *testing.T) {
		f := qf.IdTitleStatusNotArchived("goal").Build()
		expectedFields := []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus}
		if !reflect.DeepEqual(f.Fields, expectedFields) {
			t.Errorf("expected fields %v, got %v", expectedFields, f.Fields)
		}
		statMap, ok := f.Filters[objects.FieldKeyStatus].(map[string]any)
		if !ok || statMap["$ne"] != objects.ObjectStatusArchived {
			t.Errorf("expected status $ne archived, got %v", f.Filters[objects.FieldKeyStatus])
		}
	})
}
