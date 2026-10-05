package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_AuditAggregationSession_UncoveredPhases(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() { _ = sp.Shutdown(ctx) }()

	handler := NewAuditAggregationHandler(sp).(*AuditAggregationHandler)

	job := &ScheduledJob{
		ID:       DefaultAuditEventAggregationSchedulerJobID,
		JobType:  JobTypeAuditEventAggregation,
		Category: CategoryMaintenance,
		EnvironmentVariables: map[string]string{
			"PRE_AGGREGATION_CLEANUP": "true",
			"ARCHIVE_ENABLED":         "true",
			"DELETE_ENABLED":          "false",
			"BATCH_SIZE":              "10",
			"MAX_BATCHES":             "2",
			"MAX_RUNTIME_SECONDS":     "600",
		},
		MaxRuntimeSeconds: 600,
	}

	ctxDeadline, cancel := context.WithDeadline(ctx, time.Now().Add(5*time.Minute))
	defer cancel()

	sess := &auditAggregationSession{
		handler:             handler,
		ctx:                 ctxDeadline,
		job:                 job,
		phaseDurations:      make(map[string]float64),
		effectiveRetention:  1 * time.Hour,
		deleteAfterDuration: 2 * time.Hour,
		windowDuration:      10 * time.Minute,
		catchUpMaxBatches:   2,
		retentionMaxBatches: 2,
		archiveEnabled:      true,
		deleteEnabled:       false,
	}
	_ = sess.loadConfiguration()
	sess.setupPhaseContexts()

	// 1. runPreChecks
	_, _ = sess.runPreChecks()

	// 2. runCatchUp
	sess.runCatchUp()

	// 3. runAggressiveCleanup
	sess.runAggressiveCleanup()

	// 4. runProactiveCleanup
	sess.runProactiveCleanup()

	// 5. runRetentionSecondPass
	sess.runRetentionSecondPass()
}

func TestExtended_JobStateRegistry_DeferAndRetention(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	// 1. Register an execution and then DeferExecution
	jobID := "SCH-defer-test-1"
	execID := "exec-defer-1"
	err := reg.RegisterExecution(jobID, execID, os.Getpid())
	if err != nil {
		t.Fatalf("RegisterExecution failed: %v", err)
	}

	until := time.Now().Add(10 * time.Minute)
	err = reg.DeferExecution(jobID, "waiting for dependency", &until)
	if err != nil {
		t.Errorf("DeferExecution failed: %v", err)
	}

	// Defer with empty jobID
	if reg.DeferExecution("", "reason", nil) == nil {
		t.Errorf("expected error for empty jobID in DeferExecution")
	}

	// 2. findExecutionStateByExecutionID
	path, st, err := reg.findExecutionStateByExecutionID(execID)
	if err != nil || st == nil || path == "" {
		t.Errorf("findExecutionStateByExecutionID failed: %v, st=%+v", err, st)
	}
	_, stNotFound, _ := reg.findExecutionStateByExecutionID("non-existent-exec-id")
	if stNotFound != nil {
		t.Errorf("expected nil st for non-existent execution ID")
	}

	// 3. writeStateHintsFile & Summarize
	_ = reg.writeStateHintsFile()

	sum, err := reg.Summarize(5 * time.Minute)
	if err != nil || sum == nil {
		t.Errorf("Summarize failed: %v", err)
	}

	// 4. cleanupCompletedBestEffort & recordStateRetentionCleanup
	_ = reg.cleanupCompletedBestEffort()
	reg.recordStateRetentionCleanup(stateRetentionCleanupStats{
		RemovedStateFiles: 2,
		RemovedEmptyDirs:  1,
		RemoveErrors:      0,
	})

	// 5. withFileLock
	lockPath := filepath.Join(tmpDir, "test.lock")
	err = withFileLock(lockPath, 2*time.Second, func() error {
		return nil
	})
	if err != nil {
		t.Errorf("withFileLock failed: %v", err)
	}

	err = withFileLock(lockPath, 2*time.Second, func() error {
		return fmt.Errorf("simulated lock error")
	})
	if err == nil {
		t.Errorf("expected error from withFileLock callback")
	}
}

func TestExtended_PIDFile_Deep(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	pid := os.Getpid()

	// 1. Process inspection helpers
	_ = IsDaemonProcessAlive(pid)
	_ = IsDaemonProcessAlive(99999999) // Non-existent PID
	_ = IsProcessRunning(pid)
	_ = IsProcessRunning(99999999)
	_ = isZombieProcess(pid)
	_ = isSchedulerDaemonProcess(pid)
	_ = processBelongsToProjectRoot(pid, tmpDir)

	// 2. Keepalive functions
	err := writeKeepAlive(tmpDir)
	if err != nil {
		t.Errorf("writeKeepAlive failed: %v", err)
	}
	ts, err := readKeepAlive(tmpDir)
	if err != nil || ts.IsZero() {
		t.Errorf("readKeepAlive failed: %v, ts=%v", err, ts)
	}
	err = removeKeepAlive(tmpDir)
	if err != nil {
		t.Errorf("removeKeepAlive failed: %v", err)
	}
	err = RemoveKeepAlive(tmpDir)
	if err != nil {
		t.Errorf("RemoveKeepAlive failed: %v", err)
	}

	// 3. WaitForSchedulerDaemonExit (non-existent PID should exit immediately)
	err = WaitForSchedulerDaemonExit(tmpDir, 99999999, 100*time.Millisecond)
	if err != nil {
		t.Errorf("WaitForSchedulerDaemonExit failed for non-existent PID: %v", err)
	}
}
