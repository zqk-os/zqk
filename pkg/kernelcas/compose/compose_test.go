package compose

import (
	"os"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestCompileAllCritical_producesDefinitions(t *testing.T) {
	ResetDefaultForTest()
	c := NewCompiler("")
	defs, err := c.CompileAllCritical()
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) < 7*10 {
		t.Fatalf("expected many defs, got %d", len(defs))
	}
	var bliTransition *Definition
	for _, d := range defs {
		if d.Key.ObjectKind == objects.KindBacklogItem && d.Key.PipelineKind == KindTransition {
			bliTransition = d
			break
		}
	}
	if bliTransition == nil {
		t.Fatal("missing backlog_item transition definition")
	}
	foundOverlay := false
	for _, st := range bliTransition.Stages {
		for _, r := range st.Rules {
			if r.ID == "bli_hierarchical_chain" {
				foundOverlay = true
			}
		}
	}
	if !foundOverlay {
		t.Fatal("expected bli_hierarchical_chain overlay rule")
	}
	foundTransition := false
	foundMembership := false
	for _, st := range bliTransition.Stages {
		for _, r := range st.Rules {
			if r.ID == "bli_transition_estimated_effort" {
				foundTransition = true
			}
			if r.ID == "bli_execution_facing_membership" {
				foundMembership = true
			}
		}
	}
	if !foundTransition {
		t.Fatal("expected bli_transition_estimated_effort on transition definition")
	}
	if !foundMembership {
		t.Fatal("expected bli_execution_facing_membership on transition definition")
	}
	foundPlanGate := false
	for _, st := range bliTransition.Stages {
		for _, r := range st.Rules {
			if r.ID == "bli_in_progress_requires_execution_facing_plan" {
				foundPlanGate = true
			}
		}
	}
	if !foundPlanGate {
		t.Fatal("expected bli_in_progress_requires_execution_facing_plan on transition definition")
	}
	foundShovelReady := false
	for _, st := range bliTransition.Stages {
		for _, r := range st.Rules {
			if r.ID == "bli_transition_shovel_ready" {
				foundShovelReady = true
			}
		}
	}
	if !foundShovelReady {
		t.Fatal("expected bli_transition_shovel_ready on transition definition")
	}
	foundPlannedComplete := false
	for _, st := range bliTransition.Stages {
		for _, r := range st.Rules {
			if r.ID == "bli_planned_refuses_complete_plan" {
				foundPlannedComplete = true
			}
		}
	}
	if !foundPlannedComplete {
		t.Fatal("expected bli_planned_refuses_complete_plan on transition definition")
	}
	foundArchiveCeiling := false
	for _, st := range bliTransition.Stages {
		for _, r := range st.Rules {
			if r.ID == "bli_archived_requires_archived_plan" {
				foundArchiveCeiling = true
			}
		}
	}
	if !foundArchiveCeiling {
		t.Fatal("expected bli_archived_requires_archived_plan on transition definition")
	}
}

func TestValidateObject_backlogItemRequiresRefs(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyStatus: objects.ObjectStatusPlanned,
		objects.FieldKeyID:     "BLI-test",
	}
	errs := ValidateObject(t.Context(), Default(), objects.KindBacklogItem, obj, nil)
	if len(errs) == 0 {
		t.Fatal("expected hierarchical chain error")
	}
}

func TestValidateObject_priorityPlanRefusesBacklogItemRefs(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{
		objects.FieldKeyKind:            objects.KindPriorityPlan,
		objects.FieldKeyStatus:          objects.ObjectStatusActive,
		objects.FieldKeyID:              "PRI-test",
		objects.FieldKeyActiveOrder:     1,
		objects.FieldKeyBacklogItemRefs: []any{"BLI-1"},
	}
	errs := ValidateObject(t.Context(), Default(), objects.KindPriorityPlan, obj, nil)
	found := false
	for _, e := range errs {
		if e.Field == objects.FieldKeyBacklogItemRefs && strings.Contains(e.Message, "must not store backlog_item_refs") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pri_no_backlog_item_refs refuse, got %#v", errs)
	}
	clean := map[string]any{
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
		objects.FieldKeyID:          "PRI-clean",
		objects.FieldKeyActiveOrder: 1,
	}
	errs = ValidateObject(t.Context(), Default(), objects.KindPriorityPlan, clean, nil)
	for _, e := range errs {
		if e.Field == objects.FieldKeyBacklogItemRefs {
			t.Fatalf("clean priority_plan must not refuse backlog_item_refs: %#v", e)
		}
	}
}

func TestValidateObject_priorityPlanRefusesBacklogItemsInRelatedRefs(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{
		objects.FieldKeyKind:              objects.KindPriorityPlan,
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyID:                "PRI-test",
		objects.FieldKeyActiveOrder:       1,
		objects.FieldKeyRelatedObjectRefs: []any{"REQ-1", "BLI-1"},
	}
	errs := ValidateObject(t.Context(), Default(), objects.KindPriorityPlan, obj, nil)
	for _, e := range errs {
		if e.Field == objects.FieldKeyRelatedObjectRefs && strings.Contains(e.Message, "must not contain backlog items") {
			return
		}
	}
	t.Fatalf("expected pri_no_backlog_related_refs refuse, got %#v", errs)
}

func TestValidateObject_criteriaRefusesRequirementAndMilestoneRefs(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{
		objects.FieldKeyKind:            objects.KindCriteria,
		objects.FieldKeyStatus:          objects.ObjectStatusValidated,
		objects.FieldKeyID:              "CRIT-test",
		objects.FieldKeyTitle:           "cycle probe",
		objects.FieldKeyRequirementRefs: []any{"REQ-1"},
		objects.FieldKeyMilestoneRefs:   []any{"MIL-1"},
	}
	errs := ValidateObject(t.Context(), Default(), objects.KindCriteria, obj, nil)
	var sawReq, sawMil bool
	for _, e := range errs {
		if e.Field == objects.FieldKeyRequirementRefs && strings.Contains(e.Message, "must not store requirement_refs") {
			sawReq = true
		}
		if e.Field == objects.FieldKeyMilestoneRefs && strings.Contains(e.Message, "must not store milestone_refs") {
			sawMil = true
		}
	}
	if !sawReq || !sawMil {
		t.Fatalf("expected crit_no_requirement_refs and crit_no_milestone_refs, got %#v", errs)
	}
}

func TestValidateObject_criteriaRequiresCategory(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyID:     "CRIT-nocat",
		objects.FieldKeyTitle:  "missing category",
	}
	errs := ValidateObject(t.Context(), Default(), objects.KindCriteria, obj, nil)
	for _, e := range errs {
		if e.Field == objects.FieldKeyCategory && strings.Contains(e.Message, "requires category") {
			return
		}
	}
	t.Fatalf("expected crit_require_category, got %#v", errs)
}

func TestValidateObject_occupancyParentsRefuseBacklogItemRefs(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		kind string
		id   string
	}{
		{objects.KindGoal, "GOAL-test"},
		{objects.KindMilestone, "MLS-test"},
		{objects.KindRequirement, "REQ-test"},
	}
	for _, tc := range cases {
		obj := map[string]any{
			objects.FieldKeyKind:            tc.kind,
			objects.FieldKeyStatus:          objects.ObjectStatusDraft,
			objects.FieldKeyID:              tc.id,
			objects.FieldKeyBacklogItemRefs: []any{"BLI-1"},
		}
		errs := ValidateObject(t.Context(), Default(), tc.kind, obj, nil)
		found := false
		for _, e := range errs {
			if e.Field == objects.FieldKeyBacklogItemRefs && strings.Contains(e.Message, "must not store backlog_item_refs") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s: expected occupancy refuse of backlog_item_refs, got %#v", tc.kind, errs)
		}
	}
}

func TestValidateObject_occupancyParentsRefuseGanttReverseLists(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		kind  string
		id    string
		field string
		msg   string
	}{
		{objects.KindGoal, "GOAL-req", objects.FieldKeyRequirementRefs, "must not store requirement_refs"},
		{objects.KindMilestone, "MLS-req", objects.FieldKeyRequirementRefs, "must not store requirement_refs"},
		{objects.KindWorkstream, "WS-req", objects.FieldKeyRequirementRefs, "must not store requirement_refs"},
		{objects.KindWorkstream, "WS-mil", objects.FieldKeyMilestoneRefs, "must not store milestone_refs"},
		{objects.KindRequirement, "REQ-tc", objects.FieldKeyTestCaseRefs, "must not store test_case_refs"},
		{objects.KindRequirement, "REQ-ts", objects.FieldKeyTechnicalSpecRefs, "must not store technical_spec_refs"},
		{objects.KindPipeline, "PIPE-atk", objects.FieldKeyAgentTaskRefs, "must not store agent_task_refs"},
		{objects.KindConvergenceSession, "CVS-bli", objects.FieldKeyBacklogItemRefs, "must not store backlog_item_refs"},
		{objects.KindDisplay, "DSP-comp", objects.FieldKeyComponentRefs, "must not store component_refs"},
		{objects.KindDepartment, "DEPT-team", objects.FieldKeyTeamRefs, "must not store team_refs"},
		{objects.KindDivision, "DIV-team", objects.FieldKeyTeamRefs, "must not store team_refs"},
		{objects.KindOrganizationalChange, "ORGCHG-ia", objects.FieldKeyImpactAnalysisRefs, "must not store impact_analysis_refs"},
		{objects.KindGoal, "GOAL-mil", objects.FieldKeyMilestoneRefs, "must not store milestone_refs"},
		{objects.KindMission, "MIS-per", objects.FieldKeyPersonaRefs, "must not store persona_refs"},
		{objects.KindOrganization, "ORG-part", objects.FieldKeyPartnershipRefs, "must not store partnership_refs"},
		{objects.KindComponent, "COMP-child", objects.FieldKeyChildComponentRefs, "must not store child_component_refs"},
		{objects.KindDivision, "DIV-child", objects.FieldKeyChildDivisionRefs, "must not store child_division_refs"},
	}
	for _, tc := range cases {
		obj := map[string]any{
			objects.FieldKeyKind:   tc.kind,
			objects.FieldKeyStatus: objects.ObjectStatusDraft,
			objects.FieldKeyID:     tc.id,
			tc.field:               []any{"X-1"},
		}
		errs := ValidateObject(t.Context(), Default(), tc.kind, obj, nil)
		found := false
		for _, e := range errs {
			if e.Field == tc.field && strings.Contains(e.Message, tc.msg) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s %s: expected occupancy refuse, got %#v", tc.kind, tc.field, errs)
		}
	}
}

func TestValidateObject_relatedRefsRefuseTypedDuplicates(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	req := map[string]any{
		objects.FieldKeyKind:              objects.KindRequirement,
		objects.FieldKeyStatus:            objects.ObjectStatusDraft,
		objects.FieldKeyID:                "REQ-rel",
		objects.FieldKeyRelatedObjectRefs: []any{"CRIT-1"},
	}
	errs := ValidateObject(t.Context(), Default(), objects.KindRequirement, req, nil)
	found := false
	for _, e := range errs {
		if e.Field == objects.FieldKeyRelatedObjectRefs && strings.Contains(e.Message, "must not contain criteria") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected req_no_crit_related_refs, got %#v", errs)
	}
	crit := map[string]any{
		objects.FieldKeyKind:              objects.KindCriteria,
		objects.FieldKeyStatus:            objects.ObjectStatusValidated,
		objects.FieldKeyID:                "CRIT-rel",
		objects.FieldKeyTitle:             "related prefix probe",
		objects.FieldKeyRelatedObjectRefs: []any{"REQ-1"},
	}
	errs = ValidateObject(t.Context(), Default(), objects.KindCriteria, crit, nil)
	found = false
	for _, e := range errs {
		if e.Field == objects.FieldKeyRelatedObjectRefs && strings.Contains(e.Message, "must not contain requirements") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected crit_no_req_related_refs, got %#v", errs)
	}
}

func TestValidateObject_priorityPlanActiveOrderMatchesLifecycleSemantics(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	active := map[string]any{
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyID:     "PRI-active-without-order",
	}
	if !validationErrorsContainField(ValidateObject(t.Context(), Default(), objects.KindPriorityPlan, active, nil), objects.FieldKeyActiveOrder) {
		t.Fatal("active priority plan without active_order must be refused")
	}
	inProgress := map[string]any{
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		objects.FieldKeyID:     "PRI-in-progress-without-order",
	}
	if validationErrorsContainField(ValidateObject(t.Context(), Default(), objects.KindPriorityPlan, inProgress, nil), objects.FieldKeyActiveOrder) {
		t.Fatal("in_progress priority plan must clear active_order for roadmap slot hygiene")
	}
	inProgressWithOrder := map[string]any{
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
		objects.FieldKeyID:          "PRI-in-progress-with-order",
		objects.FieldKeyActiveOrder: 1,
	}
	if !validationErrorsContainField(ValidateObject(t.Context(), Default(), objects.KindPriorityPlan, inProgressWithOrder, nil), objects.FieldKeyActiveOrder) {
		t.Fatal("in_progress priority plan with active_order must be refused")
	}
	completeWithOrder := map[string]any{
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyStatus:      objects.ObjectStatusComplete,
		objects.FieldKeyID:          "PRI-complete-with-order",
		objects.FieldKeyActiveOrder: 1,
	}
	if !validationErrorsContainField(ValidateObject(t.Context(), Default(), objects.KindPriorityPlan, completeWithOrder, nil), objects.FieldKeyActiveOrder) {
		t.Fatal("complete priority plan with active_order must be refused")
	}
}

func validationErrorsContainField(errs []ValidationError, field string) bool {
	for _, err := range errs {
		if err.Field == field {
			return true
		}
	}
	return false
}

func TestValidateObjectIntent_transitionRequiresEffort(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyID:              "BLI-tr",
		objects.FieldKeyGoalRefs:        []any{"GOL-1"},
		objects.FieldKeyPriority:        "high",
		objects.FieldKeyPriorityPlanRef: "PRI-1",
		objects.FieldKeyMilestoneRefs:   []any{"MIL-1"},
		objects.FieldKeyPriorityTier:    "P0",
		objects.FieldKeyRequirementRefs: []any{"REQ-1"},
		objects.FieldKeyCriteriaRefs:    []any{"CRIT-1"},
		objects.FieldKeyPersonaRefs:     []any{"PER-DEFAULT-OPERATOR"},
	}
	errs := ValidateObjectIntent(t.Context(), Default(), objects.KindBacklogItem, KindTransition, IntentTransition, obj, nil)
	found := false
	for _, e := range errs {
		if e.Field == objects.FieldKeyEstimatedEffort {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected estimated_effort error on transition validate, got %#v", errs)
	}
}

func TestValidateObjectIntent_transitionRequiresShovelReady(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	// Hold overlays satisfied; CRI-SHOVEL-READY refs missing. TRACK: REDACTED
	obj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyID:              "BLI-thin-ip",
		objects.FieldKeyGoalRefs:        []any{"GOL-1"},
		objects.FieldKeyPriority:        "high",
		objects.FieldKeyPriorityPlanRef: "PRI-1",
		objects.FieldKeyMilestoneRefs:   []any{"MIL-1"},
		objects.FieldKeyPriorityTier:    "P0",
		objects.FieldKeyEstimatedEffort: "2h",
	}
	errs := ValidateObjectIntent(t.Context(), Default(), objects.KindBacklogItem, KindTransition, IntentTransition, obj, nil)
	if !validationErrorsContainField(errs, FieldCRIShovelReady) {
		t.Fatalf("expected %s on thin in_progress transition, got %#v", FieldCRIShovelReady, errs)
	}
}

func TestValidateObjectIntent_plannedDoesNotUseShovelReadyOverlay(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	// Demote/pause lands on planned; overlay must not refuse (lifecycle token is the planned gate).
	obj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyID:              "BLI-thin-planned",
		objects.FieldKeyGoalRefs:        []any{"GOL-1"},
		objects.FieldKeyPriority:        "high",
		objects.FieldKeyPriorityPlanRef: "PRI-1",
		objects.FieldKeyMilestoneRefs:   []any{"MIL-1"},
		objects.FieldKeyPriorityTier:    "P0",
	}
	errs := ValidateObjectIntent(t.Context(), Default(), objects.KindBacklogItem, KindTransition, IntentTransition, obj, nil)
	if validationErrorsContainField(errs, FieldCRIShovelReady) {
		t.Fatalf("planned destination must not use shovel-ready overlay (demote recovery); got %#v", errs)
	}
}

func TestDecide_eraseCriticalRefusesWithoutReason(t *testing.T) {
	if os.Getenv(zqkenv.JobID()) != "" || os.Getenv(zqkenv.SchedulerJobID()) != "" {
		t.Skip("Flaky under scheduler concurrency")
	}
	t.Setenv(zqkenv.TestRoot(), "")
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	out := Decide(t.Context(), Default(), KindErase, MutationInput{
		Kind: objects.KindBacklogItem, ID: "BLI-x", Intent: IntentEraseLogical,
	})
	if out.Plan != PlanRefuse {
		t.Fatalf("plan=%s want refuse", out.Plan)
	}
}

func TestMembershipOverlay_bareForceDoesNotSkip(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyStatus:          objects.ObjectStatusDraft,
		objects.FieldKeyID:              "BLI-mem",
		objects.FieldKeyPriorityPlanRef: "PRI-active",
		objects.FieldKeyGoalRefs:        []any{"GOL-1"},
		objects.FieldKeyPriority:        "high",
		objects.FieldKeyMilestoneRefs:   []any{"MIL-1"},
		objects.FieldKeyPriorityTier:    "P0",
	}
	lookup := func(id string) (map[string]any, error) {
		if id == "PRI-active" {
			return map[string]any{
				objects.FieldKeyKind:   objects.KindPriorityPlan,
				objects.FieldKeyStatus: objects.ObjectStatusActive,
				objects.FieldKeyID:     id,
			}, nil
		}
		return nil, nil
	}
	normalCtx := t.Context()
	errs := ValidateObject(normalCtx, Default(), objects.KindBacklogItem, obj, lookup)
	found := false
	for _, e := range errs {
		if e.Rule == "execution_facing_membership" && strings.Contains(e.Message, "cannot link") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("normal context must not skip membership; errs=%#v", errs)
	}
	glass := pkgctx.WithLifecycleBreakGlass(t.Context(), "test membership break_glass")
	errs = ValidateObject(glass, Default(), objects.KindBacklogItem, obj, lookup)
	for _, e := range errs {
		if e.Rule == "execution_facing_membership" && strings.Contains(e.Message, "cannot link") {
			t.Fatalf("break_glass should skip membership; got %#v", e)
		}
	}
}

func TestValidateObject_selfRefsRefuseLoops(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	self := map[string]any{
		objects.FieldKeyKind:         objects.KindDecision,
		objects.FieldKeyStatus:       objects.ObjectStatusDraft,
		objects.FieldKeyID:           "DEC-1",
		objects.FieldKeyDecisionRefs: []any{"DEC-1"},
	}
	errs := ValidateObject(t.Context(), Default(), objects.KindDecision, self, nil)
	found := false
	for _, e := range errs {
		if e.Field == objects.FieldKeyDecisionRefs && strings.Contains(e.Message, "must not contain this object's id") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected self-id refuse, got %#v", errs)
	}
	lookup := ObjectLookup(func(id string) (map[string]any, error) {
		if id == "DEC-2" {
			return map[string]any{objects.FieldKeyDecisionRefs: []any{"DEC-1"}}, nil
		}
		return nil, nil
	})
	cycle := map[string]any{
		objects.FieldKeyKind:         objects.KindDecision,
		objects.FieldKeyStatus:       objects.ObjectStatusDraft,
		objects.FieldKeyID:           "DEC-1",
		objects.FieldKeyDecisionRefs: []any{"DEC-2"},
	}
	errs = ValidateObject(t.Context(), Default(), objects.KindDecision, cycle, lookup)
	found = false
	for _, e := range errs {
		if e.Field == objects.FieldKeyDecisionRefs && strings.Contains(e.Message, "must not form a 2-cycle") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 2-cycle refuse, got %#v", errs)
	}
}
