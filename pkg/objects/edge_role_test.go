package objects

import "testing"

func TestParseEdgeRole(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want EdgeRole
		ok   bool
	}{
		{"membership", EdgeRoleMembership, true},
		{" Composition ", EdgeRoleComposition, true},
		{"ASSOCIATE", EdgeRoleAssociate, true},
		{"association", emptyValue, false},
		{"", emptyValue, false},
	}
	for _, tc := range cases {
		got, ok := ParseEdgeRole(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParseEdgeRole(%q) = %q,%v want %q,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestEdgeRoleForKindField_DNA(t *testing.T) {
	t.Parallel()
	if got := EdgeRoleForKindField(KindBacklogItem, FieldKeyPriorityPlanRef, nil); got != EdgeRoleMembership {
		t.Fatalf("priority_plan_ref = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindRequirement, FieldKeyCriteriaRefs, nil); got != EdgeRoleComposition {
		t.Fatalf("criteria_refs = %s, want composition", got)
	}
	if got := EdgeRoleForKindField(KindPriorityPlan, FieldKeyRelatedObjectRefs, nil); got != EdgeRoleAssociate {
		t.Fatalf("related_object_refs = %s, want associate", got)
	}
	if got := EdgeRoleForKindField(KindCriteria, FieldKeyGoalRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("criteria.goal_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindBacklogItem, FieldKeyGoalRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("backlog_item.goal_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindBacklogItem, FieldKeyRequirementRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("backlog_item.requirement_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindBacklogItem, FieldKeyMilestoneRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("backlog_item.milestone_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindRequirement, FieldKeyGoalRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("requirement.goal_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindRequirement, FieldKeyMilestoneRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("requirement.milestone_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindRequirement, FieldKeyWorkstreamRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("requirement.workstream_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindMilestone, FieldKeyWorkstreamRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("milestone.workstream_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindTestCase, FieldKeyRequirementRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("test_case.requirement_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindTechnicalSpec, FieldKeyRequirementRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("technical_spec.requirement_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindAgentTask, FieldKeyPipelineRef, nil); got != EdgeRoleMembership {
		t.Fatalf("agent_task.pipeline_ref = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindAgentTask, FieldKeyBacklogItemRef, nil); got != EdgeRoleMembership {
		t.Fatalf("agent_task.backlog_item_ref = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindBacklogItem, FieldKeyConvergenceSessionRef, nil); got != EdgeRoleMembership {
		t.Fatalf("backlog_item.convergence_session_ref = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindComponent, FieldKeyDisplayRef, nil); got != EdgeRoleMembership {
		t.Fatalf("component.display_ref = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindTeam, FieldKeyDepartmentRef, nil); got != EdgeRoleMembership {
		t.Fatalf("team.department_ref = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindTeam, FieldKeyDivisionRef, nil); got != EdgeRoleMembership {
		t.Fatalf("team.division_ref = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindImpactAnalysis, FieldKeyChangeRef, nil); got != EdgeRoleMembership {
		t.Fatalf("impact_analysis.change_ref = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindMilestone, FieldKeyGoalRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("milestone.goal_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindPersona, FieldKeyMissionRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("persona.mission_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindPartnership, FieldKeyOrganizationRefs, nil); got != EdgeRoleComposition {
		t.Fatalf("partnership.organization_refs = %s, want composition", got)
	}
	if got := EdgeRoleForKindField(KindComponent, FieldKeyParentComponentRefs, nil); got != EdgeRoleMembership {
		t.Fatalf("component.parent_component_refs = %s, want membership", got)
	}
	if got := EdgeRoleForKindField(KindDivision, FieldKeyParentDivisionRef, nil); got != EdgeRoleMembership {
		t.Fatalf("division.parent_division_ref = %s, want membership", got)
	}
}

func TestEdgeRoleFromSpec_WinsOverDNA(t *testing.T) {
	t.Parallel()
	spec := &Spec{ResolvedFields: map[string]any{
		FieldKeyPriorityPlanRef: map[string]any{SpecKeyEdgeRole: string(EdgeRoleAssociate)},
	}}
	if got := EdgeRoleForKindField(KindBacklogItem, FieldKeyPriorityPlanRef, spec); got != EdgeRoleAssociate {
		t.Fatalf("spec annotation must win, got %s", got)
	}
}

func TestHopMatchesFilter(t *testing.T) {
	t.Parallel()
	if !HopMatchesFilter(FieldKeyPriorityPlanRef, EdgeRoleMembership, emptyValue) {
		t.Fatal("empty filter should match")
	}
	if !HopMatchesFilter(FieldKeyPriorityPlanRef, EdgeRoleMembership, string(EdgeRoleMembership)) {
		t.Fatal("role filter should match membership")
	}
	if HopMatchesFilter(FieldKeyPriorityPlanRef, EdgeRoleMembership, string(EdgeRoleComposition)) {
		t.Fatal("composition filter must not match membership hop")
	}
	if !HopMatchesFilter(FieldKeyPriorityPlanRef, EdgeRoleMembership, FieldKeyPriorityPlanRef) {
		t.Fatal("field-name filter should match")
	}
	if HopMatchesFilter(FieldKeyPriorityPlanRef, EdgeRoleMembership, FieldKeyCriteriaRefs) {
		t.Fatal("other field-name filter must not match")
	}
}
