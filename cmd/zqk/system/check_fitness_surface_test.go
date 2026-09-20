package system

import "testing"

func TestFilterIssuesForSurface(t *testing.T) {
	t.Parallel()
	issues := []Issue{
		{Tier: 1, Category: "lifecycle", Message: "Invalid lifecycle status 'complete' for kind 'agent_task'"},
		{Tier: 1, Category: "instance_validation", Message: "title: Field title is required"},
		{Tier: 1, Category: "policy", Message: "account is not RBAC-ready"},
	}
	list := filterIssuesForSurface("list_default", issues)
	if len(list) != 1 || list[0].IssueClass != "process_failure" {
		t.Fatalf("list_default got %#v", list)
	}
	auth := filterIssuesForSurface("auth", issues)
	if len(auth) != 2 {
		t.Fatalf("auth want process+employment, got %#v", auth)
	}
	admin := filterIssuesForSurface("admin_form", issues)
	if len(admin) != 1 || admin[0].IssueClass != "data_completeness" {
		t.Fatalf("admin_form got %#v", admin)
	}
}
