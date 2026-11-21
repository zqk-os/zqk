package community

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func validTestSandbox() SandboxPolicy {
	return SandboxPolicy{
		NetworkIsolation: true,
		ReadOnlyRootFS:   true,
		MaxMemoryMB:      256,
		AllowedPaths:     []string{"/workspace/sandbox"},
	}
}

func validTestManifest() AgentManifest {
	return AgentManifest{
		ManifestID:          "manifest-code-auditor",
		Role:                "security_auditor",
		MaxTokenBudget:      512,
		AllowedCapabilities: []string{"read_file", "zqk_grep"},
		Sandbox:             validTestSandbox(),
	}
}

func newTestSynthesizer(t *testing.T) *DynamicAgentSynthesizer {
	t.Helper()
	cfg := SynthesizerConfig{
		SynthesizerID:             "synth-primary",
		DefaultTokenBudget:        512,
		MaxConcurrentWorkers:      8,
		MaxMemoryMB:               512,
		GlobalAllowedCapabilities: []string{"read_file", "zqk_grep", "run_tests"},
	}
	synth, err := NewDynamicAgentSynthesizer(cfg)
	require.NoError(t, err)
	require.NotNil(t, synth)
	return synth
}

// TestDynamicAgentSynthesizer_StaticFloorInvariant verifies manifest, sandbox, and config validation invariants.
func TestDynamicAgentSynthesizer_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	synth := newTestSynthesizer(t)
	assert.Equal(t, 0, synth.ActiveWorkerCount())

	validManifest := validTestManifest()
	require.NoError(t, validManifest.Validate())

	invalidConfigs := []struct {
		name string
		edit func(c *SynthesizerConfig)
		want error
	}{
		{"blank synthesizer id", func(c *SynthesizerConfig) { c.SynthesizerID = "  " }, ErrInvalidSynthesizerConfig},
		{"zero token budget", func(c *SynthesizerConfig) { c.DefaultTokenBudget = 0 }, ErrInvalidSynthesizerConfig},
		{"zero max workers", func(c *SynthesizerConfig) { c.MaxConcurrentWorkers = 0 }, ErrInvalidSynthesizerConfig},
		{"blank global cap", func(c *SynthesizerConfig) { c.GlobalAllowedCapabilities = []string{" "} }, ErrInvalidSynthesizerConfig},
	}
	for _, tc := range invalidConfigs {
		cfg := SynthesizerConfig{
			SynthesizerID:        "synth-check",
			DefaultTokenBudget:   256,
			MaxConcurrentWorkers: 4,
		}
		tc.edit(&cfg)
		_, err := NewDynamicAgentSynthesizer(cfg)
		assert.ErrorIs(t, err, tc.want, tc.name)
	}

	invalidManifests := []struct {
		name string
		edit func(m *AgentManifest)
		want error
	}{
		{"blank manifest id", func(m *AgentManifest) { m.ManifestID = "" }, ErrInvalidManifest},
		{"blank role", func(m *AgentManifest) { m.Role = "   " }, ErrInvalidManifest},
		{"non-positive budget", func(m *AgentManifest) { m.MaxTokenBudget = 0 }, ErrInvalidManifest},
		{"empty capabilities", func(m *AgentManifest) { m.AllowedCapabilities = nil }, ErrInvalidManifest},
		{"blank capability entry", func(m *AgentManifest) { m.AllowedCapabilities = []string{"read_file", " "} }, ErrInvalidManifest},
		{"missing network isolation", func(m *AgentManifest) { m.Sandbox.NetworkIsolation = false }, ErrSandboxViolation},
		{"missing readonly rootfs", func(m *AgentManifest) { m.Sandbox.ReadOnlyRootFS = false }, ErrSandboxViolation},
		{"non-positive memory", func(m *AgentManifest) { m.Sandbox.MaxMemoryMB = 0 }, ErrSandboxViolation},
		{"blank allowed path", func(m *AgentManifest) { m.Sandbox.AllowedPaths = []string{"  "} }, ErrSandboxViolation},
	}
	for _, tc := range invalidManifests {
		m := validTestManifest()
		tc.edit(&m)
		assert.ErrorIs(t, m.Validate(), tc.want, tc.name)
	}
}

// TestDynamicAgentSynthesizer_OperationalDynamicProof verifies concurrent materialization,
// lifecycle transitions (pending -> running -> torn_down), SHA-256 digests, and deterministic teardown.
func TestDynamicAgentSynthesizer_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	synth := newTestSynthesizer(t)
	const workerCount = 6
	workerIDs := make([]string, workerCount)
	errCh := make(chan error, workerCount)

	wg := goroutinelabels.StartTestGoroutines(t, workerCount, "synth_worker_lifecycle", "concurrent worker materialization and execution", func(id int) {
		m := validTestManifest()
		m.ManifestID = fmt.Sprintf("manifest-%d", id)
		w, err := synth.MaterializeWorker(m, fmt.Sprintf("audit module %d", id))
		if err != nil {
			errCh <- err
			return
		}
		if w.State != WorkerStatePending || len(w.RuntimeDigest) != 64 {
			errCh <- fmt.Errorf("unexpected initial state %s or digest len %d", w.State, len(w.RuntimeDigest))
			return
		}
		if err := synth.ExecuteWorker(w.WorkerID, "read_file", 10); err != nil {
			errCh <- err
			return
		}
		workerIDs[id] = w.WorkerID
	})
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
	assert.Equal(t, workerCount, synth.ActiveWorkerCount())

	for _, id := range workerIDs {
		snap, err := synth.GetWorker(id)
		require.NoError(t, err)
		assert.Equal(t, WorkerStateRunning, snap.State)
		assert.True(t, snap.HasCapability("read_file"))
		assert.False(t, snap.HasCapability("shell_exec"))
		assert.Contains(t, snap.SystemPrompt, "role:security_auditor")
	}

	require.NoError(t, synth.TeardownWorker(workerIDs[0]))
	snap0, err := synth.GetWorker(workerIDs[0])
	require.NoError(t, err)
	assert.Equal(t, WorkerStateTornDown, snap0.State)
	assert.Empty(t, snap0.SystemPrompt)
	assert.Equal(t, workerCount-1, synth.ActiveWorkerCount())

	cleaned := synth.TeardownAll()
	assert.Equal(t, workerCount-1, cleaned)
	assert.Equal(t, 0, synth.ActiveWorkerCount())
}

// TestDynamicAgentSynthesizer_NegativeAdversarialBoundary verifies fail-closed rejection of
// capability escalations, sandbox violations, token budget exhaustion, capacity limits, and post-teardown execution.
func TestDynamicAgentSynthesizer_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	synth := newTestSynthesizer(t)

	// 1. Global capability allowlist escalation rejected.
	escalatedManifest := validTestManifest()
	escalatedManifest.AllowedCapabilities = []string{"read_file", "shell_exec"}
	_, err := synth.MaterializeWorker(escalatedManifest, "escalate privileges")
	assert.ErrorIs(t, err, ErrCapabilityDenied)

	// 2. Sandbox memory cap violation rejected.
	memManifest := validTestManifest()
	memManifest.Sandbox.MaxMemoryMB = 2048
	_, err = synth.MaterializeWorker(memManifest, "allocate excessive memory")
	assert.ErrorIs(t, err, ErrSandboxViolation)

	// 3. Prompt exceeding token budget rejected at materialization.
	tinyBudgetManifest := validTestManifest()
	tinyBudgetManifest.MaxTokenBudget = 16
	_, err = synth.MaterializeWorker(tinyBudgetManifest, strings.Repeat("token ", 40))
	assert.ErrorIs(t, err, ErrTokenBudgetExceeded)

	// 4. Materialize valid worker and test runtime boundaries.
	w, err := synth.MaterializeWorker(validTestManifest(), "inspect target")
	require.NoError(t, err)

	// Disallowed tool call rejected.
	assert.ErrorIs(t, synth.ExecuteWorker(w.WorkerID, "run_tests", 5), ErrCapabilityDenied)

	// Token budget exhaustion at execution rejected.
	assert.ErrorIs(t, synth.ExecuteWorker(w.WorkerID, "read_file", 1000), ErrTokenBudgetExceeded)

	// Negative token cost rejected.
	assert.ErrorIs(t, synth.ExecuteWorker(w.WorkerID, "read_file", -1), ErrTokenBudgetExceeded)

	// Post-teardown execution rejected.
	require.NoError(t, synth.TeardownWorker(w.WorkerID))
	assert.ErrorIs(t, synth.ExecuteWorker(w.WorkerID, "read_file", 1), ErrWorkerTornDown)

	// Non-existent worker rejected.
	assert.ErrorIs(t, synth.ExecuteWorker("missing-worker", "read_file", 1), ErrSynthesizerWorkerNotFound)
	_, err = synth.GetWorker("missing-worker")
	assert.ErrorIs(t, err, ErrSynthesizerWorkerNotFound)

	// 5. Concurrent capacity exhaustion rejected.
	capSynth, err := NewDynamicAgentSynthesizer(SynthesizerConfig{
		SynthesizerID:        "synth-small",
		DefaultTokenBudget:   256,
		MaxConcurrentWorkers: 1,
	})
	require.NoError(t, err)

	w1, err := capSynth.MaterializeWorker(validTestManifest(), "first worker")
	require.NoError(t, err)
	_, err = capSynth.MaterializeWorker(validTestManifest(), "second worker")
	assert.ErrorIs(t, err, ErrWorkerCapacityReached)

	// Teardown frees capacity for a new worker.
	require.NoError(t, capSynth.TeardownWorker(w1.WorkerID))
	_, err = capSynth.MaterializeWorker(validTestManifest(), "replacement worker")
	assert.NoError(t, err)
}
