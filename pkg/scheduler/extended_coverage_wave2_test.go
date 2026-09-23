package scheduler

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/agentclaim"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_LifecycleCoordinationFilters(t *testing.T) {
	s := &Scheduler{
		storage: storage.NewNoopObjectStorage(),
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	// matchesEventFilter
	if !s.matchesEventFilter("*", "job.completed", "system") {
		t.Fatal("expected match for *")
	}
	if !s.matchesEventFilter("", "job.completed", "system") {
		t.Fatal("expected match for empty")
	}
	if !s.matchesEventFilter("job.completed", "job.completed", "system") {
		t.Fatal("expected match for type")
	}
	if s.matchesEventFilter("job.failed", "job.completed", "system") {
		t.Fatal("expected no match")
	}
	if !s.matchesEventFilter("job.completed:system", "job.completed", "system") {
		t.Fatal("expected match for type:kind")
	}
	if s.matchesEventFilter("job.completed:other", "job.completed", "system") {
		t.Fatal("expected no match for different kind")
	}
	if s.matchesEventFilter("a:b:c", "a", "b") {
		t.Fatal("expected no match for 3 parts")
	}

	// matchesLifecycleFilter
	if !s.matchesLifecycleFilter("*", "backlog_item", "draft", "active") {
		t.Fatal("expected match for *")
	}
	if !s.matchesLifecycleFilter("", "backlog_item", "draft", "active") {
		t.Fatal("expected match for empty")
	}
	if !s.matchesLifecycleFilter("backlog_item:draft->active", "backlog_item", "draft", "active") {
		t.Fatal("expected exact match")
	}
	if s.matchesLifecycleFilter("backlog_item:draft->active", "backlog_item", "draft", "completed") {
		t.Fatal("expected no match for wrong toState")
	}
	if !s.matchesLifecycleFilter("*:*->active", "task", "draft", "active") {
		t.Fatal("expected match with wildcards")
	}
	if s.matchesLifecycleFilter("invalid_no_colon", "task", "draft", "active") {
		t.Fatal("expected false for invalid filter")
	}
	if s.matchesLifecycleFilter("task:invalid_transition", "task", "draft", "active") {
		t.Fatal("expected false for invalid transition")
	}

	// isConcurrentAllowed
	if !s.isConcurrentAllowed(JobTypeCachePrewarm, "") {
		t.Fatal("expected true for CachePrewarm")
	}
	if !s.isConcurrentAllowed(JobTypeRunWrapper, CategoryTesting) {
		t.Fatal("expected true for RunWrapper+CategoryTesting")
	}
	if s.isConcurrentAllowed(JobTypeRunWrapper, "other") {
		t.Fatal("expected false for RunWrapper+other")
	}
	if s.isConcurrentAllowed("unknown_job_type", "") {
		t.Fatal("expected false for unknown job type")
	}

	// parseCronSchedule
	sched, err := s.parseCronSchedule("0 0 * * *")
	if err != nil || sched == nil {
		t.Fatalf("expected valid cron, got err: %v", err)
	}
	if _, err := s.parseCronSchedule("invalid-cron"); err == nil {
		t.Fatal("expected error on invalid cron")
	}

	// autoCompleteMilestonesForBacklogItem with nil/empty
	s.autoCompleteMilestonesForBacklogItem(context.Background(), nil)
	s.autoCompleteMilestonesForBacklogItem(context.Background(), map[string]any{})
	s.autoCompleteMilestonesForBacklogItem(context.Background(), map[string]any{
		objects.FieldKeyMilestoneRefs: []any{"milestone-1"},
	})
	s.autoCompleteMilestonesForBacklogItem(context.Background(), map[string]any{
		objects.FieldKeyMilestoneRefs: []string{"milestone-2"},
	})

	// buildCompletionMapFromActivityCache
	_ = s.buildCompletionMapFromActivityCache()

	// buildCompletionMap
	qr := &storage.QueryResult{
		Objects: []map[string]any{
			{
				"event_type": "scheduler_job_completed",
				"target_id":  "j1",
				"created_at": time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	m := s.buildCompletionMap(qr)
	if m == nil || m["j1"].IsZero() {
		t.Fatal("expected completion time for j1")
	}

	// isJobMissed
	job := &ScheduledJob{
		ID:           "j1",
		ScheduleExpr: "0 0 * * *",
	}
	now := time.Now()
	_ = s.isJobMissed(job, now, 1*time.Hour, map[string]time.Time{}, nil)

	// hasCompletionEventForTime
	_ = s.hasCompletionEventForTime("j1", now.Add(-10*time.Minute), 5*time.Minute, nil, now.Add(-8*time.Minute))

	// recordHealthMetric
	s.recordHealthMetric(context.Background(), now, 1, 0, 0, 0, 10*time.Millisecond, 10, 4, 1024, 2048)
}

func TestExtended_RunWrapperRetryHelpers(t *testing.T) {
	job := &ScheduledJob{
		ID:                "rw-job",
		MaxRuntimeSeconds: 120,
	}

	if s := effectiveRunWrapperTimeoutSeconds(60, job); s != 60 {
		t.Fatalf("expected 60, got %d", s)
	}
	if s := effectiveRunWrapperTimeoutSeconds(0, job); s != 120 {
		t.Fatalf("expected 120, got %d", s)
	}
	if s := effectiveRunWrapperTimeoutSeconds(0, &ScheduledJob{}); s != DefaultMaxRuntimeSeconds {
		t.Fatalf("expected default %d, got %d", DefaultMaxRuntimeSeconds, s)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sp := storage.NewNoopObjectStorage()
	tmpDir := t.TempDir()
	h := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir)
	defer h.StopNotificationContext()
	rwh, ok := h.(*RunWrapperHandler)
	if !ok {
		t.Fatal("expected *RunWrapperHandler")
	}

	if exec := rwh.getExecutor(); exec == nil {
		t.Fatal("expected non-nil executor")
	}

	rwh.emitBundleProgress(context.Background(), job, "running")
}

func TestExtended_HourglassTimers(t *testing.T) {
	if a := actionForExpiredTimer(agentclaim.TimerTypeCheckin); a != actionWakeOrchestrator {
		t.Fatalf("expected wake orchestrator, got %v", a)
	}
	if a := actionForExpiredTimer(timerTypeDeadline); a != actionEscalateDeadline {
		t.Fatalf("expected escalate deadline, got %v", a)
	}
	if a := actionForExpiredTimer("unknown"); a != actionKillStuckProcess {
		t.Fatalf("expected kill stuck process, got %v", a)
	}

	blocker, err := buildMissedDeadlineRiskBlocker("b1", "task-1", "backlog_item", "Test Item")
	if err != nil || blocker == nil {
		t.Fatalf("expected blocker, got err: %v", err)
	}

	id := newMissedDeadlineRiskBlockerID()
	if !strings.HasPrefix(id, "RIS-") {
		t.Fatalf("expected RIS- prefix, got %s", id)
	}

	if !refsContainID([]string{"id1", "id2"}, "id2") {
		t.Fatal("expected true")
	}
	if refsContainID([]string{"id1"}, "id2") {
		t.Fatal("expected false")
	}
	if !refsContainID([]any{"id1", "id2"}, "id1") {
		t.Fatal("expected true for any slice")
	}
	if refsContainID("invalid", "id1") {
		t.Fatal("expected false for non-slice")
	}

	sp := storage.NewNoopObjectStorage()
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	_ = hourglassSourcePresent(ctx, sp, secCtx, "task-1")
	_ = hasOpenMissedDeadlineEscalation(ctx, sp, secCtx, "task-1")
}

func TestExtended_CapOrchestratorHelpers(t *testing.T) {
	s := "hello world 12345"
	if tr := truncateOutput(s, 5); !strings.HasPrefix(tr, "hello...") {
		t.Fatalf("expected hello... prefix, got %s", tr)
	}
	if tr := truncateOutput(s, 100); tr != s {
		t.Fatalf("expected original, got %s", tr)
	}

	raw := []byte(`{"key": "value"}`)
	if m := jsonOrRaw(raw); m == nil {
		t.Fatal("expected non-nil json")
	}
	if notJSON := jsonOrRaw([]byte("plain text")); notJSON != "plain text" {
		t.Fatalf("expected plain text, got %v", notJSON)
	}

	if n, ok := asInt(42); !ok || n != 42 {
		t.Fatalf("expected 42, got %d", n)
	}
	if n, ok := asInt(float64(10)); !ok || n != 10 {
		t.Fatalf("expected 10, got %d", n)
	}
	if n, ok := asInt(int64(7)); !ok || n != 7 {
		t.Fatalf("expected 7, got %d", n)
	}
	if _, ok := asInt("not-int"); ok {
		t.Fatal("expected false for string")
	}

	if cr := criticalRootForPackagePath("pkg/scheduler/foo.go"); cr != "pkg/scheduler" {
		t.Fatalf("expected pkg/scheduler, got %s", cr)
	}
	if cr := criticalRootForPackagePath("pkg/storage/sub/file.go"); cr != "pkg/storage" {
		t.Fatalf("expected pkg/storage, got %s", cr)
	}
	if cr := criticalRootForPackagePath("other.go"); cr != "" {
		t.Fatalf("expected empty, got %s", cr)
	}

	blockers, err := parseSystemCheckPublicBlockers([]byte(`{"issues": [{"severity": "blocker"}, {"severity": "warning"}]}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	_ = blockers

	if k := openAGIPlanPersonaKey("p1", "per1"); k != "p1\x00per1" {
		t.Fatalf("unexpected key: %s", k)
	}
	if k := openATKPlanTaskKey("p1", "t1"); k != "p1\x00t1" {
		t.Fatalf("unexpected key: %s", k)
	}

	tmpDir := t.TempDir()
	_ = defaultEscalationChain(tmpDir)
	_ = buildHumanProvider(tmpDir)
	_ = buildAgentProvider(tmpDir)

	sp := storage.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	coh := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)

	coh.recordFailure("grooming", errors.New("test failure"))
	tracker := coh.readFailureTracker()
	if tracker.ConsecutiveFailures < 1 {
		t.Fatalf("expected >= 1 failure, got %d", tracker.ConsecutiveFailures)
	}
	coh.clearFailures()
}

func TestExtended_CapStageGates(t *testing.T) {
	if !cvsStatusEligibleForCAP("active") {
		t.Fatal("expected true for active")
	}
	if cvsStatusEligibleForCAP("completed") {
		t.Fatal("expected false for completed")
	}

	if !agentTaskDeliveryTerminal("complete") {
		t.Fatal("expected true for complete")
	}
	if agentTaskDeliveryTerminal("failed") {
		t.Fatal("expected false for failed")
	}
	if agentTaskDeliveryTerminal("in_progress") {
		t.Fatal("expected false for in_progress")
	}

	if _, err := parseFlexibleTime(time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("expected valid RFC3339 parse: %v", err)
	}
	if _, err := parseFlexibleTime("invalid-time"); err == nil {
		t.Fatal("expected error on invalid time")
	}

	refs := relatedStringRefs([]any{"r1", "r2"})
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if len(relatedStringRefs("invalid")) != 0 {
		t.Fatal("expected 0 for non-slice")
	}

	now := time.Now()
	objPast := map[string]any{
		objects.FieldKeyUpdatedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
	}
	if objectTouchedSince(objPast, now.Add(-30*time.Minute)) {
		t.Fatal("expected false")
	}
	if !objectTouchedSince(objPast, now.Add(-2*time.Hour)) {
		t.Fatal("expected true")
	}

	reason := reviewResultStaleReason(map[string]any{
		"evaluated_at": now.Add(-10 * time.Minute).Format(time.RFC3339),
	}, now, 5*time.Minute)
	if reason == "" {
		t.Fatal("expected stale reason")
	}

	tmpDir := t.TempDir()
	sp := storage.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	coh := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)

	coh.writeStateFile("test_state.json", map[string]string{"foo": "bar"})
	if !coh.stateFileExists("test_state.json") {
		t.Fatal("expected state file to exist")
	}
	if !coh.stateFileNonEmpty("test_state.json") {
		t.Fatal("expected non-empty state file")
	}
	val, err := coh.readStateFile("test_state.json")
	if err != nil || val == nil {
		t.Fatalf("expected valid state file data: %v", err)
	}

	pending := capStagePending{Stage: "metrics", FailureAttempts: 7}
	if !capStageAttemptExhausted(pending) {
		t.Fatal("expected exhausted")
	}
	coh.quarantineCAPStage(pending)
	if !coh.capStageQuarantineActive(pending) {
		t.Fatal("expected quarantine active")
	}
	coh.clearCAPStageQuarantine()

	coh.recordCAPStageFailureAttempt("review")
	coh.clearCAPStageFailureAttempts("review")
}

func TestExtended_JobTriggerQueue(t *testing.T) {
	reqs := []JobTriggerRequest{
		{JobID: "j1"},
		{JobID: "j2"},
	}
	ids := extractJobIDs(reqs)
	if len(ids) != 2 || ids[0] != "j1" || ids[1] != "j2" {
		t.Fatalf("unexpected ids: %v", ids)
	}

	if !batchContainsTestBundleJobID([]JobTriggerRequest{{JobID: "SCH-run-bundle-1"}}) {
		t.Fatal("expected true for bundle job")
	}
	if batchContainsTestBundleJobID([]JobTriggerRequest{{JobID: "regular-job"}}) {
		t.Fatal("expected false for regular job")
	}

	if !jobIDLooksLikeCrossProcessTimestampTestRunner("SCH-1700000000") {
		t.Fatal("expected true")
	}
	if jobIDLooksLikeCrossProcessTimestampTestRunner("SCH-cron") {
		t.Fatal("expected false")
	}

	_ = jobIDLooksLikeCLISubmitNanos("SCH-1234567890")
	_ = triggerWarrantsCacheMissRetry(JobTriggerRequest{JobID: "SCH-run-bundle-test"})
	_ = triggerIsCLIOneShot(JobTriggerRequest{TriggerOrigin: TriggerOriginCLISubmit})
	_ = jobIDLooksLikeCASInstanceSchedulerJobID("SCH-job-123")

	p := prioritizeCachePrewarmTriggers([]JobTriggerRequest{
		{JobID: "regular"},
		{JobID: DefaultCachePrewarmJobID},
	})
	if len(p) != 2 || p[0].JobID != DefaultCachePrewarmJobID {
		t.Fatalf("expected CachePrewarm first: %v", p)
	}

	_ = batchNeedsCASReconcileBeforeTriggerReload([]JobTriggerRequest{{JobID: "SCH-job"}})

	tmpDir := t.TempDir()
	q := NewJobTriggerQueue(tmpDir)

	if err := q.EnqueueTriggerRequest("job-1"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := q.EnqueueTriggerRequestWithOrigin("job-2", "origin-test"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := q.EnqueueTriggerRequests([]string{"job-3"}, "origin-test"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	peek, err := q.PeekTriggerRequests()
	if err != nil || len(peek) < 3 {
		t.Fatalf("expected >= 3 peeked: %v, err: %v", peek, err)
	}

	has, err := q.HasPendingTriggerWithOrigin("job-2", "origin-test")
	if err != nil || !has {
		t.Fatalf("expected true, got %v, err: %v", has, err)
	}

	deq, err := q.DequeueTriggerRequests(2)
	if err != nil || len(deq) != 2 {
		t.Fatalf("expected 2 dequeued: %v, err: %v", deq, err)
	}
}

func TestExtended_SchedulerMetricsCollector(t *testing.T) {
	cfg := DefaultSchedulerMetricsConfig()
	if !cfg.Enabled {
		t.Fatal("expected enabled")
	}
	dis := DisabledSchedulerMetricsConfig()
	if dis.Enabled {
		t.Fatal("expected disabled")
	}

	coll := NewDefaultSchedulerMetricsCollector()
	coll.RecordJobLoaded("j1", "test", "cron")
	coll.RecordJobScheduled("j1", "test", "cron")
	coll.RecordJobExecutionStarted("j1", "test")
	coll.RecordJobExecutionCompleted("j1", "test", 100*time.Millisecond, true)
	coll.RecordJobExecutionFailed("j2", "test", 50*time.Millisecond, errors.New("job err"))
	coll.RecordHandlerCreated("test")
	coll.RecordHandlerCreationFailed("test", errors.New("create err"))
	coll.RecordTriggerValidation("j1", "cron", true, "ok")
	coll.RecordScheduleAttempt("j1", "cron", true)
	coll.RecordScheduleError("j1", "cron", errors.New("sched err"))
	coll.RecordConflictCheck("j1", true)
	coll.RecordConflictDetected("j1", "test")
	coll.RecordSchedulerStart(50 * time.Millisecond)
	coll.RecordSchedulerStop(20 * time.Millisecond)
	coll.RecordJobLoadError(errors.New("load err"))
	coll.RecordPoolCreationDeclined("pool", "purpose", "memory")
	coll.RecordTriggerQueueDequeued(5)
	coll.RecordTriggerQueueTriggerFailed("j1", errors.New("trig err"))
	coll.RecordTriggerQueueReloadRetry("j1")
	coll.RecordTriggerQueueReloadFailed("j1", errors.New("reload err"))
	coll.RecordDispatchPressureDropped("src", "queue full")

	metrics := coll.GetMetrics()
	if metrics.Jobs.Executed == 0 {
		t.Fatal("expected > 0 total jobs executed")
	}
}

func TestExtended_EnvelopeTickDispatch(t *testing.T) {
	if !envelopeTickDispatchJobTypeAllowed("cache_prewarm", false, nil) {
		t.Fatal("expected true for default allow")
	}
	if envelopeTickDispatchHardDeny(JobTypeDataCellEnvelopeTick) != true {
		t.Fatal("expected true for hard deny")
	}

	set := envelopeTickDispatchCommaSeparatedSet("job1, job2,job3")
	if len(set) != 3 {
		t.Fatalf("expected 3 items, got %d", len(set))
	}

	env := map[string]string{
		EnvKeyEnvelopeTickDispatchAllowlistExtra: "custom_job",
		EnvKeyEnvelopeTickDispatchDenyExtra:      "denied_job",
		EnvKeyEnvelopeTickDispatchMaxTriggers:    "5",
		EnvKeyEnvelopeTickDispatchMode:           "sequential",
		EnvKeyEnvelopeTickDispatchExpand:         "true",
	}

	_ = envelopeTickDispatchAllowlistExtraFromEnv(env)
	_ = envelopeTickDispatchDenyExtraFromEnv(env)
	maxTrig, invalid := envelopeTickDispatchMaxTriggersFromEnv(env)
	if invalid || maxTrig != 5 {
		t.Fatalf("expected 5, got %d", maxTrig)
	}
	if mode := envelopeTickDispatchModeFromJob(env); mode != "sequential" {
		t.Fatalf("expected sequential, got %s", mode)
	}
	if !envelopeTickDispatchExpandFromJob(env) {
		t.Fatal("expected true expand")
	}

	types, dupes := dedupeEnvelopeTickResolvedJobTypes([]string{"a", "b", "a"})
	if len(types) != 2 || dupes != 1 {
		t.Fatalf("expected 2 types, 1 dupe, got %v, %d", types, dupes)
	}

	if !envelopeTickDispatchParseTruthy("true") || !envelopeTickDispatchParseTruthy("1") || !envelopeTickDispatchParseTruthy("yes") {
		t.Fatal("expected truthy")
	}
	if envelopeTickDispatchParseTruthy("false") || envelopeTickDispatchParseTruthy("0") {
		t.Fatal("expected falsy")
	}

	if !envelopeTickDispatchEnvBool(env, EnvKeyEnvelopeTickDispatchExpand, false) {
		t.Fatal("expected true")
	}
}

func TestExtended_ConvergenceTestBundle(t *testing.T) {
	if !outcomeIsBad("fail") || !outcomeIsBad("test_fail") || !outcomeIsBad("timeout") {
		t.Fatal("expected bad outcome")
	}
	if outcomeIsBad("pass") {
		t.Fatal("expected false for pass")
	}

	if !outcomeIsFlake("flake") {
		t.Fatal("expected true for flake")
	}
	if !outcomeIsBuildFailure("build_failure") {
		t.Fatal("expected true for build_failure")
	}
	if !outcomeIsGood("pass") || !outcomeIsGood("ok") {
		t.Fatal("expected true for pass and ok")
	}

	cmds := parseSuggestedRerunCommands(map[string]any{
		"suggested_rerun_commands": []any{"go test ./pkg/foo"},
	})
	if len(cmds) != 1 || cmds[0] != "go test ./pkg/foo" {
		t.Fatalf("unexpected rerun commands: %v", cmds)
	}

	tmpDir := t.TempDir()
	_ = readTriggerQueuePending(tmpDir)
	_ = findFingerprintForPattern(tmpDir, "pkg/scheduler")

	lines := []map[string]any{
		{
			"job_id":                    "SCH-run-1",
			KeyBundleCommandFingerprint: "fp1",
			KeyTestOutcome:              "pass",
			"tests_failed":              0,
			"timestamp":                 time.Now().UTC().Format(time.RFC3339),
		},
	}
	snap := BuildTestBundleConvergenceSnapshot(tmpDir, lines)
	if snap == nil || snap.PassCount < 1 {
		t.Fatalf("expected snapshot with pass count >= 1: %v", snap)
	}

	_, _ = ReadTestBundleHealthTailLines(context.Background(), tmpDir, 10)
	_, _ = ReadTestBundleEventsTailLines(context.Background(), tmpDir, 10)
	_, _ = ReadTestBundleProgressTailLines(context.Background(), tmpDir, 10)
}

func TestExtended_TestBundleRerun(t *testing.T) {
	cmdStr := "go test -v ./pkg/scheduler -log=/tmp/test.log"
	_ = ExtractBundleLogPath(cmdStr)
	_ = ResolveBundleLogPath("/root", "job1", cmdStr)

	fp := FingerprintBundleCommand(cmdStr)
	if fp == "" {
		t.Fatal("expected non-empty fingerprint")
	}

	formatted := FormatRunWrapperCommandString("go", []string{"test", "./pkg/scheduler"})
	if formatted != "go test ./pkg/scheduler" {
		t.Fatalf("unexpected formatted command: %s", formatted)
	}

	pkg := extractGoTestPackageArg("go", []string{"test", "-v", "./pkg/scheduler"})
	if pkg != "./pkg/scheduler" {
		t.Fatalf("expected ./pkg/scheduler, got %s", pkg)
	}

	timeout := parseGoTestTimeoutSeconds("go test -timeout 30s ./...", 10)
	if timeout != 30 {
		t.Fatalf("expected 30, got %d", timeout)
	}

	p, fn := splitFailedTestName("pkg/scheduler.TestFoo", "pkg/scheduler")
	if fn != "TestFoo" {
		t.Fatalf("expected TestFoo, got %s", fn)
	}
	_ = p

	sug := BuildSuggestedGoTestRerunCommands("go test ./pkg/scheduler", "go", []string{"test", "./pkg/scheduler"}, []string{"TestFoo"}, 60)
	if len(sug) == 0 {
		t.Fatal("expected suggested commands")
	}

	entry := BuildTestBundleHealthEntry("j1", "completed", "pass", 0, fp, sug, "pkg/scheduler", "/tmp/log", nil)
	if entry == nil {
		t.Fatal("expected non-nil health entry")
	}

	_ = substantiveGoTestRunBlockAfterDelimiter("=== RUN TestFoo\n--- PASS: TestFoo (0.00s)")
	_ = trimToLastSchedulerTestRunForParsing("output line 1\noutput line 2")
}

func TestExtended_ConvergenceRoutingAndPhase(t *testing.T) {
	if FieldAsString("hello") != "hello" {
		t.Fatal("expected hello")
	}
	if FieldAsString(123) != "" {
		t.Fatal("expected empty for non-string")
	}
	if FieldAsString(nil) != "" {
		t.Fatal("expected empty for nil")
	}

	obj := map[string]any{
		objects.FieldKeyPredictions:         map[string]any{"estimated_duration": "5m"},
		objects.FieldKeyCurrentPhase:        "measure",
		objects.FieldKeyBeforeStateSnapshot: map[string]any{"status": "clean"},
	}
	preds := extractPredictions(obj)
	if preds == nil {
		t.Fatal("expected non-nil predictions")
	}
	before := extractBeforeStateSnapshot(obj)
	if before == nil {
		t.Fatal("expected non-nil before snapshot")
	}

	meta := map[string]any{}
	mergeObjectFieldsIntoRoutingMeta(obj, "exploring", "standard", meta)
	if effectivePhaseFromMeta(meta) == "" {
		t.Fatal("expected non-empty effective phase")
	}
	if effectiveFlowFromMeta(meta) == "" {
		t.Fatal("expected non-empty effective flow")
	}

	snap := &TestBundleConvergenceSnapshot{
		PassCount: 1,
	}
	profile := resolvePhaseRoutingProfile("standard")
	implied := measurementImpliedPhase(snap, profile)
	align := phaseAlignment(ConvergencePhase("measure"), implied)
	_ = align
	rank := phaseRank(implied)
	_ = rank
	next := allowedNextPhases(implied)
	_ = next
	route := RouteConvergencePhase(snap, "measure", "standard")
	if route == nil {
		t.Fatal("expected non-nil route")
	}
	routeMap, err := PhaseRouterResultAsMap(route)
	if err != nil || routeMap == nil {
		t.Fatalf("expected route map, got err: %v", err)
	}

	after := BuildAfterStateSnapshotMap(snap)
	if after == nil {
		t.Fatal("expected after state")
	}
	action := BuildNextActionText(snap)
	_ = action
	logEntry := BuildConvergenceActivityLogEntryNoNewWatermark("measure", snap, time.Now())
	if logEntry == nil {
		t.Fatal("expected log entry")
	}

	needed, _ := convergenceTerminalFollowUpNeeded(snap)
	_ = needed
	tmpDir := t.TempDir()
	_ = terminalFollowupSpawnDir(tmpDir)
	markerFile := terminalFollowupSpawnMarkerFile(tmpDir, "prior-1", "wm-1")
	marker := &terminalFollowupMarker{
		PriorSessionID:  "prior-1",
		NewSessionID:    "new-1",
		HealthWatermark: "2026-09-22T00:00:00Z",
	}
	_ = writeTerminalFollowupMarker(markerFile, marker)
	readMarker, err := readTerminalFollowupMarker(markerFile)
	if err != nil || readMarker == nil {
		t.Fatalf("expected read marker, err: %v", err)
	}
}

func TestExtended_Issues(t *testing.T) {
	tmpDir := t.TempDir()

	path := issuesFilePath(tmpDir)
	if !strings.HasSuffix(path, "issues.json") {
		t.Fatalf("expected issues.json path: %s", path)
	}

	ReportIssue(tmpDir, "j1", "test", "first error")
	ReportIssue(tmpDir, "j2", "build", "second error")

	payload, err := ReadIssues(tmpDir)
	if err != nil || payload == nil {
		t.Fatalf("failed to read issues: %v", err)
	}
	if len(payload.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(payload.Issues))
	}

	ClearIssuesIfOk(tmpDir)
	_, err = ClearIssues(tmpDir)
	if err != nil {
		t.Fatalf("failed to clear issues: %v", err)
	}

	emptyPayload, err := ReadIssues(tmpDir)
	if err != nil || emptyPayload == nil || len(emptyPayload.Issues) != 0 {
		t.Fatalf("expected 0 issues after clear, got %v", emptyPayload)
	}
}

func TestExtended_RetentionTolerance(t *testing.T) {
	job := &ScheduledJob{
		ID: "retention-job",
		EnvironmentVariables: map[string]string{
			"BATCH_SIZE":          "50",
			"MAX_BATCHES":         "5",
			"BULK_DELETE_WORKERS": "4",
			EnvKeyKinds:           "audit_event,base_metric",
		},
	}

	batchSize, maxBatches := getBatchConfig(job)
	if batchSize != 50 || maxBatches != 5 {
		t.Fatalf("expected (50, 5), got (%d, %d)", batchSize, maxBatches)
	}

	workers := getBulkDeleteWorkers(job)
	if workers != 4 {
		t.Fatalf("expected 4 workers, got %d", workers)
	}

	prio1 := retentionKindPriority("audit_event")
	prio2 := retentionKindPriority("unknown_kind")
	if prio1 == prio2 {
		t.Fatal("expected different priorities")
	}

	filter := getKindFilter(job)
	if !filter["audit_event"] || !filter["base_metric"] {
		t.Fatalf("unexpected filter: %v", filter)
	}

	_ = isCatalogKind("catalog_entry")
	_ = isCatalogKind("other")

	tmpDir := t.TempDir()
	sp := storage.NewNoopObjectStorage()
	h := NewRetentionToleranceHandler(sp, tmpDir)
	rth := h.(*RetentionToleranceHandler)

	var lastMsg string
	rth.SetProgressFunc(func(msg string) {
		lastMsg = msg
	})
	rth.emitProgress("test message")
	if lastMsg != "test message" {
		t.Fatalf("expected test message, got %s", lastMsg)
	}
}

func TestExtended_CleanupConfig(t *testing.T) {
	params := map[string]any{
		"path":     "/tmp/test",
		"patterns": []any{"*.log", "*.tmp"},
		"max_age":  "24h",
		"limit":    100,
	}

	if s := strParam(params, "path"); s != "/tmp/test" {
		t.Fatalf("expected /tmp/test, got %s", s)
	}
	if patterns := strSliceParam(params, "patterns"); len(patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %v", patterns)
	}
	if n := intParam(params, "limit"); n != 100 {
		t.Fatalf("expected 100, got %d", n)
	}

	dur, err := parseDuration("48h")
	if err != nil || dur != 48*time.Hour {
		t.Fatalf("expected 48h, got %v, err: %v", dur, err)
	}

	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewCleanupConfigHandler(tmpDir, logger)
	cch := h.(*CleanupConfigHandler)

	f1 := filepath.Join(tmpDir, "file1.tmp")
	_ = os.WriteFile(f1, []byte("data"), 0644)
	err = cch.runDeleteFiles(tmpDir, map[string]any{
		"path":     tmpDir,
		"patterns": []any{"*.tmp"},
	})
	if err != nil {
		t.Fatalf("failed runDeleteFiles: %v", err)
	}

	f2 := filepath.Join(tmpDir, "file2.log")
	_ = os.WriteFile(f2, []byte("large file content\nsecond line\n"), 0644)
	err = cch.runTruncateFiles(tmpDir, map[string]any{
		"path":            f2,
		"keep_last_lines": 1,
	})
	if err != nil {
		t.Fatalf("failed runTruncateFiles: %v", err)
	}

	_ = cch.runReapStaleLocks(tmpDir, map[string]any{"path": tmpDir})
	_ = cch.runReapTempFiles(tmpDir, map[string]any{"path": tmpDir})
	_ = cch.runEnforceLogRetention(tmpDir, map[string]any{"path": tmpDir, "max_age": "1h"})
}

func TestExtended_PackageConcurrencySync(t *testing.T) {
	rawJobs := []map[string]any{
		{
			objects.FieldKeyID: "SCH-run-pkg-storage",
			objects.FieldKeyMetadata: map[string]any{
				"package_concurrency_limit": 2,
				"command_args":              []any{"test", "./pkg/storage"},
			},
		},
	}

	limits := packageConcurrencyLimitsFromSchedulerJobRaw(rawJobs)
	_ = limits

	if n, ok := intFromMetadata(42); !ok || n != 42 {
		t.Fatal("expected 42")
	}
	if n, ok := intFromMetadata(float64(10)); !ok || n != 10 {
		t.Fatal("expected 10")
	}
	if _, ok := intFromMetadata("not-int"); ok {
		t.Fatal("expected false")
	}

	args := commandArgsToStrings([]any{"arg1", "arg2"})
	if len(args) != 2 || args[0] != "arg1" || args[1] != "arg2" {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestExtended_CLIBinary(t *testing.T) {
	tmpDir := t.TempDir()
	_ = resolveSchedulerCLIBinary(tmpDir)
	_ = SchedulerCLIBinaryConfigWarning(tmpDir)
	_, _ = ResolveSchedulerDaemonBinary(tmpDir)
	_, _ = firstExistingDaemonBinary(tmpDir)
	_ = isTestBinary("/path/to/my_test.test")
	_ = isTestBinary("/path/to/zqk")
	_, _, _ = binaryFromSettings(tmpDir)
}

func TestExtended_JobLoader(t *testing.T) {
	if prio := rawJobPriority(map[string]any{"priority": "high"}); prio != "high" {
		t.Fatalf("expected high, got %s", prio)
	}
	if prio := rawJobPriority(nil); prio != JobPriorityNormal {
		t.Fatalf("expected normal, got %s", prio)
	}

	sp := storage.NewNoopObjectStorage()
	jl := NewJobLoader(sp, nil, nil, nil)

	rawJob := map[string]any{
		objects.FieldKeyID:                 "SCH-test-load",
		objects.FieldKeyJobType:            JobTypeRunWrapper,
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "0 0 * * *",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyCommand:            "echo hello",
		objects.FieldKeyCommandArgs:        []any{"arg1", "arg2"},
		objects.FieldKeyEnvironmentVariables: map[string]any{
			"FOO": "BAR",
		},
		objects.FieldKeyMetadata: map[string]any{
			"test_meta": 123,
		},
		"category": "testing",
	}

	job, err := jl.HydrateJob(rawJob)
	if err != nil || job == nil {
		t.Fatalf("expected hydrated job, err: %v", err)
	}
	if job.ID != "SCH-test-load" || job.Command != "echo hello" {
		t.Fatalf("unexpected job fields: %+v", job)
	}

	jl.RefreshEnvironmentVariablesFromRaw(job, map[string]any{
		objects.FieldKeyEnvironmentVariables: map[string]any{
			"FOO": "UPDATED",
		},
	})
	if job.EnvironmentVariables["FOO"] != "UPDATED" {
		t.Fatalf("expected UPDATED, got %s", job.EnvironmentVariables["FOO"])
	}
}

func TestExtended_SchedulerState(t *testing.T) {
	_ = getPackageConcurrencyMaxWait()

	s := &Scheduler{
		logger:                     logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		jobs:                       make(map[string]*ScheduledJob),
		lockFailureCountByJobID:    make(map[string]int),
		dispatchDropRetriesByJobID: make(map[string]int),
	}

	jobID := "j1"
	if s.shouldSkipJobDueToLockFailures(jobID) {
		t.Fatal("expected false initially")
	}

	_, _, shouldDisable := s.recordLockFailure(jobID)
	_ = shouldDisable
	s.clearLockFailureCount(jobID)

	if c := s.getDispatchDropRetryCount(jobID); c != 0 {
		t.Fatalf("expected 0, got %d", c)
	}
	if c := s.incrementDispatchDropRetryCount(jobID); c != 1 {
		t.Fatalf("expected 1, got %d", c)
	}
	s.clearDispatchDropRetryCount(jobID)

	s.TouchActivity()
	if act := s.GetLastActivity(); act.IsZero() {
		t.Fatal("expected non-zero activity")
	}

	var buf bytes.Buffer
	s.SetTriggerQueueEventWriter(&buf)
	s.EmitTriggerQueueEvent(map[string]any{"event": "test"})
	if buf.Len() == 0 {
		t.Fatal("expected buffer write")
	}

	s.RecordTriggerQueueDequeued(1)
	s.RecordTriggerQueueTriggerFailed(jobID, errors.New("err"))
	s.RecordTriggerQueueReloadRetry(jobID)
	s.RecordTriggerQueueReloadFailed(jobID, errors.New("err"))

	s.SetSecurityContext(pkgctx.NewSystemSecurityContext())
	s.SetNotificationContext(NewNotificationContext(nil, nil).(*NotificationContext))
	s.SetOnlyManualJobs(true)
	s.SetValidationScanner(nil)
	if nc := s.GetNotificationContext(); nc == nil {
		t.Fatal("expected notification context")
	}

	job := &ScheduledJob{ID: "j1", Enabled: true}
	s.jobs[job.ID] = job
	if err := s.RegisterJob(job); err != nil {
		t.Fatalf("failed to register job: %v", err)
	}
	if j, ok := s.GetJob("j1"); !ok || j == nil {
		t.Fatal("expected job")
	}
	if jobs := s.ListJobs(); len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if exec := s.GetExecutor(); exec == nil {
		t.Fatal("expected non-nil executor")
	}
}
