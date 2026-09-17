package app

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/internal/cli"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"
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

func TestAuthMiddleware_Authorized_APIKey(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	home := t.TempDir()
	t.Setenv(zqkenv.OSHome().Name(), home)
	const accID = "ACC-test-api-key"
	t.Setenv(zqkenv.APIKey().Name(), accID)
	t.Setenv(zqkenv.TestBypassAuth().Name(), "0")

	projectRoot := t.TempDir()
	createSchemas(t, projectRoot)
	writeBoundAccountFixture(t, projectRoot, accID, []string{"coder_agent"}, "PER-TEST-001")

	err := AuthMiddleware(cmd, projectRoot)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
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
	const accID = "ACC-test-token"
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
	parentCmd := &cobra.Command{Use: "system"}
	cmd := &cobra.Command{Use: "init"}
	cli.RequireSession(cmd, false)
	parentCmd.AddCommand(cmd)
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
