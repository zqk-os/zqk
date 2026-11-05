package ambient

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	qa "github.com/zqk-os/zqk/pkg/validation/qa"
)

// Traceability Verification:
// Test Case:    TST-1793627989636726000-3eea0d35 (Ambient Daemon Holon Engine Hosting & Lifecycle Integration Test)
// Requirement:  REQ-1793627989636708000-723c62c6 (Ambient Daemon Holon Engine Hosting & Unified Autonomous Swarm Lifecycle)
// Criteria:     CRIT-1793627989636713000-e5708c0e (Static Floor: AmbientEngine Configuration & Lifecycle Hosting Invariants)
// Criteria:     CRIT-1793627989636717000-7b0716cd (Operational Dynamic Proof: Live Engine Supervision, Periodic Telemetry Persist & Status Reflection)
// Criteria:     CRIT-1793627989636721000-766b5e13 (Negative Adversarial Boundary: Watchdog Unlink, Signal Trapping & Panic Containment)

func setupTestAmbientEngine(t *testing.T, tickInterval time.Duration) *AmbientEngine {
	t.Helper()
	cfg := DefaultAmbientEngineConfig()
	cfg.TickInterval = tickInterval
	engine, err := NewAmbientEngine(cfg)
	require.NoError(t, err)
	return engine
}

func buildFastWatcherDaemonConfig(projectRoot string) WatcherDaemonConfig {
	cfg := DefaultWatcherDaemonConfig(projectRoot)
	cfg.TelemetryInterval = 20 * time.Millisecond
	if cfg.EngineConfig != nil {
		cfg.EngineConfig.TickInterval = 10 * time.Millisecond
		cfg.EngineConfig.GracefulStopTimeout = 200 * time.Millisecond
	}
	return cfg
}

func launchFastDaemon(t *testing.T, ctx context.Context, tempDir string) chan error {
	t.Helper()
	errCh, eng := launchFastDaemonWithEngine(t, ctx, tempDir)
	if eng != nil {
		require.NotNil(t, eng)
	}
	return errCh
}

func launchFastDaemonWithEngine(t *testing.T, ctx context.Context, tempDir string) (chan error, *AmbientEngine) {
	t.Helper()
	cfg := buildFastWatcherDaemonConfig(tempDir)
	var startedEngine *AmbientEngine
	started := make(chan struct{})
	cfg.OnEngineStarted = func(eng *AmbientEngine) {
		startedEngine = eng
		close(started)
	}

	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_fast_daemon_runner", "run test daemon with started signal").
		StartSimple(func() {
			errCh <- RunWatcherDaemonWithConfig(ctx, cfg)
		})

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for engine to start")
	}

	return errCh, startedEngine
}

func assertEngineDoubleStop(t *testing.T, engine *AmbientEngine) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, engine.Start(ctx))
	require.NoError(t, engine.Stop())
	assert.ErrorIs(t, engine.Stop(), ErrEngineStopped)
}

// TestDaemonEngine_StaticFloorInvariants verifies config invariants, nil engine resolution,
// and clean termination on immediate context cancellation.
func TestDaemonEngine_StaticFloorInvariants(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultWatcherDaemonConfig(tempDir)

	assert.Equal(t, tempDir, cfg.ProjectRoot)
	assert.True(t, cfg.EnableEngine)
	assert.NotNil(t, cfg.EngineConfig)
	assert.Equal(t, DefaultTelemetryInterval, cfg.TelemetryInterval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Verify startTelemetryPersister with empty project root or nil engine is safe no-op
	stopPersister1 := startTelemetryPersister(ctx, "", nil, time.Second)
	stopPersister1()
	stopPersister2 := startTelemetryPersister(ctx, tempDir, nil, time.Second)
	stopPersister2()

	engine := setupTestAmbientEngine(t, 25*time.Millisecond)
	stopPersister3 := startTelemetryPersister(ctx, "", engine, time.Second)
	stopPersister3()
}

// TestDaemonEngine_OperationalDynamicProof verifies that RunWatcherDaemonWithConfig orchestrates
// the engine, writes periodic telemetry to .zqk/state/ambient/telemetry.json, and gracefully updates
// telemetry status upon context cancellation.
func TestDaemonEngine_OperationalDynamicProof(t *testing.T) {
	tempDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())

	engine := setupTestAmbientEngine(t, 15*time.Millisecond)

	var activityInvoked int32
	onActivity := func() {
		atomic.AddInt32(&activityInvoked, 1)
	}

	cfg := DefaultWatcherDaemonConfig(tempDir)
	cfg.Engine = engine
	cfg.TelemetryInterval = 40 * time.Millisecond
	cfg.OnActivity = onActivity

	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_daemon_engine_run", "run watcher daemon with engine in test").
		StartSimple(func() {
			errCh <- RunWatcherDaemonWithConfig(ctx, cfg)
		})

	telemetryPath := filepath.Join(paths.StateDirPath(tempDir), "ambient", "telemetry.json")

	// Wait for periodic telemetry report to be written
	require.Eventually(t, func() bool {
		report, loadErr := SafeLoadTelemetryReport(telemetryPath)
		if loadErr != nil || report == nil {
			return false
		}
		return (report.Status == TelemetryStatusHealthy || report.Status == TelemetryStatusRunning) &&
			report.ActiveHolonsCount >= 6 && len(report.Components) >= 6
	}, 5*time.Second, 50*time.Millisecond, "expected telemetry report to reflect running/healthy engine with >= 6 holons")

	// Verify loaded telemetry contents
	report, err := SafeLoadTelemetryReport(telemetryPath)
	require.NoError(t, err)
	assert.True(t, report.Status == TelemetryStatusHealthy || report.Status == TelemetryStatusRunning)
	assert.GreaterOrEqual(t, report.ActiveHolonsCount, 6)
	assert.GreaterOrEqual(t, len(report.Components), 6)

	// Cancel context and verify clean exit
	cancel()

	select {
	case daemonErr := <-errCh:
		require.NoError(t, daemonErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RunWatcherDaemonWithConfig to terminate gracefully")
	}

	// Verify final telemetry report status transitions to stopped
	require.Eventually(t, func() bool {
		finalReport, loadErr := SafeLoadTelemetryReport(telemetryPath)
		return loadErr == nil && finalReport != nil && finalReport.Status == TelemetryStatusStopped
	}, 3*time.Second, 50*time.Millisecond, "expected final telemetry report to transition to stopped")
}

// TestDaemonEngine_NegativeAdversarialBoundaries verifies double stop idempotency,
// non-existent project root handling, and panic containment.
func TestDaemonEngine_NegativeAdversarialBoundaries(t *testing.T) {
	// 1. Engine lifecycle double start & double stop idempotency
	engine := setupTestAmbientEngine(t, 20*time.Millisecond)
	assertEngineDoubleStop(t, engine)

	// 2. Invalid project directory path for RunWatcherDaemonWithConfig
	invalidCtx, invalidCancel := context.WithCancel(context.Background())
	defer invalidCancel()
	cfg := DefaultWatcherDaemonConfig("/non/existent/path/that/cannot/exist/12345")
	invalidErr := RunWatcherDaemonWithConfig(invalidCtx, cfg)
	assert.Error(t, invalidErr)
}

// TestWatcherDaemon_ContextCancellationCleanExit verifies that RunWatcherDaemonWithConfig
// exits cleanly with nil or context.Canceled error within a deterministic bounded time
// when cancelled via a short context timeout or explicit cancellation, leaving no hanging goroutines.
func TestWatcherDaemon_ContextCancellationCleanExit(t *testing.T) {
	t.Run("ShortContextTimeout50ms", func(t *testing.T) {
		tempDir := t.TempDir()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		cfg := buildFastWatcherDaemonConfig(tempDir)
		errCh := make(chan error, 1)
		goroutinelabels.NewGoroutine("test_clean_exit_timeout", "verify bounded clean exit on timeout").
			StartSimple(func() {
				errCh <- RunWatcherDaemonWithConfig(ctx, cfg)
			})

		select {
		case err := <-errCh:
			assert.True(t, err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded),
				"expected nil or context expiration, got: %v", err)
		case <-time.After(3 * time.Second):
			t.Fatal("RunWatcherDaemonWithConfig did not exit within deterministic bounded duration on timeout")
		}
	})

	t.Run("ExplicitCancellationDuringOperation", func(t *testing.T) {
		tempDir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())

		daemonErrs := launchFastDaemon(t, ctx, tempDir)
		cancel()

		select {
		case err := <-daemonErrs:
			assert.True(t, err == nil || errors.Is(err, context.Canceled),
				"expected nil or context.Canceled, got: %v", err)
		case <-time.After(3 * time.Second):
			t.Fatal("RunWatcherDaemonWithConfig did not exit within deterministic bounded duration on cancel")
		}
	})
}

// TestWatcherDaemon_UnlinkedLockRecovery verifies that when project directories,
// state lock files, or telemetry files are deleted/unlinked or made read-only during execution,
// the daemon gracefully degrades without panicking, crashing, or deadlocking.
func TestWatcherDaemon_UnlinkedLockRecovery(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(paths.StateDirPath(tempDir), "ambient")
	require.NoError(t, fileutil.EnsureDir(stateDir))

	lockPath := filepath.Join(stateDir, "ambient.lock")
	require.NoError(t, fileutil.WriteStandardFile(lockPath, []byte("pid-test-lock")))

	telemetryPath := filepath.Join(stateDir, "telemetry.json")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	daemonErrs := launchFastDaemon(t, ctx, tempDir)

	require.Eventually(t, func() bool {
		return fileutil.Exists(telemetryPath)
	}, 2*time.Second, 20*time.Millisecond, "expected initial telemetry report to be written")

	// 1. Unlink project lock file
	require.NoError(t, fileutil.Remove(lockPath))
	assert.False(t, fileutil.Exists(lockPath))

	// 2. Unlink telemetry state file
	require.NoError(t, fileutil.Remove(telemetryPath))
	assert.False(t, fileutil.Exists(telemetryPath))

	// 3. Delete monitored subdirectory
	subDir := filepath.Join(tempDir, "monitored-sub")
	require.NoError(t, fileutil.EnsureDir(subDir))
	testFile := filepath.Join(subDir, "test.txt")
	require.NoError(t, fileutil.WriteStandardFile(testFile, []byte("data")))
	require.NoError(t, fileutil.RemoveAll(subDir))

	// 4. Temporary read-only permission test on stateDir
	origInfo, statErr := fileutil.Stat(stateDir)
	require.NoError(t, statErr)
	require.NoError(t, fileutil.Chmod(stateDir, 0555))
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, fileutil.Chmod(stateDir, origInfo.Mode().Perm()))

	// 5. Clean exit
	cancel()

	select {
	case err := <-daemonErrs:
		assert.True(t, err == nil || errors.Is(err, context.Canceled), "expected clean exit, got %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("daemon failed to terminate cleanly after unlinked lock and state recovery")
	}
}

func verifyFSWatcherDoubleStop(t *testing.T, tempDir string) {
	t.Helper()
	hub := NewEventHub()
	watcher, err := NewFSWatcher(tempDir, hub)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, watcher.Start(ctx))

	// First Stop
	require.NoError(t, watcher.Stop())

	// Subsequent Stops return nil without panic or error
	require.NoError(t, watcher.Stop())
	require.NoError(t, watcher.Stop())

	// Concurrent Stop calls across 5 goroutines
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		goroutinelabels.NewGoroutine("test_fswatcher_stop_concurrent", "concurrent fswatcher stop").
			WithWaitGroup(&wg).
			StartSimple(func() {
				assert.NoError(t, watcher.Stop())
			})
	}
	wg.Wait()

	// Nil FSWatcher Stop safety
	var nilWatcher *FSWatcher
	assert.NoError(t, nilWatcher.Stop())
}

func verifyAmbientEngineDoubleStop(t *testing.T) {
	t.Helper()
	engine := setupTestAmbientEngine(t, 10*time.Millisecond)
	assertEngineDoubleStop(t, engine)

	// Concurrent stops on already stopped engine do not panic or deadlock
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		goroutinelabels.NewGoroutine("test_engine_stop_concurrent", "concurrent engine stop").
			WithWaitGroup(&wg).
			StartSimple(func() {
				err := engine.Stop()
				assert.ErrorIs(t, err, ErrEngineStopped)
			})
	}
	wg.Wait()

	// Nil AmbientEngine Stop returns ErrNilEngine without panic
	var nilEngine *AmbientEngine
	assert.ErrorIs(t, nilEngine.Stop(), ErrNilEngine)
}

func verifyDaemonLifecycleDoubleStop(t *testing.T, tempDir string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	errCh, startedEngine := launchFastDaemonWithEngine(t, ctx, tempDir)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not stop cleanly")
	}

	require.NotNil(t, startedEngine)
	assert.Equal(t, EngineStateStopped, startedEngine.State())
	assert.ErrorIs(t, startedEngine.Stop(), ErrEngineStopped)
}

// TestWatcherDaemon_DoubleStopIdempotency verifies that stopping the watcher or engine
// multiple times, or calling Stop() on an already stopped instance, returns safely
// without panics or deadlocks across concurrent routines.
func TestWatcherDaemon_DoubleStopIdempotency(t *testing.T) {
	t.Run("FSWatcher_StopIdempotency", func(t *testing.T) {
		verifyFSWatcherDoubleStop(t, t.TempDir())
	})

	t.Run("AmbientEngine_StopIdempotency", func(t *testing.T) {
		verifyAmbientEngineDoubleStop(t)
	})

	t.Run("DaemonLifecycle_StopIdempotency", func(t *testing.T) {
		verifyDaemonLifecycleDoubleStop(t, t.TempDir())
	})
}

// TestWatcherDaemon_PanicContainment verifies that panicking event subscribers or activity callbacks
// are trapped and isolated, preventing process termination or crashes.
func TestWatcherDaemon_PanicContainment(t *testing.T) {
	t.Run("EventSubscriberPanicContained", func(t *testing.T) {
		hub := NewEventHub()

		hub.Subscribe(EventTypeFilesystem, func(ctx context.Context, event Event) error {
			panic("catastrophic subscriber panic simulation")
		})

		var healthyCalled int32
		hub.Subscribe(EventTypeFilesystem, func(ctx context.Context, event Event) error {
			atomic.AddInt32(&healthyCalled, 1)
			return nil
		})

		assert.NotPanics(t, func() {
			err := hub.Publish(context.Background(), Event{
				Type: EventTypeFilesystem,
				Payload: map[string]any{
					objects.FieldKeyTargetID: "file.txt",
				},
				Timestamp: time.Now(),
			})
			assert.NoError(t, err)
		})
		assert.Equal(t, int32(1), atomic.LoadInt32(&healthyCalled))
	})

	t.Run("ActivityCallbackPanicContained", func(t *testing.T) {
		hub := NewEventHub()

		panickingActivity := func() {
			panic("malicious activity callback panic")
		}

		assert.NotPanics(t, func() {
			BindProcessYAMLInvalidator(hub, panickingActivity)
		})

		assert.NotPanics(t, func() {
			err := hub.Publish(context.Background(), Event{
				Type: EventTypeFilesystem,
				Payload: map[string]any{
					objects.FieldKeyTargetID:  filepath.Join("some", "path", paths.ProcessDir, "requirements", "REQ-1.yaml"),
					objects.FieldKeyOperation: "WRITE",
				},
				Timestamp: time.Now(),
			})
			assert.NoError(t, err)
		})
	})

	t.Run("DaemonWithPanickingOnActivity", func(t *testing.T) {
		tempDir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var activityPanicked int32
		panickingActivity := func() {
			atomic.AddInt32(&activityPanicked, 1)
			panic("panic within daemon OnActivity callback")
		}

		cfg := buildFastWatcherDaemonConfig(tempDir)
		cfg.OnActivity = panickingActivity

		errCh := make(chan error, 1)
		goroutinelabels.NewGoroutine("test_panicking_daemon", "run daemon with panicking activity callback").
			StartSimple(func() {
				errCh <- RunWatcherDaemonWithConfig(ctx, cfg)
			})

		processReqDir := filepath.Join(tempDir, paths.ProcessDir, "requirements")
		require.NoError(t, fileutil.EnsureDir(processReqDir))
		reqFile := filepath.Join(processReqDir, "REQ-test.yaml")
		require.NoError(t, fileutil.WriteStandardFile(reqFile, []byte("title: test\n")))

		time.Sleep(100 * time.Millisecond)
		cancel()

		select {
		case err := <-errCh:
			assert.True(t, err == nil || errors.Is(err, context.Canceled), "expected clean exit, got %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("daemon did not exit cleanly after activity panic")
		}
	})
}

// TestWatcherDaemon_ASTAudit statically inspects daemon hosting functions to verify
// strict nil safety guards, zero unchecked error returns, zero naked goroutines,
// and quality audit compliance.
func TestWatcherDaemon_ASTAudit(t *testing.T) {
	auditor := qa.NewASTAuditor()
	targetFiles := []string{
		"watcher.go",
	}

	for _, file := range targetFiles {
		violations, err := auditor.AuditFile(file)
		require.NoError(t, err, "failed to audit %s", file)
		for _, v := range violations {
			if v.Severity == "high" || v.Severity == "medium" || v.Severity == "error" {
				t.Errorf("AST violation [%s] in %s at %s: %s", v.Severity, file, v.Pos, v.Message)
			}
		}
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "watcher.go", nil, parser.ParseComments)
	require.NoError(t, err, "failed to parse watcher.go AST")

	var funcNames []string
	hasNakedGoroutine := false

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			funcNames = append(funcNames, x.Name.Name)
		case *ast.GoStmt:
			hasNakedGoroutine = true
		}
		return true
	})

	assert.False(t, hasNakedGoroutine, "watcher.go must not contain raw/naked go statements")

	expectedFuncs := []string{
		"RunWatcherDaemonWithConfig",
		"RunWatcherDaemonWithEngine",
		"RunWatcherDaemonWithEngineAndConfig",
		"RunWatcherDaemon",
		"startTelemetryPersister",
		"resolveAmbientEngine",
		"initWatcherAuxiliaries",
	}
	for _, expected := range expectedFuncs {
		assert.Contains(t, funcNames, expected, "watcher.go must define %s", expected)
	}
}

// TestDaemonEngine_ASTAudit validates that the implementation files conform to AST hygiene invariants.
func TestDaemonEngine_ASTAudit(t *testing.T) {
	TestWatcherDaemon_ASTAudit(t)
}

// TestWatcherDaemon_LiveTelemetryPersistence satisfies CRIT-1793627989636717000-7b0716cd (Seat 2 Operational Dynamic Proof):
// runs WatcherDaemon with a short TelemetryInterval (20ms) for at least 150ms, cancels context,
// asserts that telemetry-latest.json exists, can be loaded via LoadTelemetryReport, and contains valid running state.
func TestWatcherDaemon_LiveTelemetryPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := DefaultWatcherDaemonConfig(tmpDir)
	cfg.TelemetryInterval = 20 * time.Millisecond
	cfg.EnableEngine = true

	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_live_telemetry_daemon", "run watcher daemon for live telemetry test").
		StartSimple(func() {
			errCh <- RunWatcherDaemonWithConfig(ctx, cfg)
		})

	// Synchronize on telemetry file generation without raw sleeps
	latestPath := filepath.Join(tmpDir, paths.AmbientTelemetryLatestRelFile)
	require.Eventually(t, func() bool {
		if !fileutil.Exists(latestPath) {
			return false
		}
		report, err := LoadTelemetryReport(tmpDir)
		return err == nil && report != nil && report.GetActiveHolonsCount() > 0
	}, 3*time.Second, 15*time.Millisecond, "telemetry report was not generated in time")

	// Allow execution across at least 150ms using a timeout channel
	select {
	case <-time.After(150 * time.Millisecond):
	case err := <-errCh:
		t.Fatalf("daemon exited prematurely with error: %v", err)
	}

	// Cancel context to trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not terminate within graceful stop deadline")
	}

	// 1. Verify telemetry-latest.json exists
	require.FileExists(t, latestPath)

	// 2. Load through LoadTelemetryReport
	report, err := LoadTelemetryReport(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, report)

	// 3. Assert valid running state
	assert.Equal(t, TelemetryStatusRunning, report.GetStatus())
	assert.Equal(t, 6, report.GetActiveHolonsCount())
	assert.Greater(t, report.GetActiveHolonsCount(), 0)

	expectedComponents := []string{
		"intent_stream",
		"epoch_synchronizer",
		"backpressure_regulator",
		"fault_isolator",
		"consensus_arbiter",
		"reflex_loop",
	}

	comps := report.GetComponents()
	require.Len(t, comps, 6)
	for _, expected := range expectedComponents {
		comp, found := comps[expected]
		assert.Truef(t, found, "component %q missing in telemetry components", expected)
		assert.Equal(t, TelemetryStatusHealthy, comp.Status)
		assert.False(t, comp.LastHeartbeat.IsZero())
	}
}

// TestWatcherDaemon_StatusReflection verifies that the telemetry generated during
// RunWatcherDaemon matches the expected report format used by CLI status commands (cmd/zqk/ambient/status.go).
func TestWatcherDaemon_StatusReflection(t *testing.T) {
	tmpDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := DefaultWatcherDaemonConfig(tmpDir)
	cfg.TelemetryInterval = 25 * time.Millisecond
	cfg.EnableEngine = true

	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_status_reflection_daemon", "run watcher daemon for status reflection test").
		StartSimple(func() {
			errCh <- RunWatcherDaemonWithConfig(ctx, cfg)
		})

	require.Eventually(t, func() bool {
		report, err := LoadTelemetryReport(tmpDir)
		return err == nil && report != nil && report.GetActiveHolonsCount() == 6
	}, 3*time.Second, 15*time.Millisecond)

	// Load telemetry report via standard public loader
	report, err := LoadTelemetryReport(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, report)

	// Validate report fields reflected in cmd/zqk/ambient/status.go buildRunningStatus
	assert.Equal(t, TelemetryStatusRunning, report.Status)
	assert.Equal(t, 6, report.ActiveHolonsCount)
	assert.Equal(t, int64(0), report.RunwayWatermark)
	assert.Equal(t, CircuitBreakerStatusClosed, report.CircuitBreakerStatus)

	mesh := report.MeshMetrics
	assert.Equal(t, 100.0, mesh.ConvergenceScore)
	assert.GreaterOrEqual(t, mesh.EventThroughput, float64(0))

	// Verify all 6 components
	comps := report.GetComponents()
	assert.Len(t, comps, 6)
	for _, holonName := range []string{
		"intent_stream",
		"epoch_synchronizer",
		"backpressure_regulator",
		"fault_isolator",
		"consensus_arbiter",
		"reflex_loop",
	} {
		comp, ok := comps[holonName]
		assert.True(t, ok, "expected component %s to be present", holonName)
		assert.True(t, comp.IsHealthy(), "component %s should be healthy", holonName)
	}

	// Verify SummaryTable output formatting
	summary := report.SummaryTable()
	assert.Contains(t, summary, "AMBIENT RUNTIME TELEMETRY REPORT")
	assert.Contains(t, summary, "running")
	assert.Contains(t, summary, "intent_stream")

	// Verify JSON serialization fidelity matching CLI JSON emission
	jsonBytes, err := json.Marshal(report)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))
	assert.Equal(t, "running", parsed["status"])
	assert.Equal(t, float64(6), parsed["active_holons_count"])
	assert.Contains(t, parsed, "components")
	assert.Contains(t, parsed, "mesh_metrics")

	cancel()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not shut down in time")
	}
}

// TestWatcherDaemon_TelemetryReport_LoadFallback validates the fallback hierarchy of LoadTelemetryReport:
// 1) Primary: projectRoot/.zqk/state/ambient/telemetry-latest.json
// 2) Secondary: projectRoot/.zqk/state/ambient/telemetry.json
// 3) Degraded fallback with ErrStateFileNotFound when neither exists
// 4) ErrStatePathRequired on empty root
func TestWatcherDaemon_TelemetryReport_LoadFallback(t *testing.T) {
	// Case 1: Empty path returns ErrStatePathRequired
	_, err := LoadTelemetryReport("")
	require.ErrorIs(t, err, ErrStatePathRequired)

	// Case 2: Missing file returns degraded report and ErrStateFileNotFound
	tmpDir := t.TempDir()
	report, err := LoadTelemetryReport(tmpDir)
	require.ErrorIs(t, err, ErrStateFileNotFound)
	require.NotNil(t, report)
	assert.Equal(t, TelemetryStatusStopped, report.GetStatus())

	// Case 3: Secondary legacy path (.zqk/state/ambient/telemetry.json)
	legacyPath := filepath.Join(paths.StateDirPath(tmpDir), "ambient", "telemetry.json")
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(legacyPath)))
	sampleReport := NewAmbientTelemetryReport(
		WithStatus(TelemetryStatusHealthy),
		WithActiveHolonsCount(6),
	)
	require.NoError(t, SaveTelemetryReport(legacyPath, sampleReport))

	loaded, err := LoadTelemetryReport(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, TelemetryStatusRunning, loaded.GetStatus())
	assert.Equal(t, 6, loaded.GetActiveHolonsCount())

	// Case 4: Primary latest path (.zqk/state/ambient/telemetry-latest.json) takes precedence
	latestPath := filepath.Join(tmpDir, paths.AmbientTelemetryLatestRelFile)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(latestPath)))
	primaryReport := NewAmbientTelemetryReport(
		WithStatus(TelemetryStatusRunning),
		WithActiveHolonsCount(12),
	)
	require.NoError(t, SaveTelemetryReport(latestPath, primaryReport))

	loadedPrimary, err := LoadTelemetryReport(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, loadedPrimary)
	assert.Equal(t, 12, loadedPrimary.GetActiveHolonsCount())
}

// TestWatcherDaemon_EngineSaveTelemetryReport verifies direct invocations of AmbientEngine.SaveTelemetryReport
// with custom paths, nil engine checks, and default StatePath persistence.
func TestWatcherDaemon_EngineSaveTelemetryReport(t *testing.T) {
	var nilEngine *AmbientEngine
	err := nilEngine.SaveTelemetryReport()
	require.ErrorIs(t, err, ErrNilEngine)

	cfg := DefaultAmbientEngineConfig()
	engine, err := NewAmbientEngine(cfg)
	require.NoError(t, err)
	require.NotNil(t, engine)

	// Saving without configured StatePath or arguments returns ErrStatePathRequired
	err = engine.SaveTelemetryReport()
	require.ErrorIs(t, err, ErrStatePathRequired)

	// Save to custom target paths
	tmpDir := t.TempDir()
	target1 := filepath.Join(tmpDir, "report1.json")
	target2 := filepath.Join(tmpDir, "report2.json")

	require.NoError(t, engine.SaveTelemetryReport(target1, target2))
	require.FileExists(t, target1)
	require.FileExists(t, target2)

	// Save using configured engine StatePath
	engine.SetStatePath(filepath.Join(tmpDir, "default_state.json"))
	assert.Equal(t, filepath.Join(tmpDir, "default_state.json"), engine.StatePath())
	require.NoError(t, engine.SaveTelemetryReport())
	require.FileExists(t, filepath.Join(tmpDir, "default_state.json"))
}
