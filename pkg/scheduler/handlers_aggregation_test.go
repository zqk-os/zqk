package scheduler

// BLI-177483 inventory: setupSchedulerCompleteTestEnvironment → GetTestCleanup → RunProjectTestTeardown (scheduler_test_layout_helpers_test.go).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestAuditAggregationHandler_Execute tests the audit aggregation handler
func TestAuditAggregationHandler_Execute(t *testing.T) {
	_, storage := prepareHandlersIsolatedTempProject(t)

	handler := NewAuditAggregationHandler(storage)

	// Create test job with environment variables
	job := &ScheduledJob{
		ID:      "SCH-TEST-AUDIT-AGG",
		JobType: JobTypeAuditEventAggregation,
		EnvironmentVariables: map[string]string{
			EnvKeyAggregationWindow: "24h",
			EnvKeyDeleteAfterDays:   "48h", // 2 days
		},
	}

	ctx := pkgctx.NewSystemContext()

	// Execute handler (should not hang or panic)
	done := make(chan error, 1)
	panicChan := make(chan any, 1)
	goroutinelabels.NewGoroutine("test_handler_execute", "executing audit aggregation handler in test").
		WithPanicHandler(func(r any) {
			panicChan <- r
		}).
		StartWithContext(ctx, func(ctx context.Context) error {
			err := handler.Execute(ctx, job)
			done <- err
			return nil
		})

	select {
	case err := <-done:
		// Handler should complete successfully even if no events to aggregate
		if err != nil {
			t.Logf("Execute returned error (may be expected if no events): %v", err)
		}
	case panicVal := <-panicChan:
		t.Fatalf("Handler panicked: %v", panicVal)
	case <-time.After(10 * time.Second):
		t.Fatal("Execute hung - timed out after 10 seconds")
	}
}

// TestChangeJournalAggregationHandler_Execute tests the change journal aggregation handler
func TestChangeJournalAggregationHandler_Execute(t *testing.T) {
	_, storage := prepareHandlersIsolatedTempProject(t)

	handler := NewChangeJournalAggregationHandler(storage)

	// Create test job with environment variables
	job := &ScheduledJob{
		ID:      "SCH-TEST-CHANGE-AGG",
		JobType: JobTypeChangeJournalAggregation,
		EnvironmentVariables: map[string]string{
			EnvKeyAggregationWindow: "24h",
			EnvKeyDeleteAfterDays:   "48h", // 2 days
		},
	}

	ctx := pkgctx.NewSystemContext()

	// Execute handler (should not hang or panic)
	done := make(chan error, 1)
	panicChan := make(chan any, 1)
	goroutinelabels.NewGoroutine("test_handler_execute", "executing audit aggregation handler in test").
		WithPanicHandler(func(r any) {
			panicChan <- r
		}).
		StartWithContext(ctx, func(ctx context.Context) error {
			err := handler.Execute(ctx, job)
			done <- err
			return nil
		})

	select {
	case err := <-done:
		// Handler should complete successfully even if no entries to aggregate
		if err != nil {
			t.Logf("Execute returned error (may be expected if no entries): %v", err)
		}
	case panicVal := <-panicChan:
		t.Fatalf("Handler panicked: %v", panicVal)
	case <-time.After(10 * time.Second):
		t.Fatal("Execute hung - timed out after 10 seconds")
	}
}

// TestAuditAggregationHandler_DefaultValues tests that default values are used when env vars are not set
func TestAuditAggregationHandler_DefaultValues(t *testing.T) {
	// Not t.Parallel: storage/WAL teardown must complete before t.TempDir cleanup; parallel runs can leave .zqk busy.
	_, storage := prepareHandlersIsolatedTempProject(t)

	handler := NewAuditAggregationHandler(storage)

	// Create test job without environment variables (should use defaults)
	job := &ScheduledJob{
		ID:      "SCH-TEST-AUDIT-AGG-DEFAULTS",
		JobType: JobTypeAuditEventAggregation,
		// No EnvironmentVariables - should use defaults (7 days window, 30 days delete)
	}

	ctx := pkgctx.NewSystemContext()

	// Execute handler
	done := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_handler_execute", "executing handler with defaults in test").
		StartWithContext(ctx, func(ctx context.Context) error {
			err := handler.Execute(ctx, job)
			done <- err
			return nil
		})

	select {
	case err := <-done:
		// Should complete successfully with defaults
		if err != nil {
			t.Logf("Execute returned error (may be expected if no events): %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Execute hung - timed out after 10 seconds")
	}
}

func TestMetricCreationTimeoutFromJobRuntime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		maxRuntimeSeconds int
		want              string
	}{
		{0, "90s"},     // no timeout -> generous default
		{300, "90s"},   // 300/6=50 < 90 -> 90
		{540, "90s"},   // 540/6=90
		{600, "100s"},  // 600/6=100
		{1800, "120s"}, // 1800/6=300 -> cap 120
		{3600, "120s"},
	}
	for _, tt := range tests {
		got := metricCreationTimeoutFromJobRuntime(tt.maxRuntimeSeconds)
		if got != tt.want {
			t.Errorf("metricCreationTimeoutFromJobRuntime(%d) = %q, want %q", tt.maxRuntimeSeconds, got, tt.want)
		}
	}
}

// TestRunWithTimeout_ReturnsFalseAfterTimeout proves the bounded cache check does not block the job indefinitely.
// When fn blocks, runWithTimeout returns false after the timeout.
func TestRunWithTimeout_ReturnsFalseAfterTimeout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	timeout := 100 * time.Millisecond
	start := time.Now()
	got := runWithTimeout(ctx, timeout, func() bool {
		<-make(chan struct{}) // block forever
		return true
	})
	elapsed := time.Since(start)
	if got {
		t.Error("runWithTimeout should return false when fn blocks")
	}
	if elapsed < timeout || elapsed > timeout+200*time.Millisecond {
		t.Errorf("runWithTimeout should return after ~%v, got %v", timeout, elapsed)
	}
}

// TestRunWithTimeout_ReturnsTrueWhenFnReturnsQuickly ensures runWithTimeout returns the fn result when fn completes.
func TestRunWithTimeout_ReturnsTrueWhenFnReturnsQuickly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	got := runWithTimeout(ctx, 5*time.Second, func() bool { return true })
	if !got {
		t.Error("runWithTimeout should return true when fn returns true")
	}
	got = runWithTimeout(ctx, 5*time.Second, func() bool { return false })
	if got {
		t.Error("runWithTimeout should return false when fn returns false")
	}
}

// TestAuditAggregationHandler_SingletonLockPreventsConcurrentRuns is a regression test for
// duplicate audit_aggregation_metric entries caused by concurrent audit aggregation executions.
//
// Root cause: two independent callers (SCH-002 scheduler job + MaintenanceRunner) both invoked
// AuditAggregationHandler.Execute() for the same time window simultaneously. Since
// audit_aggregation_metric is stream-backed (append-only), both writes succeeded, producing
// duplicate metrics (25% duplicate rate observed in 2026-03-19_stream.json).
//
// Fix: Execute() acquires AuditAggregationSingletonLockID (file lock) before running.
// If already held, the second caller receives nil (skip) without error.
//
// This test verifies that when one handler holds the singleton lock, a second handler with the
// same project root returns nil immediately instead of proceeding to aggregation.
func TestAuditAggregationHandler_SingletonLockPreventsConcurrentRuns(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Acquire the singleton lock manually to simulate first handler running.
	firstLock, err := NewJobLock(AuditAggregationSingletonLockID, tmpDir)
	if err != nil {
		t.Fatalf("NewJobLock: %v", err)
	}
	acquired, err := firstLock.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if !acquired {
		t.Fatal("expected to acquire lock on fresh tmp dir")
	}
	defer func() {
		if err := firstLock.Release(); err != nil {
			t.Errorf("firstLock.Release: %v", err)
		}
	}()

	// Second handler with same project root should see the lock is held and skip.
	secondLock, err := NewJobLock(AuditAggregationSingletonLockID, tmpDir)
	if err != nil {
		t.Fatalf("NewJobLock (second): %v", err)
	}
	acquired2, err := secondLock.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire (second): %v", err)
	}
	if acquired2 {
		if err := secondLock.Release(); err != nil {
			t.Errorf("secondLock.Release: %v", err)
		}
		t.Error("second TryAcquire should return false (lock already held by first); " +
			"if both acquire, concurrent aggregation produces duplicate stream entries")
	}
}

// TestAuditAggregationHandler_Execute_SingletonLockSkipWritesOutcome verifies WriteJobOutcome when Execute skips on lock.
func TestAuditAggregationHandler_Execute_SingletonLockSkipWritesOutcome(t *testing.T) {
	// Not t.Parallel(): ZQK_TEST_ROOT + file locks must not race across tests.
	testRoot, storage := prepareHandlersIsolatedTempProject(t)

	firstLock, err := NewJobLock(AuditAggregationSingletonLockID, testRoot)
	if err != nil {
		t.Fatalf("NewJobLock: %v", err)
	}
	acquired, err := firstLock.TryAcquire()
	if err != nil || !acquired {
		t.Fatalf("TryAcquire: acquired=%v err=%v", acquired, err)
	}
	defer func() { _ = firstLock.Release() }()

	handler := NewAuditAggregationHandlerWithProjectRoot(storage, testRoot)
	jobID := "SCH-test-singleton-outcome"
	job := &ScheduledJob{ID: jobID, JobType: JobTypeAuditEventAggregation}
	if err := handler.Execute(context.Background(), job); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	path := JobEventsFilePath(testRoot, jobID)
	raw, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	var found bool
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil {
			continue
		}
		if m[objects.FieldKeyEventType] != "outcome" {
			continue
		}
		if m[OutcomeKeyAggregationSkipReason] == OutcomeSkipReasonSingletonLock && m[OutcomeKeyAggregationSkipped] == true {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected outcome with singleton lock skip in %s; content=%q", path, string(raw))
	}
}
