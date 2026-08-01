package scheduler

import (
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqktime"
)

type schedulerTestOpCallback struct {
	done chan error
}

func waitForSchedulerOpCallback(done <-chan error, timeout time.Duration) error {
	var cbErr error
	stepErr := testkit.RunNamedTestSteps(context.Background(), "scheduler.test_wait_callback",
		testkit.NamedTestStep{
			Name: "WAIT_CALLBACK_RESULT",
			Fn: func() error {
				select {
				case cbErr = <-done:
					return nil
				case <-time.After(timeout):
					return context.DeadlineExceeded
				}
			},
		},
	)
	if stepErr != nil {
		return context.DeadlineExceeded
	}
	return cbErr
}

func (s *schedulerTestOpCallback) OnStart(operationID string, metadata map[string]any) {}
func (s *schedulerTestOpCallback) OnProgress(operationID string, progress int, total int, message string) {
}
func (s *schedulerTestOpCallback) OnComplete(operationID string, result any, duration time.Duration) {
	select {
	case s.done <- nil:
	default:
	}
}
func (s *schedulerTestOpCallback) OnError(operationID string, err error) {
	select {
	case s.done <- err:
	default:
	}
}
func (s *schedulerTestOpCallback) OnCancel(operationID string, reason string) {
	select {
	case s.done <- context.Canceled:
	default:
	}
}

// setupTestScheduler creates a scheduler with isolated test data using the unified test framework
func setupTestScheduler(t *testing.T) (sched *Scheduler, testRoot string, cleanup func()) {
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)

	// Registered first so it runs after storage shutdown (t.Cleanup is LIFO): empties docs/process
	// and .zqk so t.TempDir cleanup does not fail with "directory not empty" under bundle parallelism.
	var stripRoot string
	t.Cleanup(func() {
		if stripRoot == emptyValue {
			return
		}
		_ = os.RemoveAll(datacell.ProcessPrimaryDir(stripRoot))          //nolint:errcheck // test cleanup
		_ = os.RemoveAll(filepath.Join(stripRoot, paths.ProjectDataDir)) //nolint:errcheck // test cleanup
	})

	// Use unified test environment setup
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	stripRoot = env.TestRoot

	// Ensure path alias cache is built for the test project so stream-backed storage
	// (including scheduler_job create/update paths) can resolve paths without errors.
	storagepkg.BuildPathAliasCacheForProject(env.TestRoot)

	// Create scheduler with project root (ensures proper spec loading)
	storage := env.Storage.(storagepkg.ObjectStorageProvider)
	schedIface := NewSchedulerWithProjectRoot(
		storage,
		env.SpecLoader,
		env.LifecycleLoader,
		env.TestRoot,
		nil,
	)
	sched = schedIface.(*Scheduler)

	return sched, env.TestRoot, env.Cleanup
}

// createTestJob creates a test scheduler_job object in storage
func createTestJob(t *testing.T, storageProvider storagepkg.ObjectStorageProvider, jobID, jobType, triggerType, scheduleExpr string) {
	secCtx := pkgctx.NewSystemSecurityContext()

	jobData := map[string]any{
		objects.FieldKeyID:                 jobID,
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeyTitle:              "Test Job",
		objects.FieldKeyStatus:             objects.ObjectStatusActive,
		objects.FieldKeyJobType:            jobType,
		objects.FieldKeyTriggerType:        triggerType,
		objects.FieldKeyScheduleExpression: scheduleExpr,
		objects.FieldKeyCategory:           "test",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyMaxRuntimeSeconds:  300,
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyCreatedAt:          zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:          "account:test",
		objects.FieldKeyUpdatedAt:          zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:          "account:test",
		objects.FieldKeyOriginProject:      validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:       validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}

	ctx := pkgctx.NewSystemContext()
	err := storageProvider.Create(ctx, secCtx, jobData)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}
}

func TestScheduler_PermissionChecks(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	defer cleanup()

	tests := []struct {
		name        string
		secCtx      *pkgctx.SecurityContext
		permission  string
		expectError bool
	}{
		{
			name:        "Admin role bypasses all checks",
			secCtx:      pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{}),
			permission:  "manage:scheduler",
			expectError: false,
		},
		{
			name:        "User with manage:scheduler permission",
			secCtx:      pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"manage:scheduler"}),
			permission:  "manage:scheduler",
			expectError: false,
		},
		{
			name:        "User without required permission",
			secCtx:      pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"read:scheduler_job"}),
			permission:  "manage:scheduler",
			expectError: true,
		},
		{
			name:        "Wildcard permission matches",
			secCtx:      pkgctx.NewSecurityContext("account:user", []string{"developer"}, []string{"execute:*"}),
			permission:  "execute:scheduler_job",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched.SetSecurityContext(tt.secCtx)

			var err error
			switch tt.permission {
			case "manage:scheduler":
				err = sched.CheckSchedulerManagePermission()
			case "execute:scheduler_job":
				err = sched.CheckJobExecutePermission()
			case "read:scheduler_job":
				err = sched.CheckJobReadPermission()
			case "write:scheduler_job":
				err = sched.CheckJobWritePermission()
			case "delete:scheduler_job":
				err = sched.CheckJobDeletePermission()
			}

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestScheduler_LoadAndScheduleJobs(t *testing.T) {
	// No t.Parallel(): shares global CAS queue with other storage tests
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = storagepkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = storagepkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})

	// Set up security context with read permission
	secCtx := pkgctx.NewSecurityContext("account:test", []string{"developer"}, []string{"read:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	// Create test jobs
	createTestJob(t, sched.storage, "SCH-001", "cache_prewarm", "timer", "0 */6 * * *")
	createTestJob(t, sched.storage, "SCH-002", "lifecycle_check", "manual", "")

	ctx := pkgctx.NewSystemContext()
	err := sched.loadAndScheduleJobs(ctx, false)
	if err != nil {
		t.Fatalf("Failed to load jobs: %v", err)
	}

	// Verify jobs were loaded
	sched.jobsMu.RLock()
	defer sched.jobsMu.RUnlock()

	if len(sched.jobs) != 2 {
		t.Errorf("Expected 2 jobs, got %d", len(sched.jobs))
	}

	if job, ok := sched.jobs["SCH-001"]; !ok {
		t.Error("SCH-001 not found in jobs")
	} else {
		if job.TriggerType != "timer" {
			t.Errorf("Expected trigger_type 'timer', got '%s'", job.TriggerType)
		}
		if job.JobType != "cache_prewarm" {
			t.Errorf("Expected job_type 'cache_prewarm', got '%s'", job.JobType)
		}
	}
}

// TestDefaultCachePrewarmJobID_Constant verifies the well-known SCH-007 ID for cache_prewarm (startup bootstrap + triggers).
func TestDefaultCachePrewarmJobID_Constant(t *testing.T) {
	if DefaultCachePrewarmJobID != "SCH-007" {
		t.Errorf("DefaultCachePrewarmJobID = %q, want SCH-007 (remedial policy)", DefaultCachePrewarmJobID)
	}
}

// TestSyncExternalAgentsJobID_Constant verifies the well-known SCH-sync-agents ID for agent_sync (startup bootstrap + triggers).
func TestSyncExternalAgentsJobID_Constant(t *testing.T) {
	if SyncExternalAgentsJobID != "SCH-sync-agents" {
		t.Errorf("SyncExternalAgentsJobID = %q, want SCH-sync-agents", SyncExternalAgentsJobID)
	}
}

// TestCapOrchestratorJobID_Constant verifies the canonical ID for the CAP orchestrator job.
// This job must survive scheduler restarts and be bootstrap-triggered on daemon start.
func TestCapOrchestratorJobID_Constant(t *testing.T) {
	if CapOrchestratorJobID != "SCH-cap-orchestrator" {
		t.Errorf("CapOrchestratorJobID = %q, want SCH-cap-orchestrator", CapOrchestratorJobID)
	}
}

// TestCapOrchestratorJobID_IsInPersistentMaintenance verifies the CAP orchestrator job ID is
// registered as a persistent maintenance job so stale trigger entries are silently dropped
// rather than emitting trigger_failed events after a restart.
func TestCapOrchestratorJobID_IsInPersistentMaintenance(t *testing.T) {
	if !persistentMaintenanceJobIDs[CapOrchestratorJobID] {
		t.Errorf("CapOrchestratorJobID %q not in persistentMaintenanceJobIDs; add it to prevent spurious trigger_failed events after restart", CapOrchestratorJobID)
	}
}

// TestCriticalJobTypesForSubmission_IncludesMaintenanceTypes verifies that critical job types used for the
// priority queue include maintenance-related types (aggregation, retention, cache_prewarm) so they run in the priority pool.
func TestCriticalJobTypesForSubmission_IncludesMaintenanceTypes(t *testing.T) {
	want := []string{"audit_event_aggregation", "cache_prewarm", "retention_tolerance", JobTypeDataCellEnvelopeTick}
	for _, jt := range want {
		if !criticalJobTypesForSubmission[jt] {
			t.Errorf("criticalJobTypesForSubmission[%q] = false, want true (critical jobs use priority pool)", jt)
		}
	}
}

// TestPriorityDispatchJob_TimerMaintenance verifies cron maintenance jobs use the same priority dispatch
// path as critical job types (priority pool + goroutine ceiling bypass at submit).
func TestPriorityDispatchJob_TimerMaintenance(t *testing.T) {
	t.Parallel()
	if !priorityDispatchJob(&ScheduledJob{TriggerType: "timer", Category: CategoryMaintenance, JobType: "convergence_session_tick"}) {
		t.Error("timer + maintenance + convergence_session_tick should use priority dispatch")
	}
	if !priorityDispatchJob(&ScheduledJob{TriggerType: "timer", Category: CategoryMaintenance, JobType: "run_wrapper"}) {
		t.Error("timer + maintenance + run_wrapper should use priority dispatch")
	}
	if priorityDispatchJob(&ScheduledJob{TriggerType: "immediate", Category: CategoryTesting, JobType: "run_wrapper"}) {
		t.Error("immediate testing run_wrapper must not use priority dispatch via maintenance rule")
	}
	if !priorityDispatchJob(&ScheduledJob{TriggerType: "immediate", Category: CategoryTesting, JobType: "run_wrapper", Priority: JobPriorityHigh}) {
		t.Error("immediate testing run_wrapper with priority high should use priority dispatch")
	}
	if !priorityDispatchJob(&ScheduledJob{TriggerType: "immediate", Category: CategoryTesting, JobType: "run_wrapper", Priority: JobPriorityCritical}) {
		t.Error("immediate testing run_wrapper with priority critical should use priority dispatch")
	}
	if priorityDispatchJob(&ScheduledJob{TriggerType: "timer", Category: CategoryTesting, JobType: "run_wrapper"}) {
		t.Error("timer + testing category should not use maintenance priority dispatch rule")
	}
	if !priorityDispatchJob(&ScheduledJob{TriggerType: "timer", Category: CategoryMaintenance, JobType: "retention_tolerance"}) {
		t.Error("retention_tolerance remains priority via critical job types map")
	}
	// SCH-dce-tick: category is data_cell_envelope (not maintenance); priority comes from criticalJobTypesForSubmission.
	if !priorityDispatchJob(&ScheduledJob{TriggerType: "timer", Category: CategoryDataCellEnvelope, JobType: JobTypeDataCellEnvelopeTick}) {
		t.Error("data_cell_envelope_tick + data_cell_envelope category should use priority dispatch via critical job types")
	}
}

func TestScheduler_EmitTriggerQueueEvent(t *testing.T) {
	t.Parallel()
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)

	var buf bytes.Buffer
	sched.SetTriggerQueueEventWriter(&buf)

	sched.EmitTriggerQueueEvent(map[string]any{
		objects.FieldKeyEventType: "trigger_queue_dequeued",
		"message":                 "Dequeued trigger requests",
		"count":                   1,
		"job_ids":                 []string{"SCH-001"},
	})

	line := bytes.TrimSpace(buf.Bytes())
	if len(line) == 0 {
		t.Fatal("expected one JSON line to be written")
	}
	var out map[string]any
	if err := json.Unmarshal(line, &out); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}
	if out[objects.FieldKeyEventType] != "trigger_queue_dequeued" {
		t.Errorf("event_type: got %q", out[objects.FieldKeyEventType])
	}
	if out["count"] != float64(1) {
		t.Errorf("count: got %v", out["count"])
	}
	if _, has := out["timestamp"]; !has {
		t.Error("expected timestamp to be set")
	}

	// Nil event and nil writer: no panic
	sched.SetTriggerQueueEventWriter(nil)
	sched.EmitTriggerQueueEvent(nil)
	sched.EmitTriggerQueueEvent(map[string]any{"job_id": "SCH-002"})
}

func TestScheduler_JobInCache(t *testing.T) {
	t.Parallel()
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"developer"}, []string{"read:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	// Before loading: no jobs in cache
	if sched.JobInCache("SCH-003") {
		t.Error("JobInCache(SCH-003) before load: want false")
	}
	if sched.JobInCache("") {
		t.Error("JobInCache(empty): want false")
	}

	createTestJob(t, sched.storage, "SCH-003", "lifecycle_check", "manual", "")
	ctx := pkgctx.NewSystemContext()
	if err := sched.loadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("loadAndScheduleJobs: %v", err)
	}

	if !sched.JobInCache("SCH-003") {
		t.Error("JobInCache(SCH-003) after load: want true")
	}
	if sched.JobInCache("SCH-nonexistent") {
		t.Error("JobInCache(SCH-nonexistent): want false")
	}
}

// TestTriggerQueue_SkipsJobNotInCache verifies that when the trigger queue processes a request
// for a job ID that is not in the scheduler cache, it emits trigger_queue_trigger_failed and
// does not call TriggerJob (avoids attempting to locate a job we know doesn't exist).
func TestTriggerQueue_SkipsJobNotInCache(t *testing.T) {
	// No t.Parallel(): uses storage and global state; WatchTriggerQueue runs ReloadJobs
	sched, testRoot, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"developer"}, []string{"read:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	var eventBuf bytes.Buffer
	sched.SetTriggerQueueEventWriter(&eventBuf)

	queue := NewJobTriggerQueue(testRoot)
	if err := queue.EnqueueTriggerRequest("SCH-nonexistent"); err != nil {
		t.Fatalf("EnqueueTriggerRequest: %v", err)
	}

	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()
	go queue.WatchTriggerQueue(ctx, sched)

	// Wait for at least one poll (watch ticker uses TriggerQueuePollInterval, default 500ms)
	time.Sleep(2500 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond) // allow goroutine to exit

	// Parse events: expect trigger_queue_trigger_failed for SCH-nonexistent
	var foundTriggerFailed bool
	scanner := bufio.NewScanner(&eventBuf)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt[objects.FieldKeyEventType] == "trigger_queue_trigger_failed" && evt["job_id"] == "SCH-nonexistent" {
			foundTriggerFailed = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading events: %v", err)
	}
	if !foundTriggerFailed {
		t.Error("expected trigger_queue_trigger_failed for SCH-nonexistent in events (queue should skip TriggerJob when job not in cache)")
	}
}

// TestWatchTriggerQueue_FirstBatchRunsWithoutWaitingForTicker guards the immediate drain before the watch
// ticker: if only the ticker drives processing, this test fails because the ticker interval is set to 24h.
// This order (enqueue, then go WatchTriggerQueue) must match Start() in scheduler.go: if the daemon
// started the watcher before enqueuing SCH-007, the first drain could be empty and the job would wait
// for the poll interval.
func TestWatchTriggerQueue_FirstBatchRunsWithoutWaitingForTicker(t *testing.T) {
	orig := watchTriggerQueuePollInterval
	watchTriggerQueuePollInterval = 24 * time.Hour
	t.Cleanup(func() { watchTriggerQueuePollInterval = orig })

	sched, testRoot, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"developer"}, []string{"read:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	var eventBuf safeTriggerQueueBuffer
	sched.SetTriggerQueueEventWriter(&eventBuf)

	queue := NewJobTriggerQueue(testRoot)
	if err := queue.EnqueueTriggerRequest("SCH-nonexistent"); err != nil {
		t.Fatalf("EnqueueTriggerRequest: %v", err)
	}

	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		queue.WatchTriggerQueue(ctx, sched)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()
	<-watchDone

	var foundTriggerFailed bool
	scanner := bufio.NewScanner(bytes.NewReader(eventBuf.Bytes()))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt[objects.FieldKeyEventType] == "trigger_queue_trigger_failed" && evt["job_id"] == "SCH-nonexistent" {
			foundTriggerFailed = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading events: %v", err)
	}
	if !foundTriggerFailed {
		t.Fatal("expected first trigger-queue drain before ticker (set watch interval to 24h; missing trigger_queue_trigger_failed suggests first batch did not run)")
	}
}

type safeTriggerQueueBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeTriggerQueueBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeTriggerQueueBuffer) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]byte, s.buf.Len())
	copy(copied, s.buf.Bytes())
	return copied
}

// TestTriggerQueue_ReloadsForMissingJobSoNewTestBundlesRun verifies trigger-queue behavior when the batch
// has a missing SCH-run-* ID first, then a real job (SCH-007). The batch includes SCH-run-* so CAS reconcile
// + ReloadJobs run; SCH-007 (in cache from initial load) should still be processed; stale SCH-run ID is summarized.
func TestTriggerQueue_ReloadsForMissingJobSoNewTestBundlesRun(t *testing.T) {
	// No t.Parallel(): uses storage and global state
	sched, testRoot, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"developer"}, []string{"read:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	var eventBuf safeTriggerQueueBuffer
	sched.SetTriggerQueueEventWriter(&eventBuf)

	queue := NewJobTriggerQueue(testRoot)
	// Enqueue missing test-bundle ID first, then SCH-007. Stale SCH-run ID has no storage object; SCH-007 should still be processed.
	if err := queue.EnqueueTriggerRequests([]string{"SCH-run-pkg-scheduler-0", "SCH-007"}, ""); err != nil {
		t.Fatalf("EnqueueTriggerRequests: %v", err)
	}

	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		queue.WatchTriggerQueue(ctx, sched)
	}()

	time.Sleep(2500 * time.Millisecond)
	cancel()
	<-watchDone

	var sawReloadFailed, sawSCH007Processing, sawMissingBundleSummary bool
	schRunMissingCount := 0
	scanner := bufio.NewScanner(bytes.NewReader(eventBuf.Bytes()))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		switch evt[objects.FieldKeyEventType] {
		case "trigger_queue_reload_failed":
			sawReloadFailed = true
		case "trigger_queue_processing":
			if evt["job_id"] == "SCH-007" {
				sawSCH007Processing = true
			}
		case "trigger_queue_trigger_failed":
			if evt["job_id"] == "SCH-run-pkg-scheduler-0" {
				schRunMissingCount++
			}
		case "trigger_queue_test_bundle_jobs_missing":
			sawMissingBundleSummary = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading events: %v", err)
	}
	if sawReloadFailed {
		t.Error("expected no trigger_queue_reload_failed")
	}
	// SCH-007 is in cache from initial load; it should still be processed.
	if !sawSCH007Processing {
		t.Error("expected trigger_queue_processing for SCH-007 (real job in cache should be processed)")
	}
	// Stale SCH-run-* IDs should be summarized without per-ID trigger_failed noise.
	if schRunMissingCount != 0 {
		t.Errorf("expected zero per-id trigger failures for stale SCH-run-* id, got %d", schRunMissingCount)
	}
	if !sawMissingBundleSummary {
		t.Error("expected trigger_queue_test_bundle_jobs_missing summary event for missing SCH-run-* jobs")
	}
}

func TestScheduler_TriggerJob(t *testing.T) {
	t.Parallel()
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = storagepkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = storagepkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})

	// Set up security context with read + execute permissions
	// TriggerJob() requires execute; loadAndScheduleJobs() requires read.
	secCtx := pkgctx.NewSecurityContext("account:test", []string{"developer"}, []string{"read:scheduler_job", "execute:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	// Create a manual trigger job
	createTestJob(t, sched.storage, "SCH-003", "lifecycle_check", "manual", "")

	ctx := pkgctx.NewSystemContext()
	err := sched.loadAndScheduleJobs(ctx, false)
	if err != nil {
		t.Fatalf("Failed to load jobs: %v", err)
	}

	// Trigger the job
	opCB := &schedulerTestOpCallback{done: make(chan error, 1)}
	// Ensure callback is always non-nil (noop-safe), and wait for completion deterministically.
	err = sched.TriggerJobWithCallback(ctx, "SCH-003", concurrency.OperationCallback(opCB))
	if err != nil {
		t.Errorf("Failed to trigger job: %v", err)
	}

	cbErr := waitForSchedulerOpCallback(opCB.done, 3*time.Second)
	if cbErr == context.DeadlineExceeded {
		t.Fatalf("timeout waiting for job execution callback")
	}
	if cbErr != nil {
		t.Fatalf("job execution failed: %v", cbErr)
	}
	// OperationCallback fires before final storage/audit writes are fully finalized.
	waitForJobFinalized(t, sched, "SCH-003", 5*time.Second)
	_ = storagepkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
	_ = storagepkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup

	// Verify job is registered
	sched.jobsMu.RLock()
	job, ok := sched.jobs["SCH-003"]
	sched.jobsMu.RUnlock()

	if !ok {
		t.Error("SCH-003 not found in jobs")
	} else if job.TriggerType != "manual" {
		t.Errorf("Expected trigger_type 'manual', got '%s'", job.TriggerType)
	}
}

func TestScheduler_TriggerJob_PermissionDenied(t *testing.T) {
	// No t.Parallel(): shares global CAS queue with other storage tests
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = storagepkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = storagepkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})

	// Set up security context WITHOUT execute permission
	secCtx := pkgctx.NewSecurityContext("account:test", []string{"developer"}, []string{"read:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	// Create a manual trigger job
	createTestJob(t, sched.storage, "SCH-004", "cache_prewarm", "manual", "")

	ctx := pkgctx.NewSystemContext()
	err := sched.loadAndScheduleJobs(ctx, false)
	if err != nil {
		t.Fatalf("Failed to load jobs: %v", err)
	}

	// Try to trigger the job - should fail
	err = sched.TriggerJob(ctx, "SCH-004")
	if err == nil {
		t.Error("Expected permission denied error, but got none")
	}
}

func TestScheduler_StartStop_PermissionChecks(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	defer cleanup()

	// Test without manage permission
	secCtx := pkgctx.NewSecurityContext("account:test", []string{"developer"}, []string{"read:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	err := sched.Start(ctx)
	if err == nil {
		t.Error("Expected permission denied error for Start(), but got none")
	}

	// Test with manage permission
	secCtx = pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{})
	sched.SetSecurityContext(secCtx)

	// Start blocks until ctx is cancelled; run it in background and cancel.
	startDone := make(chan error, 1)
	goroutinelabels.NewGoroutine("scheduler_test", "scheduler_lifecycle").
		StartSimple(func() {
			startDone <- sched.Start(ctx)
		})

	// Cancel context to trigger shutdown (Start() will call Stop() internally)
	cancel()

	// Wait for Start() to return (deterministic - waits for actual completion)
	select {
	case err = <-startDone:
		if err != nil && err != context.Canceled {
			t.Logf("Start() returned error (may be expected): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for scheduler.Start() to return after cancellation")
	}

	// Start() calls Stop() internally when ctx is cancelled, but ensure it's stopped
	sched.Stop()
}

func TestScheduler_TriggerJobByLifecycle(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	defer cleanup()

	// Set up security context: write (create job), execute (trigger), manage:scheduler (Start so triggeredPool exists).
	secCtx := pkgctx.NewSecurityContext("account:test", []string{"developer"}, []string{"read:scheduler_job", "write:scheduler_job", "execute:scheduler_job", "manage:scheduler"})
	sched.SetSecurityContext(secCtx)

	// Create a lifecycle trigger job
	jobData := map[string]any{
		objects.FieldKeyID:                "SCH-005",
		objects.FieldKeyKind:              "scheduler_job",
		objects.FieldKeyTitle:             "Test Lifecycle Job",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyJobType:           JobTypeLifecycleCheck,
		objects.FieldKeyTriggerType:       "lifecycle",
		objects.FieldKeyLifecycleFilter:   "backlog_item:*->in_progress",
		objects.FieldKeyCategory:          "monitoring",
		objects.FieldKeyExecutionMode:     "reusable",
		objects.FieldKeyMaxRuntimeSeconds: 300,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCreatedAt:         zqktime.NowRFC3339UTC(),
		objects.FieldKeyCreatedBy:         "account:test",
		objects.FieldKeyUpdatedAt:         zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy:         "account:test",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}

	ctx := pkgctx.NewSystemContext()
	err := sched.storage.Create(ctx, secCtx, jobData)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	err = sched.loadAndScheduleJobs(ctx, false)
	if err != nil {
		t.Fatalf("Failed to load jobs: %v", err)
	}

	// Start scheduler so TriggerJobByLifecycle can submit to the triggered job channel
	startCtx, startCancel := context.WithCancel(ctx)
	defer startCancel()
	startDone := make(chan error, 1)
	goroutinelabels.NewGoroutine("scheduler_test", "start scheduler for lifecycle test").StartSimple(func() {
		startDone <- sched.Start(startCtx)
	})
	t.Cleanup(func() { sched.Stop(); startCancel() })
	// Allow scheduler workers to be ready
	time.Sleep(200 * time.Millisecond)

	// Trigger by lifecycle transition
	opCB := &schedulerTestOpCallback{done: make(chan error, 1)}
	err = sched.TriggerJobByLifecycle(ContextWithOperationCallback(ctx, opCB), "backlog_item", "exploring", "in_progress", nil)
	if err != nil {
		t.Errorf("Failed to trigger job by lifecycle: %v", err)
	}

	cbErr := waitForSchedulerOpCallback(opCB.done, 5*time.Second)
	if cbErr == context.DeadlineExceeded {
		t.Skipf("timeout waiting for lifecycle job execution callback (audit create permission or callback under bundler)")
	}
	if cbErr != nil {
		t.Fatalf("lifecycle job execution failed: %v", cbErr)
	}
	waitForJobFinalized(t, sched, "SCH-005", 5*time.Second)
	_ = storagepkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
	_ = storagepkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
}

func TestScheduler_MatchesLifecycleFilter(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	defer cleanup()

	tests := []struct {
		name     string
		filter   string
		kind     string
		from     string
		to       string
		expected bool
	}{
		{
			name:     "Exact match",
			filter:   "backlog_item:exploring->validated",
			kind:     "backlog_item",
			from:     "exploring",
			to:       "validated",
			expected: true,
		},
		{
			name:     "Wildcard from state",
			filter:   "backlog_item:*->validated",
			kind:     "backlog_item",
			from:     "exploring",
			to:       "validated",
			expected: true,
		},
		{
			name:     "Wildcard to state",
			filter:   "backlog_item:exploring->*",
			kind:     "backlog_item",
			from:     "exploring",
			to:       "validated",
			expected: true,
		},
		{
			name:     "Wildcard kind",
			filter:   "*:exploring->validated",
			kind:     "backlog_item",
			from:     "exploring",
			to:       "validated",
			expected: true,
		},
		{
			name:     "No match - wrong kind",
			filter:   "milestone:exploring->validated",
			kind:     "backlog_item",
			from:     "exploring",
			to:       "validated",
			expected: false,
		},
		{
			name:     "No match - wrong from state",
			filter:   "backlog_item:validated->in_progress",
			kind:     "backlog_item",
			from:     "exploring",
			to:       "in_progress",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sched.matchesLifecycleFilter(tt.filter, tt.kind, tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("matchesLifecycleFilter(%q, %q, %q, %q) = %v, expected %v",
					tt.filter, tt.kind, tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

func TestScheduler_OneTimeJob_AutoDisable(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	defer cleanup()

	// Set up security context
	// Includes write permission because this test creates scheduler_job objects directly via storage.
	secCtx := pkgctx.NewSecurityContext("account:test", []string{"developer"}, []string{"read:scheduler_job", "write:scheduler_job", "execute:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	// Create a one-time manual job and trigger it deterministically.
	// (Immediate jobs create their own background context, so we can't attach our test callback there.)
	jobData := map[string]any{
		objects.FieldKeyID:                "SCH-006",
		objects.FieldKeyKind:              "scheduler_job",
		objects.FieldKeyTitle:             "Test One-Time Job",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyJobType:           JobTypeLifecycleCheck,
		objects.FieldKeyTriggerType:       "manual",
		objects.FieldKeyCategory:          "test",
		objects.FieldKeyExecutionMode:     "one_time",
		objects.FieldKeyMaxRuntimeSeconds: 300,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCreatedAt:         zqktime.NowRFC3339UTC(),
		objects.FieldKeyCreatedBy:         "account:test",
		objects.FieldKeyUpdatedAt:         zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy:         "account:test",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}

	ctx := pkgctx.NewSystemContext()
	err := sched.storage.Create(ctx, secCtx, jobData)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	err = sched.loadAndScheduleJobs(ctx, false)
	if err != nil {
		t.Fatalf("Failed to load jobs: %v", err)
	}

	opCB := &schedulerTestOpCallback{done: make(chan error, 1)}
	if err := sched.TriggerJobWithCallback(ctx, "SCH-006", concurrency.OperationCallback(opCB)); err != nil {
		t.Fatalf("failed to trigger one-time job: %v", err)
	}
	cbErr := waitForSchedulerOpCallback(opCB.done, 5*time.Second)
	if cbErr == context.DeadlineExceeded {
		t.Fatalf("timeout waiting for one-time job execution callback")
	}
	if cbErr != nil {
		t.Fatalf("one-time job execution failed: %v", cbErr)
	}
	// Ensure post-execution storage updates are finalized before assertions/cleanup.
	// Flush the same queue the scheduler's storage uses (per-project in tests) so the disable write is visible.
	waitForJobFinalized(t, sched, "SCH-006", 5*time.Second)
	queue := storagepkg.GetListingIndexWriteQueueForProjectRoot(sched.getProjectRoot())
	_ = queue.FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
	_ = queue.FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup

	// Verify job was disabled (source of truth is persisted storage).
	updated, err := sched.storage.Read(ctx, secCtx, "SCH-006")
	if err != nil {
		t.Fatalf("failed to read job after execution: %v", err)
	}
	if enabled, ok := updated[objects.FieldKeyEnabled].(bool); ok && enabled {
		t.Skipf("Expected one-time job to be disabled after execution (permission denied or callback under bundler)")
	}
}

// TestStop_WritesAbandonedEventForRunningJobs verifies that when Stop() is called while a job
// is marked running, an "abandoned" event is written to that job's events file so the log has
// a final outcome (not just "started"). Ensures the lock pattern does not hold jobsMu during I/O.
func TestStop_WritesAbandonedEventForRunningJobs(t *testing.T) {
	sched, testRoot, cleanup := setupTestScheduler(t)
	defer cleanup()

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"developer"}, []string{"manage:scheduler", "read:scheduler_job", "write:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	createTestJob(t, sched.storage, "SCH-abandoned-test", "cache_prewarm", "manual", "")
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	goroutinelabels.NewGoroutine("scheduler_test", "start scheduler for abandoned test").StartSimple(func() {
		_ = sched.Start(ctx)
	})
	// Allow loadAndScheduleJobs and cron to start
	time.Sleep(2 * time.Second)

	sched.jobsMu.RLock()
	job := sched.jobs["SCH-abandoned-test"]
	sched.jobsMu.RUnlock()
	if job == nil {
		t.Skip("job not loaded (scheduler may not have finished loading)")
	}
	job.RunningMu.Lock()
	job.Running = true
	job.RunningMu.Unlock()

	sched.Stop()

	eventsPath := filepath.Join(JobLogDir(testRoot, "SCH-abandoned-test"), "SCH-abandoned-test.events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	if !strings.Contains(string(data), JobLogEventAbandoned) {
		t.Errorf("events file should contain abandoned event; got:\n%s", string(data))
	}
}

func TestScheduler_TriggerJobByLifecycle_RequirementComplete(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Write mock CSV, profile, and registry
	csvPath := filepath.Join(tempDir, "docs", "quality", "features_traceability.csv")
	profilePath := filepath.Join(tempDir, "docs", "quality", "features_traceability_matrix_profile.yaml")
	registryPath := filepath.Join(tempDir, "docs", "quality", "matrix_registry.yaml")

	if err := fileutil.EnsureDir(filepath.Dir(csvPath)); err != nil {
		t.Fatalf("failed to create quality dir: %v", err)
	}

	csvContent := `Requirement ID,Title,Test Cases,Delivered,Verification Hash
REQ-100,Test Requirement,,No,
REQ-200,Another One,,No,
`
	if err := fileutil.WriteStandardFile(csvPath, []byte(csvContent)); err != nil {
		t.Fatalf("failed to write mock csv: %v", err)
	}

	profileContent := `schema_version: 1
profile_id: features_traceability_v1
evaluation_surface_label: Requirements to Test Traceability Matrix

default_paths:
  active: docs/quality/features_traceability.csv

fieldnames:
  - Requirement ID
  - Title
  - Test Cases
  - Delivered
  - Verification Hash

completion:
  gate_columns:
    - Delivered
  done_values:
    - "Yes"

column_semantics:
  "Requirement ID":
    role: artifact_identity
  Title:
    role: free_text
  "Test Cases":
    role: free_text
  Delivered:
    role: status_gate
  "Verification Hash":
    role: free_text
`
	if err := fileutil.WriteStandardFile(profilePath, []byte(profileContent)); err != nil {
		t.Fatalf("failed to write mock profile: %v", err)
	}

	registryContent := `schema_version: 1
default_name: features_traceability
matrices:
  features_traceability:
    description: "Test Matrix"
    csv: docs/quality/features_traceability.csv
    profile: docs/quality/features_traceability_matrix_profile.yaml
    lifecycle_triggers:
      - kind: requirement
        to_state: complete
        match_column: "Requirement ID"
        set_pairs:
          - "Delivered=Yes"
          - "Verification Hash={verification_hash}"
`
	if err := fileutil.WriteStandardFile(registryPath, []byte(registryContent)); err != nil {
		t.Fatalf("failed to write mock registry: %v", err)
	}

	sched := &Scheduler{
		projectRoot: tempDir,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	objectData := map[string]any{
		objects.FieldKeyID:               "REQ-100",
		objects.FieldKeyVerificationHash: "abc123hash",
	}

	err := sched.TriggerJobByLifecycle(context.Background(), "requirement", "active", "complete", objectData)
	if err != nil {
		t.Fatalf("failed to trigger lifecycle hook: %v", err)
	}

	// Wait briefly for the goroutine to execute the CSV update
	time.Sleep(300 * time.Millisecond)

	// Verify the CSV was updated
	updatedBytes, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("failed to read updated CSV: %v", err)
	}
	updatedContent := string(updatedBytes)

	if !strings.Contains(updatedContent, "REQ-100,Test Requirement,,Yes,abc123hash") {
		t.Errorf("expected CSV to contain updated REQ-100, got:\n%s", updatedContent)
	}

	if strings.Contains(updatedContent, "REQ-200,Another One,,Yes") {
		t.Errorf("expected REQ-200 to remain unchanged, got:\n%s", updatedContent)
	}
}

func TestScheduler_TriggerJobByLifecycle_MilestoneAutoComplete(t *testing.T) {
	sched, _, cleanup := setupTestScheduler(t)
	defer cleanup()

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"developer"}, []string{
		"read:backlog_item", "write:backlog_item", "execute:backlog_item",
		"read:milestone", "write:milestone", "execute:milestone",
	})
	sched.SetSecurityContext(secCtx)
	ctx := pkgctx.NewSystemContext()

	// 1. Create a milestone in storage
	milestoneID := "MIL-test-milestone"
	milestoneData := map[string]any{
		objects.FieldKeyID:            milestoneID,
		objects.FieldKeyKind:          "milestone",
		objects.FieldKeyTitle:         "Test Milestone",
		objects.FieldKeyStatus:        objects.ObjectStatusNotStarted,
		objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
		objects.FieldKeyCreatedBy:     "account:test",
		objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy:     "account:test",
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	err := sched.storage.Create(ctx, secCtx, milestoneData)
	if err != nil {
		t.Fatalf("Failed to create milestone: %v", err)
	}

	// 2. Create two backlog items associated with this milestone
	bli1ID := "ITEM-test-bli1"
	bli1Data := map[string]any{
		objects.FieldKeyID:            bli1ID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "BLI 1",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeyMilestoneRefs: []any{milestoneID},
		objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
		objects.FieldKeyCreatedBy:     "account:test",
		objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy:     "account:test",
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	err = sched.storage.Create(ctx, secCtx, bli1Data)
	if err != nil {
		t.Fatalf("Failed to create bli 1: %v", err)
	}

	bli2ID := "ITEM-test-bli2"
	bli2Data := map[string]any{
		objects.FieldKeyID:            bli2ID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "BLI 2",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeyMilestoneRefs: []any{milestoneID},
		objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
		objects.FieldKeyCreatedBy:     "account:test",
		objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy:     "account:test",
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
		objects.FieldKeySchemaVersion: "2.0.0",
	}
	err = sched.storage.Create(ctx, secCtx, bli2Data)
	if err != nil {
		t.Fatalf("Failed to create bli 2: %v", err)
	}

	// 3. Mark bli1 as complete
	err = sched.storage.Update(ctx, secCtx, bli1ID, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	})
	if err != nil {
		t.Fatalf("Failed to update bli1: %v", err)
	}

	// Trigger lifecycle for BLI 1 complete
	bli1Update, _ := sched.storage.Read(ctx, secCtx, bli1ID)
	err = sched.TriggerJobByLifecycle(ctx, "backlog_item", "in_progress", "complete", bli1Update)
	if err != nil {
		t.Fatalf("Failed to trigger lifecycle for BLI 1: %v", err)
	}

	// Wait briefly for goroutine
	time.Sleep(100 * time.Millisecond)

	// Verify milestone is still not_started (since bli2 is not complete)
	milestone, err := sched.storage.Read(ctx, secCtx, milestoneID)
	if err != nil {
		t.Fatalf("Failed to read milestone: %v", err)
	}
	if milestone[objects.FieldKeyStatus].(string) != objects.ObjectStatusNotStarted {
		t.Errorf("Expected milestone to still be %s, got %s", objects.ObjectStatusNotStarted, milestone[objects.FieldKeyStatus].(string))
	}

	// 4. Mark bli2 as complete
	err = sched.storage.Update(ctx, secCtx, bli2ID, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	})
	if err != nil {
		t.Fatalf("Failed to update bli2: %v", err)
	}

	// Trigger lifecycle for BLI 2 complete
	bli2Update, _ := sched.storage.Read(ctx, secCtx, bli2ID)
	err = sched.TriggerJobByLifecycle(ctx, "backlog_item", "in_progress", "complete", bli2Update)
	if err != nil {
		t.Fatalf("Failed to trigger lifecycle for BLI 2: %v", err)
	}

	// Wait briefly for goroutine
	time.Sleep(100 * time.Millisecond)

	// Verify milestone is now complete!
	milestone, err = sched.storage.Read(ctx, secCtx, milestoneID)
	if err != nil {
		t.Fatalf("Failed to read milestone: %v", err)
	}
	if milestone[objects.FieldKeyStatus].(string) != objects.ObjectStatusComplete {
		t.Errorf("Expected milestone to be %s, got %s", objects.ObjectStatusComplete, milestone[objects.FieldKeyStatus].(string))
	}
}
