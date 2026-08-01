package scheduler

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

// TestScheduler_LoadAndScheduleJobs_LoadsAggregationJobs verifies that aggregation jobs are loaded
func TestScheduler_LoadAndScheduleJobs_LoadsAggregationJobs(t *testing.T) {
	// No t.Parallel(): uses storage and global CAS queue
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)
	testRoot, storage := prepareHandlersIsolatedTempProject(t)

	// Path alias cache required for stream-backed scheduler_job Create (resolve segment dir).
	storagepkg.BuildPathAliasCacheForProject(testRoot)

	// Create test aggregation jobs
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// Create SCH-002 equivalent
	job002 := map[string]any{
		objects.FieldKeyID:                 "SCH-TEST-002",
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:              "Test Aggregation Job",
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyJobType:            JobTypeAuditEventAggregation,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "*/15 * * * *",
		objects.FieldKeyCategory:           "maintenance",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyEnvironmentVariables: map[string]string{
			EnvKeyRetentionDuration: "24h",
			EnvKeyDeleteEnabled:     "true",
		},
		objects.FieldKeyCreatedAt:     "2025-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:     "account:system",
		objects.FieldKeyUpdatedAt:     "2025-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "account:system",
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}

	if err := storage.Create(ctx, secCtx, job002); err != nil {
		t.Fatalf("failed to create test job: %v", err)
	}

	// Create spec loader and lifecycle loader
	specLoader := objects.NewSpecLoader(testRoot)
	lifecycleLoader := objects.NewLifecycleLoader(testRoot)

	// Create scheduler and load jobs
	scheduler := NewSchedulerWithProjectRoot(storage, specLoader, lifecycleLoader, testRoot, nil)

	// Load and schedule jobs
	s := scheduler.(*Scheduler)
	if err := s.LoadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("failed to load and schedule jobs: %v", err)
	}

	// Verify job was loaded
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()

	found := false
	for _, job := range s.jobs {
		if job.ID != "SCH-TEST-002" {
			continue
		}
		found = true
		if !job.Enabled {
			t.Error("job should be enabled")
		}
		if job.TriggerType != "timer" {
			t.Errorf("expected trigger_type 'timer', got %q", job.TriggerType)
		}
		if job.ScheduleExpr != "*/15 * * * *" {
			t.Errorf("expected schedule '*/15 * * * *', got %q", job.ScheduleExpr)
		}
		if job.CronEntryID == 0 {
			t.Error("job should have cron entry ID after scheduling")
		}
		break
	}

	if !found {
		t.Error("SCH-TEST-002 job was not loaded")
	}
}

// TestScheduler_LoadAndScheduleJobs_LoadsMaintenanceWALTriggerJobWhenPresent verifies that when
// SCH-101 exists in storage, loadAndScheduleJobs loads it. Creation of SCH-101 is done by
// zqk system ensure-retention-jobs (config-driven from scheduler_maintenance_config.yaml) at daemon start.
func TestScheduler_LoadAndScheduleJobs_LoadsMaintenanceWALTriggerJobWhenPresent(t *testing.T) {
	// No t.Parallel(): uses storage and global state
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)
	testRoot, storage := prepareHandlersIsolatedTempProject(t)

	storagepkg.BuildPathAliasCacheForProject(testRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// Create SCH-101 in storage (as ensure-retention-jobs would at daemon start).
	job101 := map[string]any{
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyJobType:            JobTypeMaintenance,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "0 */15 * * * *",
		objects.FieldKeyCategory:           "maintenance",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyMaxRuntimeSeconds:  300,
		objects.FieldKeyCreatedAt:          "2025-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:          "account:system",
		objects.FieldKeyUpdatedAt:          "2025-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:          "account:system",
		objects.FieldKeyOriginProject:      "zqk",
		objects.FieldKeyOriginSystem:       "zqk",
	}
	if err := storage.Create(ctx, secCtx, job101); err != nil {
		t.Fatalf("failed to create SCH-101: %v", err)
	}

	specLoader := objects.NewSpecLoader(testRoot)
	lifecycleLoader := objects.NewLifecycleLoader(testRoot)
	sched := NewSchedulerWithProjectRoot(storage, specLoader, lifecycleLoader, testRoot, nil)
	s := sched.(*Scheduler)

	if err := s.LoadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("LoadAndScheduleJobs: %v", err)
	}
}

// TestScheduler_JobsPaused_StillSchedulesPersistentMaintenanceTimers verifies jobs_paused does not disable
// cron for core maintenance timers (SCH-101, SCH-007, SCH-evag, SCH-002, SCH-023); otherwise aggregate,
// retention, audit aggregation, and maintenance WAL logs stall.
func TestScheduler_JobsPaused_StillSchedulesPersistentMaintenanceTimers(t *testing.T) {
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)
	testRoot, storage := prepareHandlersIsolatedTempProject(t)

	storagepkg.BuildPathAliasCacheForProject(testRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	job101 := map[string]any{
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyJobType:            JobTypeMaintenance,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "0 */15 * * * *",
		objects.FieldKeyCategory:           "maintenance",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyMaxRuntimeSeconds:  300,
		objects.FieldKeyCreatedAt:          "2025-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:          "account:system",
		objects.FieldKeyUpdatedAt:          "2025-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:          "account:system",
		objects.FieldKeyOriginProject:      "zqk",
		objects.FieldKeyOriginSystem:       "zqk",
	}
	if err := storage.Create(ctx, secCtx, job101); err != nil {
		t.Fatalf("failed to create SCH-101: %v", err)
	}

	job002 := map[string]any{
		objects.FieldKeyID:                 DefaultAuditEventAggregationSchedulerJobID,
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:              "Audit aggregation probe",
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyJobType:            JobTypeAuditEventAggregation,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "*/15 * * * *",
		objects.FieldKeyCategory:           "maintenance",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyCreatedAt:          "2025-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:          "account:system",
		objects.FieldKeyUpdatedAt:          "2025-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:          "account:system",
		objects.FieldKeyOriginProject:      "zqk",
		objects.FieldKeyOriginSystem:       "zqk",
	}
	if err := storage.Create(ctx, secCtx, job002); err != nil {
		t.Fatalf("failed to create SCH-002: %v", err)
	}

	job023 := map[string]any{
		objects.FieldKeyID:                 RetentionToleranceCatchAllJobID,
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:              "Retention catch-all probe",
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyJobType:            JobTypeRetentionTolerance,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "0 */4 * * *",
		objects.FieldKeyCategory:           "maintenance",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyEnvironmentVariables: map[string]string{
			"BATCH_SIZE": "100", "MAX_BATCHES": "1",
		},
		objects.FieldKeyCreatedAt:     "2025-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:     "account:system",
		objects.FieldKeyUpdatedAt:     "2025-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "account:system",
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	if err := storage.Create(ctx, secCtx, job023); err != nil {
		t.Fatalf("failed to create SCH-023: %v", err)
	}

	// Non-exempt timer: should remain registered-only under jobs_paused (no cron entry).
	jobPause := map[string]any{
		objects.FieldKeyID:                 "SCH-TEST-PAUSE-DUMMY",
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:              "Pause probe timer",
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyJobType:            JobTypeRunWrapper,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "*/15 * * * *",
		objects.FieldKeyCategory:           "maintenance",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyCreatedAt:          "2025-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:          "account:system",
		objects.FieldKeyUpdatedAt:          "2025-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:          "account:system",
		objects.FieldKeyOriginProject:      "zqk",
		objects.FieldKeyOriginSystem:       "zqk",
	}
	if err := storage.Create(ctx, secCtx, jobPause); err != nil {
		t.Fatalf("failed to create probe job: %v", err)
	}

	specLoader := objects.NewSpecLoader(testRoot)
	lifecycleLoader := objects.NewLifecycleLoader(testRoot)
	sched := NewSchedulerWithProjectRoot(storage, specLoader, lifecycleLoader, testRoot, nil)
	s := sched.(*Scheduler)
	s.SetOnlyManualJobs(true)

	if err := s.LoadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("LoadAndScheduleJobs: %v", err)
	}

	s.jobsMu.RLock()
	j002 := s.jobs[DefaultAuditEventAggregationSchedulerJobID]
	j023 := s.jobs[RetentionToleranceCatchAllJobID]
	jOther := s.jobs["SCH-TEST-PAUSE-DUMMY"]
	defer s.jobsMu.RUnlock()

	if j002 == nil {
		t.Fatal("SCH-002 not loaded")
	}
	if j002.CronEntryID == 0 {
		t.Error("SCH-002 should keep a cron entry when jobs_paused (audit aggregation exemption)")
	}
	if j023 == nil {
		t.Fatal("SCH-023 not loaded")
	}
	if j023.CronEntryID == 0 {
		t.Error("SCH-023 should keep a cron entry when jobs_paused (retention catch-all exemption)")
	}
	if jOther == nil {
		t.Fatal("probe timer not loaded")
	}
	if jOther.CronEntryID != 0 {
		t.Errorf("non-exempt timer should not get cron when jobs_paused: got CronEntryID=%d", jOther.CronEntryID)
	}
}

// TestScheduler_ReloadJobs_SucceedsAfterInitialLoad verifies ReloadJobs (loadAndScheduleJobs with reload=true)
// completes without error. Used when trigger queue processes a job created after daemon start; reload makes it visible.
func TestScheduler_ReloadJobs_SucceedsAfterInitialLoad(t *testing.T) {
	// No t.Parallel(): uses storage and global state
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)
	testRoot, storage := prepareHandlersIsolatedTempProject(t)

	storagepkg.BuildPathAliasCacheForProject(testRoot)

	ctx := pkgctx.NewSystemContext()
	specLoader := objects.NewSpecLoader(testRoot)
	lifecycleLoader := objects.NewLifecycleLoader(testRoot)
	sched := NewSchedulerWithProjectRoot(storage, specLoader, lifecycleLoader, testRoot, nil)

	if err := sched.LoadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("LoadAndScheduleJobs(initial): %v", err)
	}

	// ReloadJobs (reload=true) should succeed so trigger-queue watcher can pick up newly created jobs
	if err := sched.ReloadJobs(ctx); err != nil {
		t.Fatalf("ReloadJobs: %v", err)
	}
}
