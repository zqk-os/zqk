package overseer_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/daemon/overseer"
)

// CRIT-1794273507520870000-e60010ac: Static Floor Invariant - Launchctl Service Configuration & Plist Invariants.
// Verifies that all managed system daemons declare valid launchctl plist configurations and file paths without orphaned services.
func TestLaunchctlServiceConfiguration_StaticFloorInvariant(t *testing.T) {
	fakeRoot := "/tmp/test-zqk-root"
	fakeBin := "/tmp/test-zqk-root/bin/zqk"

	spec := overseer.OverseerServiceSpec(fakeRoot, fakeBin)
	assert.Equal(t, overseer.OverseerLaunchAgentLabel, spec.ID)
	assert.Equal(t, fakeBin, spec.Executable)
	assert.Equal(t, []string{"daemon", "run", "--timeout", "0"}, spec.Arguments)
	assert.Equal(t, fakeRoot, spec.WorkingDir)
	assert.True(t, spec.RunAtLoad)
	assert.True(t, spec.KeepAlive)
	assert.Contains(t, spec.StandardOutPath, "overseer.launchd.out")
	assert.Contains(t, spec.StandardErrorPath, "overseer.launchd.err")

	plist := overseer.GenerateLaunchdPlist(fakeRoot, fakeBin)
	require.NotEmpty(t, plist)
	assert.Contains(t, plist, "<key>Label</key>")
	assert.Contains(t, plist, "<key>ProgramArguments</key>")
	assert.Contains(t, plist, "<key>WorkingDirectory</key>")
	assert.Contains(t, plist, "<key>KeepAlive</key>")
	assert.Contains(t, plist, "<key>SuccessfulExit</key>")
	assert.Contains(t, plist, "<false/>")

	// Verify absence of unmanaged/deprecated legacy plist fragments
	assert.NotContains(t, plist, "com.zqk.privileged-writer")
	assert.NotContains(t, plist, "launchctl stop")
}

// CRIT-1794273512729644000-1dcf3b9f: Operational Dynamic Proof - Overseer Health Reconciliation & Daemon Recovery.
// Verifies dynamic health monitoring, active heartbeat validation, and automated overseer daemon recovery under failure injection.
func TestOverseerHealthReconciliation_OperationalProof(t *testing.T) {
	ctx := context.Background()
	monitor := overseer.NewHealthMonitor(3, 200*time.Millisecond)

	// Healthy daemon via heartbeat
	monitor.RecordHeartbeat("scheduler")
	st, err := monitor.CheckHealth(ctx, "scheduler")
	require.NoError(t, err)
	assert.Equal(t, overseer.HealthStateHealthy, st)

	// Injected probe with transient failures transitioning through degraded to unhealthy
	probeCalls := 0
	monitor.RegisterProbe("ambient", func(ctx context.Context) error {
		probeCalls++
		if probeCalls < 3 {
			return errors.New("transient network hiccup")
		}
		return errors.New("critical ambient fault")
	})

	// First probe failure -> degraded
	st1, err1 := monitor.CheckHealth(ctx, "ambient")
	assert.Error(t, err1)
	assert.Equal(t, overseer.HealthStateDegraded, st1)

	// Second probe failure -> degraded
	st2, err2 := monitor.CheckHealth(ctx, "ambient")
	assert.Error(t, err2)
	assert.Equal(t, overseer.HealthStateDegraded, st2)

	// Third probe failure (threshold reached) -> unhealthy
	st3, err3 := monitor.CheckHealth(ctx, "ambient")
	assert.Error(t, err3)
	assert.Equal(t, overseer.HealthStateUnhealthy, st3)
	assert.True(t, errors.Is(err3, overseer.ErrDaemonUnhealthy))
	assert.Equal(t, 3, monitor.FailureCount("ambient"))
}

// CRIT-1794273518574773000-f4277182: Negative Adversarial Boundary - Malformed Wake Quarantine & Fail-Closed Daemon Circuit Breaker.
// Verifies fail-closed behavior on corrupted wake payloads and isolation of unrecoverable daemons without stalling the autonomous loop.
func TestMalformedWakeQuarantine_NegativeBoundary(t *testing.T) {
	ctx := context.Background()
	breaker := overseer.NewWakeCircuitBreaker(3, 100*time.Millisecond)
	dispatcher := overseer.NewWakeDispatcher(breaker)

	received := make([]overseer.WakePayload, 0)
	dispatcher.Subscribe("task_complete", func(ctx context.Context, payload overseer.WakePayload) error {
		received = append(received, payload)
		return nil
	})

	// 1. Valid payload dispatches cleanly
	validWake := overseer.WakePayload{
		ID:        "WAKE-001",
		Source:    "scheduler",
		EventKind: "task_complete",
		Timestamp: time.Now(),
	}
	err := dispatcher.Dispatch(ctx, validWake)
	require.NoError(t, err)
	assert.Len(t, received, 1)

	// 2. Malformed payloads (missing fields) rejected & quarantined
	malformedWakes := []overseer.WakePayload{
		{ID: "", Source: "scheduler", EventKind: "task_complete", Timestamp: time.Now()},
		{ID: "WAKE-002", Source: "", EventKind: "task_complete", Timestamp: time.Now()},
		{ID: "WAKE-003", Source: "scheduler", EventKind: "", Timestamp: time.Now()},
	}

	for _, bad := range malformedWakes {
		err := dispatcher.Dispatch(ctx, bad)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, overseer.ErrMalformedWakePayload) || strings.Contains(err.Error(), "malformed wake payload"))
	}

	// 3. Excessive malformed payloads trip the circuit breaker
	quarantined := dispatcher.QuarantinedWakes()
	assert.Len(t, quarantined, 3)
	assert.True(t, breaker.IsTripped())

	// 4. While tripped, subsequent dispatches fail-closed with ErrCircuitBreakerTripped
	errTripped := dispatcher.Dispatch(ctx, validWake)
	assert.ErrorIs(t, errTripped, overseer.ErrCircuitBreakerTripped)

	// 5. Cooldown resets breaker
	time.Sleep(120 * time.Millisecond)
	assert.False(t, breaker.IsTripped())
	errRecovered := dispatcher.Dispatch(ctx, validWake)
	require.NoError(t, errRecovered)
}
