package ontology_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/ontology"
)

func TestSystemOntology(t *testing.T) {
	t.Parallel()
	o := ontology.SystemOntology()

	if o.ID != "zqk-system-ontology" {
		t.Errorf("Expected ID zqk-system-ontology, got %s", o.ID)
	}
	if o.Version != "1.0" {
		t.Errorf("Expected Version 1.0, got %s", o.Version)
	}
	if o.SchemaVersion != "2.0.0" {
		t.Errorf("Expected SchemaVersion 2.0.0, got %s", o.SchemaVersion)
	}

	expectedClasses := []string{
		objects.KindGoal, objects.KindMilestone, objects.KindWorkstream, objects.KindPriorityPlan,
		objects.KindBacklogItem, objects.KindRequirement, objects.KindCriteria, objects.KindTestCase,
		objects.KindRoadmap, objects.KindMission, objects.KindVision, objects.KindDecision,
		objects.KindComponent, objects.KindAccount,
	}

	for _, class := range expectedClasses {
		if _, ok := o.Classes[class]; !ok {
			t.Errorf("Missing expected class in SystemOntology: %s", class)
		}
	}
}

func TestSystemOntology_requirementCriteriaComposition(t *testing.T) {
	t.Parallel()
	o := ontology.SystemOntology()

	req, ok := o.Classes[objects.KindRequirement]
	if !ok {
		t.Fatal("requirement class missing")
	}
	critProp, ok := req.Properties[objects.FieldKeyCriteriaRefs]
	if !ok {
		t.Fatal("requirement must store criteria_refs (parent-owned composition)")
	}
	if critProp.EdgeRole != string(objects.EdgeRoleComposition) {
		t.Fatalf("requirement.criteria_refs edge_role = %q, want composition", critProp.EdgeRole)
	}
	if _, ok := req.Properties[objects.FieldKeyTestCaseRefs]; ok {
		t.Fatal("requirement must not store test_case_refs; occupancy is test_case.requirement_refs")
	}

	crit, ok := o.Classes[objects.KindCriteria]
	if !ok {
		t.Fatal("criteria class missing")
	}
	if _, ok := crit.Properties[objects.FieldKeyRequirementRefs]; ok {
		t.Fatal("criteria must not store requirement_refs; composition is requirement.criteria_refs")
	}
	if _, ok := crit.Properties[objects.FieldKeyMilestoneRefs]; ok {
		t.Fatal("criteria must not store milestone_refs; composition is milestone.criteria_refs")
	}
	if _, ok := crit.Properties[objects.FieldKeyGoalRefs]; !ok {
		t.Fatal("criteria must store goal_refs (child-owned membership)")
	}
	if _, ok := crit.Properties[objects.FieldKeyRelatedObjectRefs]; ok {
		t.Fatal("related_object_refs is untyped associate; do not project as a typed edge")
	}

	tc, ok := o.Classes[objects.KindTestCase]
	if !ok {
		t.Fatal("test_case class missing")
	}
	if _, ok := tc.Properties[objects.FieldKeyRequirementRefs]; !ok {
		t.Fatal("test_case must store requirement_refs (child-owned occupancy)")
	}

	goal, ok := o.Classes[objects.KindGoal]
	if !ok {
		t.Fatal("goal class missing")
	}
	if _, ok := goal.Properties[objects.FieldKeyMilestoneRefs]; ok {
		t.Fatal("goal must not store milestone_refs; occupancy is milestone.goal_refs")
	}
	if _, ok := goal.Properties[objects.FieldKeyRequirementRefs]; ok {
		t.Fatal("goal must not store requirement_refs; occupancy is requirement.goal_refs")
	}

	mil, ok := o.Classes[objects.KindMilestone]
	if !ok {
		t.Fatal("milestone class missing")
	}
	if _, ok := mil.Properties[objects.FieldKeyGoalRefs]; !ok {
		t.Fatal("milestone must store goal_refs (child-owned occupancy)")
	}
	if _, ok := mil.Properties[objects.FieldKeyCriteriaRefs]; ok {
		t.Fatal("milestone must not store criteria_refs; milestones summarize child backlog items")
	}
	if _, ok := mil.Properties[objects.FieldKeyRequirementRefs]; ok {
		t.Fatal("milestone must not store requirement_refs; occupancy is requirement.milestone_refs")
	}
}
