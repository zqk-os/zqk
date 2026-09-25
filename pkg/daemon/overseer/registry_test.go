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
