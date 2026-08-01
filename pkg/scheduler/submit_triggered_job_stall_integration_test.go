package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// stallIntegrationHandler blocks in Execute until hold is closed — same external effect as a stuck job.
type stallIntegrationHandler struct {
	hold chan struct{}
}

func (h stallIntegrationHandler) Execute(context.Context, *ScheduledJob) error {
	<-h.hold
	return nil
}

// startTestStallTriggeredPools wires a single-worker, unbuffered triggered pool so the second
// submitTriggeredJob blocks on Pool.Submit until the dispatch context expires (cron_pool_submit semantics).
func startTestStallTriggeredPools(t *testing.T, s *Scheduler, runCtx context.Context) {
	t.Helper()
	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 64})
	goroutinelabels.SetDefaultBudget(budget)
	pool := budget.NewPool("scheduler_stall_integration", "submit stall integration", 1, 0)
	s.triggeredPool = pool
	s.triggeredPriorityPool = nil
	pool.Start(runCtx)
	t.Cleanup(func() { pool.Stop() })
}

// TestSubmitTriggeredJob_integrationPoolStallDeadlineExceeded exercises the full scheduler path:
// dispatchContextForScheduledJob → submitTriggeredJob → goroutinelabels.Pool.Submit with real executeJob
// preamble (storage read, conflict, lock), then a blocking handler. Produces the same Pool.Submit
// deadline failure as dispatch_pressure source cron_pool_submit / dispatch_resource_wait_deadline_exceeded
// without requiring queue saturation or high worker counts.
//
// Do not call t.Parallel: shares process-wide dispatch wait override and prepareHandlersIsolatedTempProject.
func TestSubmitTriggeredJob_integrationPoolStallDeadlineExceeded(t *testing.T) {
	if testing.Short() {
		t.Skip("temp project + real executeJob path")
	}
	setDispatchResourceWaitMaxForTest(2 * time.Second)
	t.Cleanup(func() { setDispatchResourceWaitMaxForTest(0) })

	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)

	testRoot, storage := prepareHandlersIsolatedTempProject(t)
	storagepkg.BuildPathAliasCacheForProject(testRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	base := map[string]any{
		objects.FieldKeyKind:          "scheduler_job",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyCategory:      CategoryTesting,
		objects.FieldKeyExecutionMode: "reusable",
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "account:system",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     "account:system",
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	merge := func(m map[string]any) map[string]any {
		out := make(map[string]any, len(base)+len(m))
		for k, v := range base {
			out[k] = v
		}
		for k, v := range m {
			out[k] = v
		}
		return out
	}
	jobSpec := func(id string) map[string]any {
		return merge(map[string]any{
			objects.FieldKeyID:                     id,
			objects.FieldKeyTitle:                  "stall integration",
			objects.FieldKeyJobType:                JobTypeLifecycleCheck,
			objects.FieldKeyTriggerType:            TriggerTypeManual,
			objects.FieldKeyMaxRuntimeSeconds:      600,
			objects.FieldKeyAllowParallelExecution: true,
		})
	}
	for _, id := range []string{"SCH-STALL-INT-1", "SCH-STALL-INT-2"} {
		if err := storage.Create(ctx, secCtx, jobSpec(id)); err != nil {
			t.Fatalf("create job %s: %v", id, err)
		}
	}

	schedIface := NewSchedulerWithProjectRoot(storage, objects.NewSpecLoader(testRoot), objects.NewLifecycleLoader(testRoot), testRoot, nil)
	sched := schedIface.(*Scheduler)
	startTestStallTriggeredPools(t, sched, ctx)

	if err := sched.loadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("loadAndScheduleJobs: %v", err)
	}

	sched.jobsMu.RLock()
	j1 := sched.jobs["SCH-STALL-INT-1"]
	j2 := sched.jobs["SCH-STALL-INT-2"]
	sched.jobsMu.RUnlock()
	if j1 == nil || j2 == nil {
		t.Fatalf("jobs not loaded: j1=%v j2=%v", j1, j2)
	}

	hold := make(chan struct{})
	h := stallIntegrationHandler{hold: hold}

	ctx1, cancel1 := dispatchContextForScheduledJob(j1)
	defer cancel1()
	work1 := triggeredJobWork{job: j1, handler: h, ctx: ctx1, cancel: cancel1}
	if err := sched.submitTriggeredJob(work1); err != nil {
		t.Fatalf("first submitTriggeredJob: %v", err)
	}

	ctx2, cancel2 := dispatchContextForScheduledJob(j2)
	defer cancel2()
	work2 := triggeredJobWork{job: j2, handler: h, ctx: ctx2, cancel: cancel2}
	err := sched.submitTriggeredJob(work2)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second submitTriggeredJob: got %v want DeadlineExceeded", err)
	}

	close(hold)
}
