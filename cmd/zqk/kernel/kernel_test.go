package kernel_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/kernel"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestKernelCommandStructure(t *testing.T) {
	cmd := kernel.NewKernelCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "kernel", cmd.Use)

	stewardCmd, _, err := cmd.Find([]string{"steward"})
	require.NoError(t, err)
	require.NotNil(t, stewardCmd)
	assert.Equal(t, "steward", stewardCmd.Use)

	daemonCmd, _, err := cmd.Find([]string{"steward", "daemon"})
	require.NoError(t, err)
	require.NotNil(t, daemonCmd)
	assert.Equal(t, "daemon", daemonCmd.Use)

	sweepCmd, _, err := cmd.Find([]string{"steward", "sweep"})
	require.NoError(t, err)
	require.NotNil(t, sweepCmd)
	assert.Equal(t, "sweep", sweepCmd.Use)
}

func TestKernelStewardSweepExecution(t *testing.T) {
	cmd := kernel.NewKernelCmd()
	testRoot := t.TempDir()
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil))
	})
	cli.SetContext(cmd, cli.ContextForProjectRoot(testRoot))
	cmd.SetArgs([]string{"steward", "sweep"})
	err := cmd.ExecuteContext(context.Background())
	assert.NoError(t, err)
}
