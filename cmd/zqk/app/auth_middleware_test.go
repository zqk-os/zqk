package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/internal/cli"

	"github.com/zqk-os/zqk/pkg/authcred"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestAuthMiddleware_Unauthorized(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	t.Setenv(zqkenv.APIKey().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	projectRoot := t.TempDir()

	err := AuthMiddleware(cmd, projectRoot)
	if err == nil {
		t.Fatalf("expected unauthorized error, got nil")
	}
}

func TestAuthMiddleware_UninitializedKernelHint(t *testing.T) {
	cmd := &cobra.Command{Use: "object"}
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	const accID = "ACC-test-token"
	t.Setenv(zqkenv.APIKey().Name(), accID)
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	// Empty dir with no .zqk directory
	emptyProjectRoot := t.TempDir()

	err := AuthMiddleware(cmd, emptyProjectRoot)
	if err == nil {
		t.Fatalf("expected error for uninitialized kernel, got nil")
	}
	if !strings.Contains(err.Error(), "kernel not initialized: run") || !strings.Contains(err.Error(), "system init") {
		t.Fatalf("expected fail-closed uninitialized kernel hint, got: %v", err)
	}
}

func TestAuthMiddleware_RejectsUnboundAPIKey(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	t.Setenv(zqkenv.APIKey().Name(), "ACC-TEST-AUTH-001")
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	projectRoot := t.TempDir()
	createSchemas(t, projectRoot)

	err := AuthMiddleware(cmd, projectRoot)
	if err == nil {
		t.Fatal("expected reject for unbound ACC without index/role/persona")
	}
}

func TestAuthMiddleware_RejectsNonACCForm(t *testing.T) {
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")
	projectRoot := t.TempDir()
	createSchemas(t, projectRoot)

	for _, cred := range []string{"test-api-key", "account:system"} {
		t.Run(cred, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			t.Setenv(zqkenv.APIKey().Name(), cred)
			err := AuthMiddleware(cmd, projectRoot)
			if err == nil {
				t.Fatal("expected reject for non-ACC id")
			}
			if !strings.Contains(err.Error(), "invalid API key") && !strings.Contains(err.Error(), "ACC-*") && !strings.Contains(err.Error(), "account index missing") {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestAuthMiddleware_AcceptsIssuedAPIKey(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	const accID = "ACC-TEST-KEY-001"
	testKey := "zqk_test_middleware_probe_key"
	t.Setenv(zqkenv.APIKey().Name(), testKey)
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	projectRoot := t.TempDir()
	createSchemas(t, projectRoot)
	writeBoundAccountFixture(t, projectRoot, accID, []string{"coder_agent"}, "PER-TEST-001")

	dir := filepath.Join(projectRoot, paths.ProcessKeystoreDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	hash := authcred.HashAPIKey(testKey)
	entry := "account_id: " + accID + "\ncredential_hash: " + hash + "\nid: KEY-MW-001\nkey_type: api_key\nrevoked: false\nstatus: active\n"
	if err := fileutil.WriteSecureFile(filepath.Join(dir, "aabbcc.yaml"), []byte(entry)); err != nil {
		t.Fatal(err)
	}

	err := AuthMiddleware(cmd, projectRoot)
	if err != nil {
		t.Fatalf("expected issued key to auth: %v", err)
	}
	sec := pkgctx.GetSecurityContext(cmd.Context())
	if sec == nil || sec.AccountID != accID {
		t.Fatalf("expected account %s, got %#v", accID, sec)
	}
}

func TestAuthMiddleware_IssuedSystemKeyGetsSystemSecurityContext(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	t.Setenv(zqkenv.OSHome().Name(), t.TempDir())
	const testKey = "zqk_test_system_middleware_probe"
	t.Setenv(zqkenv.APIKey().Name(), testKey)
	t.Setenv(zqkenv.Persona().Name(), "PER-SYSTEM-PROBE")
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	projectRoot := t.TempDir()
	createSchemas(t, projectRoot)

	dir := filepath.Join(projectRoot, paths.ProcessKeystoreDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	entry := "account_id: " + pkgctx.SystemAccountID +
		"\ncredential_hash: " + authcred.HashAPIKey(testKey) +
		"\nid: KEY-MW-SYSTEM\nkey_type: api_key\nrevoked: false\nstatus: active\n"
	if err := fileutil.WriteSecureFile(filepath.Join(dir, "system-key.yaml"), []byte(entry)); err != nil {
		t.Fatal(err)
	}

	if err := AuthMiddleware(cmd, projectRoot); err != nil {
		t.Fatalf("expected issued system key to authenticate: %v", err)
	}
	sec := pkgctx.GetSecurityContext(cmd.Context())
	if sec == nil || sec.AccountID != pkgctx.SystemAccountID {
		t.Fatalf("expected system account security context, got %#v", sec)
	}
	if sec.PersonaID != "PER-SYSTEM-PROBE" {
		t.Fatalf("expected persona environment binding, got %#v", sec)
	}
	if len(sec.Roles) != 1 || sec.Roles[0] != "admin" {
		t.Fatalf("expected admin role, got %#v", sec.Roles)
	}
	if !authcred.HasExactPermission(sec, "write:*") || !authcred.HasExactPermission(sec, "delete:*") {
		t.Fatalf("expected system write/delete permissions, got %#v", sec.Permissions)
	}
}

func TestAuthMiddleware_RequiresRoleAndPersona(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	const accID = "ACC-TEST-BOUND-001"
	t.Setenv(zqkenv.APIKey().Name(), accID)
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	projectRoot := t.TempDir()
	createSchemas(t, projectRoot)
	writeBoundAccountFixture(t, projectRoot, accID, []string{"coder_agent"}, "PER-TEST-001")

	err := AuthMiddleware(cmd, projectRoot)
	if err != nil {
		t.Fatalf("expected bound account to auth: %v", err)
	}
	sec := pkgctx.GetSecurityContext(cmd.Context())
	if sec == nil || sec.PersonaID != "PER-TEST-001" {
		t.Fatalf("expected persona bound on security context, got %#v", sec)
	}
}

func TestAuthMiddleware_RejectsMissingPersona(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	const accID = "ACC-TEST-UNBOUND-001"
	t.Setenv(zqkenv.APIKey().Name(), accID)
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	projectRoot := t.TempDir()
	createSchemas(t, projectRoot)
	writeBoundAccountFixture(t, projectRoot, accID, []string{"coder_agent"}, "")

	err := AuthMiddleware(cmd, projectRoot)
	if err == nil {
		t.Fatal("expected reject for missing persona")
	}
	if !strings.Contains(err.Error(), "RBAC-ready") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestAuthMiddleware_LiveTestRootFallsBackToHomeCredentials(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	t.Setenv(zqkenv.APIKey().Name(), "")
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	credDir := filepath.Join(home, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(credDir); err != nil {
		t.Fatal(err)
	}
	const accID = "ACC-TEST-LIVE-ROOT-001"
	if err := fileutil.WriteSecureFile(filepath.Join(credDir, "credentials"), []byte(accID)); err != nil {
		t.Fatal(err)
	}

	projectRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), projectRoot)
	createSchemas(t, projectRoot)
	writeBoundAccountFixture(t, projectRoot, accID, []string{"coder_agent"}, "PER-TEST-001")

	if err := AuthMiddleware(cmd, projectRoot); err != nil {
		t.Fatalf("expected home credentials when TEST_ROOT is the live project: %v", err)
	}
}

func TestAuthMiddleware_IsolatedTestRootDoesNotUseHome(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	t.Setenv(zqkenv.APIKey().Name(), "")
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	credDir := filepath.Join(home, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(credDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(credDir, "credentials"), []byte("ACC-TEST-ISOLATE-HOME")); err != nil {
		t.Fatal(err)
	}

	projectRoot := t.TempDir()
	isolate := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), isolate)
	createSchemas(t, projectRoot)

	err := AuthMiddleware(cmd, projectRoot)
	if err == nil {
		t.Fatal("expected unauthorized when isolated TEST_ROOT has no credentials")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestAuthMiddleware_Authorized_Credentials(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	t.Setenv(zqkenv.APIKey().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	credDir := filepath.Join(home, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(credDir); err != nil {
		t.Fatal(err)
	}
	const accID = "ACC-TEST-CRED-001"
	credPath := filepath.Join(credDir, "credentials")
	if err := fileutil.WriteSecureFile(credPath, []byte(accID)); err != nil {
		t.Fatal(err)
	}

	projectRoot := t.TempDir()
	createSchemas(t, projectRoot)
	writeBoundAccountFixture(t, projectRoot, accID, []string{"coder_agent"}, "PER-TEST-001")

	err := AuthMiddleware(cmd, projectRoot)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestAuthMiddleware_BypassInit(t *testing.T) {
	systemCmd := &cobra.Command{Use: "system"}
	cmd := &cobra.Command{Use: "init"}
	cli.RequireSession(cmd, false)
	systemCmd.AddCommand(cmd)
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	t.Setenv(zqkenv.APIKey().Name(), "")
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	projectRoot := t.TempDir()

	err := AuthMiddleware(cmd, projectRoot)
	if err != nil {
		t.Fatalf("expected no error for bypassed command, got %v", err)
	}
}

func createSchemas(t *testing.T, projectRoot string) {
	t.Helper()
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir, "kernel")
	if err := fileutil.EnsureDir(specsDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(filepath.Join(specsDir, "account.yaml"), []byte("schema: account")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(filepath.Join(specsDir, "role.yaml"), []byte("schema: role")); err != nil {
		t.Fatal(err)
	}
}

func writeBoundAccountFixture(t *testing.T, projectRoot, accID string, roles []string, personaRef string) {
	t.Helper()
	accountsDir := filepath.Join(projectRoot, paths.ProcessDir, "accounts")
	if err := fileutil.EnsureDir(accountsDir); err != nil {
		t.Fatal(err)
	}
	hashName := "fixturehash001"
	rolesYAML := ""
	if len(roles) > 0 {
		rolesYAML = "roles:\n"
		for _, r := range roles {
			rolesYAML += "  - " + r + "\n"
		}
	}
	personaYAML := ""
	if personaRef != "" {
		personaYAML = "persona_ref: " + personaRef + "\n"
	}
	body := "id: " + accID + "\nkind: account\n" + rolesYAML + personaYAML
	if err := fileutil.WriteStandardFile(filepath.Join(accountsDir, hashName+".yaml"), []byte(body)); err != nil {
		t.Fatal(err)
	}
	index := `{"mappings":{"` + accID + `":"` + hashName + `"}}`
	if err := fileutil.WriteStandardFile(filepath.Join(accountsDir, ".account.index"), []byte(index)); err != nil {
		t.Fatal(err)
	}
}

// TestAuthMiddleware_NoHardcodedSystemFallback tests REQ-CEF-R2-SEC-HARDCODED-ACC / CRIT-CEF-R2-SEC-HARDCODED-ACC-A.
// Asserts that missing credentials never fallback to system account, and unauthenticated requests fail closed.
func TestAuthMiddleware_NoHardcodedSystemFallback(t *testing.T) {
	cmd := &cobra.Command{Use: "status"}
	cmd.SetContext(context.Background())
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	t.Setenv(zqkenv.APIKey().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	projectRoot := t.TempDir()
	createSchemas(t, projectRoot)

	err := AuthMiddleware(cmd, projectRoot)
	if err == nil {
		t.Fatal("expected unauthorized error when credentials are missing, got nil (fail-closed)")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized error, got: %v", err)
	}

	// Ensure no SecurityContext was injected with the system account
	sec := pkgctx.GetSecurityContext(cmd.Context())
	if sec != nil && sec.AccountID == pkgctx.SystemAccountID {
		t.Fatalf("security context was unexpectedly set to system account: %#v", sec)
	}
}
