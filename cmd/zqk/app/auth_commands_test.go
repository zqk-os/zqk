package app_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/cmd/zqk/app"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestAuthCommands_Structure(t *testing.T) {
	authCmd := app.NewAuthCmd()
	require.NotNil(t, authCmd)
	assert.Equal(t, "auth", authCmd.Use)
	assert.NotEmpty(t, authCmd.Commands())

	loginCmd := app.NewLoginCmd()
	require.NotNil(t, loginCmd)
	assert.Equal(t, "login", loginCmd.Use)

	logoutCmd := app.NewLogoutCmd()
	require.NotNil(t, logoutCmd)
	assert.Equal(t, "logout", logoutCmd.Use)
}

func TestAuthCommands_LoginAndLogoutLifecycle(t *testing.T) {
	testRoot, _ := setupIntegrationTestWithSpecsApp(t, "auth-lifecycle")
	sp, err := storage.NewFileObjectStorageForTest(testRoot)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, testRoot, sp)
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, sp))
	})

	t.Setenv("ZQK_PROJECT_ROOT", testRoot)

	// 1. Initial Login
	loginCmd := app.NewLoginCmd()
	buf := new(bytes.Buffer)
	loginCmd.SetOut(buf)
	loginCmd.SetErr(buf)
	err = loginCmd.Execute()
	require.NoError(t, err)

	// Flush CAS index
	if q := caspkg.GetListingIndexWriteQueueForProjectRoot(testRoot); q != nil {
		_ = q.FlushAllWithDeadline(time.Now().Add(3 * time.Second))
	}

	// 2. Second Login (reusing session)
	loginCmd2 := app.NewLoginCmd()
	buf2 := new(bytes.Buffer)
	loginCmd2.SetOut(buf2)
	loginCmd2.SetErr(buf2)
	err = loginCmd2.Execute()
	require.NoError(t, err)

	// 3. Logout
	logoutCmd := app.NewLogoutCmd()
	buf3 := new(bytes.Buffer)
	logoutCmd.SetOut(buf3)
	logoutCmd.SetErr(buf3)
	err = logoutCmd.Execute()
	require.NoError(t, err)

	// 4. Logout again when no session is active
	logoutCmd2 := app.NewLogoutCmd()
	buf4 := new(bytes.Buffer)
	logoutCmd2.SetOut(buf4)
	logoutCmd2.SetErr(buf4)
	err = logoutCmd2.Execute()
	require.NoError(t, err)
}
