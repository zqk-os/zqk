package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func TestSchedulerJobRetentionHandler_Execute(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // test cleanup
		env.Cleanup()
	}()

	storage := env.Storage.(storagepkg.ObjectStorageProvider)
	projectRoot := env.TestRoot
	// Path cache required for Create (audit events use stream storage and need path alias resolution).
	storagepkg.BuildPathAliasCacheForProject(projectRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	handler := NewSchedulerJobRetentionHandler(storage, projectRoot)

	// Create test job (the retention job itself - should not be deleted)
	retentionJobID := "SCH-RETENTION-TEST"
	retentionJob := &ScheduledJob{
		ID:      retentionJobID,
		JobType: JobTypeSchedulerJobRetention,
		EnvironmentVariables: map[string]string{
			EnvKeyRetentionDays: "7",
			EnvKeyBatchSize:     "10",
			EnvKeyMaxBatches:    "5",
		},
	}
	// Persist retention job to storage so test can verify it wasn't deleted
	retentionJobData := map[string]any{
		objects.FieldKeyID:            retentionJobID,
		objects.FieldKeyKind:          "scheduler_job",
		objects.FieldKeyTitle:         "Retention Test Job",
		objects.FieldKeyJobType:       JobTypeSchedulerJobRetention,
		objects.FieldKeyTriggerType:   "timer",
		objects.FieldKeyExecutionMode: "reusable", // Retention jobs are reusable
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedBy:     "ACC-TEST",
	}
	if err := storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, retentionJobData); err != nil {
		t.Fatalf("Failed to create retention job %s: %v", retentionJobID, err)
	}

	// Create old one_time jobs that SHOULD be deleted (older than 7 days, enabled=false)
	oldTime := time.Now().UTC().Add(-10 * 24 * time.Hour) // 10 days ago
	oldJobIDs := []string{"SCH-OLD-1", "SCH-OLD-2", "SCH-OLD-3"}
	for _, id := range oldJobIDs {
		jobData := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "scheduler_job",
			objects.FieldKeyTitle:         "Old Test Job " + id,
			objects.FieldKeyJobType:       JobTypeRunWrapper,
			objects.FieldKeyTriggerType:   "manual",
			objects.FieldKeyExecutionMode: "one_time",
			objects.FieldKeyEnabled:       false,
			objects.FieldKeyStatus:        StatusDisabled,
			objects.FieldKeyLastRunAt:     oldTime.Format(time.RFC3339),
			objects.FieldKeyCreatedAt:     oldTime.Format(time.RFC3339),
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedBy:     "ACC-TEST",
		}
		if err := storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
			t.Fatalf("Failed to create old job %s: %v", id, err)
		}
		// Create log directory for this job
		logDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, "scheduler", id)
		if err := fileutil.MkdirAll(logDir, paths.DirPerm755); err != nil {
			t.Fatalf("Failed to create log dir for %s: %v", id, err)
		}
	}

	// Create recent one_time jobs that should NOT be deleted (less than 7 days old)
	recentTime := time.Now().UTC().Add(-2 * 24 * time.Hour) // 2 days ago
	recentJobIDs := []string{"SCH-RECENT-1", "SCH-RECENT-2"}
	for _, id := range recentJobIDs {
		jobData := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "scheduler_job",
			objects.FieldKeyTitle:         "Recent Test Job " + id,
			objects.FieldKeyJobType:       JobTypeRunWrapper,
			objects.FieldKeyTriggerType:   "manual",
			objects.FieldKeyExecutionMode: "one_time",
			objects.FieldKeyEnabled:       false,
			objects.FieldKeyStatus:        StatusDisabled,
			objects.FieldKeyLastRunAt:     recentTime.Format(time.RFC3339),
			objects.FieldKeyCreatedAt:     recentTime.Format(time.RFC3339),
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedBy:     "ACC-TEST",
		}
		if err := storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
			t.Fatalf("Failed to create recent job %s: %v", id, err)
		}
	}

	// Create reusable jobs that should NOT be deleted (reusable jobs are never deleted)
	reusableJobIDs := []string{"SCH-REUSABLE-1", "SCH-REUSABLE-2"}
	for _, id := range reusableJobIDs {
		jobData := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "scheduler_job",
			objects.FieldKeyTitle:         "Reusable Test Job " + id,
			objects.FieldKeyJobType:       JobTypeRunWrapper,
			objects.FieldKeyTriggerType:   "timer",
			objects.FieldKeyExecutionMode: "reusable",
			objects.FieldKeyEnabled:       true,
			objects.FieldKeyStatus:        objects.ObjectStatusActive,
			objects.FieldKeyCreatedAt:     oldTime.Format(time.RFC3339), // Old but reusable
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedBy:     "ACC-TEST",
		}
		if err := storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
			t.Fatalf("Failed to create reusable job %s: %v", id, err)
		}
	}

	// Create enabled=true one_time jobs that should NOT be deleted (still enabled)
	enabledOldJobIDs := []string{"SCH-ENABLED-OLD-1"}
	for _, id := range enabledOldJobIDs {
		jobData := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "scheduler_job",
			objects.FieldKeyTitle:         "Enabled Old Test Job " + id,
			objects.FieldKeyJobType:       JobTypeRunWrapper,
			objects.FieldKeyTriggerType:   "manual",
			objects.FieldKeyExecutionMode: "one_time",
			objects.FieldKeyEnabled:       true, // Still enabled
			objects.FieldKeyStatus:        objects.ObjectStatusActive,
			objects.FieldKeyCreatedAt:     oldTime.Format(time.RFC3339),
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedBy:     "ACC-TEST",
		}
		if err := storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
			t.Fatalf("Failed to create enabled old job %s: %v", id, err)
		}
	}

	// Archived one_time jobs often stay enabled=true; those were invisible to the
	// enabled=false / status=disabled list passes.
	archivedEnabledIDs := []string{"SCH-ARCHIVED-ENABLED-1"}
	for _, id := range archivedEnabledIDs {
		jobData := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "scheduler_job",
			objects.FieldKeyTitle:         "Archived enabled " + id,
			objects.FieldKeyJobType:       JobTypeRunWrapper,
			objects.FieldKeyTriggerType:   "immediate",
			objects.FieldKeyExecutionMode: ExecutionModeOneTime,
			objects.FieldKeyEnabled:       true,
			objects.FieldKeyStatus:        objects.ObjectStatusArchived,
			objects.FieldKeyLastRunAt:     oldTime.Format(time.RFC3339),
			objects.FieldKeyCreatedAt:     oldTime.Format(time.RFC3339),
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedBy:     "ACC-TEST",
		}
		if err := storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
			t.Fatalf("Failed to create archived job %s: %v", id, err)
		}
	}

	// Execute handler
	err := handler.Execute(ctx, retentionJob)
	if err != nil {
		t.Fatalf("Handler execution failed: %v", err)
	}

	// Verify old jobs were deleted
	for _, id := range oldJobIDs {
		_, err := storage.Read(ctx, secCtx, id)
		if err == nil {
			t.Errorf("Old job %s should have been deleted but still exists", id)
		}
		// Verify log directory was removed
		logDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, "scheduler", id)
		if _, err := fileutil.Stat(logDir); err == nil {
			t.Errorf("Log directory for deleted job %s should have been removed but still exists", id)
		}
	}

	for _, id := range archivedEnabledIDs {
		if _, err := storage.Read(ctx, secCtx, id); err == nil {
			t.Errorf("Archived enabled one_time job %s should have been deleted but still exists", id)
		}
	}

	// Verify recent jobs were NOT deleted
	for _, id := range recentJobIDs {
		_, err := storage.Read(ctx, secCtx, id)
		if err != nil {
			t.Errorf("Recent job %s should NOT have been deleted but was: %v", id, err)
		}
	}

	// Verify reusable jobs were NOT deleted
	for _, id := range reusableJobIDs {
		_, err := storage.Read(ctx, secCtx, id)
		if err != nil {
			t.Errorf("Reusable job %s should NOT have been deleted but was: %v", id, err)
		}
	}

	// Verify enabled old jobs were NOT deleted
	for _, id := range enabledOldJobIDs {
		_, err := storage.Read(ctx, secCtx, id)
		if err != nil {
			t.Errorf("Enabled old job %s should NOT have been deleted but was: %v", id, err)
		}
	}

	// Verify retention job itself was NOT deleted
	_, err = storage.Read(ctx, secCtx, retentionJobID)
	if err != nil {
		t.Errorf("Retention job %s should NOT have been deleted but was: %v", retentionJobID, err)
	}
}

func TestSchedulerJobRetentionHandler_Execute_NoOldJobs(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // test cleanup
		env.Cleanup()
	}()

	storage := env.Storage.(storagepkg.ObjectStorageProvider)
	handler := NewSchedulerJobRetentionHandler(storage, env.TestRoot)

	job := &ScheduledJob{
		ID:      "SCH-RETENTION-TEST-2",
		JobType: JobTypeSchedulerJobRetention,
		EnvironmentVariables: map[string]string{
			EnvKeyRetentionDays: "7",
		},
	}

	// Execute with no old jobs - should complete without error
	err := handler.Execute(context.Background(), job)
	if err != nil {
		t.Errorf("Handler should complete successfully with no old jobs: %v", err)
	}
}

func TestSchedulerJobRetentionHandler_Execute_ZeroRetentionDays(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // test cleanup
		env.Cleanup()
	}()

	storage := env.Storage.(storagepkg.ObjectStorageProvider)
	projectRoot := env.TestRoot
	storagepkg.BuildPathAliasCacheForProject(projectRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	handler := NewSchedulerJobRetentionHandler(storage, projectRoot)

	// Create job with RETENTION_DAYS=0 (delete all matching regardless of age)
	job := &ScheduledJob{
		ID:      "SCH-RETENTION-TEST-3",
		JobType: JobTypeSchedulerJobRetention,
		EnvironmentVariables: map[string]string{
			EnvKeyRetentionDays: "0",
			EnvKeyBatchSize:     "10",
			EnvKeyMaxBatches:    "5",
		},
	}

	// Create recent disabled one_time jobs (should be deleted with RETENTION_DAYS=0)
	recentTime := time.Now().UTC().Add(-1 * 24 * time.Hour) // 1 day ago
	recentJobIDs := []string{"SCH-RECENT-DELETE-1", "SCH-RECENT-DELETE-2"}
	for _, id := range recentJobIDs {
		jobData := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "scheduler_job",
			objects.FieldKeyTitle:         "Recent Delete Test Job " + id,
			objects.FieldKeyJobType:       JobTypeRunWrapper,
			objects.FieldKeyTriggerType:   "manual",
			objects.FieldKeyExecutionMode: "one_time",
			objects.FieldKeyEnabled:       false,
			objects.FieldKeyStatus:        StatusDisabled,
			objects.FieldKeyLastRunAt:     recentTime.Format(time.RFC3339),
			objects.FieldKeyCreatedAt:     recentTime.Format(time.RFC3339),
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedBy:     "ACC-TEST",
		}
		if err := storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
			t.Fatalf("Failed to create recent job %s: %v", id, err)
		}
	}

	archivedID := "SCH-RECENT-ARCHIVED-ENABLED"
	archivedData := map[string]any{
		objects.FieldKeyID:            archivedID,
		objects.FieldKeyKind:          "scheduler_job",
		objects.FieldKeyTitle:         "Recent archived enabled",
		objects.FieldKeyJobType:       JobTypeRunWrapper,
		objects.FieldKeyTriggerType:   "immediate",
		objects.FieldKeyExecutionMode: ExecutionModeOneTime,
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyStatus:        objects.ObjectStatusArchived,
		objects.FieldKeyLastRunAt:     recentTime.Format(time.RFC3339),
		objects.FieldKeyCreatedAt:     recentTime.Format(time.RFC3339),
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedBy:     "ACC-TEST",
	}
	if err := storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, archivedData); err != nil {
		t.Fatalf("Failed to create archived job: %v", err)
	}

	// Execute handler
	err := handler.Execute(ctx, job)
	if err != nil {
		t.Fatalf("Handler execution failed: %v", err)
	}

	// Verify recent jobs were deleted (RETENTION_DAYS=0 means delete all matching)
	for _, id := range recentJobIDs {
		_, err := storage.Read(ctx, secCtx, id)
		if err == nil {
			t.Errorf("Job %s should have been deleted with RETENTION_DAYS=0 but still exists", id)
		}
	}
	if _, err := storage.Read(ctx, secCtx, archivedID); err == nil {
		t.Errorf("Archived enabled job %s should have been deleted with RETENTION_DAYS=0 but still exists", archivedID)
	}
}

func TestGetSchedulerJobRetentionConfig(t *testing.T) {
	t.Parallel()

	// Test defaults
	retentionDays, batchSize, maxBatches := getSchedulerJobRetentionConfig(nil)
	if retentionDays != defaultSchedulerJobRetentionDays {
		t.Errorf("getSchedulerJobRetentionConfig(nil) retentionDays = %d; want %d", retentionDays, defaultSchedulerJobRetentionDays)
	}
	if batchSize != defaultSchedulerJobRetentionBatchSize {
		t.Errorf("getSchedulerJobRetentionConfig(nil) batchSize = %d; want %d", batchSize, defaultSchedulerJobRetentionBatchSize)
	}
	if maxBatches != defaultSchedulerJobRetentionMaxBatches {
		t.Errorf("getSchedulerJobRetentionConfig(nil) maxBatches = %d; want %d", maxBatches, defaultSchedulerJobRetentionMaxBatches)
	}

	// Test with environment variables
	job := &ScheduledJob{
		ID: "TEST-JOB",
		EnvironmentVariables: map[string]string{
			EnvKeyRetentionDays: "14",
			EnvKeyBatchSize:     "50",
			EnvKeyMaxBatches:    "10",
		},
	}
	retentionDays, batchSize, maxBatches = getSchedulerJobRetentionConfig(job)
	if retentionDays != 14 {
		t.Errorf("getSchedulerJobRetentionConfig(env) retentionDays = %d; want 14", retentionDays)
	}
	if batchSize != 50 {
		t.Errorf("getSchedulerJobRetentionConfig(env) batchSize = %d; want 50", batchSize)
	}
	if maxBatches != 10 {
		t.Errorf("getSchedulerJobRetentionConfig(env) maxBatches = %d; want 10", maxBatches)
	}

	// Test invalid values fall back to defaults
	job.EnvironmentVariables = map[string]string{
		EnvKeyRetentionDays: "-5",  // Invalid (negative)
		EnvKeyBatchSize:     "0",   // Invalid (must be > 0)
		EnvKeyMaxBatches:    "abc", // Invalid (not a number)
	}
	retentionDays, batchSize, maxBatches = getSchedulerJobRetentionConfig(job)
	if retentionDays != defaultSchedulerJobRetentionDays {
		t.Errorf("getSchedulerJobRetentionConfig(invalid) retentionDays = %d; want default %d", retentionDays, defaultSchedulerJobRetentionDays)
	}
	if batchSize != defaultSchedulerJobRetentionBatchSize {
		t.Errorf("getSchedulerJobRetentionConfig(invalid) batchSize = %d; want default %d", batchSize, defaultSchedulerJobRetentionBatchSize)
	}
	if maxBatches != defaultSchedulerJobRetentionMaxBatches {
		t.Errorf("getSchedulerJobRetentionConfig(invalid) maxBatches = %d; want default %d", maxBatches, defaultSchedulerJobRetentionMaxBatches)
	}
}
