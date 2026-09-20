package app

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/authcred"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// INV-RBAC-PD-001 — planner denied code; doer denied strategic write + orchestration.
// TST-1785905543229739000-341d327c
func TestINV_RBAC_PD_001_PlannerDoerSplit(t *testing.T) {
	const (
		doerACC     = "ACC-TEST-DOER-001"
		plannerACC  = "ACC-TEST-PLAN-001"
		doerPersona = "PER-TEST-DOER"
		planPersona = "PER-TEST-PLAN"
	)

	projectRoot := t.TempDir()
	createSchemas(t, projectRoot)
	writeBoundAccountFixture(t, projectRoot, doerACC, []string{"swarm_worker"}, doerPersona)
	writeBoundAccountFixture(t, projectRoot, plannerACC, []string{"agent-cap-orchestrator"}, planPersona)
	// Second account overwrote index — fix by writing both into index.
	writeDualAccountIndex(t, projectRoot, map[string]string{
		doerACC:    "doerhash001",
		plannerACC: "planhash001",
	}, map[string]string{
		doerACC:    "id: " + doerACC + "\nkind: account\nroles:\n  - swarm_worker\npersona_ref: " + doerPersona + "\n",
		plannerACC: "id: " + plannerACC + "\nkind: account\nroles:\n  - agent-cap-orchestrator\npersona_ref: " + planPersona + "\n",
	})
	writeRoleFixture(t, projectRoot, "ROL-AGENT-SWARM", "agent-swarm-worker", []string{"swarm_worker"}, []string{"read:*", "write:code", "write:agent_task", "write:audit_event"})
	writeRoleFixture(t, projectRoot, "ROL-AGENT-ORCH", "agent-cap-orchestrator", []string{"cap_orchestrator"}, []string{"read:*", "write:priority_plan", "write:agent_instruction", authcred.PermissionAgentOrchestrate})

	t.Run("doer_loads_swarm_perms_via_alias", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		home := t.TempDir()
		t.Setenv(zqkenv.OSHome().Name(), home)
		t.Setenv(zqkenv.APIKey().Name(), doerACC)
		t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

		if err := AuthMiddleware(cmd, projectRoot); err != nil {
			t.Fatalf("auth: %v", err)
		}
		sec := pkgctx.GetSecurityContext(cmd.Context())
		if sec == nil {
			t.Fatal("nil sec")
		}
		if !slices.Contains(sec.Permissions, "write:agent_task") {
			t.Fatalf("doer missing write:agent_task: %v", sec.Permissions)
		}
		if !slices.Contains(sec.Permissions, "write:code") {
			t.Fatalf("doer missing write:code: %v", sec.Permissions)
		}
		if err := storage.CheckPermission(sec, "write", objects.KindPriorityPlan); err == nil {
			t.Fatal("doer must be denied write priority_plan")
		}
		if err := storage.CheckPermission(sec, "write", objects.KindAgentTask); err != nil {
			t.Fatalf("doer must write agent_task: %v", err)
		}
		if err := authcred.DenyOrchestrateIfDoer(sec); err == nil {
			t.Fatal("doer must be denied orchestrate")
		}
		if authcred.ValidateDoerPermissions(sec.Permissions) != nil {
			t.Fatalf("doer perms failed matrix: %v", sec.Permissions)
		}
	})

	t.Run("planner_denied_code_allowed_orchestrate", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		home := t.TempDir()
		t.Setenv(zqkenv.OSHome().Name(), home)
		t.Setenv(zqkenv.APIKey().Name(), plannerACC)
		t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

		if err := AuthMiddleware(cmd, projectRoot); err != nil {
			t.Fatalf("auth: %v", err)
		}
		sec := pkgctx.GetSecurityContext(cmd.Context())
		if authcred.HasWriteCode(sec) {
			t.Fatal("planner must not have write:code")
		}
		if err := storage.CheckPermission(sec, "write", objects.KindPriorityPlan); err != nil {
			t.Fatalf("planner must write priority_plan: %v", err)
		}
		if err := authcred.DenyOrchestrateIfDoer(sec); err != nil {
			t.Fatal(err)
		}
		if authcred.ValidatePlannerPermissions(sec.Permissions) != nil {
			t.Fatalf("planner perms failed matrix: %v", sec.Permissions)
		}
	})
}

func writeDualAccountIndex(t *testing.T, projectRoot string, mappings map[string]string, bodies map[string]string) {
	t.Helper()
	accountsDir := filepath.Join(projectRoot, paths.ProcessDir, "accounts")
	if err := fileutil.EnsureDir(accountsDir); err != nil {
		t.Fatal(err)
	}
	for accID, hash := range mappings {
		body := bodies[accID]
		if err := fileutil.WriteStandardFile(filepath.Join(accountsDir, hash+".yaml"), []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal(map[string]any{"mappings": mappings})
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(filepath.Join(accountsDir, ".account.index"), raw); err != nil {
		t.Fatal(err)
	}
}

func writeRoleFixture(t *testing.T, projectRoot, roleObjID, roleID string, aliases, perms []string) {
	t.Helper()
	rolesDir := filepath.Join(projectRoot, paths.ProcessDir, "roles")
	if err := fileutil.EnsureDir(rolesDir); err != nil {
		t.Fatal(err)
	}
	hash := "rolehash_" + roleID
	permYAML := "permissions:\n"
	for _, p := range perms {
		permYAML += "  - " + p + "\n"
	}
	aliasYAML := ""
	if len(aliases) > 0 {
		aliasYAML = "aliases:\n"
		for _, a := range aliases {
			aliasYAML += "  - " + a + "\n"
		}
	}
	body := "id: " + roleObjID + "\nkind: role\nrole_id: " + roleID + "\n" + aliasYAML + permYAML
	if err := fileutil.WriteStandardFile(filepath.Join(rolesDir, hash+".yaml"), []byte(body)); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(rolesDir, ".role.index")
	mappings := map[string]string{}
	if data, err := fileutil.ReadFile(indexPath); err == nil {
		var idx struct {
			Mappings map[string]string `json:"mappings"`
		}
		_ = json.Unmarshal(data, &idx)
		if idx.Mappings != nil {
			mappings = idx.Mappings
		}
	}
	mappings[roleObjID] = hash
	raw, err := json.Marshal(map[string]any{"mappings": mappings})
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(indexPath, raw); err != nil {
		t.Fatal(err)
	}
}
