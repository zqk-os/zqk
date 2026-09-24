package scheduler

import (
	"sort"
	"testing"
)

func TestConflictManager_HasRunningJob(t *testing.T) {
	t.Parallel()
	cm := &ConflictManager{runningJobs: make(map[string]*ScheduledJob)}
	if cm.HasRunningJob("SCH-x") {
		t.Fatal("unexpected busy")
	}
	j := &ScheduledJob{ID: "SCH-x", JobType: JobTypeLifecycleCheck, Running: true}
	cm.RegisterRunning(j)
	if !cm.HasRunningJob("SCH-x") {
		t.Fatal("expected busy while registered")
	}
	cm.UnregisterRunning(j)
	if cm.HasRunningJob("SCH-x") {
		t.Fatal("expected not busy after unregister")
	}
}

func TestConflictManager_CanRun_SameJobIDBlocked(t *testing.T) {
	t.Parallel()
	cm := &ConflictManager{
		runningJobs: make(map[string]*ScheduledJob),
	}

	job := &ScheduledJob{ID: "SCH-002", JobType: JobTypeAuditEventAggregation, ConcurrentAllowed: false}
	if !cm.CanRun(job) {
		t.Error("Expected CanRun true when job not yet running")
	}

	cm.RegisterRunning(job)
	if cm.CanRun(job) {
		t.Error("Expected CanRun false when same job ID already running (prevents duplicate started events)")
	}
	cm.UnregisterRunning(job)

	if !cm.CanRun(job) {
		t.Error("Expected CanRun true again after unregister")
	}
}

func TestConflictManager_CanRun_ConcurrentAllowedDifferentIDs(t *testing.T) {
	t.Parallel()
	cm := &ConflictManager{
		runningJobs: make(map[string]*ScheduledJob),
	}

	// JobTypeCachePrewarm is the only type that allows concurrent execution; use it here.
	// audit_event_aggregation was previously marked concurrent but is now non-concurrent
	// (it uses AuditAggregationSingletonLockID to prevent duplicate metric creation instead).
	j1 := &ScheduledJob{ID: "SCH-001", JobType: JobTypeCachePrewarm, ConcurrentAllowed: true}
	j2 := &ScheduledJob{ID: "SCH-002", JobType: JobTypeCachePrewarm, ConcurrentAllowed: true}
	cm.RegisterRunning(j1)
	if !cm.CanRun(j2) {
		t.Error("Expected CanRun true for different job ID when ConcurrentAllowed")
	}
	cm.UnregisterRunning(j1)
}

// TestConflictManager_AuditAggregationNotConcurrent guards against re-enabling concurrent
// execution for audit_event_aggregation via isConcurrentAllowed. Concurrent runs produce
// duplicate audit_aggregation_metric stream entries (root cause: scheduler SCH-002 +
// MaintenanceRunner both calling Execute() for the same event window). The fix uses
// AuditAggregationSingletonLockID (file lock in Execute()); isConcurrentAllowed must
// return false so the conflict manager also blocks scheduler-tracked duplicate runs.
func TestConflictManager_AuditAggregationNotConcurrent(t *testing.T) {
	t.Parallel()
	cm := &ConflictManager{
		runningJobs: make(map[string]*ScheduledJob),
	}

	j1 := &ScheduledJob{ID: "SCH-001", JobType: JobTypeAuditEventAggregation, ConcurrentAllowed: false, Running: true}
	j2 := &ScheduledJob{ID: "SCH-002", JobType: JobTypeAuditEventAggregation, ConcurrentAllowed: false}
	cm.RegisterRunning(j1)
	if cm.CanRun(j2) {
		t.Error("audit_event_aggregation must NOT allow concurrent execution: two simultaneous runs " +
			"produce duplicate audit_aggregation_metric entries (stream-backed, no deduplication). " +
			"isConcurrentAllowed(\"audit_event_aggregation\") must return false.")
	}
	cm.UnregisterRunning(j1)
}

func TestConflictManager_CanRun_RunWrapperDifferentIDsAllowed(t *testing.T) {
	t.Parallel()
	cm := &ConflictManager{
		runningJobs: make(map[string]*ScheduledJob),
	}

	j1 := &ScheduledJob{ID: "SCH-001", JobType: JobTypeRunWrapper, ConcurrentAllowed: false, Running: true}
	j2 := &ScheduledJob{ID: "SCH-002", JobType: JobTypeRunWrapper, ConcurrentAllowed: false}
	cm.RegisterRunning(j1)
	if !cm.CanRun(j2) {
		t.Error("Expected CanRun true for different run_wrapper job IDs; only the same ID may not overlap")
	}
	cm.UnregisterRunning(j1)
}

// TestConflictManager_TestBundleRunWrappersConcurrent documents leftover SCH-run-* behavior: testing-category
// run_wrapper jobs get ConcurrentAllowed from isConcurrentAllowed so the worker pool does not
// drop SCH-run-* triggers while another bundle is still running (see lifecycle_coordination isConcurrentAllowed).
func TestConflictManager_TestBundleRunWrappersConcurrent(t *testing.T) {
	t.Parallel()
	cm := &ConflictManager{
		runningJobs: make(map[string]*ScheduledJob),
	}

	j1 := &ScheduledJob{ID: "SCH-run-bundle-a", JobType: JobTypeRunWrapper, Category: CategoryTesting, ConcurrentAllowed: true, Running: true}
	j2 := &ScheduledJob{ID: "SCH-run-bundle-b", JobType: JobTypeRunWrapper, Category: CategoryTesting, ConcurrentAllowed: true}
	cm.RegisterRunning(j1)
	if !cm.CanRun(j2) {
		t.Error("Expected CanRun true for second test-bundle run_wrapper when ConcurrentAllowed (different job IDs)")
	}
	cm.UnregisterRunning(j1)
}

// TestConflictManager_MaintenanceRunWrapperNotBlockedByTestingBundles guards against non-concurrent
// maintenance run_wrappers being starved while SCH-run-* bundles hold worker slots: concurrent
// runners must be ignored when evaluating conflicts for a non-concurrent job.
func TestConflictManager_MaintenanceRunWrapperNotBlockedByTestingBundles(t *testing.T) {
	t.Parallel()
	cm := &ConflictManager{
		runningJobs: make(map[string]*ScheduledJob),
	}

	testingBundle := &ScheduledJob{
		ID: "SCH-run-pkg-storage-1", JobType: JobTypeRunWrapper, Category: CategoryTesting,
		ConcurrentAllowed: true, Running: true,
	}
	maintenanceScript := &ScheduledJob{
		ID: "SCH-016", JobType: JobTypeRunWrapper, Category: CategoryMaintenance,
		ConcurrentAllowed: false,
	}
	cm.RegisterRunning(testingBundle)
	if !cm.CanRun(maintenanceScript) {
		t.Error("Expected CanRun true for maintenance run_wrapper while concurrent testing bundles are running")
	}
	cm.UnregisterRunning(testingBundle)

	// Two different maintenance run_wrappers may run concurrently (per-job-ID serialization only).
	maintA := &ScheduledJob{ID: "SCH-016", JobType: JobTypeRunWrapper, Category: CategoryMaintenance, ConcurrentAllowed: false, Running: true}
	maintB := &ScheduledJob{ID: "SCH-018", JobType: JobTypeRunWrapper, Category: CategoryMaintenance, ConcurrentAllowed: false}
	cm.RegisterRunning(maintA)
	if !cm.CanRun(maintB) {
		t.Error("Expected CanRun true for a second maintenance run_wrapper with a different job ID")
	}
	cm.UnregisterRunning(maintA)
}

func TestConflictManager_GetRunningJobIDs(t *testing.T) {
	t.Parallel()
	cm := &ConflictManager{
		runningJobs: make(map[string]*ScheduledJob),
	}

	// Empty
	ids := cm.GetRunningJobIDs()
	if len(ids) != 0 {
		t.Errorf("Expected 0 running IDs, got %d", len(ids))
	}

	// Register two jobs
	j1 := &ScheduledJob{ID: "SCH-001", JobType: JobTypeRunWrapper}
	j2 := &ScheduledJob{ID: "SCH-002", JobType: JobTypeCachePrewarm}
	cm.RegisterRunning(j1)
	cm.RegisterRunning(j2)

	ids = cm.GetRunningJobIDs()
	if len(ids) != 2 {
		t.Errorf("Expected 2 running IDs, got %d", len(ids))
	}
	sort.Strings(ids)
	if ids[0] != "SCH-001" || ids[1] != "SCH-002" {
		t.Errorf("Expected [SCH-001 SCH-002], got %v", ids)
	}

	// Unregister one
	cm.UnregisterRunning(j1)
	ids = cm.GetRunningJobIDs()
	if len(ids) != 1 {
		t.Errorf("Expected 1 running ID after unregister, got %d", len(ids))
	}
	if ids[0] != "SCH-002" {
		t.Errorf("Expected [SCH-002], got %v", ids)
	}

	// Unregister the other
	cm.UnregisterRunning(j2)
	ids = cm.GetRunningJobIDs()
	if len(ids) != 0 {
		t.Errorf("Expected 0 running IDs after all unregistered, got %d", len(ids))
	}
}
