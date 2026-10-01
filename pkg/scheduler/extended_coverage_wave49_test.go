package scheduler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_SwarmWorker_Wave49(t *testing.T) {
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

	sched := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)

	// 1. Static helpers
	_ = getSwarmWorkerPool()
	_ = resolveSwarmMCPPath(tmpDir)
	_ = fallbackChatModel()

	cfg := &llm.Config{ChatModel: "gemini-2.5-flash"}
	applyCodeDraftSampling(cfg)
	_ = cfg.Temperature

	st, ok := orphanRecoveryStatus(objects.ObjectStatusInProgress)
	if !ok || st != objects.ObjectStatusApproved {
		t.Errorf("unexpected orphanRecoveryStatus in_progress: %s, %v", st, ok)
	}
	_, ok2 := orphanRecoveryStatus("completed")
	if ok2 {
		t.Errorf("expected false for completed")
	}

	claimant := resolveSwarmClaimant(tmpDir, map[string]any{})
	if claimant != "swarm-scheduler" {
		t.Errorf("unexpected claimant: %s", claimant)
	}
	_ = resolveSwarmClaimant(tmpDir, map[string]any{"assignee_persona_ref": "persona-1"})

	// 2. recoverOrphanedTasks and pollAndSpawnSwarmTasks
	sched.recoverOrphanedTasks(ctx)
	sched.pollAndSpawnSwarmTasks(ctx)
}

func TestExtended_ConvergenceSessionTick_Helpers_Wave49(t *testing.T) {
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

	h := NewConvergenceSessionTickHandler(sp, tmpDir).(*ConvergenceSessionTickHandler)

	// 1. Static map and string helpers
	merged := mergeStringAnyMaps(map[string]any{"a": 1}, map[string]any{"b": 2})
	if len(merged) != 2 {
		t.Errorf("unexpected merge: %v", merged)
	}
	_ = mergeStringAnyMaps(nil, nil)

	prev := truncateOrchestrateOutputPreview("short", 10)
	if prev != "short" {
		t.Errorf("unexpected preview: %s", prev)
	}
	prevLong := truncateOrchestrateOutputPreview("long long long", 4)
	if len(prevLong) <= 4 {
		t.Errorf("expected ellipsis: %s", prevLong)
	}

	if thresholdMaxTicksPerHour(map[string]any{"max_ticks_per_hour": 10}) != 10 {
		t.Errorf("unexpected int threshold")
	}
	if thresholdMaxTicksPerHour(map[string]any{"max_ticks_per_hour": float64(20)}) != 20 {
		t.Errorf("unexpected float threshold")
	}
	if thresholdMaxTicksPerHour(nil) != 4 {
		t.Errorf("unexpected default threshold")
	}

	now := time.Now()
	activityLog := []any{
		map[string]any{"timestamp": now.Add(-10 * time.Minute).Format(time.RFC3339), "action": "measure_test_bundle_health"},
		map[string]any{"timestamp": now.Add(-2 * time.Hour).Format(time.RFC3339), "action": "measure_test_bundle_health"},
	}
	nTicks := countMeasureTicksInLastHour(activityLog, now)
	if nTicks != 1 {
		t.Errorf("expected 1 tick in last hour, got %d", nTicks)
	}

	job := &ScheduledJob{
		ID:       "SCH-cvs-tick-h",
		JobType:  JobTypeConvergenceSessionTick,
		Category: CategoryMaintenance,
		EnvironmentVariables: map[string]string{
			"ROLLUP_ON_COMPLETE":     "true",
			"CONVERGENCE_SESSION_ID": "CVS-tick-1",
		},
	}
	if envLookup(job, "ROLLUP_ON_COMPLETE") != "true" {
		t.Errorf("expected true from envLookup")
	}

	// 2. Rollup helpers
	h.MaybeRunOrchestrateRollupAfterTick(ctx, job, "CVS-tick-1")
	h.maybeRunOrchestrateRollupAfterTick(ctx, job, "CVS-tick-1")
}

func TestExtended_CallbackListener_HTTPRoutes_Wave49(t *testing.T) {
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

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sched := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)

	cbHandler := NewCallbackListenerHandler(sp, logger, sched, nil, nil).(*CallbackListenerHandler)
	cbHandler.StopNotificationContext()

	// 1. authenticateRequest
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	if !cbHandler.authenticateRequest(rec, req) {
		t.Errorf("expected authenticateRequest to pass with nil authHook")
	}

	// 2. Route handlers
	job := &ScheduledJob{
		ID:       "SCH-cb-test",
		JobType:  JobTypeCallbackListener,
		Category: CategoryMaintenance,
	}
	payload := map[string]any{"job_id": "SCH-target-1", "result": "ok"}

	cbHandler.handleHealthCheck(rec, req)
	cbHandler.handleJobComplete(rec, req, payload, job)
	cbHandler.handleJobError(rec, req, payload, job)
	cbHandler.handleJobStatus(rec, req, payload, job)
	cbHandler.handleTriggerJob(rec, req, payload, job)
	cbHandler.handleEmitEvent(rec, req, payload, job)

	cbHandler.updateActivity(payload)

	// 3. BuildHTTPServer & shutdownServer
	server := cbHandler.BuildHTTPServer("127.0.0.1:0", nil)
	if server == nil {
		t.Errorf("expected non-nil server")
	}
	cbHandler.server = server
	cbHandler.shutdownServer()
}
