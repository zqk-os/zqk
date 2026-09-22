// BLI-STARTER-COMMUNITY-035 / PRI-STARTER-COMMUNITY-035 coverage elevation
package fitness

import "testing"

func TestClassifyAndFilterRemainingBranches(t *testing.T) {
	t.Parallel()
	if ClassifyIssue(IssueInput{Category: "lifecycle", Message: "other lifecycle"}) != IssueClassProcessFailure {
		t.Fatal("lifecycle cat")
	}
	if ClassifyIssue(IssueInput{Category: "policy", Message: "x"}) != IssueClassPolicyGate {
		t.Fatal("policy cat")
	}
	if ClassifyIssue(IssueInput{Category: "reference", Message: "x"}) != IssueClassReferentialIntegrity {
		t.Fatal("ref cat")
	}
	if ClassifyIssue(IssueInput{Message: "broken reference foo"}) != IssueClassReferentialIntegrity {
		t.Fatal("broken ref")
	}
	if ClassifyIssue(IssueInput{Message: "not allowed here"}) != IssueClassPolicyGate {
		t.Fatal("not allowed")
	}
	if ClassifyIssue(IssueInput{Message: "field is empty"}) != IssueClassDataCompleteness {
		t.Fatal("empty")
	}
	if !MayDemoteLifecycleToError(IssueClassProcessFailure) || MayDemoteLifecycleToError(IssueClassPolicyGate) {
		t.Fatal("may demote")
	}
	if FamilyOfKind("scheduler_job") != KindFamilyWorkAttempt {
		t.Fatal("work")
	}
	if FamilyOfKind("technical_debt") != KindFamilyExecutionWork {
		t.Fatal("exec")
	}
	if FamilyOfKind("priority_plan") != KindFamilyIdentityGovernance {
		t.Fatal("ident")
	}
	if FamilyOfKind("requirement") != KindFamilyStrategicContent {
		t.Fatal("strat")
	}
	if FamilyOfKind("base_metric") != KindFamilyTelemetry {
		t.Fatal("telemetry")
	}
	if FamilyOfKind("unknown_kind") != KindFamilyOther {
		t.Fatal("other")
	}
	if got := FilterClasses(Surface("nope"), []IssueClass{IssueClassProcessFailure}); len(got) != 0 {
		t.Fatal("unknown surface")
	}
	got := FilterClasses(SurfaceAuth, []IssueClass{IssueClassProcessFailure, IssueClassDataCompleteness})
	if len(got) != 1 || got[0] != IssueClassProcessFailure {
		t.Fatalf("%v", got)
	}
	if FilterClasses(SurfaceAuth, nil) != nil {
		t.Fatal("empty in")
	}
}

func TestShouldDemote_TerminalsAndReasons(t *testing.T) {
	t.Parallel()
	if ShouldDemoteToError("agent_task", "", []IssueInput{{Message: "Invalid lifecycle status"}}).Demote {
		t.Fatal("empty status")
	}
	if ShouldDemoteToError("agent_task", "error", []IssueInput{{Message: "Invalid lifecycle status"}}).Demote {
		t.Fatal("already error")
	}
	d := ShouldDemoteToError("agent_task", "archived", []IssueInput{{Message: "Invalid lifecycle status x"}})
	if !d.Demote {
		t.Fatal("archived illegal")
	}
	if ShouldDemoteToError("agent_task", "archived", []IssueInput{{Message: "Precondition not met for status"}}).Demote {
		t.Fatal("archived without invalid token")
	}
	d = ShouldDemoteToError("agent_task", "planned", []IssueInput{{Category: "lifecycle", Message: ""}})
	if !d.Demote || d.Reason != "process_failure" {
		t.Fatalf("%+v", d)
	}
	if DemoteReasonFromIssues([]IssueInput{{Category: "integrity", Message: "x"}}) != "process_failure" {
		t.Fatal("integrity skip")
	}
	if DemoteReasonFromIssues([]IssueInput{{Message: "Precondition not met for status y"}}) == "process_failure" {
		t.Fatal("prefer precondition msg")
	}
	if DemoteReasonFromIssues([]IssueInput{{Message: "generic process"}, {Category: "lifecycle", Message: "Invalid lifecycle status z"}}) == "generic process" {
		t.Fatal("prefer invalid")
	}
}
