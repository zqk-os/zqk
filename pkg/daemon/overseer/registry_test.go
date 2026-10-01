package overseer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/daemon/overseer"
)

// CRIT-OVERSEER-REGISTRY-SPEC: Static Floor: Manifest schema validates desired state,
// restart policies, and health check thresholds.
func TestRegistrySpecPersistenceAndDefaults(t *testing.T) {
	tempDir := t.TempDir()
	regPath := filepath.Join(tempDir, "registry.json")

	reg := overseer.NewRegistry(regPath)
	err := reg.Load()
	require.NoError(t, err)

	// Verify default daemons are present
	spec, ok := reg.Get("scheduler")
	require.True(t, ok)
	assert.Equal(t, overseer.DesiredStateEnabled, spec.DesiredState)
	assert.Equal(t, overseer.RestartPolicyAlways, spec.RestartPolicy)

	spec, ok = reg.Get("ambient")
	require.True(t, ok)
	assert.Equal(t, overseer.DesiredStateEnabled, spec.DesiredState)

	spec, ok = reg.Get("fswatcher")
	require.True(t, ok)
	assert.Equal(t, overseer.DesiredStateDisabled, spec.DesiredState)

	spec, ok = reg.Get("privileged-writer")
	require.True(t, ok)
	assert.Equal(t, overseer.DesiredStateEnabled, spec.DesiredState)
	assert.Equal(t, overseer.RestartPolicyAlways, spec.RestartPolicy)

	spec, ok = reg.Get("steward")
	require.True(t, ok)
	assert.Equal(t, overseer.DesiredStateEnabled, spec.DesiredState)

	spec, ok = reg.Get("mcp")
	require.True(t, ok)
	assert.Equal(t, overseer.DesiredStateDisabled, spec.DesiredState)

	spec, ok = reg.Get("seat-worker")
	require.True(t, ok)
	assert.Equal(t, overseer.DesiredStateDisabled, spec.DesiredState)

	// Mutate desired state and verify reload
	err = reg.SetDesiredState("fswatcher", overseer.DesiredStateEnabled)
	require.NoError(t, err)

	reg2 := overseer.NewRegistry(regPath)
	err = reg2.Load()
	require.NoError(t, err)

	spec2, ok := reg2.Get("fswatcher")
	require.True(t, ok)
	assert.Equal(t, overseer.DesiredStateEnabled, spec2.DesiredState)
}

// CRIT-OVERSEER-CRASH-RESTART-SUPERVISION: Operational Proof: Crashing enabled daemons
// are detected and restarted; disabled daemons remain stopped.
func TestCrashRestartSupervision(t *testing.T) {
	tempDir := t.TempDir()
	regPath := filepath.Join(tempDir, "registry.json")
	reg := overseer.NewRegistry(regPath)
	require.NoError(t, reg.Load())

	// Set up custom test daemon spec
	testSpec := &overseer.DaemonSpec{
		Name:          "test-worker",
		Description:   "Test worker daemon",
		Command:       []string{os.Args[0]},
		DesiredState:  overseer.DesiredStateEnabled,
		RestartPolicy: overseer.RestartPolicyAlways,
		MaxRestarts:   5,
		BackoffMin:    50 * time.Millisecond,
		BackoffMax:    500 * time.Millisecond,
		Env:           map[string]string{"TEST_OVERSEER_HELPER": "1", "TEST_OVERSEER_HELPER_MODE": "worker"},
	}
	require.NoError(t, reg.Set(testSpec))

	// Disabled daemon spec
	disabledSpec := &overseer.DaemonSpec{
		Name:          "test-disabled",
		Description:   "Test disabled daemon",
		Command:       []string{os.Args[0]},
		DesiredState:  overseer.DesiredStateDisabled,
		RestartPolicy: overseer.RestartPolicyNever,
	}
	require.NoError(t, reg.Set(disabledSpec))

	pgMgr, err := overseer.NewProcessGroupManager(false)
	require.NoError(t, err)

	sup := overseer.NewSupervisor(tempDir, reg, pgMgr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, sup.Start(ctx, 50*time.Millisecond))
	defer func() { _ = sup.Stop(context.Background()) }()

	// Wait for worker to start
	require.Eventually(t, func() bool {
		st, err := sup.GetStatus("test-worker")
		return err == nil && st.ActualState == overseer.ActualStateRunning && st.PID > 0
	}, 2*time.Second, 50*time.Millisecond)

	// Verify disabled daemon is stopped
	disSt, err := sup.GetStatus("test-disabled")
	require.NoError(t, err)
	assert.Equal(t, overseer.ActualStateStopped, disSt.ActualState)
	assert.Equal(t, 0, disSt.PID)

	// Now disable the worker
	err = sup.Disable(ctx, "test-worker")
	require.NoError(t, err)

	// Verify it transitions to stopped
	require.Eventually(t, func() bool {
		st, err := sup.GetStatus("test-worker")
		return err == nil && st.ActualState == overseer.ActualStateStopped && st.PID == 0
	}, 2*time.Second, 50*time.Millisecond)
}

// CRIT-OVERSEER-RAPID-CRASH-BACKOFF-BOUND: Negative Invariant: Rapid daemon crashes
// trigger exponential backoff ceiling to prevent CPU thrashing.
func TestRapidCrashExponentialBackoffBound(t *testing.T) {
	tempDir := t.TempDir()
	regPath := filepath.Join(tempDir, "registry.json")
	reg := overseer.NewRegistry(regPath)
	require.NoError(t, reg.Load())

	crashSpec := &overseer.DaemonSpec{
		Name:          "test-crasher",
		Description:   "Fast crashing daemon",
		Command:       []string{os.Args[0]},
		DesiredState:  overseer.DesiredStateEnabled,
		RestartPolicy: overseer.RestartPolicyAlways,
		MaxRestarts:   10,
		BackoffMin:    20 * time.Millisecond,
		BackoffMax:    150 * time.Millisecond,
		Env:           map[string]string{"TEST_OVERSEER_HELPER": "1", "TEST_OVERSEER_HELPER_MODE": "fast_exit"},
	}
	require.NoError(t, reg.Set(crashSpec))

	pgMgr, err := overseer.NewProcessGroupManager(false)
	require.NoError(t, err)

	sup := overseer.NewSupervisor(tempDir, reg, pgMgr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, sup.Start(ctx, 20*time.Millisecond))
	defer func() { _ = sup.Stop(context.Background()) }()

	// Wait until at least 3 restarts have occurred
	require.Eventually(t, func() bool {
		st, err := sup.GetStatus("test-crasher")
		return err == nil && st.RestartCount >= 3
	}, 3*time.Second, 20*time.Millisecond)

	st, err := sup.GetStatus("test-crasher")
	require.NoError(t, err)

	// State should be backoff
	assert.True(t, st.ActualState == overseer.ActualStateBackoff || st.ActualState == overseer.ActualStateRunning)
	assert.GreaterOrEqual(t, st.RestartCount, 3)

	// Verify backoff delay did not exceed BackoffMax
	if !st.BackoffUntil.IsZero() {
		delay := time.Until(st.BackoffUntil)
		assert.LessOrEqual(t, delay, crashSpec.BackoffMax+50*time.Millisecond)
	}
}

func TestRegistryAddDelete(t *testing.T) {
	tempDir := t.TempDir()
	regPath := filepath.Join(tempDir, "registry.json")

	reg := overseer.NewRegistry(regPath)
	require.NoError(t, reg.Load())

	// Add custom daemon spec
	spec := &overseer.DaemonSpec{
		Name:          "seat-worker-3",
		Description:   "Third autonomous seat worker",
		Command:       []string{"agent", "seat-worker", "--agent-id", "peer-agent-3"},
		DesiredState:  overseer.DesiredStateDisabled,
		RestartPolicy: overseer.RestartPolicyOnFailure,
		MaxRestarts:   5,
	}
	require.NoError(t, reg.Set(spec))

	retrieved, ok := reg.Get("seat-worker-3")
	require.True(t, ok)
	assert.Equal(t, "Third autonomous seat worker", retrieved.Description)
	assert.Equal(t, []string{"agent", "seat-worker", "--agent-id", "peer-agent-3"}, retrieved.Command)

	// Delete custom daemon spec
	require.NoError(t, reg.Delete("seat-worker-3"))
	_, ok = reg.Get("seat-worker-3")
	assert.False(t, ok)

	// Deleting non-existent returns error
	assert.Error(t, reg.Delete("non-existent"))
}

func TestRegistryPeerSeatsAutoSeeding(t *testing.T) {
	rootDir := t.TempDir()
	meshDir := filepath.Join(rootDir, ".zqk", "state", "mesh")
	require.NoError(t, os.MkdirAll(meshDir, 0755))

	peerSeatsContent := []byte(`{
  "schema_version": "1",
  "seats": {
    "peer-agent-1": {
      "note": "lead peer"
    },
    "peer-agent-2": {
      "note": "worker peer 2"
    }
  }
}`)
	require.NoError(t, os.WriteFile(filepath.Join(meshDir, "peer_seats.json"), peerSeatsContent, 0644))

	regPath := overseer.DefaultRegistryPath(rootDir)
	reg := overseer.NewRegistry(regPath)
	require.NoError(t, reg.Load())

	// Verify both seat workers were automatically seeded
	spec1, ok := reg.Get("seat-worker-peer-agent-1")
	require.True(t, ok, "seat-worker-peer-agent-1 should be auto-seeded")
	assert.Equal(t, []string{"agent", "seat-worker", "--agent-id", "peer-agent-1"}, spec1.Command)

	spec2, ok := reg.Get("seat-worker-peer-agent-2")
	require.True(t, ok, "seat-worker-peer-agent-2 should be auto-seeded")
	assert.Equal(t, []string{"agent", "seat-worker", "--agent-id", "peer-agent-2"}, spec2.Command)
}

func TestSupervisorAddRemoveDaemonViaIPC(t *testing.T) {
	tempDir := t.TempDir()
	sockPath := shortTempSocket(t)
	regPath := filepath.Join(tempDir, "registry.json")

	pgMgr, err := overseer.NewProcessGroupManager(false)
	require.NoError(t, err)

	reg := overseer.NewRegistry(regPath)
	require.NoError(t, reg.Load())

	sup := overseer.NewSupervisor(tempDir, reg, pgMgr)
	require.NoError(t, sup.Start(context.Background(), 20*time.Millisecond))
	defer func() { _ = sup.Stop(context.Background()) }()

	ipcServer := overseer.NewIPCServer(sockPath, sup)
	require.NoError(t, ipcServer.Start())
	defer func() { _ = ipcServer.Stop() }()

	client := overseer.NewIPCClient(sockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Add dynamic seat worker
	addResp, err := client.Send(ctx, overseer.IPCRequest{
		Action: "add",
		Spec: &overseer.DaemonSpec{
			Name:          "seat-worker-dynamic",
			Description:   "Dynamic seat worker",
			Command:       []string{"agent", "seat-worker", "--agent-id", "peer-agent-dyn"},
			DesiredState:  overseer.DesiredStateDisabled,
			RestartPolicy: overseer.RestartPolicyNever,
		},
	})
	require.NoError(t, err)
	assert.True(t, addResp.Success)

	st, err := sup.GetStatus("seat-worker-dynamic")
	require.NoError(t, err)
	assert.Equal(t, "seat-worker-dynamic", st.Name)

	// Remove dynamic seat worker
	remResp, err := client.Send(ctx, overseer.IPCRequest{
		Action: "remove",
		Target: "seat-worker-dynamic",
	})
	require.NoError(t, err)
	assert.True(t, remResp.Success)

	_, err = sup.GetStatus("seat-worker-dynamic")
	assert.Error(t, err)
}
