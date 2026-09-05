package authcred

import (
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestClassifyLane(t *testing.T) {
	if got := ClassifyLane(pkgctx.NewSystemSecurityContext()); got != LaneAdmin {
		t.Fatalf("system: %s", got)
	}
	planner := pkgctx.NewSecurityContext("ACC-PLAN", []string{"cap_orchestrator"}, []string{"read:*", PermissionAgentOrchestrate})
	if got := ClassifyLane(planner); got != LanePlanner {
		t.Fatalf("planner: %s", got)
	}
	doer := pkgctx.NewSecurityContext(DefaultSwarmWorkerAccount, []string{"swarm_worker"}, []string{"read:*", PermissionWriteCode})
	if got := ClassifyLane(doer); got != LaneDoer {
		t.Fatalf("doer: %s", got)
	}
}

func TestIsPlannerSeatRef_usesDirectory(t *testing.T) {
	dir := MemoryDirectory{
		RoleList: []RoleRecord{
			{
				ID:          "ROL-AGENT-ORCH",
				RoleID:      "agent-cap-orchestrator",
				Aliases:     []string{"cap_orchestrator"},
				Permissions: []string{PermissionAgentOrchestrate},
			},
			{
				ID:          "ROL-AGENT-SWARM",
				RoleID:      "agent-swarm-worker",
				Aliases:     []string{"swarm_worker"},
				Permissions: []string{PermissionWriteCode},
			},
		},
		AccountList: []AccountRecord{
			{
				ID:         "ACC-PLAN",
				Username:   "cursor_tpm",
				PersonaRef: "PER-PLAN-001",
				Roles:      []string{"cap_orchestrator"},
			},
		},
	}
	if !IsPlannerSeatRef(dir, "cap_orchestrator") {
		t.Fatal("alias on planner role")
	}
	if !IsPlannerSeatRef(dir, "agent-cap-orchestrator") {
		t.Fatal("role_id")
	}
	if !IsPlannerSeatRef(dir, "PER-PLAN-001") {
		t.Fatal("persona bound to planner account")
	}
	if IsPlannerSeatRef(dir, "swarm_worker") {
		t.Fatal("doer role is not planner")
	}
	if IsPlannerSeatRef(dir, "PER-UNKNOWN") {
		t.Fatal("unbound persona")
	}
	if IsPlannerSeatRef(dir, "PER-ORCH-MISSING") {
		t.Fatal("substring orch must not imply planner")
	}
	if IsPlannerSeatRef(nil, "cap_orchestrator") {
		t.Fatal("nil directory")
	}
}

func TestWriteReadIdentityStatus(t *testing.T) {
	root := t.TempDir()
	sec := pkgctx.NewSecurityContext("ACC-PLAN", []string{"cap_orchestrator"}, []string{PermissionAgentOrchestrate})
	sec.PersonaID = "PER-TPM"
	WriteIdentityStatus(root, SnapshotFromSecurityContext(sec, nil))
	got, ok := ReadIdentityStatus(root)
	if !ok {
		t.Fatal("expected snapshot")
	}
	if got.AccountID != "ACC-PLAN" || got.Lane != LanePlanner || got.PersonaID != "PER-TPM" {
		t.Fatalf("%+v", got)
	}
	want := filepath.Join(root, paths.ProjectDataDir, paths.StateDir, paths.IdentityStatusFile)
	if IdentityStatusPath(root) != want {
		t.Fatalf("path %s", IdentityStatusPath(root))
	}
}
