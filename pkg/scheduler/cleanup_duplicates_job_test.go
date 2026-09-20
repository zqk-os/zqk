package scheduler

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestCleanupDuplicatesJob tests that the cleanup-duplicates scheduler job can be created and has correct structure
func TestCleanupDuplicatesJob(t *testing.T) {
	// No t.Parallel(): shares global CAS queue with other storage tests
	sched, testRoot, cleanup := setupTestScheduler(t)
	defer cleanup()

	// Set up security context with read permission
	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"developer"}, []string{"read:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	// Create a test scheduler job for cleanup-duplicates using the helper function
	jobID := "SCH-TEST-CLEANUP"
	createTestJob(t, sched.storage, jobID, "run_wrapper", "timer", "0 * * * *")

	// Load the job data to verify structure
	ctx := pkgctx.NewSystemContext()
	retrievedJob, err := sched.storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("Failed to read test job: %v", err)
	}

	if retrievedJob[objects.FieldKeyID] != jobID {
		t.Errorf("Job ID mismatch: expected %s, got %v", jobID, retrievedJob[objects.FieldKeyID])
	}

	// Verify job type is run_wrapper
	if retrievedJob[objects.FieldKeyJobType] != "run_wrapper" {
		t.Errorf("Job type mismatch: expected 'run_wrapper', got %v", retrievedJob[objects.FieldKeyJobType])
	}

	// Verify job trigger type
	if retrievedJob[objects.FieldKeyTriggerType] != "timer" {
		t.Errorf("Trigger type mismatch: expected 'timer', got %v", retrievedJob[objects.FieldKeyTriggerType])
	}

	// Verify job schedule expression
	if retrievedJob[objects.FieldKeyScheduleExpression] != "0 * * * *" {
		t.Errorf("Schedule expression mismatch: expected '0 * * * *', got %v", retrievedJob[objects.FieldKeyScheduleExpression])
	}

	// Test that the scheduler jobs directory exists
	jobPath := datacell.CellCASPrimaryDir(testRoot, "scheduler_jobs")
	if _, err := fileutil.Stat(jobPath); err != nil {
		t.Fatalf("Scheduler jobs directory not found: %v", err)
	}
}
