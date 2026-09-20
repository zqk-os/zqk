package scheduler

import (
	"slices"
	"testing"
	"time"
)

func TestDefaultSchedulerMetricsCollector_RecordJobLoaded(t *testing.T) {
	t.Parallel()
	config := DefaultSchedulerMetricsConfig()
	collector := NewSchedulerMetricsCollectorWithConfig(config)

	collector.RecordJobLoaded("JOB-001", "cache_prewarm", "timer")

	snapshot := collector.GetMetrics()
	if snapshot.Jobs.Loaded != 1 {
		t.Errorf("Expected Jobs.Loaded=1, got %d", snapshot.Jobs.Loaded)
	}
}

func TestDefaultSchedulerMetricsCollector_RecordJobExecution(t *testing.T) {
	t.Parallel()
	config := DefaultSchedulerMetricsConfig()
	collector := NewSchedulerMetricsCollectorWithConfig(config)

	collector.RecordJobExecutionStarted("JOB-001", "cache_prewarm")
	collector.RecordJobExecutionCompleted("JOB-001", "cache_prewarm", 5*time.Second, true)

	snapshot := collector.GetMetrics()
	if snapshot.Jobs.Executed != 1 {
		t.Errorf("Expected Jobs.Executed=1, got %d", snapshot.Jobs.Executed)
	}
	if snapshot.Jobs.Succeeded != 1 {
		t.Errorf("Expected Jobs.Succeeded=1, got %d", snapshot.Jobs.Succeeded)
	}
	if snapshot.Timing.AvgExecutionTime != 5*time.Second {
		t.Errorf("Expected Timing.AvgExecutionTime=5s, got %v", snapshot.Timing.AvgExecutionTime)
	}
}

func TestDefaultSchedulerMetricsCollector_RecordJobExecutionFailed(t *testing.T) {
	t.Parallel()
	config := DefaultSchedulerMetricsConfig()
	collector := NewSchedulerMetricsCollectorWithConfig(config)

	collector.RecordJobExecutionStarted("JOB-001", "cache_prewarm")
	collector.RecordJobExecutionFailed("JOB-001", "cache_prewarm", 3*time.Second, nil)

	snapshot := collector.GetMetrics()
	if snapshot.Jobs.Failed != 1 {
		t.Errorf("Expected Jobs.Failed=1, got %d", snapshot.Jobs.Failed)
	}
}

func TestDefaultSchedulerMetricsCollector_MultipleEvents(t *testing.T) {
	t.Parallel()
	config := DefaultSchedulerMetricsConfig()
	collector := NewSchedulerMetricsCollectorWithConfig(config)

	// Record 100 events
	for i := 0; i < 100; i++ {
		collector.RecordJobLoaded("JOB-001", "cache_prewarm", "timer")
	}

	snapshot := collector.GetMetrics()
	if snapshot.Jobs.Loaded != 100 {
		t.Errorf("Expected Jobs.Loaded=100, got %d", snapshot.Jobs.Loaded)
	}
}

func TestDefaultSchedulerMetricsCollector_Disabled(t *testing.T) {
	t.Parallel()
	config := DisabledSchedulerMetricsConfig()
	collector := NewSchedulerMetricsCollectorWithConfig(config)

	collector.RecordJobLoaded("JOB-001", "cache_prewarm", "timer")
	collector.RecordJobScheduled("JOB-001", "cache_prewarm", "timer")

	snapshot := collector.GetMetrics()
	if snapshot.Jobs.Loaded != 0 {
		t.Errorf("Expected Jobs.Loaded=0 when disabled, got %d", snapshot.Jobs.Loaded)
	}
	if snapshot.Jobs.Scheduled != 0 {
		t.Errorf("Expected Jobs.Scheduled=0 when disabled, got %d", snapshot.Jobs.Scheduled)
	}
}

func TestDefaultSchedulerMetricsCollector_GetMetrics(t *testing.T) {
	t.Parallel()
	config := DefaultSchedulerMetricsConfig()
	collector := NewSchedulerMetricsCollectorWithConfig(config)

	// Record some metrics
	collector.RecordJobLoaded("JOB-001", "cache_prewarm", "timer")

	// Should be able to get metrics
	snapshot := collector.GetMetrics()
	if snapshot.Jobs.Loaded != 1 {
		t.Errorf("Expected Jobs.Loaded=1, got %d", snapshot.Jobs.Loaded)
	}
}

func TestDefaultSchedulerMetricsCollector_AllMetrics(t *testing.T) {
	t.Parallel()
	config := DefaultSchedulerMetricsConfig()
	collector := NewSchedulerMetricsCollectorWithConfig(config)

	// Test all metric types
	collector.RecordJobLoaded("JOB-001", "cache_prewarm", "timer")
	collector.RecordJobScheduled("JOB-001", "cache_prewarm", "timer")
	collector.RecordHandlerCreated("cache_prewarm")
	collector.RecordTriggerValidation("JOB-001", "timer", true, "")
	collector.RecordScheduleAttempt("JOB-001", "timer", true)
	collector.RecordConflictCheck("JOB-001", true)
	collector.RecordSchedulerStart(100 * time.Millisecond)
	collector.RecordSchedulerStop(50 * time.Millisecond)

	snapshot := collector.GetMetrics()
	if snapshot.Jobs.Loaded == 0 {
		t.Error("Jobs.Loaded should be > 0")
	}
	if snapshot.Jobs.Scheduled == 0 {
		t.Error("Jobs.Scheduled should be > 0")
	}
	if snapshot.Handlers.Created == 0 {
		t.Error("Handlers.Created should be > 0")
	}
	if snapshot.Trigger.Validations == 0 {
		t.Error("Trigger.Validations should be > 0")
	}
	if snapshot.Schedule.Attempts == 0 {
		t.Error("Schedule.Attempts should be > 0")
	}
	if snapshot.Conflict.Checks == 0 {
		t.Error("Conflict.Checks should be > 0")
	}
	if snapshot.Lifecycle.SchedulerStarts == 0 {
		t.Error("Lifecycle.SchedulerStarts should be > 0")
	}
	if snapshot.Lifecycle.SchedulerStops == 0 {
		t.Error("Lifecycle.SchedulerStops should be > 0")
	}
}

func TestDefaultSchedulerMetricsCollector_DispatchPressureDropped(t *testing.T) {
	t.Parallel()
	collector := NewSchedulerMetricsCollectorWithConfig(DefaultSchedulerMetricsConfig())
	collector.RecordDispatchPressureDropped("cron", dispatchPressureReasonResourceWaitDeadlineExceeded)
	collector.RecordDispatchPressureDropped("cron", dispatchPressureReasonResourceWaitDeadlineExceeded)
	collector.RecordDispatchPressureDropped("trigger_job_submit", dispatchPressureReasonResourceWaitDeadlineExceeded)
	collector.RecordDispatchPressureDropped("unknown_source", "x")

	snap := collector.GetMetrics()
	if snap.DispatchPressure.Total != 4 {
		t.Fatalf("DispatchPressure.Total=%d want 4", snap.DispatchPressure.Total)
	}
	if snap.DispatchPressure.Cron != 2 || snap.DispatchPressure.TriggerJobSubmit != 1 || snap.DispatchPressure.Other != 1 {
		t.Fatalf("unexpected breakdown: %+v", snap.DispatchPressure)
	}
}

// TestDefaultSchedulerMetricsCollector_RecentJobOutcomes verifies that the snapshot exposes
// which job IDs had execution started, completed, or conflict so tests and CLI can assert
// outcomes without guessing from logs.
func TestDefaultSchedulerMetricsCollector_RecentJobOutcomes(t *testing.T) {
	t.Parallel()
	collector := NewSchedulerMetricsCollectorWithConfig(DefaultSchedulerMetricsConfig())

	// Simulate: SCH-A and SCH-B started; SCH-A completed; SCH-C hit conflict
	collector.RecordJobExecutionStarted("SCH-A", "run_wrapper")
	collector.RecordJobExecutionStarted("SCH-B", "run_wrapper")
	collector.RecordJobExecutionCompleted("SCH-A", "run_wrapper", 2*time.Second, true)
	collector.RecordConflictDetected("SCH-C", "lifecycle_check")

	snapshot := collector.GetMetrics()

	if snapshot.Jobs.Executed != 2 {
		t.Errorf("Jobs.Executed: want 2, got %d", snapshot.Jobs.Executed)
	}
	if snapshot.Conflict.Detected != 1 {
		t.Errorf("Conflict.Detected: want 1, got %d", snapshot.Conflict.Detected)
	}

	if !slices.Contains(snapshot.RecentExecutionStarted, "SCH-A") {
		t.Errorf("RecentExecutionStarted should contain SCH-A, got %v", snapshot.RecentExecutionStarted)
	}
	if !slices.Contains(snapshot.RecentExecutionStarted, "SCH-B") {
		t.Errorf("RecentExecutionStarted should contain SCH-B, got %v", snapshot.RecentExecutionStarted)
	}
	if !slices.Contains(snapshot.RecentExecutionCompleted, "SCH-A") {
		t.Errorf("RecentExecutionCompleted should contain SCH-A, got %v", snapshot.RecentExecutionCompleted)
	}
	if !slices.Contains(snapshot.RecentConflictDetected, "SCH-C") {
		t.Errorf("RecentConflictDetected should contain SCH-C, got %v", snapshot.RecentConflictDetected)
	}
	// SCH-B started but did not complete in this scenario
	if slices.Contains(snapshot.RecentExecutionCompleted, "SCH-B") {
		t.Error("RecentExecutionCompleted should not contain SCH-B (only SCH-A completed)")
	}
}

func TestSchedulerMetricsConfig_Presets(t *testing.T) {
	t.Parallel()
	// Test default config
	defaultConfig := DefaultSchedulerMetricsConfig()
	if !defaultConfig.Enabled {
		t.Error("Default config should be enabled")
	}

	// Test disabled config
	disabledConfig := DisabledSchedulerMetricsConfig()
	if disabledConfig.Enabled {
		t.Error("Disabled config should be disabled")
	}
}
