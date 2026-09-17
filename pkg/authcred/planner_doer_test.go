package authcred

import (
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestRoleMatches_IDAndRoleID(t *testing.T) {
	if !RoleMatches("agent-swarm-worker", "ROL-AGENT-SWARM", "agent-swarm-worker") {
		t.Fatal("role_id should match")
	}
	if RoleMatches("swarm_worker", "ROL-AGENT-SWARM", "agent-swarm-worker") {
		t.Fatal("aliases are on RoleRecord, not RoleMatches(id, role_id)")
	}
	swarm := RoleRecord{
		ID:      "ROL-AGENT-SWARM",
		RoleID:  "agent-swarm-worker",
		Aliases: []string{"swarm_worker"},
	}
	if !swarm.Matches("swarm_worker") {
		t.Fatal("alias on the role object")
	}
	if swarm.Matches("coder_agent") {
		t.Fatal("coder_agent must not match swarm role")
	}
}

func TestExpandAssignedRoles_IncludesCanonicalFromDirectory(t *testing.T) {
	dir := MemoryDirectory{RoleList: []RoleRecord{{
		ID:      "ROL-AGENT-SWARM",
		RoleID:  "agent-swarm-worker",
		Aliases: []string{"swarm_worker"},
	}}}
	got := ExpandAssignedRoles([]string{"swarm_worker"}, dir)
	found := false
	for _, g := range got {
		if g == "agent-swarm-worker" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected agent-swarm-worker in %v", got)
	}
}

func TestValidateLanePermissions(t *testing.T) {
	if err := ValidateDoerPermissions([]string{"read:*", "write:code", "write:agent_task"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDoerPermissions([]string{"write:priority_plan"}); err == nil {
		t.Fatal("expected doer deny for write:priority_plan")
	}
	if err := ValidatePlannerPermissions([]string{"read:*", "write:priority_plan"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePlannerPermissions([]string{"write:code"}); err == nil {
		t.Fatal("expected planner deny for write:code")
	}
}

func TestMayOrchestrateAndDeny(t *testing.T) {
	doer := pkgctx.NewSecurityContext("ACC-DOER", []string{"swarm_worker"}, []string{"read:*", "write:code", "write:agent_task"})
	if MayOrchestratePeers(doer) {
		t.Fatal("doer should not orchestrate")
	}
	if err := DenyOrchestrateIfDoer(doer); err == nil || !strings.Contains(err.Error(), "POL-AGENT-PLANNER-DOER-001") {
		t.Fatalf("expected doer deny, got %v", err)
	}
	if err := DenyPeerSteerIfDoer(doer, "peer-1"); err == nil {
		t.Fatal("expected peer steer deny")
	}
	if err := DenyPeerSteerIfDoer(doer, ""); err != nil {
		t.Fatal(err)
	}

	planner := pkgctx.NewSecurityContext("ACC-PLAN", []string{"agent-cap-orchestrator"}, []string{"read:*", "write:priority_plan", PermissionAgentOrchestrate})
	if !MayOrchestratePeers(planner) {
		t.Fatal("planner should orchestrate")
	}
	if err := DenyOrchestrateIfDoer(planner); err != nil {
		t.Fatal(err)
	}
}

func TestIsStrategicKernelKind(t *testing.T) {
	if !IsStrategicKernelKind(objects.KindPriorityPlan) {
		t.Fatal("priority_plan should be strategic")
	}
	if IsStrategicKernelKind(objects.KindAgentTask) {
		t.Fatal("agent_task is doer lane")
	}
}

func TestHasWriteCode(t *testing.T) {
	planner := pkgctx.NewSecurityContext("ACC-P", []string{"owner"}, []string{"read:*", "write:priority_plan"})
	if HasWriteCode(planner) {
		t.Fatal("planner must not have write:code")
	}
	doer := pkgctx.NewSecurityContext("ACC-D", []string{"swarm_worker"}, []string{"write:code"})
	if !HasWriteCode(doer) {
		t.Fatal("doer should have write:code")
	}
}
