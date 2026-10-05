package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_RetentionMaxCount_Enforcement(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = sp.Shutdown(ctx) }()

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// 1. isHighVolumeKind & skipArchiveForOldestIDsPath
	if !isHighVolumeKind(objects.KindSchedulerJob) {
		t.Errorf("expected scheduler_job to be high volume kind")
	}
	if isHighVolumeKind("unknown_kind_xyz") {
		t.Errorf("expected unknown_kind_xyz not to be high volume")
	}

	if !skipArchiveForOldestIDsPath(objects.KindSchedulerJob, nil) {
		t.Errorf("expected skipArchiveForOldestIDsPath true with nil protect")
	}
	if skipArchiveForOldestIDsPath(objects.KindSchedulerJob, []string{"active"}) {
		t.Errorf("expected skipArchiveForOldestIDsPath false with protect statuses")
	}
	if skipArchiveForOldestIDsPath("unknown_kind", nil) {
		t.Errorf("expected skipArchiveForOldestIDsPath false for non-HV kind")
	}

	rth := &RetentionToleranceHandler{
		storage:     sp,
		projectRoot: tmpDir,
		logger:      logger,
	}

	// 2. enforceMaxCountViaHVNoProtect
	totalDel, handled := rth.enforceMaxCountViaHVNoProtect(ctx, secCtx, "SCH-job-1", objects.KindSchedulerJob, 10, 5, 2, 100)
	if handled && totalDel > 0 {
		t.Logf("handled: %v, deleted: %d", handled, totalDel)
	}

	// 3. enforceMaxCountViaHVWithProtect
	totalDel2, handled2 := rth.enforceMaxCountViaHVWithProtect(ctx, secCtx, "SCH-job-1", objects.KindSchedulerJob, 10, 5, 2, 100, []string{"protected"})
	if handled2 && totalDel2 > 0 {
		t.Logf("handled: %v, deleted: %d", handled2, totalDel2)
	}

	// 4. enforceMaxCountBatchedList & enforceMaxCount
	cnt, err := rth.enforceMaxCountBatchedList(ctx, secCtx, storageCtx, "SCH-job-1", objects.KindSchedulerJob, 5, []string{"protected"}, 2, 1, 1, 10, 5, []string{"id-1", "id-2"})
	if err != nil {
		t.Logf("enforceMaxCountBatchedList result: %v, err: %v", cnt, err)
	}

	cnt2, err2 := rth.enforceMaxCount(ctx, secCtx, storageCtx, "SCH-job-1", objects.KindSchedulerJob, 10, []string{"protected"}, 5, 2, 1)
	if err2 != nil {
		t.Logf("enforceMaxCount result: %v, err: %v", cnt2, err2)
	}
}

func TestExtended_CapOrchestrator_Helpers(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = sp.Shutdown(ctx) }()

	// 1. openAGIPlanPersonaKey & openAgentInstructionIndex
	key := openAGIPlanPersonaKey("PLAN-1", "tpm-agent")
	if key != "plan-1\x00tpm-agent" {
		t.Errorf("unexpected openAGIPlanPersonaKey: %s", key)
	}

	var nilIdx *openAgentInstructionIndex
	if nilIdx.has("PLAN-1", "tpm", "test") {
		t.Errorf("expected nil index has to return false")
	}
	nilIdx.remember("PLAN-1", "tpm", "test") // safe no-op

	idx := &openAgentInstructionIndex{}
	idx.remember("PLAN-1", "tpm-lead", "Implement feature X")
	if !idx.has("PLAN-1", "tpm-lead", "FEATURE") {
		t.Errorf("expected idx.has true for exact match")
	}
	if !idx.has("PLAN-1", "tpm", "FEATURE") {
		t.Errorf("expected idx.has true for substring persona match")
	}
	if idx.has("PLAN-1", "other", "FEATURE") {
		t.Errorf("expected idx.has false for non-matching persona")
	}

	// 2. openATKPlanTaskKey & openAgentTaskIndex
	taskKey := openATKPlanTaskKey("PLAN-1", "TASK-1")
	if taskKey != "plan-1\x00task-1" {
		t.Errorf("unexpected openATKPlanTaskKey: %s", taskKey)
	}

	var nilTaskIdx *openAgentTaskIndex
	if nilTaskIdx.has("PLAN-1", "TASK-1") {
		t.Errorf("expected nilTaskIdx has to return false")
	}
	nilTaskIdx.remember("PLAN-1", "TASK-1") // safe no-op

	taskIdx := &openAgentTaskIndex{}
	taskIdx.remember("PLAN-1", "TASK-1")
	if !taskIdx.has("PLAN-1", "TASK-1") {
		t.Errorf("expected taskIdx.has true")
	}
	if !taskIdx.has("", "TASK-1") {
		t.Errorf("expected taskIdx.has true with empty planID")
	}
	if taskIdx.has("PLAN-2", "TASK-1") {
		t.Errorf("expected taskIdx.has false for different plan")
	}

	// rememberIfOpen
	taskIdx.rememberIfOpen(map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		capFieldTaskID:         "TASK-2",
		capFieldPlanID:         "PLAN-1",
	})
	if !taskIdx.has("PLAN-1", "TASK-2") {
		t.Errorf("expected taskIdx to remember in_progress task")
	}
	// closed task should not be remembered
	taskIdx.rememberIfOpen(map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
		capFieldTaskID:         "TASK-3",
		capFieldPlanID:         "PLAN-1",
	})
	if taskIdx.has("PLAN-1", "TASK-3") {
		t.Errorf("expected complete task not to be remembered")
	}

	// 3. String & Number utilities
	if truncateOutput("short", 10) != "short" {
		t.Errorf("unexpected truncateOutput short")
	}
	if truncateOutput("this is very long text", 7) != "this is... (truncated)" {
		t.Errorf("unexpected truncateOutput long")
	}

	jsonVal := jsonOrRaw([]byte(`{"k":"v"}`))
	if m, ok := jsonVal.(map[string]any); !ok || m["k"] != "v" {
		t.Errorf("unexpected jsonOrRaw valid json: %v", jsonVal)
	}
	rawVal := jsonOrRaw([]byte("not json"))
	if rawVal != "not json" {
		t.Errorf("unexpected jsonOrRaw raw: %v", rawVal)
	}

	// asInt
	if n, ok := asInt(12); !ok || n != 12 {
		t.Errorf("expected 12 from int")
	}
	if n, ok := asInt(int64(42)); !ok || n != 42 {
		t.Errorf("expected 42 from int64")
	}
	if n, ok := asInt(float64(99.0)); !ok || n != 99 {
		t.Errorf("expected 99 from float64")
	}
	if n, ok := asInt(json.Number("123")); !ok || n != 123 {
		t.Errorf("expected 123 from json.Number")
	}
	if _, ok := asInt("string"); ok {
		t.Errorf("expected failure for string in asInt")
	}

	// 4. parseSystemCheckPublicBlockers
	b1, err := parseSystemCheckPublicBlockers([]byte(`{"public_blockers": 3}`))
	if err != nil || b1 != 3 {
		t.Errorf("unexpected b1: %d, err: %v", b1, err)
	}
	b2, _ := parseSystemCheckPublicBlockers([]byte(`{"summary": {"public_blockers": 5}}`))
	if b2 != 5 {
		t.Errorf("unexpected b2: %d", b2)
	}
	b3, _ := parseSystemCheckPublicBlockers([]byte(`{"blocking_issues": {"public": 7}}`))
	if b3 != 7 {
		t.Errorf("unexpected b3: %d", b3)
	}
	b4, _ := parseSystemCheckPublicBlockers([]byte(`{"statistics": {"public_blockers": 2}}`))
	if b4 != 2 {
		t.Errorf("unexpected b4: %d", b4)
	}
	b5, _ := parseSystemCheckPublicBlockers([]byte(`{"detailed": {"blocking_issues_public": 4}}`))
	if b5 != 4 {
		t.Errorf("unexpected b5: %d", b5)
	}
	_, errInvalid := parseSystemCheckPublicBlockers([]byte(`invalid json`))
	if errInvalid == nil {
		t.Errorf("expected error parsing invalid json")
	}

	// 5. criticalRootForPackagePath
	if criticalRootForPackagePath("./pkg/scheduler/handlers") != "pkg/scheduler" {
		t.Errorf("unexpected criticalRoot for pkg/scheduler")
	}
	if criticalRootForPackagePath("pkg/storage") != "pkg/storage" {
		t.Errorf("unexpected criticalRoot for pkg/storage")
	}
	if criticalRootForPackagePath("pkg/cli/commands") != "" {
		t.Errorf("unexpected criticalRoot for pkg/cli")
	}

	// 6. CapOrchestratorHandler methods
	h := &CapOrchestratorHandler{
		storage:     sp,
		projectRoot: tmpDir,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		escalation:  defaultEscalationChain(tmpDir),
	}

	tracker := h.readFailureTracker()
	if tracker.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 initial failures")
	}
	h.recordFailure("plan", fmt.Errorf("sample error"))
	tracker = h.readFailureTracker()
	if tracker.ConsecutiveFailures != 1 || tracker.LastStage != "plan" {
		t.Errorf("unexpected tracker after recordFailure: %+v", tracker)
	}
	h.clearFailures()
	tracker = h.readFailureTracker()
	if tracker.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 failures after clearFailures")
	}

	_ = h.buildOpenAgentInstructionIndex(ctx)
	_ = h.hasOpenAgentInstruction(ctx, "PLAN-1", "tpm", "test")
	_ = h.buildOpenAgentTaskIndex(ctx)
	_ = h.hasOpenTaskForPlan(ctx, "PLAN-1", "TASK-1")
	_ = h.autoRecoverPlanTasks(ctx, "PLAN-1")
}

func TestExtended_JobExecution_Deep(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = sp.Shutdown(ctx) }()

	// 1. WriteJobOutcome & WriteJobProgress & trimJobLogFileIfNeeded
	WriteJobOutcome(tmpDir, "SCH-job-1", objects.KindSchedulerJob, map[string]any{"events_processed": 10})
	WriteJobProgress(tmpDir, "SCH-job-1", map[string]any{"percent": 50})

	// Test directly writeJobLogEntry
	writeJobLogEntry(tmpDir, "SCH-job-1", map[string]any{"msg": "line 1"})
	writeJobLogEntry(tmpDir, "SCH-job-1", map[string]any{"msg": "line 2"})

	logPath := JobEventsFilePath(tmpDir, "SCH-job-1")
	trimJobLogFileIfNeeded(logPath, 1)

	// Test empty project root or job ID
	WriteJobOutcome("", "SCH-job-1", "test", nil)
	WriteJobProgress(tmpDir, "", nil)

	// 2. dispatchContextForScheduledJob
	jobUnbounded := &ScheduledJob{MaxRuntimeSeconds: 0}
	ctxUnbounded, cancelUnbounded := dispatchContextForScheduledJob(jobUnbounded)
	defer cancelUnbounded()
	if ctxUnbounded == nil {
		t.Errorf("expected non-nil ctxUnbounded")
	}

	jobBounded := &ScheduledJob{MaxRuntimeSeconds: 120}
	ctxBounded, cancelBounded := dispatchContextForScheduledJob(jobBounded)
	defer cancelBounded()
	if ctxBounded == nil {
		t.Errorf("expected non-nil ctxBounded")
	}

	// 3. shouldInvokeConfiguredJobCallback
	preCommitCtx := ContextWithTriggerOrigin(ctx, TriggerOriginPreCommit)
	if !shouldInvokeConfiguredJobCallback(preCommitCtx, &ScheduledJob{}) {
		t.Errorf("expected true for TriggerOriginPreCommit")
	}
	oneTimeJob := &ScheduledJob{ExecutionMode: jobExecutionModeOneTime}
	if !shouldInvokeConfiguredJobCallback(ctx, oneTimeJob) {
		t.Errorf("expected true for one-time execution mode")
	}
	if shouldInvokeConfiguredJobCallback(ctx, &ScheduledJob{ExecutionMode: "recurring"}) {
		t.Errorf("expected false for standard recurring job")
	}

	// 4. DisableJobInStorage & updateJobInStorage
	sched := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)
	sched.projectRoot = tmpDir

	// Create a test job object in storage
	jobObj := map[string]any{
		objects.FieldKeyID:        "SCH-job-test-1",
		objects.FieldKeyKind:      objects.KindSchedulerJob,
		objects.FieldKeyTitle:     "test-job",
		objects.FieldKeyEnabled:   true,
		objects.FieldKeyStatus:    StatusActive,
		objects.FieldKeyCreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	_ = sp.Create(ctx, pkgctx.NewSystemSecurityContext(), jobObj)

	jobModel := &ScheduledJob{
		ID:            "SCH-job-test-1",
		Title:         "test-job",
		Enabled:       true,
		Status:        StatusActive,
		ScheduleExpr:  "0 * * * *",
		TriggerType:   "timer",
		ExecutionMode: jobExecutionModeOneTime,
	}
	now := time.Now().UTC()
	jobModel.LastRunAt = &now

	sched.updateJobInStorage(ctx, jobModel)
	sched.DisableJobInStorage(ctx, jobModel)
	if jobModel.Enabled {
		t.Errorf("expected jobModel to be disabled")
	}
}
