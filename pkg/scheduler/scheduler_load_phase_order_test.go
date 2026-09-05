package scheduler

import (
	"context"
	"errors"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// startTestTriggeredPools mirrors Scheduler.Start pool wiring so loadAndScheduleJobs can submit immediate jobs (tests only).
func startTestTriggeredPools(t *testing.T, s *Scheduler, ctx context.Context) {
	t.Helper()
	const triggeredQueueSize = 512
	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 512})
	goroutinelabels.SetDefaultBudget(budget)
	pool := budget.NewPool("scheduler_triggered_job_worker", "test triggered", getSchedulerTriggeredPoolSize(), triggeredQueueSize)
	s.triggeredPool = pool
	pool.Start(ctx)
	pp := budget.NewPool("scheduler_triggered_priority", "test priority", 8, 96)
	s.triggeredPriorityPool = pp
	pp.Start(ctx)
	t.Cleanup(func() {
		pool.Stop()
		pp.Stop()
	})
}

// TestLoadAndScheduleJobs_timersBeforeImmediateAndHooks proves load scheduling runs timer (and other
// non-immediate) jobs in a first pass, then starts cron and runs startup bootstrap, then submits
// immediate jobs. Without this order, a full triggered-pool queue from hundreds of immediate
// scheduleImmediateJob submits can delay cache_prewarm bootstrap for many minutes.
func TestLoadAndScheduleJobs_timersBeforeImmediateAndHooks(t *testing.T) {
	if testing.Short() {
		t.Skip("uses temp project and load path")
	}
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
		objects.FieldKeyCategory:      "maintenance",
		objects.FieldKeyExecutionMode: "reusable",
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	timer := map[string]any{
		objects.FieldKeyID:                 "SCH-PHASE-TEST-TIMER",
		objects.FieldKeyTitle:              "Phase test timer",
		objects.FieldKeyJobType:            JobTypeAuditEventAggregation,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "0 0 1 1 *",
		objects.FieldKeyMaxRuntimeSeconds:  60,
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
	if err := storage.Create(ctx, secCtx, merge(timer)); err != nil {
		t.Fatalf("create timer job: %v", err)
	}
	schedIface := NewSchedulerWithProjectRoot(storage, objects.NewSpecLoader(testRoot), objects.NewLifecycleLoader(testRoot), testRoot, nil)
	sched := schedIface.(*Scheduler)
	var seq []string
	sched.TestHookLoadSchedulingPhase = func(phase string) {
		seq = append(seq, phase)
	}
	if err := sched.loadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("loadAndScheduleJobs: %v", err)
	}
	if len(seq) != 2 {
		t.Fatalf("expected 2 load phase hooks, got %d: %q", len(seq), seq)
	}
	if seq[0] != "after_timers_before_cron_and_bootstrap" {
		t.Errorf("first hook: got %q", seq[0])
	}
	if seq[1] != "after_bootstrap_before_immediate" {
		t.Errorf("second hook: got %q", seq[1])
	}
	sched.jobsMu.RLock()
	_, hasT := sched.jobs["SCH-PHASE-TEST-TIMER"]
	sched.jobsMu.RUnlock()
	if !hasT {
		t.Fatal("expected timer job scheduled")
	}
}

// TestLoadAndScheduleJobs_immediateScheduledAfterPoolsAndPhaseHooks mirrors daemon wiring: triggered pools exist,
// then load runs timer pass → cron/bootstrap → immediate pass so immediate submit cannot block bootstrap.
func TestLoadAndScheduleJobs_immediateScheduledAfterPoolsAndPhaseHooks(t *testing.T) {
	if testing.Short() {
		t.Skip("uses temp project and pools")
	}
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
		objects.FieldKeyCategory:      "maintenance",
		objects.FieldKeyExecutionMode: "reusable",
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	timer := map[string]any{
		objects.FieldKeyID:                 "SCH-PHASE-T2-TIMER",
		objects.FieldKeyTitle:              "Phase test timer",
		objects.FieldKeyJobType:            JobTypeAuditEventAggregation,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "0 0 1 1 *",
		objects.FieldKeyMaxRuntimeSeconds:  60,
	}
	imm := map[string]any{
		objects.FieldKeyID:                     "SCH-PHASE-T2-IMM",
		objects.FieldKeyTitle:                  "Phase test immediate",
		objects.FieldKeyJobType:                JobTypeRunWrapper,
		objects.FieldKeyTriggerType:            "immediate",
		objects.FieldKeyCommand:                "true",
		objects.FieldKeyCommandArgs:            []string{},
		objects.FieldKeyMaxRuntimeSeconds:      5,
		objects.FieldKeyCategory:               "test",
		objects.FieldKeyExecutionMode:          "reusable",
		objects.FieldKeyAllowParallelExecution: true,
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
	if err := storage.Create(ctx, secCtx, merge(timer)); err != nil {
		t.Fatalf("create timer job: %v", err)
	}
	if err := storage.Create(ctx, secCtx, merge(imm)); err != nil {
		t.Fatalf("create immediate job: %v", err)
	}

	schedIface := NewSchedulerWithProjectRoot(storage, objects.NewSpecLoader(testRoot), objects.NewLifecycleLoader(testRoot), testRoot, nil)
	sched := schedIface.(*Scheduler)
	startTestTriggeredPools(t, sched, ctx)

	var seq []string
	sched.TestHookLoadSchedulingPhase = func(phase string) {
		seq = append(seq, phase)
	}
	if err := sched.loadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("loadAndScheduleJobs: %v", err)
	}
	if len(seq) != 2 {
		t.Fatalf("expected 2 phase hooks, got %v", seq)
	}
	sched.jobsMu.RLock()
	_, hasT := sched.jobs["SCH-PHASE-T2-TIMER"]
	_, hasI := sched.jobs["SCH-PHASE-T2-IMM"]
	sched.jobsMu.RUnlock()
	if !hasT || !hasI {
		t.Fatalf("want timer+immediate in cache, timer=%v immediate=%v", hasT, hasI)
	}
}

// TestLoadAndScheduleJobs_chunkedImmediateSchedulesAllJobs sets immediate-load batching and verifies every
// immediate job is still registered after pass 2 (chunked path integrates with pools).
func TestLoadAndScheduleJobs_chunkedImmediateSchedulesAllJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("uses temp project and pools")
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize(), "2")
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchPause(), "0")

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
		objects.FieldKeyCategory:      "test",
		objects.FieldKeyExecutionMode: "reusable",
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	merge := func(id string) map[string]any {
		out := make(map[string]any, len(base)+12)
		for k, v := range base {
			out[k] = v
		}
		out[objects.FieldKeyID] = id
		out[objects.FieldKeyTitle] = "Chunk test " + id
		out[objects.FieldKeyJobType] = JobTypeRunWrapper
		out[objects.FieldKeyTriggerType] = "immediate"
		out[objects.FieldKeyCommand] = "true"
		out[objects.FieldKeyCommandArgs] = []string{}
		out[objects.FieldKeyMaxRuntimeSeconds] = 5
		out[objects.FieldKeyAllowParallelExecution] = true
		return out
	}
	ids := []string{"SCH-CHUNK-A", "SCH-CHUNK-B", "SCH-CHUNK-C", "SCH-CHUNK-D"}
	for _, id := range ids {
		if err := storage.Create(ctx, secCtx, merge(id)); err != nil {
			t.Fatalf("create job %s: %v", id, err)
		}
	}

	schedIface := NewSchedulerWithProjectRoot(storage, objects.NewSpecLoader(testRoot), objects.NewLifecycleLoader(testRoot), testRoot, nil)
	sched := schedIface.(*Scheduler)
	startTestTriggeredPools(t, sched, ctx)

	if err := sched.loadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("loadAndScheduleJobs: %v", err)
	}
	sched.jobsMu.RLock()
	defer sched.jobsMu.RUnlock()
	for _, id := range ids {
		if _, ok := sched.jobs[id]; !ok {
			t.Fatalf("expected job %s registered after chunked immediate load", id)
		}
	}
}

// TestLoadAndScheduleJobs_chunkedImmediateRespectsCancelDuringPause verifies pass 2 returns when the load
// context is cancelled during the pause between batches (functional acceptance: shutdown wins over backlog).
func TestLoadAndScheduleJobs_chunkedImmediateRespectsCancelDuringPause(t *testing.T) {
	if testing.Short() {
		t.Skip("uses temp project and pools")
	}
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchSize(), "1")
	t.Setenv(zqkenv.SchedulerImmediateLoadBatchPause(), "1h")

	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)
	testRoot, storage := prepareHandlersIsolatedTempProject(t)
	storagepkg.BuildPathAliasCacheForProject(testRoot)
	secCtx := pkgctx.NewSystemSecurityContext()
	wctx := pkgctx.NewSystemContext()

	base := map[string]any{
		objects.FieldKeyKind:          "scheduler_job",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyCategory:      "test",
		objects.FieldKeyExecutionMode: "reusable",
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	merge := func(id string) map[string]any {
		out := make(map[string]any, len(base)+12)
		for k, v := range base {
			out[k] = v
		}
		out[objects.FieldKeyID] = id
		out[objects.FieldKeyTitle] = "Cancel test " + id
		out[objects.FieldKeyJobType] = JobTypeRunWrapper
		out[objects.FieldKeyTriggerType] = "immediate"
		out[objects.FieldKeyCommand] = "true"
		out[objects.FieldKeyCommandArgs] = []string{}
		out[objects.FieldKeyMaxRuntimeSeconds] = 5
		out[objects.FieldKeyAllowParallelExecution] = true
		return out
	}
	if err := storage.Create(wctx, secCtx, merge("SCH-CANCEL-A")); err != nil {
		t.Fatalf("create job A: %v", err)
	}
	if err := storage.Create(wctx, secCtx, merge("SCH-CANCEL-B")); err != nil {
		t.Fatalf("create job B: %v", err)
	}

	schedIface := NewSchedulerWithProjectRoot(storage, objects.NewSpecLoader(testRoot), objects.NewLifecycleLoader(testRoot), testRoot, nil)
	sched := schedIface.(*Scheduler)
	startTestTriggeredPools(t, sched, ctx)

	var sawChunk0 bool
	sched.TestHookAfterImmediateLoadChunk = func(chunkIndex int) {
		if chunkIndex == 0 {
			sawChunk0 = true
			cancel()
		}
	}

	err := sched.loadAndScheduleJobs(ctx, false)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got err=%v", err)
	}
	if !sawChunk0 {
		t.Fatal("expected chunk 0 hook before cancel")
	}

	sched.jobsMu.RLock()
	defer sched.jobsMu.RUnlock()
	if _, ok := sched.jobs["SCH-CANCEL-A"]; !ok {
		t.Fatal("first immediate should be scheduled before pause select")
	}
	if _, ok := sched.jobs["SCH-CANCEL-B"]; ok {
		t.Fatal("second immediate must not schedule after cancellation")
	}
}
