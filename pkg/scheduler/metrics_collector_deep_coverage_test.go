package scheduler

import (
	"errors"
	"testing"
	"time"
)

func TestExtended_MetricsCollector_DeepCoverage(t *testing.T) {
	// 1. Collector with default config (enabled)
	col := NewDefaultSchedulerMetricsCollector()

	// Job lifecycle metrics
	col.RecordJobLoaded("SCH-1", "periodic", "timer")
	col.RecordJobScheduled("SCH-1", "periodic", "timer")
	col.RecordJobExecutionStarted("SCH-1", "periodic")
	col.RecordJobExecutionCompleted("SCH-1", "periodic", 100*time.Millisecond, true)
	col.RecordJobExecutionFailed("SCH-1", "periodic", 50*time.Millisecond, errors.New("fail"))

	// Handler metrics
	col.RecordHandlerCreated("periodic")
	col.RecordHandlerCreationFailed("periodic", errors.New("cannot create"))

	// Trigger validation metrics
	col.RecordTriggerValidation("SCH-1", "timer", true, "valid")
	col.RecordTriggerValidation("SCH-1", "timer", false, "invalid cron")

	// Schedule metrics
	col.RecordScheduleAttempt("SCH-1", "timer", true)
	col.RecordScheduleError("SCH-1", "timer", errors.New("schedule err"))

	// Conflict metrics
	col.RecordConflictCheck("SCH-1", true)
	col.RecordConflictCheck("SCH-1", false)
	col.RecordConflictDetected("SCH-1", "periodic")

	// Lifecycle metrics
	col.RecordSchedulerStart(200 * time.Millisecond)
	col.RecordSchedulerStop(100 * time.Millisecond)
	col.RecordJobLoadError(errors.New("load error"))

	// Pool metrics
	col.RecordPoolCreationDeclined("pool-1", "testing", "budget full")

	// Trigger-queue metrics
	col.RecordTriggerQueueDequeued(5)
	col.RecordTriggerQueueTriggerFailed("SCH-1", errors.New("trigger err"))
	col.RecordTriggerQueueReloadRetry("SCH-1")
	col.RecordTriggerQueueReloadFailed("SCH-1", errors.New("reload err"))

	// Dispatch pressure metrics
	col.RecordDispatchPressureDropped("cron", "ceiling hit")
	col.RecordDispatchPressureDropped("missed_job_recovery", "pool full")
	col.RecordDispatchPressureDropped("trigger_job_submit", "timeout")
	col.RecordDispatchPressureDropped("immediate_job_submit", "ceiling hit")
	col.RecordDispatchPressureDropped("unknown_source", "other")

	// TSDBProvider and ObservabilityRecorder
	col.SetTSDBProvider(nil)
	rec := col.GetObservabilityRecorder()
	if rec == nil {
		t.Errorf("expected non-nil recorder")
	}

	// Verify snapshot
	snap := col.GetMetrics()
	if snap.Jobs.Loaded != 1 || snap.Jobs.Scheduled != 1 || snap.Jobs.Executed != 1 {
		t.Errorf("unexpected job counts: %+v", snap.Jobs)
	}
	if snap.Handlers.Created != 1 || snap.Handlers.CreationErrors != 1 {
		t.Errorf("unexpected handler counts: %+v", snap.Handlers)
	}
	if snap.Conflict.Checks != 2 || snap.Conflict.Detected != 2 {
		t.Errorf("unexpected conflict counts: %+v", snap.Conflict)
	}
	if snap.DispatchPressure.Total != 5 {
		t.Errorf("expected 5 dispatch pressure dropped, got %d", snap.DispatchPressure.Total)
	}
	if len(snap.RecentExecutionStarted) != 1 || snap.RecentExecutionStarted[0] != "SCH-1" {
		t.Errorf("unexpected recent started: %v", snap.RecentExecutionStarted)
	}

	// 2. Collector with disabled config
	colDis := NewSchedulerMetricsCollectorWithConfig(DisabledSchedulerMetricsConfig())
	colDis.RecordJobLoaded("SCH-dis", "periodic", "timer")
	colDis.RecordJobScheduled("SCH-dis", "periodic", "timer")
	colDis.RecordJobExecutionStarted("SCH-dis", "periodic")
	colDis.RecordJobExecutionCompleted("SCH-dis", "periodic", 10*time.Millisecond, true)
	colDis.RecordJobExecutionFailed("SCH-dis", "periodic", 10*time.Millisecond, errors.New("err"))
	colDis.RecordHandlerCreated("periodic")
	colDis.RecordHandlerCreationFailed("periodic", errors.New("err"))
	colDis.RecordTriggerValidation("SCH-dis", "timer", true, "")
	colDis.RecordScheduleAttempt("SCH-dis", "timer", true)
	colDis.RecordScheduleError("SCH-dis", "timer", errors.New("err"))
	colDis.RecordConflictCheck("SCH-dis", true)
	colDis.RecordConflictDetected("SCH-dis", "periodic")
	colDis.RecordSchedulerStart(10 * time.Millisecond)
	colDis.RecordSchedulerStop(10 * time.Millisecond)
	colDis.RecordJobLoadError(errors.New("err"))
	colDis.RecordPoolCreationDeclined("p", "p", "r")
	colDis.RecordTriggerQueueDequeued(1)
	colDis.RecordTriggerQueueTriggerFailed("SCH-dis", errors.New("err"))
	colDis.RecordTriggerQueueReloadRetry("SCH-dis")
	colDis.RecordTriggerQueueReloadFailed("SCH-dis", errors.New("err"))
	colDis.RecordDispatchPressureDropped("cron", "test")

	snapDis := colDis.GetMetrics()
	if snapDis.Jobs.Loaded != 0 || snapDis.Jobs.Executed != 0 {
		t.Errorf("expected 0 for disabled metrics, got %+v", snapDis.Jobs)
	}
}
