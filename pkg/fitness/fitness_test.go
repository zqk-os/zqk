package fitness_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/fitness"
)

func TestClassifyIssue_processFailureVsCompleteness(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   fitness.IssueInput
		want fitness.IssueClass
	}{
		{
			name: "invalid_lifecycle",
			in:   fitness.IssueInput{Tier: 1, Category: "lifecycle", Message: "Invalid lifecycle status 'complete' for kind 'agent_task'"},
			want: fitness.IssueClassProcessFailure,
		},
		{
			name: "precondition",
			in:   fitness.IssueInput{Tier: 1, Category: "lifecycle", Message: "status: Precondition not met for status 'complete' (Required: estimated_effort is set)"},
			want: fitness.IssueClassProcessFailure,
		},
		{
			name: "title_required",
			in:   fitness.IssueInput{Tier: 1, Category: "instance_validation", Message: "title: Field title is required"},
			want: fitness.IssueClassDataCompleteness,
		},
		{
			name: "rbac",
			in:   fitness.IssueInput{Tier: 1, Category: "policy", Message: "account is not RBAC-ready"},
			want: fitness.IssueClassEmploymentFitness,
		},
		{
			name: "orphan",
			in:   fitness.IssueInput{Tier: 1, Category: "integrity", Message: "Orphaned file on disk"},
			want: fitness.IssueClassReferentialIntegrity,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := fitness.ClassifyIssue(tc.in); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestShouldDemoteToError_onlyProcessFailure(t *testing.T) {
	t.Parallel()
	title := fitness.IssueInput{Tier: 1, Category: "instance_validation", Message: "title: Field title is required"}
	invalid := fitness.IssueInput{Tier: 1, Category: "lifecycle", Message: "Invalid lifecycle status 'complete' for kind 'agent_task'"}
	precond := fitness.IssueInput{Tier: 1, Category: "lifecycle", Message: "status: Precondition not met for status 'complete' (Required: estimated_effort is set)"}

	if fitness.ShouldDemoteToError("agent_task", "planned", []fitness.IssueInput{title}).Demote {
		t.Fatal("data_completeness must not demote")
	}
	if !fitness.ShouldDemoteToError("agent_task", "complete", []fitness.IssueInput{invalid}).Demote {
		t.Fatal("illegal status must demote")
	}
	if !fitness.ShouldDemoteToError("agent_task", "complete", []fitness.IssueInput{precond}).Demote {
		t.Fatal("status precondition must demote")
	}
	if fitness.ShouldDemoteToError("account", "active", []fitness.IssueInput{invalid}).Demote {
		t.Fatal("identity_governance must never autofix-demote")
	}
	if fitness.ShouldDemoteToError("agent_task", "archived", []fitness.IssueInput{title}).Demote {
		t.Fatal("archived + completeness must not demote")
	}
}

func TestVisibleOnSurface(t *testing.T) {
	t.Parallel()
	if fitness.VisibleOnSurface(fitness.SurfaceListDefault, fitness.IssueClassDataCompleteness) {
		t.Fatal("list_default must suppress completeness")
	}
	if !fitness.VisibleOnSurface(fitness.SurfaceListDefault, fitness.IssueClassProcessFailure) {
		t.Fatal("list_default must show process_failure")
	}
	if !fitness.VisibleOnSurface(fitness.SurfaceAuth, fitness.IssueClassEmploymentFitness) {
		t.Fatal("auth must show employment_fitness")
	}
	if fitness.VisibleOnSurface(fitness.SurfaceAdminForm, fitness.IssueClassEmploymentFitness) {
		t.Fatal("admin_form must suppress employment by default")
	}
}

func TestFamilyOfKind(t *testing.T) {
	t.Parallel()
	if fitness.FamilyOfKind("account") != fitness.KindFamilyIdentityGovernance {
		t.Fatal("account family")
	}
	if fitness.FamilyOfKind("agent_task") != fitness.KindFamilyWorkAttempt {
		t.Fatal("agent_task family")
	}
	if fitness.AutofixMayDemoteKind("priority_plan") {
		t.Fatal("priority_plan must not autofix-demote")
	}
}
