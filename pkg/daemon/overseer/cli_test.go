package overseer_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/daemon"
	"github.com/zqk-os/zqk/pkg/daemon/overseer"
)

func shortTempSocket(t *testing.T) string {
	sock := filepath.Join("/tmp", fmt.Sprintf("ov_%d_%d.sock", os.Getpid(), time.Now().UnixNano()%1000000))
	t.Cleanup(func() { _ = os.Remove(sock) })
	return sock
}

// CRIT-OVERSEER-CLI-SCHEMA: Static Floor: Unified daemon command surface
// conforms to CLI taxonomy and JSON output contract.
func TestDaemonCLISchemaAndSubcommands(t *testing.T) {
	cmd := daemon.NewDaemonCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "daemon", cmd.Use)

	expectedSubcommands := []string{"status", "start", "stop", "restart", "enable", "disable", "add", "remove", "run"}
	foundSubcommands := make(map[string]bool)

	for _, sub := range cmd.Commands() {
		foundSubcommands[sub.Name()] = true
	}

	for _, expected := range expectedSubcommands {
		assert.True(t, foundSubcommands[expected], "Missing expected subcommand: %s", expected)
	}
}

// CRIT-OVERSEER-UNIFIED-STATUS-REPORT: Operational Proof: Unified status report
// provides comprehensive process matrix with desired vs actual state.
func TestUnifiedStatusReportViaIPC(t *testing.T) {
	tempDir := t.TempDir()
	sockPath := shortTempSocket(t)
	regPath := filepath.Join(tempDir, "registry.json")

	reg := overseer.NewRegistry(regPath)
	require.NoError(t, reg.Load())

	pgMgr, err := overseer.NewProcessGroupManager(false)
	require.NoError(t, err)

	sup := overseer.NewSupervisor(tempDir, reg, pgMgr)
	server := overseer.NewIPCServer(sockPath, sup)
	require.NoError(t, server.Start())
	defer func() { _ = server.Stop() }()

	client := overseer.NewIPCClient(sockPath)
	require.True(t, client.IsRunning())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.Send(ctx, overseer.IPCRequest{Action: "status"})
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.NotEmpty(t, resp.Daemons)

	// Verify all default daemons are represented in the status matrix
	daemonMap := make(map[string]overseer.DaemonStatus)
	for _, d := range resp.Daemons {
		daemonMap[d.Name] = d
		assert.NotEmpty(t, d.DesiredState, "desired_state must not be empty for %s", d.Name)
		assert.NotEmpty(t, d.ActualState, "actual_state must not be empty for %s", d.Name)
	}

	assert.Contains(t, daemonMap, "scheduler")
	assert.Contains(t, daemonMap, "ambient")
	assert.Contains(t, daemonMap, "steward")
	assert.Contains(t, daemonMap, "fswatcher")
	assert.Contains(t, daemonMap, "mcp")
}

// CRIT-OVERSEER-INVALID-DAEMON-ACTION-REJECT: Negative Invariant: Unknown daemon
// identifiers or invalid action requests fail closed with structured error.
func TestInvalidDaemonActionRejection(t *testing.T) {
	tempDir := t.TempDir()
	sockPath := shortTempSocket(t)
	regPath := filepath.Join(tempDir, "registry.json")

	reg := overseer.NewRegistry(regPath)
	require.NoError(t, reg.Load())

	pgMgr, err := overseer.NewProcessGroupManager(false)
	require.NoError(t, err)

	sup := overseer.NewSupervisor(tempDir, reg, pgMgr)
	server := overseer.NewIPCServer(sockPath, sup)
	require.NoError(t, server.Start())
	defer func() { _ = server.Stop() }()

	client := overseer.NewIPCClient(sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Unknown daemon name
	resp, err := client.Send(ctx, overseer.IPCRequest{
		Action: "start",
		Target: "nonexistent-daemon-xyz",
	})
	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "unknown daemon: nonexistent-daemon-xyz")

	// 2. Unknown action
	resp, err = client.Send(ctx, overseer.IPCRequest{
		Action: "self-destruct",
		Target: "scheduler",
	})
	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "unknown action: self-destruct")

	// 3. Missing target daemon name
	resp, err = client.Send(ctx, overseer.IPCRequest{
		Action: "start",
	})
	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "missing target daemon name")
}
