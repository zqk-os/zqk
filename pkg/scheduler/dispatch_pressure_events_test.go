package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/circuitbreaker"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
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
