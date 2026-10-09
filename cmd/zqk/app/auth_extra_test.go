package app_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/app"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestAuthCommands_RunLogin_MissingProjectRoot(t *testing.T) {
	t.Setenv("ZQK_PROJECT_ROOT", "")
	t.Setenv("ZQK_TEST_ROOT", "")
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	require.NoError(t, os.Chdir(tmp))
	defer func() { _ = os.Chdir(origDir) }()

	loginCmd := app.NewLoginCmd()
	err := loginCmd.Execute()
	assert.Error(t, err)
}

func TestAuthCommands_RunLogout_MissingProjectRoot(t *testing.T) {
	t.Setenv("ZQK_PROJECT_ROOT", "")
	logoutCmd := app.NewLogoutCmd()
	err := logoutCmd.Execute()
	assert.NoError(t, err)
}

func TestAuthCommands_RunLogin_WithCustomSecurityAccount(t *testing.T) {
	testRoot, _ := setupIntegrationTestWithSpecsApp(t, "auth-sec-account")
	sp, err := storage.NewFileObjectStorageForTest(testRoot)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, testRoot, sp)
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, sp))
	})

	t.Setenv("ZQK_PROJECT_ROOT", testRoot)

	customSecCtx := pkgctx.NewSecurityContext("operator-user-123", []string{"developer"}, []string{"read", "write"})
	cmdCtx := pkgctx.WithSecurityContext(context.Background(), customSecCtx)

	loginCmd := app.NewLoginCmd()
	loginCmd.SetContext(cmdCtx)
	err = loginCmd.Execute()
	require.NoError(t, err)

	// Logout with custom security context
	logoutCmd := app.NewLogoutCmd()
	logoutCmd.SetContext(cmdCtx)
	err = logoutCmd.Execute()
	require.NoError(t, err)
}
