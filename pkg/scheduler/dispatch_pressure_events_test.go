package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/circuitbreaker"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestShouldReenqueueTriggerAfterDispatchDrop(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		job      *ScheduledJob
		prior    string
		wantReen bool
	}{
		{
			name: "test_bundle_any_reason",
			job: &ScheduledJob{
				ID:                TestBundleJobIDPrefix + "foo",
				JobType:           jobTypeRunWrapper,
				ConcurrentAllowed: true,
			},
			prior:    jobExecReasonPackageConcurrencyLimit,
			wantReen: true,
		},
		{
			name: "maintenance_conflict",
			job: &ScheduledJob{
				ID:                "SCH-objcount-report",
				JobType:           jobTypeRunWrapper,
				ConcurrentAllowed: false,
			},
			prior:    jobExecReasonConflict,
			wantReen: true,
		},
		{
			name: "maintenance_dispatch_deadline_no_reen",
			job: &ScheduledJob{
				ID:                "SCH-objcount-report",
				JobType:           jobTypeRunWrapper,
				ConcurrentAllowed: false,
			},
			prior:    jobExecReasonDispatchDeadline,
			wantReen: false,
		},
		{
			name: "conflict_but_concurrent_testing",
			job: &ScheduledJob{
				ID:                TestBundleJobIDPrefix + "x",
				JobType:           jobTypeRunWrapper,
				ConcurrentAllowed: true,
			},
			prior:    jobExecReasonConflict,
			wantReen: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldReenqueueTriggerAfterDispatchDrop(tc.job, tc.prior)
			if got != tc.wantReen {
				t.Fatalf("shouldReenqueueTriggerAfterDispatchDrop(...) = %v, want %v", got, tc.wantReen)
			}
		})
	}
}

func TestAppendDispatchPressureJSONL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rec := map[string]any{
		objects.FieldKeyEventType: "dispatch_pressure",
		"job_id":                  "SCH-test",
		objects.FieldKeySource:    "cron",
	}
	appendDispatchPressureJSONL(dir, rec)

	p := filepath.Join(dir, paths.ProjectDataDir, paths.SchedulerDir, dispatchPressureJSONLFile)
	data, err := fileutil.ReadFile(p)
	if err != nil {
		t.Fatalf("read dispatch_pressure.jsonl: %v", err)
	}
	line := strings.TrimSpace(string(data))
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal line: %v", err)
	}
	if got["job_id"] != "SCH-test" || got[objects.FieldKeySource] != "cron" {
		t.Fatalf("unexpected record: %#v", got)
	}
}

func TestRecordDispatchAttemptDropped_packageConcurrencyAddsPackagePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := logging.GetLoggerFromProfile("test")
	lim := circuitbreaker.NewConcurrencyLimiter(logger, 30*time.Minute)
	lim.MergeLimits(map[string]int{"pkg/scheduler": 2})
	s := &Scheduler{
		projectRoot:               dir,
		packageConcurrencyLimiter: lim,
	}
	job := &ScheduledJob{
		ID:          "SCH-run-pkg-scheduler-34",
		JobType:     jobTypeRunWrapper,
		Category:    CategoryTesting,
		TriggerType: TriggerTypeImmediate,
		Command:     "/bin/sh",
		CommandArgs: []string{"-c", "go test ./pkg/scheduler -timeout 60s -count=1"},
	}
	s.recordDispatchAttemptDropped("execute_job", jobExecReasonPackageConcurrencyLimit, job, errors.New("slot timeout"))

	p := filepath.Join(dir, paths.ProjectDataDir, paths.SchedulerDir, dispatchPressureJSONLFile)
	data, err := fileutil.ReadFile(p)
	if err != nil {
		t.Fatalf("read dispatch_pressure.jsonl: %v", err)
	}
	line := strings.TrimSpace(string(data))
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["package_path"] != "pkg/scheduler" {
		t.Fatalf("package_path: %#v", got["package_path"])
	}
	if limV, ok := got["package_concurrency_limit"].(float64); !ok || int(limV) != 2 {
		t.Fatalf("package_concurrency_limit: %#v", got["package_concurrency_limit"])
	}
	if slots, ok := got["package_slots_in_use"].(float64); !ok || int(slots) != 0 {
		t.Fatalf("package_slots_in_use: %#v", got["package_slots_in_use"])
	}
}

func TestRecordDispatchAttemptDropped_resourceWaitAddsTriggeredPoolSnapshot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 64})
	pool := budget.NewPool("test_dp", "dp test", 2, 5)
	pool.Start(context.Background())
	t.Cleanup(func() { pool.Stop() })

	s := &Scheduler{projectRoot: dir, triggeredPool: pool}
	job := &ScheduledJob{
		ID:                "SCH-run-x",
		JobType:           jobTypeRunWrapper,
		MaxRuntimeSeconds: 600,
	}
	s.recordDispatchAttemptDropped("cron_pool_submit", dispatchPressureReasonResourceWaitDeadlineExceeded, job)

	p := filepath.Join(dir, paths.ProjectDataDir, paths.SchedulerDir, dispatchPressureJSONLFile)
	data, err := fileutil.ReadFile(p)
	if err != nil {
		t.Fatalf("read dispatch_pressure.jsonl: %v", err)
	}
	line := strings.TrimSpace(string(data))
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if int(got["triggered_pool_workers"].(float64)) != 2 {
		t.Fatalf("triggered_pool_workers: %#v", got["triggered_pool_workers"])
	}
	if int(got["triggered_pool_queue_capacity"].(float64)) != 5 {
		t.Fatalf("triggered_pool_queue_capacity: %#v", got["triggered_pool_queue_capacity"])
	}
	if got["triggered_pool_route"].(string) != "default" {
		t.Fatalf("triggered_pool_route: %#v", got["triggered_pool_route"])
	}
	if got["triggered_pool_name"].(string) != "test_dp" {
		t.Fatalf("triggered_pool_name: %#v", got["triggered_pool_name"])
	}
	if got["dispatch_wait_stage"].(string) != dispatchWaitStageTriggeredPoolSubmit {
		t.Fatalf("dispatch_wait_stage: %#v", got["dispatch_wait_stage"])
	}
	if int(got["triggered_pool_queue_depth_pct"].(float64)) != 0 {
		t.Fatalf("triggered_pool_queue_depth_pct: %#v", got["triggered_pool_queue_depth_pct"])
	}
}

func TestRecordDispatchAttemptDropped_goroutineCeilingStage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 64})
	pool := budget.NewPool("ambient", "ambient", 1, 2)
	pool.Start(context.Background())
	t.Cleanup(func() { pool.Stop() })

	s := &Scheduler{projectRoot: dir, triggeredPool: pool}
	job := &ScheduledJob{
		ID:                "SCH-maint-timer",
		JobType:           jobTypeRunWrapper,
		Category:          CategoryMaintenance,
		TriggerType:       TriggerTypeTimer,
		MaxRuntimeSeconds: 600,
	}
	s.recordDispatchAttemptDropped("cron", dispatchPressureReasonResourceWaitDeadlineExceeded, job)

	p := filepath.Join(dir, paths.ProjectDataDir, paths.SchedulerDir, dispatchPressureJSONLFile)
	data, err := fileutil.ReadFile(p)
	if err != nil {
		t.Fatalf("read dispatch_pressure.jsonl: %v", err)
	}
	line := strings.TrimSpace(string(data))
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["dispatch_wait_stage"].(string) != dispatchWaitStageGoroutineCeiling {
		t.Fatalf("dispatch_wait_stage: %#v", got["dispatch_wait_stage"])
	}
}

func TestRecordDispatchAttemptDropped_executeJobOuterContextStage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 64})
	pool := budget.NewPool("ambient", "ambient", 1, 2)
	pool.Start(context.Background())
	t.Cleanup(func() { pool.Stop() })

	s := &Scheduler{projectRoot: dir, triggeredPool: pool}
	job := &ScheduledJob{
		ID:                "SCH-run-x",
		JobType:           jobTypeRunWrapper,
		MaxRuntimeSeconds: 600,
	}
	s.recordDispatchAttemptDropped("execute_job", jobExecReasonDispatchDeadline, job)

	p := filepath.Join(dir, paths.ProjectDataDir, paths.SchedulerDir, dispatchPressureJSONLFile)
	data, err := fileutil.ReadFile(p)
	if err != nil {
		t.Fatalf("read dispatch_pressure.jsonl: %v", err)
	}
	line := strings.TrimSpace(string(data))
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["dispatch_wait_stage"].(string) != dispatchWaitStageExecuteJobOuterCtx {
		t.Fatalf("dispatch_wait_stage: %#v", got["dispatch_wait_stage"])
	}
}

func TestDispatchDropBackoffDuration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		retries int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{10, 8 * time.Second},
	}
	for _, tc := range cases {
		got := dispatchDropBackoffDuration(tc.retries)
		if got != tc.want {
			t.Errorf("dispatchDropBackoffDuration(%d) = %v, want %v", tc.retries, got, tc.want)
		}
	}
}

func TestIsConcurrencyDropReason(t *testing.T) {
	t.Parallel()
	if !isConcurrencyDropReason(jobExecReasonPackageConcurrencyLimit) {
		t.Errorf("expected true for %s", jobExecReasonPackageConcurrencyLimit)
	}
	if !isConcurrencyDropReason(jobExecReasonGlobalTestConcurrencyLimit) {
		t.Errorf("expected true for %s", jobExecReasonGlobalTestConcurrencyLimit)
	}
	if isConcurrencyDropReason(jobExecReasonConflict) {
		t.Errorf("expected false for %s", jobExecReasonConflict)
	}
	if isConcurrencyDropReason(jobExecReasonDispatchDeadline) {
		t.Errorf("expected false for %s", jobExecReasonDispatchDeadline)
	}
}

func TestReenqueueTriggerAfterDispatchDrop_ConcurrencyRetryLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := &Scheduler{
		projectRoot:                dir,
		dispatchDropRetriesByJobID: make(map[string]int),
	}
	// Zero-delay hook for synchronous unit test
	s.testHookDispatchDropBackoffDuration = func(retries int) time.Duration {
		return 0
	}

	job := &ScheduledJob{
		ID:                TestBundleJobIDPrefix + "pkg-storage-1",
		JobType:           jobTypeRunWrapper,
		ConcurrentAllowed: true,
	}

	queue := NewJobTriggerQueue(dir)

	// Attempts 1 to 3 should succeed and re-enqueue
	for attempt := 1; attempt <= maxDispatchDropReenqueueRetries; attempt++ {
		s.reenqueueTriggerAfterDispatchDrop(job, jobExecReasonPackageConcurrencyLimit)
		if count := s.getDispatchDropRetryCount(job.ID); count != attempt {
			t.Fatalf("attempt %d: got retry count %d, want %d", attempt, count, attempt)
		}
		reqs, err := queue.DequeueTriggerRequests(10)
		if err != nil {
			t.Fatalf("dequeue attempt %d: %v", attempt, err)
		}
		if len(reqs) != 1 || reqs[0].JobID != job.ID {
			t.Fatalf("attempt %d: expected 1 request for %s, got %v", attempt, job.ID, reqs)
		}
	}

	// Attempt 4 should exceed maxDispatchDropReenqueueRetries and NOT re-enqueue
	s.reenqueueTriggerAfterDispatchDrop(job, jobExecReasonPackageConcurrencyLimit)
	if count := s.getDispatchDropRetryCount(job.ID); count != maxDispatchDropReenqueueRetries+1 {
		t.Fatalf("attempt 4: got retry count %d, want %d", count, maxDispatchDropReenqueueRetries+1)
	}
	reqs, err := queue.DequeueTriggerRequests(10)
	if err != nil {
		t.Fatalf("dequeue attempt 4: %v", err)
	}
	if len(reqs) != 0 {
		t.Fatalf("attempt 4 should not re-enqueue, got %d requests", len(reqs))
	}

	// Clearing retry count resets state
	s.clearDispatchDropRetryCount(job.ID)
	if count := s.getDispatchDropRetryCount(job.ID); count != 0 {
		t.Fatalf("after clear: got retry count %d, want 0", count)
	}
}
