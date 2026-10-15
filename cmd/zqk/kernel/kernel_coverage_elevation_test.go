package kernel

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestStewardDaemon_TickerIteration(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, ".zqk", "state"), 0755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, ".zqk", "tmp"), 0755))
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil))
	})

	cmd := newStewardDaemonCmd()
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()

	cli.SetContext(cmd, cli.ContextForProjectRoot(tmpDir))
	cmd.SetArgs([]string{"--interval", "1"})
	err := cmd.ExecuteContext(ctx)
	assert.NoError(t, err)
}

func TestStewardDaemon_NegativeIntervalBranch(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, ".zqk", "state"), 0755))
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil))
	})

	cmd := newStewardDaemonCmd()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancelled

	cli.SetContext(cmd, cli.ContextForProjectRoot(tmpDir))
	cmd.SetArgs([]string{"--interval", "-10"})
	err := cmd.ExecuteContext(ctx)
	assert.NoError(t, err)
}

func TestKernelStewardCommand_FlagsAndHelp(t *testing.T) {
	cmd := NewKernelCmd()
	assert.NotNil(t, cmd)

	stewardCmd, _, err := cmd.Find([]string{"steward"})
	require.NoError(t, err)
	require.NotNil(t, stewardCmd)

	daemonCmd, _, err := stewardCmd.Find([]string{"daemon"})
	require.NoError(t, err)
	require.NotNil(t, daemonCmd)
	intervalFlag := daemonCmd.Flags().Lookup("interval")
	require.NotNil(t, intervalFlag)
	assert.Equal(t, "30", intervalFlag.DefValue)

	sweepCmd, _, err := stewardCmd.Find([]string{"sweep"})
	require.NoError(t, err)
	require.NotNil(t, sweepCmd)
}

func TestResolveStewardDaemon_FallbackProjectRoot(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, ".zqk", "state"), 0755))
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil))
	})
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

	cmd := NewKernelCmd()
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)
	daemon, root, err := resolveStewardDaemon(proc)
	assert.NoError(t, err)
	assert.Equal(t, tmpDir, root)
	assert.NotNil(t, daemon)
}

func TestStewardDaemon_MissingIntervalFlagFallback(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, ".zqk", "state"), 0755))
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil))
	})

	daemonCmd := newStewardDaemonCmd()
	bareCmd := &cobra.Command{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled
	bareCmd.SetContext(ctx)
	cli.SetContext(bareCmd, cli.ContextForProjectRoot(tmpDir))

	err := daemonCmd.RunE(bareCmd, []string{})
	assert.NoError(t, err)
}
