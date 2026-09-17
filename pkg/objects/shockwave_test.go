package objects

import "testing"

func TestMergeShockwavePolicy_ChildModeInheritsLineageKinds(t *testing.T) {
	t.Parallel()
	parent := DefaultArchiveShockwavePolicy()
	child := ShockwavePolicy{Mode: ShockwaveModePrune, Fail: ShockwaveFailClosed}
	got := MergeShockwavePolicy(parent, child)
	if got.Mode != ShockwaveModePrune {
		t.Fatalf("mode = %s, want prune", got.Mode)
	}
	if len(got.LineageKinds) == 0 {
		t.Fatal("lineage_kinds should inherit from parent")
	}
	if !got.IsLineageKind(KindGoal) {
		t.Fatal("goal must remain a lineage kind after prune overlay")
	}
	if !got.IsLineageKind(KindWorkstream) {
		t.Fatal("workstream must remain a shared trunk after prune overlay")
	}
	if got.Fail != ShockwaveFailClosed {
		t.Fatalf("fail = %s", got.Fail)
	}
}

func TestShockwavePolicy_HopRoleConsultsEdgeRole(t *testing.T) {
	t.Parallel()
	p := DefaultArchiveShockwavePolicy()
	if got := p.HopRole(KindBacklogItem, FieldKeyPriorityPlanRef); got != EdgeRoleMembership {
		t.Fatalf("priority_plan_ref HopRole = %s, want membership", got)
	}
	if !p.ShouldNoteLineage(KindBacklogItem, FieldKeyPriorityPlanRef) {
		t.Fatal("membership field must note lineage")
	}
	if got := p.HopRole(KindRequirement, FieldKeyCriteriaRefs); got != EdgeRoleComposition {
		t.Fatalf("criteria_refs HopRole = %s, want composition", got)
	}
	if !p.ShouldClusterHop(KindRequirement, FieldKeyCriteriaRefs) {
		t.Fatal("composition field must cluster-hop")
	}
	if got := p.HopRole(KindPriorityPlan, FieldKeyRelatedObjectRefs); got != EdgeRoleAssociate {
		t.Fatalf("related_object_refs HopRole = %s, want associate", got)
	}
	if p.ShouldClusterHop(KindPriorityPlan, FieldKeyRelatedObjectRefs) {
		t.Fatal("associate must not cluster-hop")
	}
}
