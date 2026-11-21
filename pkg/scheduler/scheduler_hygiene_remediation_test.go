package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func mustCreateManualAutofixJob(t *testing.T, storage storagepkg.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, id, createdAt string) {
	t.Helper()
	obj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindSchedulerJob,
		objects.FieldKeyTitle:         "Autofix Job " + id,
		objects.FieldKeyJobType:       JobTypeRunWrapper,
		objects.FieldKeyTriggerType:   TriggerTypeManual,
		objects.FieldKeyExecutionMode: ExecutionModeOneTime,
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyCreatedAt:     createdAt,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
	}
	if err := storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, obj); err != nil {
		t.Fatalf("Create %s: %v", id, err)
	}
}

func assertSchedulerJobExists(t *testing.T, storage storagepkg.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, id string) {
	t.Helper()
	if _, err := storage.Read(ctx, secCtx, id); err != nil {
		t.Errorf("expected SCH-AUTOFIX job %s to be preserved: %v", id, err)
	}
}

func TestSchedulerJobRetentionHandler_ReapsStaleAutofixAndOrphanLogs(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer func() {
		if err := caspkg.GetGlobalListingIndexWriteQueue().FlushKind(objects.KindSchedulerJob, 2*time.Second); err != nil {
			t.Logf("flush warning: %v", err)
		}
		env.Cleanup()
	}()

	storage := env.Storage.(storagepkg.ObjectStorageProvider)
	projectRoot := env.TestRoot
	storagepkg.BuildPathAliasCacheForProject(projectRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	handler := NewSchedulerJobRetentionHandler(storage, projectRoot)
	retentionJob := &ScheduledJob{
		ID:      "SCH-RETENTION-AUTOFIX-TEST",
		JobType: JobTypeSchedulerJobRetention,
		EnvironmentVariables: map[string]string{
			EnvKeyRetentionDays: "7",
			EnvKeyBatchSize:     "50",
			EnvKeyMaxBatches:    "5",
		},
	}

	oldTime := time.Now().UTC().Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	recentTime := time.Now().UTC().Add(-1 * 24 * time.Hour).Format(time.RFC3339)

	staleID := "SCH-AUTOFIX-STALE-001"
	recentID := "SCH-AUTOFIX-RECENT-001"
	runningID := "SCH-AUTOFIX-RUNNING-001"
	mustCreateManualAutofixJob(t, storage, ctx, secCtx, staleID, oldTime)
	mustCreateManualAutofixJob(t, storage, ctx, secCtx, recentID, recentTime)
	mustCreateManualAutofixJob(t, storage, ctx, secCtx, runningID, oldTime)

	// Mark runningID as active in the running-jobs snapshot so retention skips it.
	if err := PersistRunningJobsSnapshot(projectRoot, []string{runningID}); err != nil {
		t.Fatalf("PersistRunningJobsSnapshot: %v", err)
	}

	// Create an orphaned log directory (no matching scheduler_job) and a live log dir.
	orphanLogDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, "scheduler", "SCH-ORPHAN-LOG-DIR")
	liveLogDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, "scheduler", recentID)
	for _, d := range []string{orphanLogDir, liveLogDir} {
		if err := fileutil.MkdirAll(d, paths.DirPerm755); err != nil {
			t.Fatalf("MkdirAll %s: %v", d, err)
		}
	}

	if err := handler.Execute(storagepkg.WithCLIOperation(ctx), retentionJob); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if _, err := storage.Read(ctx, secCtx, staleID); err == nil {
		t.Errorf("expected stale SCH-AUTOFIX job %s to be deleted", staleID)
	}
	assertSchedulerJobExists(t, storage, ctx, secCtx, recentID)
	assertSchedulerJobExists(t, storage, ctx, secCtx, runningID)
	if _, err := fileutil.Stat(orphanLogDir); !fileutil.IsNotExist(err) {
		t.Errorf("expected orphan log dir %s to be removed, stat err=%v", orphanLogDir, err)
	}
	if _, err := fileutil.Stat(liveLogDir); err != nil {
		t.Errorf("expected live log dir %s to be preserved: %v", liveLogDir, err)
	}
}

func newTestSchedulerWithEnv(t *testing.T) (*schedulerCompleteTestEnvironment, *Scheduler) {
	t.Helper()
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	sched := NewScheduler(env.Storage.(storagepkg.ObjectStorageProvider), env.SpecLoader, env.LifecycleLoader).(*Scheduler)
	return env, sched
}

func TestMaybeReloadJobsOnWatchTick_SkipsUnchangedTree(t *testing.T) {
	env, sched := newTestSchedulerWithEnv(t)
	defer env.Cleanup()
	sched.running = true
	ctx := pkgctx.NewSystemContext()

	jobsDir := filepath.Join(env.TestRoot, paths.ProcessDir, "scheduler_jobs")
	if err := fileutil.MkdirAll(jobsDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	fp, ok := schedulerJobsTreeFingerprint(env.TestRoot)
	if !ok || fp == emptyValue {
		t.Fatalf("expected valid initial fingerprint, got (%q, %v)", fp, ok)
	}

	// Unchanged tree must not trigger reload.
	if sched.maybeReloadJobsOnWatchTick(ctx, &fp) {
		t.Fatal("expected maybeReloadJobsOnWatchTick to skip reload when tree is unchanged")
	}

	// Adding a file changes the fingerprint and triggers reload.
	markerFile := filepath.Join(jobsDir, "watch_tick_marker.yaml")
	if err := fileutil.WriteSecureFile(markerFile, []byte("id: SCH-WATCH-MARKER\n")); err != nil {
		t.Fatalf("WriteSecureFile: %v", err)
	}
	if !sched.maybeReloadJobsOnWatchTick(ctx, &fp) {
		t.Fatal("expected maybeReloadJobsOnWatchTick to reload when tree changed")
	}
	// Subsequent tick with no further changes skips again.
	if sched.maybeReloadJobsOnWatchTick(ctx, &fp) {
		t.Fatal("expected maybeReloadJobsOnWatchTick to skip after fingerprint updated")
	}
}

func TestExecuteJob_ConflictDropSkipsReenqueueForTimerJobs(t *testing.T) {
	env, sched := newTestSchedulerWithEnv(t)
	defer env.Cleanup()
	queue := NewJobTriggerQueue(env.TestRoot).(*JobTriggerQueue)

	// Occupy conflict manager with a running non-concurrent run_wrapper job.
	blocker := &ScheduledJob{
		ID:                "SCH-BLOCKER",
		JobType:           JobTypeRunWrapper,
		TriggerType:       TriggerTypeManual,
		ConcurrentAllowed: false,
		Running:           true,
	}
	sched.conflictMgr.RegisterRunning(blocker)
	defer sched.conflictMgr.UnregisterRunning(blocker)

	timerJob := &ScheduledJob{
		ID:                "SCH-TIMER-CONFLICT-TEST",
		JobType:           JobTypeRunWrapper,
		TriggerType:       TriggerTypeTimer,
		ConcurrentAllowed: false,
		Enabled:           true,
	}
	sched.executeJob(context.Background(), timerJob, nil)

	requests, err := queue.DequeueTriggerRequests(50)
	if err != nil {
		t.Fatalf("DequeueTriggerRequests: %v", err)
	}
	for _, req := range requests {
		if req.JobID == timerJob.ID {
			t.Fatalf("expected timer job %s not to be re-enqueued on conflict drop", timerJob.ID)
		}
	}
}
