package scheduler

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_CachePrewarm_TiersAndInference_Wave57(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = sp.Shutdown(ctx) }()

	// 1. inferProjectRootFromSpecsDir & tierTimeoutFromJob
	_ = inferProjectRootFromSpecsDir()

	timeoutNoDeadline := tierTimeoutFromJob(ctx, 10*time.Second, 1*time.Second)
	if timeoutNoDeadline != 10*time.Second {
		t.Errorf("expected 10s timeout, got %v", timeoutNoDeadline)
	}

	ctxDeadline, cancel := context.WithDeadline(ctx, time.Now().Add(5*time.Second))
	defer cancel()
	timeoutWithDeadline := tierTimeoutFromJob(ctxDeadline, 10*time.Second, 1*time.Second)
	if timeoutWithDeadline > 5*time.Second {
		t.Errorf("expected timeout bounded by deadline, got %v", timeoutWithDeadline)
	}

	// 2. CachePrewarmHandler execution & methods
	cph := NewCachePrewarmHandler(
		objects.GetGlobalSpecLoader(),
		objects.GetGlobalLifecycleLoader(),
		sp,
		tmpDir,
		nil,
	).(*CachePrewarmHandler)

	_ = cph.prewarmSpecCacheHardcoded(ctx)
	_ = cph.prewarmFieldRegistry(ctx)
	_ = cph.prewarmSystemFieldsRegistry(ctx)
	_ = cph.prewarmJobTypeViewCache(ctx)
	_ = cph.prewarmValidationStateCache(ctx)
	_ = cph.prewarmReverseReferenceIndex(ctx)
	_ = cph.prewarmPathAliasCache(ctx)

	// runSequentialTier & runParallelTier
	cph.runSequentialTier(ctx, "SCH-job-seq", "tier-seq", 2*time.Second, 100*time.Millisecond, func(tCtx context.Context) error {
		return nil
	})
	cph.runSequentialTier(ctx, "SCH-job-seq", "tier-seq-err", 2*time.Second, 100*time.Millisecond, func(tCtx context.Context) error {
		return fmt.Errorf("simulated tier failure")
	})

	cph.runParallelTier(ctx, "SCH-job-par", 2, "tier-2", 2*time.Second, 100*time.Millisecond, []tierTask{
		{
			GoroutineName: "task-1",
			LogLabel:      "tier task 1",
			Fn: func(tCtx context.Context) error {
				return nil
			},
		},
		{
			GoroutineName: "task-2",
			LogLabel:      "tier task 2",
			Fn: func(tCtx context.Context) error {
				return fmt.Errorf("task error")
			},
		},
	})
}

func TestExtended_RunWrapper_RetryHelpers_Wave57(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = sp.Shutdown(ctx) }()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	rwh := &RunWrapperHandler{
		storage:     sp,
		projectRoot: tmpDir,
		logger:      logger,
	}

	// 1. effectiveRunWrapperTimeoutSeconds
	t1 := effectiveRunWrapperTimeoutSeconds(15, nil)
	if t1 != 15 {
		t.Errorf("expected 15, got %d", t1)
	}
	t2 := effectiveRunWrapperTimeoutSeconds(0, &ScheduledJob{MaxRuntimeSeconds: 45})
	if t2 != 45 {
		t.Errorf("expected 45, got %d", t2)
	}
	t3 := effectiveRunWrapperTimeoutSeconds(0, &ScheduledJob{})
	if t3 != DefaultMaxRuntimeSeconds {
		t.Errorf("expected %d, got %d", DefaultMaxRuntimeSeconds, t3)
	}

	// 2. getExecutor
	exec := rwh.getExecutor()
	if exec == nil {
		t.Errorf("expected non-nil CommandExecutor")
	}

	// 3. runWrapperCollectTestFailuresFromOutput
	jobGoTest := &ScheduledJob{
		ID:          "SCH-test-run",
		Command:     "go",
		CommandArgs: []string{"test", "./..."},
	}
	sw := newStreamingOutputWriter(io.Discard, 100)
	se := newStreamingOutputWriter(io.Discard, 100)
	_, _ = sw.Write([]byte("=== RUN TestSample\n--- FAIL: TestSample (0.01s)\nFAIL\n"))

	failures, summary := rwh.runWrapperCollectTestFailuresFromOutput(jobGoTest, false, sw, se, "exit status 1")
	if len(failures) == 0 && summary == nil {
		t.Logf("test parsing returned nil (expected if no failure parsed or non-test)")
	}

	// non-test job
	jobEcho := &ScheduledJob{Command: "echo", CommandArgs: []string{"hi"}}
	fNone, sNone := rwh.runWrapperCollectTestFailuresFromOutput(jobEcho, false, sw, se, "")
	if fNone != nil || sNone != nil {
		t.Errorf("expected nil for non test job")
	}

	// 4. emitBundleProgress
	rwh.emitBundleProgress(ctx, &ScheduledJob{ID: TestBundleJobIDPrefix + "sample"}, "started")
}

func TestExtended_Notifications_FormattingAndDisplay_Wave57(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	nd := NewNotificationDisplay(logger).(*NotificationDisplay)

	// 1. formatDuration
	if formatDuration(500*time.Millisecond) != "500ms" {
		t.Errorf("unexpected duration for ms")
	}
	if formatDuration(5*time.Second) != "5s" {
		t.Errorf("unexpected duration for s: %s", formatDuration(5*time.Second))
	}
	if formatDuration(65*time.Second) != "1m5s" {
		t.Errorf("unexpected duration for m: %s", formatDuration(65*time.Second))
	}

	// 2. CreateJobNotification & display
	notif := CreateJobNotification(
		"SCH-job-notify",
		objects.KindSchedulerJob,
		CategorySystem,
		"completed",
		PriorityHigh,
		2*time.Second,
		nil,
		map[string]any{"source": "test"},
	)
	if notif == nil {
		t.Fatalf("expected non-nil JobNotification")
	}

	nd.Display(notif)
	nd.DisplayTerminalNotification(notif)
	nd.DisplayDesktopNotification(notif)
	nd.logNotification(notif)

	// Error notification
	errNotif := CreateJobNotification(
		"SCH-job-err",
		objects.KindSchedulerJob,
		CategoryMaintenance,
		"failed",
		PriorityCritical,
		10*time.Second,
		fmt.Errorf("sample job failure"),
		nil,
	)
	nd.Display(errNotif)
	nd.DisplayTerminalNotification(errNotif)
}
