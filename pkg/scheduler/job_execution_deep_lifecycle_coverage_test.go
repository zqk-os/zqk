package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	cron "github.com/robfig/cron/v3"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_JobExecution_DeepLifecycle(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-job-exec-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()

	s := &Scheduler{
		cron:                cron.New(cron.WithSeconds(), cron.WithLocation(time.UTC)),
		notificationContext: NewNotificationContext(logger, nil),
		storage:             sp,
		logger:              logger,
		secCtx:              secCtx,
		projectRoot:         tmpDir,
		jobs:                make(map[string]*ScheduledJob),
		conflictMgr:         &ConflictManager{projectRoot: tmpDir, runningJobs: make(map[string]*ScheduledJob)},
		stateRegistry:       NewJobStateRegistry(tmpDir).(*JobStateRegistry),
	}

	jobID := "SCH-exec-lifecycle-1"
	job := &ScheduledJob{
		ID:                jobID,
		JobType:           "test_job",
		Category:          "testing",
		Enabled:           true,
		MaxRuntimeSeconds: 10,
		ExecutionMode:     jobExecutionModeOneTime,
		TriggerType:       "timer",
		ScheduleExpr:      "0 * * * * *",
	}

	// Create job in storage
	jobObj := map[string]any{
		objects.FieldKeyID:            jobID,
		objects.FieldKeyKind:          objects.KindSchedulerJob,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyJobType:       "test_job",
	}
	if err := sp.Create(ctx, secCtx, jobObj); err != nil {
		t.Fatalf("failed to create job object: %v", err)
	}

	// 1. executeJob with successful handler
	mockHandler := &mockExecHandler{
		fn: func(ctx context.Context, j *ScheduledJob) error {
			return nil
		},
	}
	s.executeJob(ctx, job, mockHandler)

	// 2. executeJob with error handler
	job.Enabled = true // re-enable for test
	_ = sp.Update(ctx, secCtx, jobID, map[string]any{objects.FieldKeyEnabled: true})
	mockErrHandler := &mockExecHandler{
		fn: func(ctx context.Context, j *ScheduledJob) error {
			return fmt.Errorf("simulated failure")
		},
	}
	s.executeJob(ctx, job, mockErrHandler)

	// 3. executeJob with TestBundleSuccessWithFailuresPrefix
	job.Enabled = true
	_ = sp.Update(ctx, secCtx, jobID, map[string]any{objects.FieldKeyEnabled: true})
	mockWarnHandler := &mockExecHandler{
		fn: func(ctx context.Context, j *ScheduledJob) error {
			return fmt.Errorf("%s 2 tests failed", TestBundleSuccessWithFailuresPrefix)
		},
	}
	s.executeJob(ctx, job, mockWarnHandler)

	// 4. executeJob when job is disabled in storage (early exit)
	job.Enabled = false
	_ = sp.Update(ctx, secCtx, jobID, map[string]any{objects.FieldKeyEnabled: false})
	s.executeJob(ctx, job, mockHandler)

	// 5. executeJob when context already expired (dispatch dropped branch)
	expiredCtx, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	s.executeJob(expiredCtx, job, mockHandler)

	// 6. Test WriteJobOutcome & WriteJobProgress
	WriteJobOutcome(tmpDir, jobID, "test_job", map[string]any{"tests_run": 5})
	WriteJobProgress(tmpDir, jobID, map[string]any{"percent": 50})

	// 7. Test trimJobLogFileIfNeeded
	logFile := filepath.Join(tmpDir, "test_trim.log")
	f, _ := os.Create(logFile)
	for i := 0; i < 20; i++ {
		_, _ = f.WriteString(fmt.Sprintf("line %d\n", i))
	}
	_ = f.Close()
	trimJobLogFileIfNeeded(logFile, 5)

	// 8. Test dispatchContextForScheduledJob
	ctx1, cancel1 := dispatchContextForScheduledJob(&ScheduledJob{MaxRuntimeSeconds: 30})
	defer cancel1()
	if ctx1 == nil {
		t.Error("expected non-nil ctx from dispatchContextForScheduledJob")
	}

	ctx2, cancel2 := dispatchContextForScheduledJob(&ScheduledJob{MaxRuntimeSeconds: 0})
	defer cancel2()
	if ctx2 == nil {
		t.Error("expected non-nil ctx from dispatchContextForScheduledJob with 0 max runtime")
	}

	// 9. Test DisableJobInStorage directly
	s.DisableJobInStorage(ctx, job)
}
