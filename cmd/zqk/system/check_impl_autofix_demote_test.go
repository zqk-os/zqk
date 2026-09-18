package system

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestShouldDemoteStatusToErrorForUnresolvedIssues_policy(t *testing.T) {
	t.Parallel()
	tier1Block := Issue{
		Tier:        1,
		Category:    "instance_validation",
		Message:     "title: Field title is required",
		AutoFixable: false,
	}
	invalidATK := Issue{
		Tier:        1,
		Category:    "lifecycle",
		Message:     "Invalid lifecycle status 'complete' for kind 'agent_task'",
		AutoFixable: true,
	}
	precond := Issue{
		Tier:        1,
		Category:    "lifecycle",
		Message:     "status: Precondition not met for status 'complete' (Required: estimated_effort is set)",
		AutoFixable: false,
	}
	if !shouldDemoteStatusToErrorForUnresolvedIssues("agent_task", objects.ObjectStatusComplete, []Issue{invalidATK}) {
		t.Fatal("illegal agent_task complete must demote to error")
	}
	if !shouldDemoteStatusToErrorForUnresolvedIssues("agent_task", objects.ObjectStatusComplete, []Issue{precond}) {
		t.Fatal("complete without estimated_effort must demote to error")
	}
	// REQ-KERNEL-LIFECYCLE-FITNESS-001: completeness is not process_failure.
	if shouldDemoteStatusToErrorForUnresolvedIssues("agent_task", objects.ObjectStatusPlanned, []Issue{tier1Block}) {
		t.Fatal("data_completeness must not demote planned→error")
	}
	if shouldDemoteStatusToErrorForUnresolvedIssues("agent_task", objects.ObjectStatusArchived, []Issue{tier1Block}) {
		t.Fatal("true terminal archived must not demote for unrelated field issues")
	}
	if shouldDemoteStatusToErrorForUnresolvedIssues("account", objects.ObjectStatusActive, []Issue{invalidATK}) {
		t.Fatal("identity_governance must never autofix-demote")
	}
}

func TestWouldDemoteStatusToErrorForUnresolvedIssues_policy(t *testing.T) {
	t.Parallel()
	tier2HCI := Issue{
		Tier:        2,
		Category:    "instance_validation",
		Message:     "goal_refs/requirement_refs: backlog_item must have at least one goal_ref",
		AutoFixable: false,
	}
	tier1LifecycleInvalid := Issue{
		Tier:        1,
		Category:    "instance_validation",
		Message:     "status: Invalid lifecycle status 'archived' for kind 'question'",
		AutoFixable: false,
	}
	tier1Block := Issue{
		Tier:        1,
		Category:    "instance_validation",
		Message:     "title: Field title is required",
		AutoFixable: false,
	}
	integrity := Issue{
		Tier:        1,
		Category:    "integrity",
		Message:     "Orphaned file on disk",
		AutoFixable: true,
	}
	precond := Issue{
		Tier:        1,
		Category:    "lifecycle",
		Message:     "status: Precondition not met for status 'complete' (Required: estimated_effort is set)",
		AutoFixable: false,
	}

	cases := []struct {
		name   string
		kind   string
		status string
		issues []Issue
		want   bool
	}{
		{name: "tier2_does_not_demote_complete", kind: "backlog_item", status: objects.ObjectStatusComplete, issues: []Issue{tier2HCI}, want: false},
		{name: "tier2_does_not_demote_archived", kind: "backlog_item", status: objects.ObjectStatusArchived, issues: []Issue{tier2HCI}, want: false},
		{name: "tier2_does_not_demote_exploring", kind: "backlog_item", status: objects.ObjectStatusExploring, issues: []Issue{tier2HCI}, want: false},
		{name: "invalid_lifecycle_demotes", kind: "question", status: objects.ObjectStatusComplete, issues: []Issue{tier1LifecycleInvalid}, want: true},
		{name: "precondition_demotes_complete", kind: "agent_task", status: objects.ObjectStatusComplete, issues: []Issue{precond}, want: true},
		{name: "completeness_does_not_demote_planned", kind: "agent_task", status: objects.ObjectStatusPlanned, issues: []Issue{tier1Block}, want: false},
		{name: "completeness_does_not_demote_archived", kind: "agent_task", status: objects.ObjectStatusArchived, issues: []Issue{tier1Block}, want: false},
		{name: "completeness_does_not_demote_answered", kind: "question", status: objects.ObjectStatusAnswered, issues: []Issue{tier1Block}, want: false},
		{name: "already_error", kind: "agent_task", status: objects.ObjectStatusError, issues: []Issue{tier1Block}, want: false},
		{name: "integrity_ignored", kind: "agent_task", status: objects.ObjectStatusPlanned, issues: []Issue{integrity}, want: false},
		{name: "mixed_completeness_and_invalid", kind: "agent_task", status: objects.ObjectStatusPlanned, issues: []Issue{tier2HCI, tier1LifecycleInvalid}, want: true},
		{name: "account_never_demotes", kind: "account", status: objects.ObjectStatusActive, issues: []Issue{tier1LifecycleInvalid}, want: false},
		{name: "priority_plan_never_demotes", kind: "priority_plan", status: objects.ObjectStatusActive, issues: []Issue{tier1LifecycleInvalid}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := wouldDemoteStatusToErrorForUnresolvedIssues(tc.kind, tc.status, tc.issues)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
