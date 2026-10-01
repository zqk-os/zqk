package daemon_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/daemon"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

func TestDaemonCommandStructure(t *testing.T) {
	cmd := daemon.NewDaemonCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "daemon", cmd.Use)

	expected := []string{"status", "start", "stop", "restart", "enable", "disable", "add", "remove", "run", "service"}
	for _, exp := range expected {
		sub, _, err := cmd.Find([]string{exp})
		require.NoError(t, err)
		require.NotNil(t, sub, "expected subcommand %s", exp)
	}
}

func TestDaemonAddStatusRemoveExecution(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Add daemon
	addCmd := daemon.NewDaemonCmd()
	cli.SetContext(addCmd, cli.ContextForProjectRoot(tempDir))
	addCmd.SetArgs([]string{"add", "seat-worker-custom", "--description", "Custom worker", "--", "agent", "seat-worker", "--agent-id", "peer-custom"})
	err := addCmd.ExecuteContext(context.Background())
	require.NoError(t, err)

	// 2. Query status
	statusCmd := daemon.NewDaemonCmd()
	cli.SetContext(statusCmd, cli.ContextForProjectRoot(tempDir))
	statusCmd.SetArgs([]string{"status", "seat-worker-custom"})
	err = statusCmd.ExecuteContext(context.Background())
	require.NoError(t, err)

	// 3. Remove daemon
	removeCmd := daemon.NewDaemonCmd()
	cli.SetContext(removeCmd, cli.ContextForProjectRoot(tempDir))
	removeCmd.SetArgs([]string{"remove", "seat-worker-custom"})
	err = removeCmd.ExecuteContext(context.Background())
	require.NoError(t, err)

	// 4. Status should now fail to find it
	verifyCmd := daemon.NewDaemonCmd()
	cli.SetContext(verifyCmd, cli.ContextForProjectRoot(tempDir))
	verifyCmd.SetArgs([]string{"status", "seat-worker-custom"})
	err = verifyCmd.ExecuteContext(context.Background())
	assert.Error(t, err)
}

func TestDaemonServiceCommandStructure(t *testing.T) {
	cmd := daemon.NewDaemonCmd()
	sub, _, err := cmd.Find([]string{"service"})
	require.NoError(t, err)
	require.NotNil(t, sub)

	expected := []string{"install", "uninstall", "status", "cleanup-legacy"}
	for _, exp := range expected {
		child, _, err := sub.Find([]string{exp})
		require.NoError(t, err)
		require.NotNil(t, child, "expected daemon service subcommand %s", exp)
	}
}
