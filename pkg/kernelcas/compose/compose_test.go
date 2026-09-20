package compose

import (
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
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
			if r.ID == "trait_transition_estimated_effort" {
				foundTransition = true
			}
			if r.ID == "bli_execution_facing_membership" {
				foundMembership = true
			}
		}
	}
	if !foundTransition {
		t.Fatal("expected trait_transition_estimated_effort on transition definition")
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
	// Hold overlays satisfied; CRI-SHOVEL-READY refs missing. TRACK: CRIT-1785885889228395000-15c56d02
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

func TestValidateObjectIntent_occupiableRequiresClaimedByOnInProgress(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}

	objWithoutClaim := map[string]any{
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		objects.FieldKeyID:     "ATK-1786411312347141000-abcd1234",
	}
	errs := ValidateObjectIntent(t.Context(), Default(), objects.KindAgentTask, KindTransition, IntentTransition, objWithoutClaim, nil)
	if len(errs) == 0 {
		t.Fatal("transitioning occupiable kind to in_progress without claimed_by must fail")
	}

	objWithClaim := map[string]any{
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyStatus:    objects.ObjectStatusInProgress,
		objects.FieldKeyID:        "ATK-1786411312347141000-abcd1234",
		objects.FieldKeyClaimedBy: "agent-1",
	}
	errs = ValidateObjectIntent(t.Context(), Default(), objects.KindAgentTask, KindTransition, IntentTransition, objWithClaim, nil)
	for _, e := range errs {
		if strings.Contains(e.Message, "claimed_by") {
			t.Fatalf("unexpected claimed_by error when claimed_by is present: %v", e)
		}
	}
}

// TestValidateObjectIntent_BLI_TDE_LifecyclePromoteClaim tests the contract for
// BLI-TDE-LIFECYCLE-PROMOTE-CLAIM-001: occupiable kinds require claimed_by on in_progress,
// while non-occupiable kinds (e.g. backlog_item) are not forced to hold claimed_by.
func TestValidateObjectIntent_BLI_TDE_LifecyclePromoteClaim(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}

	// backlog_item is not occupiable: in_progress should not fail for missing claimed_by
	bliObj := map[string]any{
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		objects.FieldKeyID:     "BLI-1786411312347141000-abcd1234",
	}
	errs := ValidateObjectIntent(t.Context(), Default(), objects.KindBacklogItem, KindTransition, IntentTransition, bliObj, nil)
	for _, e := range errs {
		if strings.Contains(e.Message, "claimed_by") {
			t.Fatalf("backlog_item must not require claimed_by: %v", e)
		}
	}
}

func TestDecide_eraseCriticalRefusesWithoutReason(t *testing.T) {
	if zqkenv.JobID().Get() != "" || zqkenv.SchedulerJobID().Get() != "" {
		t.Skip("Flaky under scheduler concurrency")
	}
	t.Setenv(zqkenv.TestRoot().Name(), "")
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

func TestValidateObject_RefuseUnknownFieldsAllowsCompositionFields(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}

	// 1. Backlog item with legitimate composition fields (claimed_by, claimed_at, effort_variance, percent_complete, tags)
	validObj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyID:              "BLI-1785008248438506000-22976ac6",
		objects.FieldKeyTitle:           "Valid Title For BLI",
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef: "PRI-1786084814786868000-df9be3b3",
		objects.FieldKeyMilestoneRefs:   []any{"MIL-001"},
		objects.FieldKeyPriorityTier:    "P1",
		objects.FieldKeyClaimedBy:       "agent-42",
		objects.FieldKeyClaimedAt:       "2026-09-01T12:00:00Z",
		objects.FieldKeyEffortVariance:  15.5,
		objects.FieldKeyPercentComplete: 50.0,
		objects.FieldKeyTags:            []any{"tag1", "tag2"},
	}
	errs := ValidateObject(t.Context(), Default(), objects.KindBacklogItem, validObj, nil)
	for _, e := range errs {
		if e.Rule == "unknown_field" {
			t.Errorf("unexpected unknown_field error for legitimate composition field: %v", e)
		}
	}

	// 2. Backlog item with truly unknown field
	bogusObj := map[string]any{
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyID:              "BLI-1785008248438506000-22976ac6",
		objects.FieldKeyTitle:           "Valid Title For BLI",
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef: "PRI-1786084814786868000-df9be3b3",
		objects.FieldKeyMilestoneRefs:   []any{"MIL-001"},
		objects.FieldKeyPriorityTier:    "P1",
		"totally_bogus_field_xyz":       "bad",
	}
	bogusErrs := ValidateObject(t.Context(), Default(), objects.KindBacklogItem, bogusObj, nil)
	foundBogus := false
	for _, e := range bogusErrs {
		if e.Field == "totally_bogus_field_xyz" && e.Rule == "unknown_field" {
			foundBogus = true
			break
		}
	}
	if !foundBogus {
		t.Fatalf("expected unknown_field error for totally_bogus_field_xyz, got: %#v", bogusErrs)
	}
}

func TestValidateObject_BacklogItemPriorityPairingAndValidation(t *testing.T) {
	baseBLI := func() map[string]any {
		return map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-1785008248438506000-22976ac6",
			objects.FieldKeyTitle:           "Valid Title For BLI",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-1786084814786868000-df9be3b3",
			objects.FieldKeyDescription:     "A valid description for testing",
			objects.FieldKeyGoalRefs:        []any{"GOAL-001"},
		}
	}

	// 1. Only priority provided -> passes priority check
	objOnlyPri := baseBLI()
	objOnlyPri[objects.FieldKeyPriority] = "high"
	errs := ValidateObject(t.Context(), Default(), objects.KindBacklogItem, objOnlyPri, nil)
	for _, e := range errs {
		if e.Field == objects.FieldKeyPriority || e.Field == objects.FieldKeyPriorityTier {
			t.Errorf("unexpected priority error with only priority set: %v", e)
		}
	}

	// 2. Only priority_tier provided -> passes priority check
	objOnlyTier := baseBLI()
	objOnlyTier[objects.FieldKeyPriorityTier] = "P1"
	errs = ValidateObject(t.Context(), Default(), objects.KindBacklogItem, objOnlyTier, nil)
	for _, e := range errs {
		if e.Field == objects.FieldKeyPriority || e.Field == objects.FieldKeyPriorityTier {
			t.Errorf("unexpected priority error with only priority_tier set: %v", e)
		}
	}

	// 3. Both provided with matching legitimate values -> passes
	objBoth := baseBLI()
	objBoth[objects.FieldKeyPriority] = "high"
	objBoth[objects.FieldKeyPriorityTier] = "P1"
	errs = ValidateObject(t.Context(), Default(), objects.KindBacklogItem, objBoth, nil)
	for _, e := range errs {
		if e.Field == objects.FieldKeyPriority || e.Field == objects.FieldKeyPriorityTier {
			t.Errorf("unexpected priority error with both legitimate values set: %v", e)
		}
	}

	// 4. Both provided with different legitimate values -> passes (providing both just validates they are legitimate values)
	objBothDiff := baseBLI()
	objBothDiff[objects.FieldKeyPriority] = "high"
	objBothDiff[objects.FieldKeyPriorityTier] = "P2"
	errs = ValidateObject(t.Context(), Default(), objects.KindBacklogItem, objBothDiff, nil)
	for _, e := range errs {
		if e.Field == objects.FieldKeyPriority || e.Field == objects.FieldKeyPriorityTier {
			t.Errorf("unexpected priority error with both legitimate values set: %v", e)
		}
	}

	// 5. Neither provided when planned -> fails priority requirement
	objNeither := baseBLI()
	errs = ValidateObject(t.Context(), Default(), objects.KindBacklogItem, objNeither, nil)
	foundPriorityReq := false
	for _, e := range errs {
		if (e.Field == objects.FieldKeyPriority || e.Field == objects.FieldKeyPriorityTier) &&
			e.Message == "priority or priority_tier must be set when backlog_item is planned or in_progress" {
			foundPriorityReq = true
			break
		}
	}
	if !foundPriorityReq {
		t.Errorf("expected priority requirement error when neither is set on planned BLI, got: %#v", errs)
	}

	// 6. Invalid priority -> fails legitimate values validation
	objInvalidPri := baseBLI()
	objInvalidPri[objects.FieldKeyPriority] = "ultra_urgent"
	errs = ValidateObject(t.Context(), Default(), objects.KindBacklogItem, objInvalidPri, nil)
	foundInvalidPri := false
	for _, e := range errs {
		if e.Field == objects.FieldKeyPriority && e.Message == "priority must be one of (critical, high, medium, low)" {
			foundInvalidPri = true
			break
		}
	}
	if !foundInvalidPri {
		t.Errorf("expected error for invalid priority, got: %#v", errs)
	}

	// 7. Invalid priority_tier -> fails legitimate values validation
	objInvalidTier := baseBLI()
	objInvalidTier[objects.FieldKeyPriorityTier] = "P9"
	errs = ValidateObject(t.Context(), Default(), objects.KindBacklogItem, objInvalidTier, nil)
	foundInvalidTier := false
	for _, e := range errs {
		if e.Field == objects.FieldKeyPriorityTier && e.Message == "priority_tier must be one of (P0, P1, P2, P3)" {
			foundInvalidTier = true
			break
		}
	}
	if !foundInvalidTier {
		t.Errorf("expected error for invalid priority_tier, got: %#v", errs)
	}
}

func TestValidateObject_CASBoundaryRequiresDescription(t *testing.T) {
	ResetDefaultForTest()
	// Goal without description at 'active' (non-preliminary) status must fail
	goalNoDesc := map[string]any{
		objects.FieldKeyID:     "GOAL-TEST-001",
		objects.FieldKeyKind:   objects.KindGoal,
		objects.FieldKeyTitle:  "Test Goal",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyMetric: "latency",
		objects.FieldKeyTarget: "p99 < 50ms",
	}
	errs := ValidateObject(t.Context(), Default(), objects.KindGoal, goalNoDesc, nil)
	foundDescErr := false
	for _, e := range errs {
		if e.Field == "description" && strings.Contains(e.Message, "description must be populated (CAS boundary requirement)") {
			foundDescErr = true
			break
		}
	}
	if !foundDescErr {
		t.Errorf("expected CAS boundary description requirement error on non-preliminary goal, got: %#v", errs)
	}

	// Requirement without description at 'active' must also fail
	reqNoDesc := map[string]any{
		objects.FieldKeyID:     "REQ-TEST-001",
		objects.FieldKeyKind:   objects.KindRequirement,
		objects.FieldKeyTitle:  "Test Requirement",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	errs = ValidateObject(t.Context(), Default(), objects.KindRequirement, reqNoDesc, nil)
	foundDescErr = false
	for _, e := range errs {
		if e.Field == "description" && strings.Contains(e.Message, "description must be populated (CAS boundary requirement)") {
			foundDescErr = true
			break
		}
	}
	if !foundDescErr {
		t.Errorf("expected CAS boundary description requirement error on non-preliminary requirement, got: %#v", errs)
	}

	// Goal with valid description must pass description gate
	goalWithDesc := map[string]any{
		objects.FieldKeyID:     "GOAL-TEST-001",
		objects.FieldKeyKind:   objects.KindGoal,
		objects.FieldKeyTitle:  "Test Goal",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
		objects.FieldKeyMetric: "latency",
		objects.FieldKeyTarget: "p99 < 50ms",
		"description":          "A substantive and detailed description of the goal scope.",
	}
	errs = ValidateObject(t.Context(), Default(), objects.KindGoal, goalWithDesc, nil)
	for _, e := range errs {
		if e.Field == "description" && strings.Contains(e.Message, "description must be populated") {
			t.Errorf("unexpected description error on goal with valid description: %v", e)
		}
	}

	// Goal at preliminary status ('conceptual') skips the requirement
	goalPreliminary := map[string]any{
		objects.FieldKeyID:     "GOAL-TEST-001",
		objects.FieldKeyKind:   objects.KindGoal,
		objects.FieldKeyTitle:  "Test Goal",
		objects.FieldKeyStatus: objects.ObjectStatusConceptual,
	}
	errs = ValidateObject(t.Context(), Default(), objects.KindGoal, goalPreliminary, nil)
	for _, e := range errs {
		if e.Field == "description" && strings.Contains(e.Message, "description must be populated") {
			t.Errorf("unexpected description error on preliminary goal: %v", e)
		}
	}
}

func TestValidateObjectIntent_requirementTracePipelineCriteria(t *testing.T) {
	ResetDefaultForTest()
	if err := WarmDefaultRegistry(""); err != nil {
		t.Fatal(err)
	}
	originated := map[string]any{
		objects.FieldKeyID:          "REQ-trace-001",
		objects.FieldKeyKind:        objects.KindRequirement,
		objects.FieldKeyTitle:       "Trace pipeline gate",
		objects.FieldKeyStatus:      objects.ObjectStatusOriginated,
		objects.FieldKeyDescription: "A substantive requirement description that crosses the CAS description barrier.",
	}
	for _, intent := range []struct {
		pk string
		in string
	}{
		{KindCreate, IntentCreate},
		{KindTransition, IntentTransition},
	} {
		errs := ValidateObjectIntent(t.Context(), Default(), objects.KindRequirement, intent.pk, intent.in, originated, nil)
		found := false
		for _, e := range errs {
			if e.Field == objects.FieldKeyCriteriaRefs && strings.Contains(e.Message, "gen-trace-pipeline") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s/%s: expected criteria_refs trace-pipeline refuse, got %#v", intent.pk, intent.in, errs)
		}
	}

	updateErrs := ValidateObject(t.Context(), Default(), objects.KindRequirement, originated, nil)
	for _, e := range updateErrs {
		if e.Field == objects.FieldKeyCriteriaRefs && strings.Contains(e.Message, "gen-trace-pipeline") {
			t.Fatalf("update of originated REQ without criteria_refs must stay editable for residual repair, got %#v", updateErrs)
		}
	}

	conceptual := map[string]any{
		objects.FieldKeyID:          "REQ-trace-002",
		objects.FieldKeyKind:        objects.KindRequirement,
		objects.FieldKeyTitle:       "Trace pipeline draft",
		objects.FieldKeyStatus:      objects.ObjectStatusConceptual,
		objects.FieldKeyDescription: "Draft plane requirement may exist before gen-trace-pipeline.",
	}
	errs := ValidateObjectIntent(t.Context(), Default(), objects.KindRequirement, KindCreate, IntentCreate, conceptual, nil)
	for _, e := range errs {
		if e.Field == objects.FieldKeyCriteriaRefs {
			t.Fatalf("conceptual REQ must skip trace-pipeline overlay, got %#v", errs)
		}
	}

	linked := map[string]any{
		objects.FieldKeyID:           "REQ-trace-003",
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyTitle:        "Trace pipeline linked",
		objects.FieldKeyStatus:       objects.ObjectStatusOriginated,
		objects.FieldKeyDescription:  "Linked requirement with a criteria_ref from gen-trace-pipeline.",
		objects.FieldKeyCriteriaRefs: []any{"CRIT-trace-001"},
	}
	errs = ValidateObjectIntent(t.Context(), Default(), objects.KindRequirement, KindTransition, IntentTransition, linked, nil)
	for _, e := range errs {
		if e.Field == objects.FieldKeyCriteriaRefs {
			t.Fatalf("originated REQ with criteria_refs must pass, got %#v", errs)
		}
	}
}
