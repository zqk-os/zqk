package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_CachePrewarm_DeepCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
	}()

	specLoader := objects.NewSpecLoader(tmpDir)
	lifecycleLoader := objects.NewLifecycleLoader(tmpDir)

	handlerIface := NewCachePrewarmHandler(specLoader, lifecycleLoader, sp, tmpDir, nil)
	handler := handlerIface.(*CachePrewarmHandler)

	ctx := context.Background()

	// 1. tierTimeoutFromJob
	tDef := tierTimeoutFromJob(ctx, 5*time.Second, 1*time.Second)
	if tDef != 5*time.Second {
		t.Errorf("expected default timeout 5s, got %v", tDef)
	}
	ctxDeadline, cancel := context.WithDeadline(ctx, time.Now().Add(10*time.Second))
	defer cancel()
	tDead := tierTimeoutFromJob(ctxDeadline, 30*time.Second, 2*time.Second)
	if tDead >= 10*time.Second {
		t.Errorf("expected timeout < 10s derived from deadline, got %v", tDead)
	}

	// 2. inferProjectRootFromSpecsDir
	_ = inferProjectRootFromSpecsDir()

	// 3. Direct prewarm helpers
	_ = handler.prewarmSpecCache(ctx)
	_ = handler.prewarmSpecCacheHardcoded(ctx)
	_ = handler.prewarmLifecycleCache(ctx)
	_ = handler.prewarmFieldRegistry(ctx)
	_ = handler.prewarmSystemFieldsRegistry(ctx)
	_ = handler.prewarmJobTypeViewCache(ctx)
	_ = handler.prewarmPathAliasCache(ctx)
	_ = handler.prewarmObjectIDCache(ctx)
	_ = handler.prewarmValidationStateCache(ctx)
	_ = handler.prewarmEnqueueValidation(ctx)
	_ = handler.prewarmReverseReferenceIndex(ctx)

	// 4. WithValidationScanner
	hWithScanner := handler.WithValidationScanner(nil)
	if hWithScanner == nil {
		t.Errorf("expected non-nil handler")
	}

	// 5. Execute with job
	job := &ScheduledJob{ID: "SCH-cache-prewarm"}
	_ = handler.Execute(ctx, job)
}

func TestExtended_JobManagementLoggingHelpers_DeepCoverage(t *testing.T) {
	job := &ScheduledJob{
		ID:           "SCH-log-test",
		JobType:      JobTypeCachePrewarm,
		TriggerType:  TriggerTypeTimer,
		Category:     CategoryMaintenance,
		ScheduleExpr: "*/5 * * * *",
		Command:      "echo",
		CommandArgs:  []string{"hello"},
	}
	lastRun := time.Now()
	job.LastRunAt = &lastRun

	// 1. Logging field helpers
	f1 := jobLogFields(job)
	if len(f1) == 0 {
		t.Errorf("expected non-empty fields")
	}

	f2 := jobLogFieldsWithErr(job, errors.New("sample error"))
	if len(f2) == 0 {
		t.Errorf("expected non-empty fields with err")
	}

	f3 := jobLogFieldsByIDAndErr("SCH-log-test", errors.New("sample error"))
	if len(f3) == 0 {
		t.Errorf("expected non-empty fields by id and err")
	}

	f4 := jobLogFieldsByID("SCH-log-test")
	if len(f4) == 0 {
		t.Errorf("expected non-empty fields by id")
	}

	f5 := logErrField(errors.New("err"))
	if len(f5) == 0 {
		t.Errorf("expected non-empty logErrField")
	}

	f6 := jobLogFieldsWithSchedule(job)
	if len(f6) == 0 {
		t.Errorf("expected non-empty fields with schedule")
	}

	f7 := jobLogFieldsForImmediate(job)
	if len(f7) == 0 {
		t.Errorf("expected non-empty fields for immediate")
	}

	f8 := jobLogFieldsForReexec(job)
	if len(f8) == 0 {
		t.Errorf("expected non-empty fields for reexec")
	}

	f9 := jobLogFieldsWithTriggerType(job)
	if len(f9) == 0 {
		t.Errorf("expected non-empty fields with trigger type")
	}

	f10 := jobLogFieldsWithCategory(job)
	if len(f10) == 0 {
		t.Errorf("expected non-empty fields with category")
	}

	f11 := jobLogFieldsWithCategoryAndDuration(job, 100*time.Millisecond)
	if len(f11) == 0 {
		t.Errorf("expected non-empty fields with category and duration")
	}

	f12 := jobLogFieldsForFailedCommand(job, "echo hello", "exit_error", "exit status 1", 1, 1, 50*time.Millisecond, errors.New("failed"))
	if len(f12) == 0 {
		t.Errorf("expected non-empty fields for failed command")
	}

	f13 := jobLogFieldsForPanic("SCH-log-test", JobTypeCachePrewarm, CategoryMaintenance, "echo", []byte("stack trace"))
	if len(f13) == 0 {
		t.Errorf("expected non-empty fields for panic")
	}

	// 2. priorityDispatchJob
	if !priorityDispatchJob(&ScheduledJob{JobType: JobTypeAuditEventAggregation}) {
		t.Errorf("expected priorityDispatchJob for audit aggregation")
	}
	if priorityDispatchJob(&ScheduledJob{JobType: "other_job"}) {
		t.Errorf("expected false for other job type")
	}
	if priorityDispatchJob(nil) {
		t.Errorf("expected false for nil job")
	}
}
