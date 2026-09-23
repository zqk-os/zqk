package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_ActivityCache_Wave53(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	if !activityCacheableJobID("SCH-job-1") || activityCacheableJobID(string(make([]byte, 300))) {
		t.Errorf("unexpected activityCacheableJobID")
	}

	ac := NewActivityCache().(*ActivityCache)
	defer ac.Stop()

	// 1. Update events
	ac.UpdateEvent("SCH-test-1", "started", 0, nil)
	ac.UpdateEvent("SCH-test-1", "completed", 500*time.Millisecond, nil)
	ac.UpdateEvent("SCH-test-2", "failed", 100*time.Millisecond, fmt.Errorf("sample error"))

	// 2. Query entries
	entry, ok := ac.GetEntry("SCH-test-1")
	if !ok || entry == nil {
		t.Errorf("expected entry for SCH-test-1")
	}
	allEntries := ac.GetAllEntries()
	if len(allEntries) < 2 {
		t.Errorf("expected at least 2 entries")
	}
	subset := ac.GetEntriesForJobs([]string{"SCH-test-1", "SCH-missing"})
	if len(subset) != 1 {
		t.Errorf("expected 1 subset entry")
	}

	// 3. Save & Load cache
	err := ac.doSaveCache(tmpDir)
	if err != nil {
		t.Errorf("doSaveCache failed: %v", err)
	}

	meta := ac.GetMetadata()
	if meta == nil {
		t.Errorf("expected non-nil metadata after save")
	}

	_ = ac.SaveCache(tmpDir)

	err = ac.LoadCache(tmpDir)
	if err != nil {
		t.Errorf("LoadCache failed: %v", err)
	}

	_ = GetGlobalActivityCache()
}

func TestExtended_MetricsCollector_Deep_Wave53(t *testing.T) {
	_ = DefaultSchedulerMetricsConfig()
	_ = DisabledSchedulerMetricsConfig()

	m := NewDefaultSchedulerMetricsCollector().(*DefaultSchedulerMetricsCollector)

	m.SetTSDBProvider("dummy_tsdb")
	m.RecordJobLoaded("SCH-1", "cleanup", "timer")
	m.RecordJobScheduled("SCH-1", "cleanup", "timer")
	m.RecordJobExecutionStarted("SCH-1", "cleanup")
	m.RecordJobExecutionCompleted("SCH-1", "cleanup", 100*time.Millisecond, true)
	m.RecordJobExecutionFailed("SCH-1", "cleanup", 50*time.Millisecond, fmt.Errorf("fail"))
	m.RecordHandlerCreated("cleanup")
	m.RecordHandlerCreationFailed("cleanup", fmt.Errorf("factory err"))
	m.RecordTriggerValidation("SCH-1", "timer", true, "ok")
	m.RecordScheduleAttempt("SCH-1", "timer", true)
	m.RecordScheduleError("SCH-1", "timer", fmt.Errorf("err"))
	m.RecordConflictCheck("SCH-1", true)
	m.RecordConflictDetected("SCH-1", "cleanup")
	m.RecordSchedulerStart(200 * time.Millisecond)
	m.RecordSchedulerStop(100 * time.Millisecond)
	m.RecordJobLoadError(fmt.Errorf("load err"))
	m.RecordPoolCreationDeclined("pool1", "test", "quota")
	m.RecordTriggerQueueDequeued(5)
	m.RecordTriggerQueueTriggerFailed("SCH-1", fmt.Errorf("trig err"))
	m.RecordTriggerQueueReloadRetry("SCH-1")
	m.RecordTriggerQueueReloadFailed("SCH-1", fmt.Errorf("reload err"))
	m.RecordDispatchPressureDropped("source1", "backpressure")

	slice := []string{"a", "b"}
	m.appendRecentJobID(&slice, "c")
	m.appendRecentJobIDUnlocked(&slice, "d")
	_ = copyRecent(slice)

	snap := m.GetMetrics()
	if snap.Jobs.Loaded == 0 {
		t.Errorf("expected non-zero jobs loaded")
	}
}

func TestExtended_NotificationsAndStages_Wave53(t *testing.T) {
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
	nd := NewNotificationDisplay(logger)

	// 1. Notification Display
	notif := CreateJobNotification(
		"SCH-notif-1",
		JobTypeCachePrewarm,
		CategoryMaintenance,
		"completed",
		PriorityMedium,
		2*time.Second,
		nil,
		map[string]any{"key": "val"},
	)
	if notif == nil {
		t.Fatalf("expected non-nil notification")
	}

	nd.Display(notif)
	nd.DisplayTerminalNotification(notif)
	nd.DisplayDesktopNotification(notif)

	notifFail := CreateJobNotification(
		"SCH-notif-2",
		JobTypeRunWrapper,
		CategoryMaintenance,
		"failed",
		PriorityHigh,
		5*time.Second,
		fmt.Errorf("fatal job failure"),
		nil,
	)
	nd.Display(notifFail)

	// 2. Cap stage AGI instruction helpers
	capHandler := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)
	_ = capHandler.hasOpenTPMGroomingInstruction(ctx, "PRI-stage-1")
	_ = capHandler.capStageAGIInstruction(ctx, "review", "PRI-stage-1")
	_ = capHandler.capStageAGIInstruction(ctx, "metrics", "PRI-stage-1")
	_ = capHandler.capStageAGIInstruction(ctx, "unknown", "PRI-stage-1")
}
