package utility

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/testkit"
	"testing"
)

func TestNewMigrateCmd(t *testing.T) {
	cmd := NewMigrateCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "migrate", cmd.Use)

	assert.NotNil(t, cmd.Flags().Lookup("source"))
	assert.NotNil(t, cmd.Flags().Lookup("target"))
	assert.NotNil(t, cmd.Flags().Lookup("phase"))
	assert.NotNil(t, cmd.Flags().Lookup("version-constraint"))
}

func TestRunMigrate_InvalidConstraint(t *testing.T) {
	cmd := NewMigrateCmd()
	require.NoError(t, cmd.Flags().Set("version-constraint", "invalid-constraint-format-!!!"))

	err := runMigrate(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version constraint")
}

func TestRunMigrate_MissingBinary(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	cmd := NewMigrateCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(cmd.Context(), &buf)
	cmd.SetContext(ctx)

	// In test environment, zqk-migrate is not installed in PATH
	err := runMigrate(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Migration tool not available")
}
