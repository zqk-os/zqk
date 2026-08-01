package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"
)

func TestAuthMiddleware_Unauthorized(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(zqkenv.APIKey(), "")
	t.Setenv(zqkenv.TestBypassAuth(), "0")

	projectRoot := t.TempDir()

	err := AuthMiddleware(cmd, projectRoot)
	if err == nil {
		t.Fatalf("expected unauthorized error, got nil")
	}
}

func TestAuthMiddleware_Authorized_APIKey(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(zqkenv.APIKey(), "test-api-key")
	t.Setenv(zqkenv.TestBypassAuth(), "0")

	projectRoot := t.TempDir()
	createSchemas(t, projectRoot)

	err := AuthMiddleware(cmd, projectRoot)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestAuthMiddleware_Authorized_Credentials(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(zqkenv.APIKey(), "")
	t.Setenv(zqkenv.TestBypassAuth(), "0")

	credDir := filepath.Join(home, paths.ProjectDataDir)
	if err := os.MkdirAll(credDir, 0700); err != nil {
		t.Fatal(err)
	}
	credPath := filepath.Join(credDir, "credentials")
	if err := fileutil.WriteSecureFile(credPath, []byte("test-token")); err != nil {
		t.Fatal(err)
	}

	projectRoot := t.TempDir()
	createSchemas(t, projectRoot)

	err := AuthMiddleware(cmd, projectRoot)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestAuthMiddleware_BypassInit(t *testing.T) {
	parentCmd := &cobra.Command{Use: "system"}
	cmd := &cobra.Command{Use: "init"}
	parentCmd.AddCommand(cmd)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(zqkenv.APIKey(), "")
	t.Setenv(zqkenv.TestBypassAuth(), "0")

	projectRoot := t.TempDir()

	err := AuthMiddleware(cmd, projectRoot)
	if err != nil {
		t.Fatalf("expected no error for bypassed command, got %v", err)
	}
}

func createSchemas(t *testing.T, projectRoot string) {
	specsDir := filepath.Join(projectRoot, "docs", "process", "_internal", "object_specs")
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
