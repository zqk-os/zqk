package scheduler

import (
	"context"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/robfig/cron/v3"
)

// TestScheduler_UpdateJobInStorage_UpdatesLastRunAt verifies that updateJobInStorage persists last_run_at to storage
func TestScheduler_UpdateJobInStorage_UpdatesLastRunAt(t *testing.T) {
	// Run without t.Parallel() to avoid hash registry context cancellation when tests share global CAS queue
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create a test job in storage
	jobID := "SCH-TEST-UPDATE-001"
	now := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:                 jobID,
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeyTitle:              "Test Job",
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyJobType:            JobTypeCachePrewarm,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "*/15 * * * *",
		objects.FieldKeyCategory:           "test",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyMaxRuntimeSeconds:  300,
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyCreatedAt:          now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyCreatedBy:          "ACC-TEST",
		objects.FieldKeyUpdatedAt:          now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyUpdatedBy:          "ACC-TEST",
		objects.FieldKeyOriginProject:      "zqk",
		objects.FieldKeyOriginSystem:       "zqk",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}

	err := sched.storage.Create(ctx, secCtx, jobData)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}
	// Flush so hash registry and CAS index are written before we load jobs
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 5*time.Second) //nolint:errcheck // best-effort

	// Load and schedule the job
	err = sched.loadAndScheduleJobs(ctx, false)
	if err != nil {
		t.Fatalf("Failed to load jobs: %v", err)
	}

	// Get the scheduled job
	sched.jobsMu.RLock()
	job, ok := sched.jobs[jobID]
	sched.jobsMu.RUnlock()

	if !ok {
		t.Fatalf("Job %s not found after loading", jobID)
	}

	// Set last run time
	lastRunTime := time.Now().UTC().Add(-5 * time.Minute)
	job.LastRunAt = &lastRunTime

	// Call updateJobInStorage
	sched.updateJobInStorage(ctx, job)

	// Flush so any async index updates are visible before Read
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort

	// Verify the job was updated in storage
	updatedJob, err := sched.storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("Failed to read updated job: %v", err)
	}

	// Check last_run_at was persisted
	lastRunAtStr, ok := updatedJob[objects.FieldKeyLastRunAt].(string)
	if !ok {
		t.Fatal("last_run_at not found in updated job or not a string")
	}

	lastRunAt, err := time.Parse(time.RFC3339, lastRunAtStr)
	if err != nil {
		t.Fatalf("Failed to parse last_run_at: %v", err)
	}

	// Allow 1 second tolerance for time differences
	if lastRunAt.Sub(lastRunTime).Abs() > time.Second {
		t.Errorf("Expected last_run_at %v, got %v", lastRunTime, lastRunAt)
	}
}

// TestScheduler_UpdateJobInStorage_UpdatesNextRunAt verifies that updateJobInStorage persists next_run_at to storage
func TestScheduler_UpdateJobInStorage_UpdatesNextRunAt(t *testing.T) {
	// Run without t.Parallel() to avoid hash registry context cancellation when tests share global CAS queue
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create a test job in storage
	jobID := "SCH-TEST-UPDATE-002"
	now := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:                 jobID,
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeyTitle:              "Test Job",
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyJobType:            JobTypeCachePrewarm,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "*/15 * * * *",
		objects.FieldKeyCategory:           "test",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyMaxRuntimeSeconds:  300,
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyCreatedAt:          now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyCreatedBy:          "ACC-TEST",
		objects.FieldKeyUpdatedAt:          now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyUpdatedBy:          "ACC-TEST",
		objects.FieldKeyOriginProject:      "zqk",
		objects.FieldKeyOriginSystem:       "zqk",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}

	err := sched.storage.Create(ctx, secCtx, jobData)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}
	// Flush so hash registry and CAS index are written before we load jobs
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 5*time.Second) //nolint:errcheck // best-effort

	// Load and schedule the job
	err = sched.loadAndScheduleJobs(ctx, false)
	if err != nil {
		t.Fatalf("Failed to load jobs: %v", err)
	}

	// Get the scheduled job
	sched.jobsMu.RLock()
	job, ok := sched.jobs[jobID]
	sched.jobsMu.RUnlock()

	if !ok {
		t.Fatalf("Job %s not found after loading", jobID)
	}

	// Set last run time (next run time is computed from cron entry inside updateJobInStorage)
	lastRunTime := time.Now().UTC().Add(-5 * time.Minute)
	job.LastRunAt = &lastRunTime

	// Call updateJobInStorage
	sched.updateJobInStorage(ctx, job)

	// Flush so any async index updates are visible before Read
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort

	// Verify the job was updated in storage
	updatedJob, err := sched.storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("Failed to read updated job: %v", err)
	}

	// Check next_run_at was persisted
	nextRunAtStr, ok := updatedJob[objects.FieldKeyNextRunAt].(string)
	if !ok {
		t.Fatal("next_run_at not found in updated job or not a string")
	}

	nextRunAt, err := time.Parse(time.RFC3339, nextRunAtStr)
	if err != nil {
		t.Fatalf("Failed to parse next_run_at: %v", err)
	}

	// next_run_at should be present and in the future
	if !nextRunAt.After(time.Now().UTC().Add(-time.Second)) {
		t.Errorf("Expected next_run_at to be >= now, got %v", nextRunAt)
	}
}

// TestScheduler_UpdateJobInStorage_UpdatesEnabled verifies that updateJobInStorage persists enabled status to storage
func TestScheduler_UpdateJobInStorage_UpdatesEnabled(t *testing.T) {
	// No t.Parallel(): shares global CAS queue with other storage tests
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create a test job in storage
	jobID := "SCH-TEST-UPDATE-003"
	now := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:                 jobID,
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeyTitle:              "Test Job",
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyJobType:            JobTypeCachePrewarm,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "*/15 * * * *",
		objects.FieldKeyCategory:           "test",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyMaxRuntimeSeconds:  300,
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyCreatedAt:          now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyCreatedBy:          "ACC-TEST",
		objects.FieldKeyUpdatedAt:          now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyUpdatedBy:          "ACC-TEST",
		objects.FieldKeyOriginProject:      "zqk",
		objects.FieldKeyOriginSystem:       "zqk",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}

	err := sched.storage.Create(ctx, secCtx, jobData)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	// Load and schedule the job
	err = sched.loadAndScheduleJobs(ctx, false)
	if err != nil {
		t.Fatalf("Failed to load jobs: %v", err)
	}

	// Get the scheduled job
	sched.jobsMu.RLock()
	job, ok := sched.jobs[jobID]
	sched.jobsMu.RUnlock()

	if !ok {
		t.Fatalf("Job %s not found after loading", jobID)
	}

	// Set last run time and disable the job
	lastRunTime := time.Now().UTC().Add(-5 * time.Minute)
	job.LastRunAt = &lastRunTime
	job.Enabled = false

	// Call updateJobInStorage
	sched.updateJobInStorage(ctx, job)

	// Verify the job was updated in storage
	updatedJob, err := sched.storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("Failed to read updated job: %v", err)
	}

	// Check enabled was persisted
	enabled, ok := updatedJob[objects.FieldKeyEnabled].(bool)
	if !ok {
		t.Fatal("enabled not found in updated job or not a boolean")
	}

	if enabled {
		t.Error("Expected enabled to be false, got true")
	}
}

// TestScheduler_UpdateJobInStorage_HandlesStorageError verifies that updateJobInStorage handles storage errors gracefully
func TestScheduler_UpdateJobInStorage_HandlesStorageError(t *testing.T) {
	// No t.Parallel(): shares global CAS queue with other storage tests
	sched, _, cleanup := setupTestScheduler(t)
	defer cleanup()

	ctx := pkgctx.NewSystemContext()

	// Create a job that doesn't exist in storage (will cause Update to fail)
	job := &ScheduledJob{
		ID:        "SCH-NONEXISTENT",
		JobType:   JobTypeCachePrewarm,
		LastRunAt: func() *time.Time { t := time.Now().UTC(); return &t }(),
		Enabled:   true,
	}

	// This should not panic, just log a warning
	sched.updateJobInStorage(ctx, job)

	// Test passes if no panic occurred
}

// TestScheduler_UpdateJobInStorage_WithNilLastRunAt verifies boundary condition: nil last_run_at
func TestScheduler_UpdateJobInStorage_WithNilLastRunAt(t *testing.T) {
	// No t.Parallel(): shares global CAS queue with other storage tests
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create a test job in storage
	jobID := "SCH-TEST-UPDATE-004"
	now := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:                 jobID,
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeyTitle:              "Test Job",
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyJobType:            JobTypeCachePrewarm,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "*/15 * * * *",
		objects.FieldKeyCategory:           "test",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyMaxRuntimeSeconds:  300,
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyCreatedAt:          now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyCreatedBy:          "ACC-TEST",
		objects.FieldKeyUpdatedAt:          now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyUpdatedBy:          "ACC-TEST",
		objects.FieldKeyOriginProject:      "zqk",
		objects.FieldKeyOriginSystem:       "zqk",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}

	err := sched.storage.Create(ctx, secCtx, jobData)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	// Load and schedule the job
	err = sched.loadAndScheduleJobs(ctx, false)
	if err != nil {
		t.Fatalf("Failed to load jobs: %v", err)
	}

	// Get the scheduled job
	sched.jobsMu.RLock()
	job, ok := sched.jobs[jobID]
	sched.jobsMu.RUnlock()

	if !ok {
		t.Fatalf("Job %s not found after loading", jobID)
	}

	// Set LastRunAt to nil (boundary condition)
	job.LastRunAt = nil

	// This should not panic - should handle nil gracefully
	// The method should still attempt to update (will fail on format, but shouldn't panic)
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("updateJobInStorage panicked with nil LastRunAt: %v", r)
			}
		}()
		sched.updateJobInStorage(ctx, job)
	}()
}

// TestScheduler_DisableJobInStorage_DisablesOneTimeJob verifies that disableJobInStorage persists enabled=false to storage.
// When running under the full CLI, the same code path passes WithCacheInvalidate(ctx, job.ID), so the cache handler
// removes the job ID from the object-id-cache and invalidates list cache (see job_execution.go disableJobInStorage).
func TestScheduler_DisableJobInStorage_DisablesOneTimeJob(t *testing.T) {
	// No t.Parallel(): shares global CAS queue with other storage tests
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create a one-time job in storage
	jobID := "SCH-TEST-DISABLE-001"
	now := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              "scheduler_job",
		objects.FieldKeyTitle:             "Test Job",
		objects.FieldKeyStatus:            "active",
		objects.FieldKeyJobType:           JobTypeCachePrewarm,
		objects.FieldKeyTriggerType:       "immediate",
		objects.FieldKeyCategory:          "test",
		objects.FieldKeyExecutionMode:     "one_time",
		objects.FieldKeyMaxRuntimeSeconds: 300,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCreatedAt:         now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     "zqk",
		objects.FieldKeyOriginSystem:      "zqk",
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}

	err := sched.storage.Create(ctx, secCtx, jobData)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	// Create a job object with LastRunAt set
	lastRunTime := time.Now().UTC()
	job := &ScheduledJob{
		ID:        jobID,
		JobType:   JobTypeCachePrewarm,
		LastRunAt: &lastRunTime,
		Enabled:   false, // Already disabled in memory
	}

	// Call disableJobInStorage
	sched.disableJobInStorage(ctx, job)

	// Verify the job was updated in storage
	updatedJob, err := sched.storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("Failed to read updated job: %v", err)
	}

	// Check enabled was persisted as false
	enabled, ok := updatedJob[objects.FieldKeyEnabled].(bool)
	if !ok {
		t.Fatal("enabled not found in updated job or not a boolean")
	}

	if enabled {
		t.Error("Expected enabled to be false after disableJobInStorage, got true")
	}

	// Check last_run_at was also persisted if it was set
	if lastRunAtStr, ok := updatedJob[objects.FieldKeyLastRunAt].(string); ok {
		lastRunAt, err := time.Parse(time.RFC3339, lastRunAtStr)
		if err != nil {
			t.Fatalf("Failed to parse last_run_at: %v", err)
		}
		if lastRunAt.Sub(lastRunTime).Abs() > time.Second {
			t.Errorf("Expected last_run_at %v, got %v", lastRunTime, lastRunAt)
		}
	}
}

// TestScheduler_DisableJobInStorage_HandlesStorageError verifies that disableJobInStorage handles storage errors gracefully
func TestScheduler_DisableJobInStorage_HandlesStorageError(t *testing.T) {
	// No t.Parallel(): shares global CAS queue with other storage tests
	sched, _, cleanup := setupTestScheduler(t)
	defer cleanup()

	ctx := pkgctx.NewSystemContext()

	// Create a job that doesn't exist in storage (will cause Update to fail)
	lastRunTime := time.Now().UTC()
	job := &ScheduledJob{
		ID:        "SCH-NONEXISTENT-DISABLE",
		JobType:   JobTypeCachePrewarm,
		LastRunAt: &lastRunTime,
		Enabled:   false,
	}

	// This should not panic, just log a warning
	sched.disableJobInStorage(ctx, job)

	// Test passes if no panic occurred
}

// TestScheduler_DisableJobInStorage_WithNilLastRunAt verifies boundary condition: nil last_run_at
func TestScheduler_DisableJobInStorage_WithNilLastRunAt(t *testing.T) {
	// No t.Parallel(): shares global CAS queue with other storage tests
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create a one-time job in storage
	jobID := "SCH-TEST-DISABLE-002"
	now := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              "scheduler_job",
		objects.FieldKeyTitle:             "Test Job",
		objects.FieldKeyStatus:            "active",
		objects.FieldKeyJobType:           JobTypeCachePrewarm,
		objects.FieldKeyTriggerType:       "immediate",
		objects.FieldKeyCategory:          "test",
		objects.FieldKeyExecutionMode:     "one_time",
		objects.FieldKeyMaxRuntimeSeconds: 300,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCreatedAt:         now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     "zqk",
		objects.FieldKeyOriginSystem:      "zqk",
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}

	err := sched.storage.Create(ctx, secCtx, jobData)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	// Create a job object with nil LastRunAt
	job := &ScheduledJob{
		ID:        jobID,
		JobType:   JobTypeCachePrewarm,
		LastRunAt: nil, // Boundary condition
		Enabled:   false,
	}

	// This should not panic - should handle nil gracefully
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("disableJobInStorage panicked with nil LastRunAt: %v", r)
			}
		}()
		sched.disableJobInStorage(ctx, job)
	}()

	// Verify the job was still updated (enabled should be false)
	updatedJob, err := sched.storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("Failed to read updated job: %v", err)
	}

	enabled, ok := updatedJob[objects.FieldKeyEnabled].(bool)
	if !ok {
		t.Fatal("enabled not found in updated job or not a boolean")
	}

	if enabled {
		t.Error("Expected enabled to be false after disableJobInStorage, got true")
	}
}

// TestScheduler_UpdateJobInStorage_WithInvalidCronEntry verifies boundary condition: invalid cron entry
func TestScheduler_UpdateJobInStorage_WithInvalidCronEntry(t *testing.T) {
	// No t.Parallel(): shares global CAS queue with other storage tests
	sched, _, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create a test job in storage
	jobID := "SCH-TEST-UPDATE-005"
	now := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:                 jobID,
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeyTitle:              "Test Job",
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyJobType:            JobTypeCachePrewarm,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "*/15 * * * *",
		objects.FieldKeyCategory:           "test",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyMaxRuntimeSeconds:  300,
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyCreatedAt:          now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyCreatedBy:          "ACC-TEST",
		objects.FieldKeyUpdatedAt:          now.Format("2006-01-02T15:04:05Z"),
		objects.FieldKeyUpdatedBy:          "ACC-TEST",
		objects.FieldKeyOriginProject:      "zqk",
		objects.FieldKeyOriginSystem:       "zqk",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}

	err := sched.storage.Create(ctx, secCtx, jobData)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	// Load and schedule the job
	err = sched.loadAndScheduleJobs(ctx, false)
	if err != nil {
		t.Fatalf("Failed to load jobs: %v", err)
	}

	// Get the scheduled job
	sched.jobsMu.RLock()
	job, ok := sched.jobs[jobID]
	sched.jobsMu.RUnlock()

	if !ok {
		t.Fatalf("Job %s not found after loading", jobID)
	}

	// Set an invalid cron entry ID (boundary condition)
	job.CronEntryID = cron.EntryID(99999) // Invalid entry ID

	lastRunTime := time.Now().UTC()
	job.LastRunAt = &lastRunTime

	// This should not panic - should handle invalid entry gracefully
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("updateJobInStorage panicked with invalid cron entry: %v", r)
			}
		}()
		sched.updateJobInStorage(ctx, job)
	}()

	// Verify the job was still updated (last_run_at should be set)
	updatedJob, err := sched.storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("Failed to read updated job: %v", err)
	}

	// Check last_run_at was persisted even with invalid cron entry
	lastRunAtStr, ok := updatedJob[objects.FieldKeyLastRunAt].(string)
	if !ok {
		t.Fatal("last_run_at not found in updated job or not a string")
	}

	lastRunAt, err := time.Parse(time.RFC3339, lastRunAtStr)
	if err != nil {
		t.Fatalf("Failed to parse last_run_at: %v", err)
	}

	if lastRunAt.Sub(lastRunTime).Abs() > time.Second {
		t.Errorf("Expected last_run_at %v, got %v", lastRunTime, lastRunAt)
	}
}

// updateContextCaptor is a minimal ObjectStorageProvider that captures the cache context
// passed to Update. Used to verify disableJobInStorage passes WithCacheInvalidate
// without touching file I/O (fast, deterministic).
type updateContextCaptor struct {
	mu       sync.Mutex
	captured *pkgctx.CacheContext
}

func (u *updateContextCaptor) Update(ctx context.Context, _ *pkgctx.SecurityContext, _ string, _ map[string]any) error {
	u.mu.Lock()
	if cc := pkgctx.GetCacheContext(ctx); cc != nil {
		u.captured = &pkgctx.CacheContext{
			Operation: cc.Operation,
			OldID:     cc.OldID,
			NewID:     cc.NewID,
			Kind:      cc.Kind,
			FilePath:  cc.FilePath,
		}
	}
	u.mu.Unlock()
	return nil
}

func (u *updateContextCaptor) Create(context.Context, *pkgctx.SecurityContext, map[string]any) error {
	return nil
}

func (u *updateContextCaptor) GenerateID(ctx context.Context, kind string) (string, error) {
	return "MOCK-123", nil
}
func (u *updateContextCaptor) Read(context.Context, *pkgctx.SecurityContext, string) (map[string]any, error) {
	return nil, storagepkg.ErrObjectNotFound
}
func (u *updateContextCaptor) Delete(context.Context, *pkgctx.SecurityContext, string, bool) error {
	return nil
}

//nolint:gocritic // Interface requires value semantics for ListFilter
func (u *updateContextCaptor) List(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, storagepkg.ListFilter) (*storagepkg.QueryResult, error) {
	return &storagepkg.QueryResult{Objects: []map[string]any{}, Groups: map[string][]map[string]any{}, Meta: map[string]any{}}, nil
}
func (u *updateContextCaptor) Query(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, storagepkg.Query) (*storagepkg.QueryResult, error) {
	return nil, nil
}
func (u *updateContextCaptor) Search(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, storagepkg.SearchQuery) (*storagepkg.SearchResult, error) {
	return nil, nil
}
func (u *updateContextCaptor) BeginTransaction(context.Context) (storagepkg.ObjectTransaction, error) {
	return nil, nil
}
func (u *updateContextCaptor) BulkCreate(context.Context, *pkgctx.SecurityContext, []map[string]any) (*storagepkg.BulkResult, error) {
	return nil, nil
}
func (u *updateContextCaptor) BulkUpdate(context.Context, *pkgctx.SecurityContext, []storagepkg.BulkUpdateItem) (*storagepkg.BulkResult, error) {
	return nil, nil
}
func (u *updateContextCaptor) BulkGet(context.Context, *pkgctx.SecurityContext, []string) (*storagepkg.BulkResult, error) {
	return nil, nil
}
func (u *updateContextCaptor) BulkDelete(context.Context, *pkgctx.SecurityContext, []string, bool) (*storagepkg.BulkResult, error) {
	return nil, nil
}
func (u *updateContextCaptor) Exists(context.Context, *pkgctx.SecurityContext, string) (bool, error) {
	return false, nil
}

//nolint:gocritic // Interface requires value semantics for ListFilter
func (u *updateContextCaptor) Count(context.Context, *pkgctx.SecurityContext, storagepkg.ListFilter) (int, error) {
	return 0, nil
}

//nolint:gocritic // Interface requires value semantics for ListFilter
func (u *updateContextCaptor) Aggregate(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, storagepkg.ListFilter, []storagepkg.Aggregation) (*storagepkg.AggregateResult, error) {
	return nil, nil
}
func (u *updateContextCaptor) GetRelated(context.Context, *pkgctx.SecurityContext, string, string, int) ([]map[string]any, error) {
	return nil, nil
}
func (u *updateContextCaptor) GetPath(context.Context, *pkgctx.SecurityContext, string, string) ([]map[string]any, error) {
	return nil, nil
}
func (u *updateContextCaptor) GetNeighbors(context.Context, *pkgctx.SecurityContext, string, string) ([]map[string]any, error) {
	return nil, nil
}
func (u *updateContextCaptor) Move(context.Context, *pkgctx.SecurityContext, string, string, bool) error {
	return nil
}
func (u *updateContextCaptor) Rename(context.Context, *pkgctx.SecurityContext, string, string, bool) error {
	return nil
}

// TestScheduler_DisableJobInStorage_PassesCacheInvalidateContext verifies that disableJobInStorage
// passes WithCacheInvalidate(ctx, job.ID) so the cache handler removes the ID from object-id-cache
// and invalidates list cache. Uses a mock storage so the test is fast and deterministic (conditions
// that are possible with the real scheduler but hard to assert in integration tests).
func TestScheduler_DisableJobInStorage_PassesCacheInvalidateContext(t *testing.T) {
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)
	captor := &updateContextCaptor{}
	sched := NewSchedulerWithProjectRoot(captor, nil, nil, "", nil)
	if sched == nil {
		t.Fatal("NewSchedulerWithProjectRoot returned nil")
	}

	jobID := "SCH-MOCK-INV-001"
	lastRunTime := time.Now().UTC()
	job := &ScheduledJob{
		ID:        jobID,
		JobType:   JobTypeCachePrewarm,
		LastRunAt: &lastRunTime,
		Enabled:   false,
	}

	ctx := pkgctx.NewSystemContext()
	sched.(*Scheduler).disableJobInStorage(ctx, job)

	captor.mu.Lock()
	cc := captor.captured
	captor.mu.Unlock()

	if cc == nil {
		t.Fatal("Update was not called with a cache context; disableJobInStorage must pass WithCacheInvalidate(ctx, job.ID) so the handler removes the ID from cache")
	}
	if cc.Operation != pkgctx.CacheOperationInvalidate {
		t.Errorf("Cache context Operation = %q, want %q (disable must request cache invalidation)", cc.Operation, pkgctx.CacheOperationInvalidate)
	}
	if cc.OldID != jobID {
		t.Errorf("Cache context OldID = %q, want %q (must invalidate the disabled job ID)", cc.OldID, jobID)
	}
}

func (u *updateContextCaptor) Shutdown(context.Context) error { return nil }
