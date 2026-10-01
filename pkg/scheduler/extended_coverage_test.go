package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/agentclaim"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/clusterstatus"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/telemetry"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func TestExtended_Permissions(t *testing.T) {
	s := &Scheduler{}

	// Security context not set
	if err := s.checkPermission("read:scheduler_job"); err == nil {
		t.Fatal("expected error when secCtx is nil")
	}

	// Admin role bypasses
	s.secCtx = &pkgctx.SecurityContext{Roles: []string{"admin"}}
	if err := s.CheckJobReadPermission(); err != nil {
		t.Fatalf("unexpected error for admin: %v", err)
	}
	if err := s.CheckJobWritePermission(); err != nil {
		t.Fatalf("unexpected error for admin: %v", err)
	}
	if err := s.CheckJobDeletePermission(); err != nil {
		t.Fatalf("unexpected error for admin: %v", err)
	}
	if err := s.CheckJobExecutePermission(); err != nil {
		t.Fatalf("unexpected error for admin: %v", err)
	}
	if err := s.CheckSchedulerManagePermission(); err != nil {
		t.Fatalf("unexpected error for admin: %v", err)
	}

	// Exact permission
	s.secCtx = &pkgctx.SecurityContext{Permissions: []string{"read:scheduler_job"}}
	if err := s.CheckJobReadPermission(); err != nil {
		t.Fatalf("expected read permission to pass: %v", err)
	}
	if err := s.CheckJobWritePermission(); err == nil {
		t.Fatal("expected write permission to fail")
	}

	// Wildcard "*"
	s.secCtx = &pkgctx.SecurityContext{Permissions: []string{"*"}}
	if err := s.CheckJobWritePermission(); err != nil {
		t.Fatalf("expected wildcard to pass: %v", err)
	}

	// Pattern match "execute:*"
	s.secCtx = &pkgctx.SecurityContext{Permissions: []string{"execute:*"}}
	if err := s.CheckJobExecutePermission(); err != nil {
		t.Fatalf("expected pattern execute:* to match: %v", err)
	}
	if err := s.CheckJobReadPermission(); err == nil {
		t.Fatal("expected read permission to fail with execute:*")
	}
}

func TestExtended_RuntimeThreadCount(t *testing.T) {
	n := GetRuntimeThreadCount()
	if runtime.GOOS != "linux" && n != 0 {
		t.Fatalf("expected 0 threads on %s, got %d", runtime.GOOS, n)
	}
}

func TestExtended_ScheduledJobGuard(t *testing.T) {
	if !scheduledJobEnvMissing(nil) {
		t.Fatal("expected nil job to be missing env")
	}
	job := &ScheduledJob{}
	if !scheduledJobEnvMissing(job) {
		t.Fatal("expected job with nil EnvironmentVariables to be missing env")
	}
	job.EnvironmentVariables = map[string]string{"A": "B"}
	if scheduledJobEnvMissing(job) {
		t.Fatal("expected job with EnvironmentVariables to NOT be missing env")
	}
}

func TestExtended_ScheduledJobMap(t *testing.T) {
	job := &ScheduledJob{
		ID:          "job-1",
		JobType:     "run_wrapper",
		Category:    "test",
		Title:       "Test Job",
		Description: "Job Description",
		TriggerType: "cron",
		Enabled:     true,
		Priority:    "high",
	}
	m := job.ToMap()
	if m[objects.FieldKeyID] != "job-1" ||
		m[objects.FieldKeyJobType] != "run_wrapper" ||
		m[objects.FieldKeyCategory] != "test" ||
		m[objects.FieldKeyTitle] != "Test Job" ||
		m[objects.FieldKeyDescription] != "Job Description" ||
		m[objects.FieldKeyTriggerType] != "cron" ||
		m[objects.FieldKeyEnabled] != true ||
		m[objects.FieldKeyPriority] != "high" {
		t.Fatalf("unexpected map contents: %+v", m)
	}
}

type testDummyPayload struct {
	Job *ScheduledJob
}

func TestExtended_PipelineScheduledJobPayload(t *testing.T) {
	getJob := func(p *testDummyPayload) *ScheduledJob {
		if p == nil {
			return nil
		}
		return p.Job
	}

	// Wrong type
	if _, ok := decodeScheduledJobPayload[*testDummyPayload]("string", getJob); ok {
		t.Fatal("expected false for invalid type")
	}

	// Nil Job
	if _, ok := decodeScheduledJobPayload[*testDummyPayload](&testDummyPayload{Job: nil}, getJob); ok {
		t.Fatal("expected false for nil Job")
	}

	// Valid Job
	p, ok := decodeScheduledJobPayload[*testDummyPayload](&testDummyPayload{Job: &ScheduledJob{ID: "j-123"}}, getJob)
	if !ok || p.Job.ID != "j-123" {
		t.Fatal("expected true for valid Job payload")
	}
}

func TestExtended_NoAutoRestart(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty project root
	if err := WriteNoAutoRestartFile(""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := RemoveNoAutoRestartFile(""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if IsNoAutoRestartSet("") {
		t.Fatal("expected false for empty projectRoot")
	}

	// Normal path
	p := NoAutoRestartFilePath(tmpDir)
	if p == "" {
		t.Fatal("expected non-empty path")
	}
	if IsNoAutoRestartSet(tmpDir) {
		t.Fatal("expected false before write")
	}
	if err := WriteNoAutoRestartFile(tmpDir); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if !IsNoAutoRestartSet(tmpDir) {
		t.Fatal("expected true after write")
	}
	if err := RemoveNoAutoRestartFile(tmpDir); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if IsNoAutoRestartSet(tmpDir) {
		t.Fatal("expected false after remove")
	}
}

func TestExtended_LogBuilderDomains(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Exercise each domain wrapper to ensure coverage of all 24 logging helpers
	_ = ConvergenceSessionTickLog(logger)
	_ = RunWrapperLog(logger)
	_ = DataCellEnvelopeTickLog(logger)
	_ = RetentionToleranceLog(logger)
	_ = TestIOLog(logger)
	_ = SchedulerEventsAggregationLog(logger)
	_ = LifecycleCheckLog(logger)
	_ = NoOpHandlerLog(logger)
	_ = CachePrewarmLog(logger)
	_ = CallbackListenerLog(logger)
	_ = AuditAggregationLog(logger)
	_ = ChangeJournalAggregationLog(logger)
	_ = AggregationMetricsCleanupLog(logger)
	_ = GenericMetricsCleanupLog(logger)
	_ = CASRecoveryKindLog(logger)
	_ = SchedulerDaemonLog(logger)
	_ = SchedulerLifecycleLog(logger)
	_ = SchedulerJobManagementLog(logger)
	_ = SchedulerJobExecutionLog(logger)
	_ = SchedulerTriggerQueueLog(logger)
	_ = SchedulerDispatchLog(logger)
	_ = MaintenanceRunnerLog(logger)
	_ = SchedulerPolicyEngineLog(logger)
	_ = CVSPipelineTickSyncLog(logger)
	_ = ConvergenceRoutingLog(logger)
	_ = ChangeJournalAggregationPipelineLog(logger)
	_ = SchedulerJobLoaderLog(logger)
	_ = SchedulerNotificationsLog(logger)
}

func TestExtended_LogRotator(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	rotator := NewLogRotator(tmpDir, logger)

	oldFile := filepath.Join(tmpDir, "old.log")
	newFile := filepath.Join(tmpDir, "new.log")

	if err := os.WriteFile(oldFile, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Backdate oldFile to 10 days ago
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	if err := rotator.Rotate(context.Background(), 24*time.Hour); err != nil {
		t.Fatalf("rotate failed: %v", err)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("expected oldFile to be removed")
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatal("expected newFile to be retained")
	}
}

func TestExtended_JobCategoryLogs(t *testing.T) {
	if norm := normalizeCategoryName(""); norm != "uncategorized" {
		t.Fatalf("expected uncategorized, got %s", norm)
	}
	if norm := normalizeCategoryName("   "); norm != "uncategorized" {
		t.Fatalf("expected uncategorized, got %s", norm)
	}
	if norm := normalizeCategoryName("Data-Sync"); norm != "data-sync" {
		t.Fatalf("expected data-sync, got %s", norm)
	}
	if norm := normalizeCategoryName("Foo$$$Bar---Baz"); norm != "foo-bar-baz" {
		t.Fatalf("expected foo-bar-baz, got %s", norm)
	}

	// No-op for empty root or nil job
	appendCategoryLogEntry("", nil, "", 0, nil)

	tmpDir := t.TempDir()
	job := &ScheduledJob{
		ID:          "cat-job-1",
		JobType:     "run_wrapper",
		Category:    "Maintenance",
		Title:       "Clean Temp",
		TriggerType: "cron",
	}

	appendCategoryLogEntry(tmpDir, job, "completed", 5*time.Second, nil)
	appendCategoryLogEntry(tmpDir, job, "failed", 1*time.Second, errors.New("sample error"))

	logPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, "by-category", "maintenance.events.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected category log file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty category log")
	}
}

func TestExtended_MetricsRecorderHelpers(t *testing.T) {
	_ = buildJobLoadedMetric("j1", "type1", "cron")
	_ = buildJobScheduledMetric("j1", "type1", "cron")
	_ = buildJobExecutionStartedMetric("j1", "type1")
	_ = buildHandlerCreatedMetric("type1")
	_ = buildHandlerCreationFailedMetric("type1", nil)
	_ = buildHandlerCreationFailedMetric("type1", errors.New("err"))
	_ = buildTriggerValidationMetric("j1", "cron", true, "")
	_ = buildTriggerValidationMetric("j1", "cron", false, "reason")
	_ = buildScheduleAttemptMetric("j1", "cron", true)
	_ = buildScheduleAttemptMetric("j1", "cron", false)
	_ = buildScheduleErrorMetric("j1", "cron", errors.New("fail"))
	_ = buildConflictCheckMetric("j1", true)
	_ = buildConflictCheckMetric("j1", false)
	_ = buildConflictDetectedMetric("j1", "type1")
	_ = buildJobLoadErrorMetric(errors.New("err"))
	_ = buildTriggerQueueDequeuedMetric(10)
	_ = buildTriggerQueueTriggerFailedMetric("j1", "trigger-err")
	_ = buildTriggerQueueReloadRetryMetric("j1")
	_ = buildTriggerQueueReloadFailedMetric("j1", errors.New("reload-fail"))
	_ = buildDispatchPressureDroppedMetric("source1", "reason1")

	// metrics_recorder.go
	_ = buildJobExecutionMetric("run", "j1", "type1", time.Second, nil)
	_ = buildJobExecutionMetric("run", "j1", "type1", time.Second, errors.New("fail"))
	_ = buildSchedulerLifecycleMetric("start", time.Second)

	collector := &DefaultSchedulerMetricsCollector{}
	if rec := collector.GetObservabilityRecorder(); rec == nil {
		t.Fatal("expected non-nil recorder")
	}
	var nilCollector *DefaultSchedulerMetricsCollector
	if rec := nilCollector.GetObservabilityRecorder(); rec == nil {
		t.Fatal("expected non-nil recorder for nil collector")
	}
}

func TestExtended_CASRecoveryHelpers(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx := context.Background()

	// Empty project root or kind
	runCASRecoveryForKind(ctx, "", "", logger, nil)
	runCASRecoveryForKind(ctx, "root", "", logger, nil)
	runCASRecoveryForKind(ctx, "", "kind", logger, nil)

	// Non-existent directory (handles error gracefully)
	runCASRecoveryForKind(ctx, t.TempDir(), "non_existent_kind", logger, nil)
}

func TestExtended_ConvergenceDebrief(t *testing.T) {
	if debrief := BuildPredictionDebrief(nil, nil, ""); debrief != nil {
		t.Fatalf("expected nil debrief for nil snap, got %+v", debrief)
	}

	snap := &TestBundleConvergenceSnapshot{
		HealthWatermarkRFC3339:    "2026-09-22T00:00:00Z",
		DeltaAssessment:           "healthy",
		HadFailureInWindow:        false,
		FailingFingerprintsNow:    []string{},
		ReadyForSessionCompletion: true,
		Heartbeat:                 &ConvergenceHeartbeat{Stale: false},
	}

	preds := map[string]any{
		"expected_signal":       "decreasing fingerprints",
		"hypothesis_confidence": "high",
	}

	debrief := BuildPredictionDebrief(preds, snap, "operator notes here")
	if debrief == nil {
		t.Fatal("expected non-nil debrief")
	}
	if debrief["signal_alignment"] != "matched" {
		t.Fatalf("expected matched alignment, got %v", debrief["signal_alignment"])
	}
	if debrief["confidence_vs_outcome"] != "consistent_with_high" {
		t.Fatalf("expected consistent_with_high, got %v", debrief["confidence_vs_outcome"])
	}

	// Mismatched cases
	snapWithFails := &TestBundleConvergenceSnapshot{
		DeltaAssessment:           "degraded",
		HadFailureInWindow:        true,
		FailingFingerprintsNow:    []string{"fp1"},
		ReadyForSessionCompletion: false,
	}
	debrief2 := BuildPredictionDebrief(preds, snapWithFails, "")
	if debrief2["signal_alignment"] != "mismatched" {
		t.Fatalf("expected mismatched alignment, got %v", debrief2["signal_alignment"])
	}
	if debrief2["confidence_vs_outcome"] != "underdelivered_vs_high" {
		t.Fatalf("expected underdelivered_vs_high, got %v", debrief2["confidence_vs_outcome"])
	}

	// Partial case
	snapPartial := &TestBundleConvergenceSnapshot{
		HadFailureInWindow:     true,
		FailingFingerprintsNow: []string{},
	}
	debrief3 := BuildPredictionDebrief(preds, snapPartial, "")
	if debrief3["signal_alignment"] != "partial" {
		t.Fatalf("expected partial alignment, got %v", debrief3["signal_alignment"])
	}
	if debrief3["confidence_vs_outcome"] != "mixed_vs_high" {
		t.Fatalf("expected mixed_vs_high, got %v", debrief3["confidence_vs_outcome"])
	}

	// MergePredictionsWithRetrospective
	merged := MergePredictionsWithRetrospective(preds, snap, "test notes")
	if merged["retrospective"] == nil {
		t.Fatal("expected retrospective in merged predictions")
	}
	if merged["expected_signal"] != "decreasing fingerprints" {
		t.Fatal("expected original predictions preserved")
	}

	// nil predictions map
	mergedNil := MergePredictionsWithRetrospective(nil, snap, "")
	if mergedNil["retrospective"] == nil {
		t.Fatal("expected retrospective even with nil input predictions")
	}
}

func TestExtended_TestCommandDetector(t *testing.T) {
	d := DefaultTestCommandDetector()

	// go test
	if !d.IsTestCommand("go", []string{"test", "./..."}) {
		t.Fatal("expected go test to be detected as test command")
	}
	// command contains "test"
	if !d.IsTestCommand("pytest", []string{"tests/"}) {
		t.Fatal("expected pytest to be detected as test command")
	}
	// shell_script contains "go test"
	if !d.IsTestCommand("sh", []string{"-c", "go test -v ./..."}) {
		t.Fatal("expected shell script with go test to be detected")
	}
	// non-test command
	if d.IsTestCommand("ls", []string{"-la"}) {
		t.Fatal("expected ls to NOT be detected as test command")
	}

	// Storage loading with nil storage returns default
	dDefault := LoadTestCommandDetectorFromStorage(nil)
	if !dDefault.IsTestCommand("go", []string{"test"}) {
		t.Fatal("expected default detector behavior")
	}

	// Custom rules coverage (eq, contains, in, unknown)
	custom := &RulesTestCommandDetector{
		Rules: []TestCommandRule{
			{
				Conditions: []TestCommandCondition{
					{Field: "command", Operator: "eq", Value: "custom-runner"},
				},
			},
			{
				Conditions: []TestCommandCondition{
					{Field: "args", Operator: "eq", Value: "--run-tests"},
				},
			},
			{
				Conditions: []TestCommandCondition{
					{Field: "args", Operator: "contains", Value: "suite"},
				},
			},
			{
				Conditions: []TestCommandCondition{
					{Field: "command", Operator: "in", Value: []any{"runner1", "runner2"}},
				},
			},
			{
				Conditions: []TestCommandCondition{
					{Field: "unknown", Operator: "eq", Value: "foo"},
				},
			},
			{
				Conditions: []TestCommandCondition{
					{Field: "command", Operator: "unknown_op", Value: "foo"},
				},
			},
		},
	}

	if !custom.IsTestCommand("custom-runner", nil) {
		t.Fatal("expected custom-runner to match")
	}
	if !custom.IsTestCommand("app", []string{"--run-tests"}) {
		t.Fatal("expected args eq --run-tests to match")
	}
	if !custom.IsTestCommand("app", []string{"run-suite-1"}) {
		t.Fatal("expected args contains suite to match")
	}
	if !custom.IsTestCommand("runner2", nil) {
		t.Fatal("expected command in [runner1, runner2] to match")
	}
	if custom.IsTestCommand("other", []string{"hello"}) {
		t.Fatal("expected other to not match")
	}

	// objectToTestCommandRule helper
	if _, ok := objectToTestCommandRule(nil); ok {
		t.Fatal("expected false for nil obj")
	}
	validObj := map[string]any{
		objects.FieldKeyConditions: []any{
			map[string]any{
				"field":    "command",
				"operator": "eq",
				"value":    "go",
			},
		},
	}
	if r, ok := objectToTestCommandRule(validObj); !ok || len(r.Conditions) != 1 {
		t.Fatalf("expected valid rule, got ok=%v, rule=%+v", ok, r)
	}

	// toInt helper
	if n, ok := toInt(42); !ok || n != 42 {
		t.Fatalf("expected 42, got %d, %v", n, ok)
	}
	if n, ok := toInt(int64(42)); !ok || n != 42 {
		t.Fatalf("expected 42, got %d, %v", n, ok)
	}
	if n, ok := toInt(float64(42)); !ok || n != 42 {
		t.Fatalf("expected 42, got %d, %v", n, ok)
	}
	if _, ok := toInt("string"); ok {
		t.Fatal("expected false for string")
	}
}

func TestExtended_ActivityCache(t *testing.T) {
	c := NewActivityCache().(*ActivityCache)
	defer c.Stop()

	// Update events
	c.UpdateEvent("job-1", "scheduler_job_started", 0, nil)
	c.UpdateEvent("job-1", "scheduler_job_completed", 2*time.Second, nil)
	c.UpdateEvent("job-2", "scheduler_job_started", 0, nil)
	c.UpdateEvent("job-2", "scheduler_job_failed", 1*time.Second, errors.New("err2"))

	entry1, ok := c.GetEntry("job-1")
	if !ok || entry1.TotalCompleted != 1 {
		t.Fatalf("expected entry1 completed count 1, got %+v", entry1)
	}

	entry2, ok := c.GetEntry("job-2")
	if !ok || entry2.TotalFailed != 1 || entry2.LastError != "err2" {
		t.Fatalf("expected entry2 failed count 1, got %+v", entry2)
	}

	all := c.GetAllEntries()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}

	subset := c.GetEntriesForJobs([]string{"job-1", "non-existent"})
	if len(subset) != 1 || subset["job-1"] == nil {
		t.Fatalf("expected subset with job-1, got %+v", subset)
	}

	// Save and load
	tmpDir := t.TempDir()
	if err := c.doSaveCache(tmpDir); err != nil {
		t.Fatalf("doSaveCache failed: %v", err)
	}

	c2 := NewActivityCache().(*ActivityCache)
	defer c2.Stop()
	if err := c2.LoadCache(tmpDir); err != nil {
		t.Fatalf("LoadCache failed: %v", err)
	}
	if len(c2.GetAllEntries()) != 2 {
		t.Fatalf("expected 2 loaded entries, got %d", len(c2.GetAllEntries()))
	}
	if c2.GetMetadata() == nil || c2.GetMetadata().ProjectRoot != tmpDir {
		t.Fatalf("unexpected metadata: %+v", c2.GetMetadata())
	}

	// Load when file does not exist
	c3 := NewActivityCache().(*ActivityCache)
	defer c3.Stop()
	if err := c3.LoadCache(t.TempDir()); err != nil {
		t.Fatalf("LoadCache on empty dir failed: %v", err)
	}

	// SaveCache async call
	if err := c.SaveCache(tmpDir); err != nil {
		t.Fatalf("SaveCache failed: %v", err)
	}

	// Global cache
	if g := GetGlobalActivityCache(); g == nil {
		t.Fatal("expected non-nil global activity cache")
	}
}

func TestExtended_ProcessGroupManager(t *testing.T) {
	pgm := NewProcessGroupManager(context.Background(), 20*time.Millisecond).(*ProcessGroupManager)

	if pgm.GetShutdownContext() == nil {
		t.Fatal("expected non-nil shutdown context")
	}
	if pgm.IsShuttingDown() {
		t.Fatal("expected not shutting down initially")
	}

	killed := false
	pgm.RegisterSubprocess("p1", "j1", "desc1", 100, 100, false, func() error {
		killed = true
		return nil
	})

	status := pgm.GetStatus()
	if status.SubprocessCount != 1 || status.ShuttingDown {
		t.Fatalf("unexpected status: %+v", status)
	}

	pgm.UnregisterSubprocess("p1")
	statusAfter := pgm.GetStatus()
	if statusAfter.SubprocessCount != 0 {
		t.Fatalf("expected 0 subprocesses after unregister, got %d", statusAfter.SubprocessCount)
	}

	// Test shutdown with non-critical process
	pgm.RegisterSubprocess("p2", "j1", "desc2", 101, 101, false, func() error {
		killed = true
		return nil
	})
	if err := pgm.Shutdown("testing"); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	if !pgm.IsShuttingDown() {
		t.Fatal("expected shutting down")
	}
	if !killed {
		t.Fatal("expected killFunc to be called on shutdown")
	}

	// Registering when already shutting down immediately kills
	killedImmediate := false
	pgm.RegisterSubprocess("p3", "j1", "desc3", 102, 102, false, func() error {
		killedImmediate = true
		return nil
	})
	if !killedImmediate {
		t.Fatal("expected immediate kill when registering during shutdown")
	}
}

func TestExtended_PolicyRegistry(t *testing.T) {
	globalPolicyRegistry.SetOverride("tok-1", DispatchPolicyOverride{
		BypassExecutionDepth: true,
	})
	pol, ok := globalPolicyRegistry.GetOverride("tok-1")
	if !ok || !pol.BypassExecutionDepth {
		t.Fatalf("expected token override: %+v", pol)
	}

	globalPolicyRegistry.SetKindOverride("kind-1", PerKindOverride{
		MaxTriggers: 5,
		MaxDepth:    3,
	})
	kPol, ok := globalPolicyRegistry.GetKindOverride("kind-1")
	if !ok || kPol.MaxTriggers != 5 || kPol.MaxDepth != 3 {
		t.Fatalf("expected kind override: %+v", kPol)
	}
}

func TestExtended_DataCellEnvelopeTickMetrics(t *testing.T) {
	tmpDir := t.TempDir()
	outcome := EnvelopeTickDispatchOutcome{
		Evaluated:                   true,
		Mode:                        "auto",
		TriggeredOK:                 2,
		TriggerFailed:               0,
		AllowlistExtraN:             1,
		DenyExtraN:                  1,
		TokenPolicyDenyEntriesN:     1,
		TokenPolicyAllowEntriesN:    1,
		TokenPolicyDenyJSONPresent:  true,
		TokenPolicyAllowJSONPresent: true,
	}

	err := appendDataCellEnvelopeTickJSONL(tmpDir, "job-1", "aug1", "ov1", "types1", nil, outcome)
	if err != nil {
		t.Fatalf("appendDataCellEnvelopeTickJSONL failed: %v", err)
	}

	metricFile := filepath.Join(tmpDir, paths.ProjectDataDir, paths.MetricsDir, dataCellEnvelopeTickJSONL)
	data, err := os.ReadFile(metricFile)
	if err != nil {
		t.Fatalf("read metric file failed: %v", err)
	}
	var row map[string]any
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatalf("invalid jsonl line: %v", err)
	}
	if row["job_id"] != "job-1" || row["envelope_tick_dispatch_mode"] != "auto" {
		t.Fatalf("unexpected metric data: %+v", row)
	}
}

type dummyCoordinationChannel struct {
	lastEvent Event
}

func (d *dummyCoordinationChannel) PublishEvent(event Event) error {
	d.lastEvent = event
	return nil
}
func (d *dummyCoordinationChannel) Subscribe() <-chan Event {
	return nil
}
func (d *dummyCoordinationChannel) WatchEvents(ctx context.Context) error {
	return nil
}

func TestExtended_DriftEventbus(t *testing.T) {
	dummy := &dummyCoordinationChannel{}
	bus := NewCoordinationEventBusAdapter(dummy)

	// Irrelevant topic
	if err := bus.Publish(context.Background(), "some.topic", nil); err != nil {
		t.Fatal(err)
	}
	if dummy.lastEvent.Type != "" {
		t.Fatalf("expected no event published, got %s", dummy.lastEvent.Type)
	}

	// Drift topic
	de := telemetry.DriftEvent{
		JobID:        "j-drift",
		ProcessID:    555,
		MetricName:   "memory_bytes",
		Baseline:     100,
		Current:      150,
		DeviationPct: 50.0,
		DetectedAt:   time.Now().UTC(),
	}
	if err := bus.Publish(context.Background(), "telemetry.drift", de); err != nil {
		t.Fatal(err)
	}
	if dummy.lastEvent.Type != "drift_detected" || dummy.lastEvent.JobID != "j-drift" {
		t.Fatalf("unexpected event: %+v", dummy.lastEvent)
	}
}

func TestExtended_ClusterStatus(t *testing.T) {
	tmpDir := t.TempDir()
	s := &Scheduler{projectRoot: tmpDir, logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))}

	// Nil receiver or job does nothing
	var nilS *Scheduler
	nilS.emitClusterStatusAfterJob(nil, nil)
	s.emitClusterStatusAfterJob(nil, nil)

	// Normal run
	job := &ScheduledJob{ID: "cs-job-1"}
	s.emitClusterStatusAfterJob(job, nil)
	s.emitClusterStatusAfterJob(job, errors.New("sample run error"))

	// Decision
	dec := ClusterStatusDecide(tmpDir, clusterstatus.Watch{})
	if dec.Reason == "" {
		t.Fatal("expected reason in decision")
	}
}

func TestExtended_EvaluationSurface(t *testing.T) {
	if id := EvaluationSurfaceIDFromMaps(nil, nil); id != EvaluationSurfaceSchedulerTestBundleHealthJSONL {
		t.Fatalf("expected default surface, got %s", id)
	}
	if id := EvaluationSurfaceIDFromSessionObject(nil); id != EvaluationSurfaceSchedulerTestBundleHealthJSONL {
		t.Fatalf("expected default surface, got %s", id)
	}
	if id := EvaluationSurfaceIDFromRoutingMeta(nil); id != EvaluationSurfaceSchedulerTestBundleHealthJSONL {
		t.Fatalf("expected default surface, got %s", id)
	}
	if id := EvaluationSurfaceIDFromRoutingMeta(map[string]any{"evaluation_surface_id": "custom-surface"}); id != "custom-surface" {
		t.Fatalf("expected custom-surface, got %s", id)
	}
}

func TestExtended_JobTypeRegistry(t *testing.T) {
	if !isJobTypeIntentionalNoHandler("aggregation") {
		t.Fatal("expected aggregation to be intentional no-handler")
	}
	if isJobTypeIntentionalNoHandler("agent_sync") {
		t.Fatal("expected agent_sync to have handler")
	}

	keys := jobTypeHandlerRegistryKeys()
	if len(keys) == 0 {
		t.Fatal("expected non-empty registry keys")
	}

	reg := handlerKeyRegistry()
	if len(reg) == 0 {
		t.Fatal("expected non-empty handlerKeyRegistry")
	}
}

func TestExtended_AuditEvents(t *testing.T) {
	s := &Scheduler{}
	if r := s.getProjectRoot(); r != "" {
		t.Fatalf("expected empty project root, got %s", r)
	}

	// Empty project root returns error
	ctx := context.Background()
	if err := s.createJobAuditEvent(ctx, "exec", "j1", "type1", "cat1", true, time.Second, nil); err == nil {
		t.Fatal("expected error with empty project root")
	}

	// Nil storage returns error
	s.projectRoot = t.TempDir()
	if err := s.createJobAuditEvent(ctx, "exec", "j1", "type1", "cat1", true, time.Second, nil); err == nil {
		t.Fatal("expected error with nil storage provider")
	}
}

func TestExtended_Errors(t *testing.T) {
	errs := []error{
		ErrJobNotFound,
		ErrJobAlreadyRunning,
		ErrJobCancelled,
		ErrJobTimeout,
		ErrSchedulerDown,
		ErrQueueFull,
		ErrWorkerPoolExhausted,
		ErrInvalidCronSchedule,
		ErrJobValidationFailed,
	}
	for _, err := range errs {
		if err == nil || err.Error() == "" {
			t.Fatal("expected non-empty error")
		}
	}
}

func TestExtended_HandlersLogRotation(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewLogRotatorHandler(tmpDir, logger)
	if err := h.Execute(context.Background(), &ScheduledJob{ID: "rot-1"}); err != nil {
		t.Fatalf("LogRotatorHandler Execute failed: %v", err)
	}
}

func TestExtended_HandlersHourglassCleanup(t *testing.T) {
	h := NewHourglassCleanupHandler()
	job := &ScheduledJob{
		ID: "hg-clean",
		EnvironmentVariables: map[string]string{
			"HOURGLASS_CUTOFF_MINUTES":    "999999",
			"HOURGLASS_TARGET_SUBSTRINGS": "nonexistent_proc_string_xyz",
		},
	}
	if err := h.Execute(context.Background(), job); err != nil {
		t.Fatalf("HourglassCleanupHandler Execute failed: %v", err)
	}
}

func TestExtended_HumanInTheLoomInterrupt(t *testing.T) {
	ctx := context.Background()
	// Empty project root returns error
	if err := EmitHumanInTheLoomInterrupt(ctx, "", "sess-1", "test reason", "pol-1", 0.95, false, "main"); err == nil {
		t.Fatal("expected error with empty project root")
	}

	tmpDir := t.TempDir()
	if err := EmitHumanInTheLoomInterrupt(ctx, tmpDir, "sess-1", "test reason", "pol-1", 0.95, false, "main"); err != nil {
		t.Fatalf("EmitHumanInTheLoomInterrupt failed: %v", err)
	}
}

func TestExtended_Pipelines_NilGuards(t *testing.T) {
	ctx := context.Background()
	if err := RunAutofixBatchCleanupViaPipeline(ctx, nil, nil); err == nil {
		t.Fatal("expected error for nil autofix handler")
	}
	if err := RunAggregationMetricsCleanupViaPipeline(ctx, nil, nil); err == nil {
		t.Fatal("expected error for nil aggregation metrics handler")
	}
	if err := RunGenericMetricsCleanupViaPipeline(ctx, nil, nil); err == nil {
		t.Fatal("expected error for nil generic metrics handler")
	}
	if err := RunRetentionToleranceViaPipeline(ctx, nil, nil); err == nil {
		t.Fatal("expected error for nil retention tolerance handler")
	}
	if err := RunSchedulerJobRetentionViaPipeline(ctx, nil, nil); err == nil {
		t.Fatal("expected error for nil scheduler job retention handler")
	}
	if err := RunWatchdogEvaluationViaPipeline(ctx, nil, nil); err == nil {
		t.Fatal("expected error for nil watchdog evaluation handler")
	}
}

func TestExtended_HandlersAgentSync(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewAgentSyncHandler(tmpDir, logger)
	// With empty/non-existent agents dir
	_ = h.Execute(context.Background(), &ScheduledJob{ID: "sync-1"})
}

func TestExtended_HandlersCapacityScaling(t *testing.T) {
	tmpDir := t.TempDir()
	h := NewCapacityScalingHandler(storage.NewNoopObjectStorage(), tmpDir)
	if err := h.Execute(context.Background(), &ScheduledJob{ID: "cap-scale-1"}); err != nil {
		t.Fatalf("CapacityScalingHandler Execute failed: %v", err)
	}
}

func TestExtended_HandlersStrategicPulse(t *testing.T) {
	h := NewStrategicPulseHandler(storage.NewNoopObjectStorage(), "")
	if err := h.Execute(context.Background(), &ScheduledJob{ID: "pulse-1"}); err == nil {
		t.Fatal("expected error from NoopObjectStorage")
	}
}

func TestExtended_HandlersCascadeUpdate(t *testing.T) {
	h := NewCascadeUpdateHandler(nil)
	job := &ScheduledJob{ID: "casc-1"}
	// Without event data in ctx
	if err := h.Execute(context.Background(), job); err == nil {
		t.Fatal("expected error without event data")
	}
	// With invalid event data format
	ctxInvalid := context.WithValue(context.Background(), evtDataKey{}, "not-a-map")
	if err := h.Execute(ctxInvalid, job); err == nil {
		t.Fatal("expected error for non-map event data")
	}
}

func TestExtended_HandlersOperationExecution(t *testing.T) {
	h := NewOperationExecutionHandler(nil)
	job := &ScheduledJob{ID: "op-1"}
	// Without event data in ctx
	if err := h.Execute(context.Background(), job); err == nil {
		t.Fatal("expected error without event data")
	}
	// With invalid event data format
	ctxInvalid := context.WithValue(context.Background(), evtDataKey{}, "not-a-map")
	if err := h.Execute(ctxInvalid, job); err == nil {
		t.Fatal("expected error for non-map event data")
	}
	// Missing required operation parameters
	ctxMissing := context.WithValue(context.Background(), evtDataKey{}, map[string]any{})
	if err := h.Execute(ctxMissing, job); err == nil {
		t.Fatal("expected error for missing parameters")
	}
	// Unknown operation type
	ctxUnknown := context.WithValue(context.Background(), evtDataKey{}, map[string]any{
		"operation_type":           "unknown_op",
		"object_id":                "obj-1",
		objects.FieldKeyObjectKind: "kind-1",
	})
	if err := h.Execute(ctxUnknown, job); err == nil {
		t.Fatal("expected error for unknown operation type")
	}
	// Create without data
	ctxCreateNoData := context.WithValue(context.Background(), evtDataKey{}, map[string]any{
		"operation_type":           "create",
		"object_id":                "obj-1",
		objects.FieldKeyObjectKind: "kind-1",
	})
	if err := h.Execute(ctxCreateNoData, job); err == nil {
		t.Fatal("expected error for create without data")
	}
	// Update without data
	ctxUpdateNoData := context.WithValue(context.Background(), evtDataKey{}, map[string]any{
		"operation_type":           "update",
		"object_id":                "obj-1",
		objects.FieldKeyObjectKind: "kind-1",
	})
	if err := h.Execute(ctxUpdateNoData, job); err == nil {
		t.Fatal("expected error for update without data")
	}
}

func TestExtended_HandlersSchedulerEventsAggregation(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := NewSchedulerEventsAggregationHandler(tmpDir, logger)
	_ = h.Execute(context.Background(), &ScheduledJob{ID: "evag-1"})
}

type dummyExecHandler struct {
	err error
}

func (d *dummyExecHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return d.err
}

func TestExtended_JobExecution(t *testing.T) {
	// Outcome and Progress helpers
	WriteJobOutcome("", "", "", nil)
	tmpDir := t.TempDir()
	WriteJobOutcome(tmpDir, "j-out", "run_wrapper", map[string]any{"ok": true})
	WriteJobProgress("", "", nil)
	WriteJobProgress(tmpDir, "j-prog", map[string]any{"pct": 50})
	writeJobLogEntry("", "", nil)
	writeJobLogEntry(tmpDir, "j-log", map[string]any{"msg": "hello"})

	// trimJobLogFileIfNeeded
	trimJobLogFileIfNeeded("non_existent_file.jsonl", 5)
	logFile := filepath.Join(tmpDir, "test_trim.jsonl")
	var lines []byte
	for i := 0; i < 10; i++ {
		lines = append(lines, []byte(fmt.Sprintf("line %d\n", i))...)
	}
	if err := os.WriteFile(logFile, lines, 0644); err != nil {
		t.Fatal(err)
	}
	trimJobLogFileIfNeeded(logFile, 5)

	// dispatchContextForScheduledJob
	ctx0, cancel0 := dispatchContextForScheduledJob(&ScheduledJob{MaxRuntimeSeconds: 0})
	cancel0()
	if ctx0 == nil {
		t.Fatal("expected non-nil ctx0")
	}
	ctx10, cancel10 := dispatchContextForScheduledJob(&ScheduledJob{MaxRuntimeSeconds: 10})
	cancel10()
	if ctx10 == nil {
		t.Fatal("expected non-nil ctx10")
	}

	// shouldInvokeConfiguredJobCallback
	if !shouldInvokeConfiguredJobCallback(context.Background(), &ScheduledJob{ExecutionMode: "one_time"}) {
		t.Fatal("expected true for one_time execution mode")
	}
	if shouldInvokeConfiguredJobCallback(context.Background(), &ScheduledJob{ExecutionMode: "reusable"}) {
		t.Fatal("expected false for reusable execution mode")
	}

	// runSchedulerJobHandlerWithRetry
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	s := &Scheduler{logger: logger, storage: storage.NewNoopObjectStorage()}
	job := &ScheduledJob{ID: "j-run-retry"}
	ctx := context.Background()
	if err := s.runSchedulerJobHandlerWithRetry(ctx, ctx, job, &dummyExecHandler{err: nil}, nil, time.Now()); err != nil {
		t.Fatalf("runSchedulerJobHandlerWithRetry failed: %v", err)
	}
	// Handler returning error
	_ = s.runSchedulerJobHandlerWithRetry(ctx, ctx, job, &dummyExecHandler{err: errors.New("handler error")}, nil, time.Now())
}

func TestExtended_Hourglass(t *testing.T) {
	if act := actionForExpiredTimer(agentclaim.TimerTypeCheckin); act != actionWakeOrchestrator {
		t.Fatalf("expected actionWakeOrchestrator, got %v", act)
	}
	if act := actionForExpiredTimer(timerTypeDeadline); act != actionEscalateDeadline {
		t.Fatalf("expected actionEscalateDeadline, got %v", act)
	}
	if act := actionForExpiredTimer("unknown_timer"); act != actionKillStuckProcess {
		t.Fatalf("expected actionKillStuckProcess, got %v", act)
	}

	if !refsContainID([]string{"a", "b"}, "b") {
		t.Fatal("expected true for string slice match")
	}
	if refsContainID([]string{"a", "b"}, "c") {
		t.Fatal("expected false for string slice non-match")
	}
	if !refsContainID([]any{"a", "b"}, "b") {
		t.Fatal("expected true for any slice match")
	}
	if refsContainID([]any{"a", "b"}, "c") {
		t.Fatal("expected false for any slice non-match")
	}
	if refsContainID(nil, "a") || refsContainID(123, "a") {
		t.Fatal("expected false for non-slice types")
	}

	if rb, err := buildMissedDeadlineRiskBlocker("", "ATK-1", "task", "Missed Deadline"); err != nil || rb == nil {
		t.Fatalf("buildMissedDeadlineRiskBlocker failed: %v", err)
	}
	if rb, err := buildMissedDeadlineRiskBlocker("RIS-custom", "ATK-1", "task", "Missed Deadline"); err != nil || rb == nil {
		t.Fatalf("buildMissedDeadlineRiskBlocker with custom id failed: %v", err)
	}

	if id := newMissedDeadlineRiskBlockerID(); id == "" {
		t.Fatal("expected non-empty id")
	}

	if hourglassSourcePresent(context.Background(), nil, nil, "ATK-1") {
		t.Fatal("expected false for nil store")
	}
	if hasOpenMissedDeadlineEscalation(context.Background(), nil, nil, "ATK-1") {
		t.Fatal("expected false for nil store")
	}

	s := &Scheduler{projectRoot: ""}
	s.startHourglassWatcher(context.Background(), nil)
}

func TestExtended_LifecycleCoordination(t *testing.T) {
	s := &Scheduler{}

	// Event filter matching
	if !s.matchesEventFilter("", "ev", "kind") || !s.matchesEventFilter("*", "ev", "kind") {
		t.Fatal("expected wildcard match")
	}
	if !s.matchesEventFilter("ev:kind", "ev", "kind") {
		t.Fatal("expected match for ev:kind")
	}
	if s.matchesEventFilter("ev:kind", "ev", "other") {
		t.Fatal("expected non-match for different kind")
	}
	if !s.matchesEventFilter("ev", "ev", "other") {
		t.Fatal("expected match for type only")
	}
	if s.matchesEventFilter("ev", "other", "other") {
		t.Fatal("expected non-match for different type")
	}
	if s.matchesEventFilter("a:b:c", "a", "b") {
		t.Fatal("expected non-match for invalid format")
	}

	// Lifecycle filter matching
	if !s.matchesLifecycleFilter("", "kind", "a", "b") || !s.matchesLifecycleFilter("*", "kind", "a", "b") {
		t.Fatal("expected wildcard match")
	}
	if s.matchesLifecycleFilter("invalid", "kind", "a", "b") {
		t.Fatal("expected false for invalid filter")
	}
	if s.matchesLifecycleFilter("task:invalid_trans", "task", "a", "b") {
		t.Fatal("expected false for invalid transition")
	}
	if !s.matchesLifecycleFilter("task:a->b", "task", "a", "b") {
		t.Fatal("expected match for task:a->b")
	}
	if s.matchesLifecycleFilter("task:a->b", "other", "a", "b") {
		t.Fatal("expected false for wrong kind")
	}
	if s.matchesLifecycleFilter("task:a->b", "task", "x", "b") {
		t.Fatal("expected false for wrong fromState")
	}
	if s.matchesLifecycleFilter("task:a->b", "task", "a", "x") {
		t.Fatal("expected false for wrong toState")
	}
	if !s.matchesLifecycleFilter("*:*->*", "any", "x", "y") {
		t.Fatal("expected match for *:*->*")
	}
	if !s.matchesLifecycleFilter("task:*->b", "task", "x", "b") {
		t.Fatal("expected match for wildcard fromState")
	}
	if !s.matchesLifecycleFilter("task:a->*", "task", "a", "y") {
		t.Fatal("expected match for wildcard toState")
	}

	// Concurrency
	if !s.isConcurrentAllowed(JobTypeCachePrewarm, "") {
		t.Fatal("expected true for CachePrewarm")
	}
	if !s.isConcurrentAllowed(JobTypeRunWrapper, CategoryTesting) {
		t.Fatal("expected true for RunWrapper testing")
	}
	if s.isConcurrentAllowed(JobTypeRunWrapper, "other") {
		t.Fatal("expected false for RunWrapper other")
	}
	if s.isConcurrentAllowed("other", "") {
		t.Fatal("expected false for other type")
	}

	// Cron schedule parse
	if sched, err := s.parseCronSchedule("0 0 * * * *"); err != nil || sched == nil {
		t.Fatalf("parseCronSchedule failed: %v", err)
	}
	if _, err := s.parseCronSchedule("invalid cron"); err == nil {
		t.Fatal("expected error for invalid cron")
	}

	// Cache lookup
	s.jobs = map[string]*ScheduledJob{"j1": {ID: "j1"}}
	if !s.JobInCache("j1") || s.JobInCache("j2") {
		t.Fatal("unexpected JobInCache result")
	}
	if j, ok := s.lookupJobInCache("j1"); !ok || j.ID != "j1" {
		t.Fatal("lookupJobInCache failed")
	}
	if _, ok := s.lookupJobInCache("j2"); ok {
		t.Fatal("expected false for j2")
	}

	// Activity cache completions
	_ = s.buildCompletionMapFromActivityCache()

	// getStringFromMap
	if getStringFromMap(map[string]any{"k": "v"}, "k") != "v" {
		t.Fatal("expected v")
	}
	if getStringFromMap(map[string]any{"k": 123}, "k") != "" {
		t.Fatal("expected empty for non-string")
	}
	if getStringFromMap(nil, "k") != "" {
		t.Fatal("expected empty for nil map")
	}

	if k := EvtDataKeyForTesting(); k == nil {
		t.Fatal("expected non-nil EvtDataKeyForTesting")
	}
}

func TestExtended_RunWrapperRetry(t *testing.T) {
	if eff := effectiveRunWrapperTimeoutSeconds(10, &ScheduledJob{MaxRuntimeSeconds: 20}); eff != 10 {
		t.Fatalf("expected 10, got %d", eff)
	}
	if eff := effectiveRunWrapperTimeoutSeconds(0, &ScheduledJob{MaxRuntimeSeconds: 20}); eff != 20 {
		t.Fatalf("expected 20, got %d", eff)
	}
	if eff := effectiveRunWrapperTimeoutSeconds(0, &ScheduledJob{MaxRuntimeSeconds: 0}); eff != DefaultMaxRuntimeSeconds {
		t.Fatalf("expected DefaultMaxRuntimeSeconds, got %d", eff)
	}
}

func TestExtended_CapDispatch_And_Stages(t *testing.T) {
	var out map[string]any
	if err := decodeCAPPlanOutput([]byte(`{"plan_id": "p1"}`), &out); err != nil || out["plan_id"] != "p1" {
		t.Fatalf("decodeCAPPlanOutput failed: %v", err)
	}
	if err := decodeCAPPlanOutput([]byte(`invalid json`), &out); err == nil {
		t.Fatal("expected error for invalid json")
	}

	if id := mintAgentTaskID(); id == "" {
		t.Fatal("expected non-empty minted task ID")
	}

	plans := []whatsnext.WhatsNextPriorityPlan{
		{ID: "p-sub1"},
		{ID: "p-sub2"},
	}
	ids := capDispatchPlanIDs("p-main", plans)
	if len(ids) != 3 || ids[0] != "p-main" || ids[1] != "p-sub1" || ids[2] != "p-sub2" {
		t.Fatalf("unexpected plan IDs: %v", ids)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	h := &CapOrchestratorHandler{storage: storage.NewNoopObjectStorage(), logger: logger}
	ctx := context.Background()
	if inst := h.capStageAGIInstruction(ctx, "", "p1"); inst != "" {
		t.Fatalf("expected empty instruction, got %s", inst)
	}
	if inst := h.capStageAGIInstruction(ctx, "review", "p1"); !strings.Contains(inst, "review") {
		t.Fatalf("expected instruction containing review, got %s", inst)
	}
}

func TestExtended_RunWrapperCallbacks(t *testing.T) {
	ctx := context.Background()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Unknown callback type
	job := &ScheduledJob{ID: "j1", CallbackOnCompletion: "http://localhost:9999"}
	InvokeJobCallback(ctx, logger, nil, job, "unknown_type", nil)

	// Empty callback URL
	jobEmpty := &ScheduledJob{ID: "j1"}
	InvokeJobCallback(ctx, logger, nil, jobEmpty, "completion", nil)

	// Event callback
	jobEvent := &ScheduledJob{
		ID:                   "j1",
		CallbackType:         "event",
		CallbackOnCompletion: "my_event",
	}
	InvokeJobCallback(ctx, logger, nil, jobEvent, "completion", map[string]any{"data": 1})

	// Direct with nil logger
	executeCallbackDirectWithLogger(ctx, nil, "event", "url", nil)

	// Direct with unknown mechanism
	executeCallbackDirectWithLogger(ctx, logger, "unknown_mech", "url", map[string]any{"job_id": "j1"})
}

func TestExtended_CachePrewarm(t *testing.T) {
	// tierTimeoutFromJob without deadline
	if timeout := tierTimeoutFromJob(context.Background(), 5*time.Second, 1*time.Second); timeout != 5*time.Second {
		t.Fatalf("expected 5s timeout, got %v", timeout)
	}

	// tierTimeoutFromJob with deadline
	ctxDeadline, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if timeout := tierTimeoutFromJob(ctxDeadline, 10*time.Second, 500*time.Millisecond); timeout <= 0 || timeout > 3*time.Second {
		t.Fatalf("unexpected timeout with deadline: %v", timeout)
	}

	_ = inferProjectRootFromSpecsDir()

	tmpDir := t.TempDir()
	h := NewCachePrewarmHandler(nil, nil, storage.NewNoopObjectStorage(), tmpDir, nil)
	_ = h.WithValidationScanner(nil)
	if root := h.(*CachePrewarmHandler).getProjectRoot(); root != tmpDir {
		t.Fatalf("expected projectRoot %s, got %s", tmpDir, root)
	}
}

func TestExtended_CallbackListenerHTTP(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	s := &Scheduler{storage: storage.NewNoopObjectStorage()}
	h := NewCallbackListenerHandler(storage.NewNoopObjectStorage(), logger, s, nil, nil)
	clh, ok := h.(*CallbackListenerHandler)
	if !ok {
		t.Fatal("expected *CallbackListenerHandler")
	}
	defer clh.StopNotificationContext()

	job := &ScheduledJob{
		ID:           "listener-job",
		ListenerPort: 9999,
		ListenerPath: "/callbacks",
	}

	mux := http.NewServeMux()
	clh.registerRoutes(mux, "/callbacks", job)

	// Test Health Check
	req := httptest.NewRequest("GET", "/callbacks/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Test Job Complete with payload
	bodyComplete := strings.NewReader(`{"job_id": "j1", "duration": 5.5, "job_type": "build", "category": "system"}`)
	req = httptest.NewRequest("POST", "/callbacks/job/complete", bodyComplete)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Test Job Complete missing job_id
	req = httptest.NewRequest("POST", "/callbacks/job/complete", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Test Job Error with error
	bodyErr := strings.NewReader(`{"job_id": "j2", "error": "fatal crash", "job_type": "test", "category": "system"}`)
	req = httptest.NewRequest("POST", "/callbacks/job/error", bodyErr)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Test Job Error missing job_id
	req = httptest.NewRequest("POST", "/callbacks/job/error", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Test Job Status
	bodyStatus := strings.NewReader(`{"job_id": "j3", "status": "running"}`)
	req = httptest.NewRequest("POST", "/callbacks/job/status", bodyStatus)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Test Trigger Job missing job_id
	req = httptest.NewRequest("POST", "/callbacks/trigger", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Test Emit Event missing event_type
	req = httptest.NewRequest("POST", "/callbacks/event", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Test Emit Event valid
	req = httptest.NewRequest("POST", "/callbacks/event", strings.NewReader(`{"event_type": "custom_event"}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Test Invalid JSON
	req = httptest.NewRequest("POST", "/callbacks/job/complete", strings.NewReader(`not-json`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad json, got %d", rec.Code)
	}

	// Test authenticateRequest
	req = httptest.NewRequest("GET", "/test", nil)
	if !clh.authenticateRequest(httptest.NewRecorder(), req) {
		t.Fatal("expected auth to pass when authHook is nil")
	}

	// Test BuildHTTPServer
	srv := clh.BuildHTTPServer("127.0.0.1:0", mux)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestExtended_RetentionCleanup(t *testing.T) {
	sp := storage.NewNoopObjectStorage()
	tmpDir := t.TempDir()
	h := NewRetentionToleranceHandler(sp, tmpDir)
	rth, ok := h.(*RetentionToleranceHandler)
	if !ok {
		t.Fatal("expected *RetentionToleranceHandler")
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Call cleanupOldObjects with various parameters
	cutoff := time.Now().Add(-24 * time.Hour)
	n := rth.cleanupOldObjects(ctx, secCtx, storageCtx, "job1", "audit_event", cutoff, []string{"active"}, 10, 2, 2)
	if n < 0 {
		t.Fatalf("unexpected negative count: %d", n)
	}

	// Test with negative maxBatches (-1 = unlimited) and batchSize 0
	n2 := rth.cleanupOldObjects(ctx, secCtx, storageCtx, "job1", "unknown_kind", cutoff, nil, 0, -1, 1)
	if n2 < 0 {
		t.Fatalf("unexpected negative count: %d", n2)
	}

	// cleanupOldObjectsSlowPath
	n3 := rth.cleanupOldObjectsSlowPath(ctx, secCtx, storageCtx, "job1", "unknown_kind", nil, 10, 1, 1, cutoff.Format(time.RFC3339))
	if n3 < 0 {
		t.Fatalf("unexpected negative count: %d", n3)
	}
}

func TestExtended_MeshLeaseSupervision(t *testing.T) {
	if getFloat(float64(12.5)) != 12.5 {
		t.Fatal("failed float64")
	}
	if getFloat(float32(10.5)) != 10.5 {
		t.Fatal("failed float32")
	}
	if getFloat(int(42)) != 42.0 {
		t.Fatal("failed int")
	}
	if getFloat(int64(99)) != 99.0 {
		t.Fatal("failed int64")
	}
	if getFloat("unknown") != 0.0 {
		t.Fatal("failed default string")
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sp := storage.NewNoopObjectStorage()
	tmpDir := t.TempDir()
	handler := NewMeshLeaseSupervisionHandler(sp, tmpDir, logger)
	mlh, ok := handler.(*MeshLeaseSupervisionHandler)
	if !ok {
		t.Fatal("expected *MeshLeaseSupervisionHandler")
	}

	mlh.reapStaleSubprocesses(map[string]bool{})
	_ = mlh.Execute(context.Background(), &ScheduledJob{ID: "lease-sup"})
}

func TestExtended_AutofixBatchCleanupCore(t *testing.T) {
	tmpDir := t.TempDir()
	autofixDir := filepath.Join(tmpDir, paths.ProjectDataDir, "autofix")
	if err := os.MkdirAll(autofixDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Old unprocessed AUTOFIX file
	oldFile := filepath.Join(autofixDir, "AUTOFIX-old.json")
	_ = os.WriteFile(oldFile, []byte(`{"issues": []}`), 0644)
	past := time.Now().Add(-5 * time.Hour)
	_ = os.Chtimes(oldFile, past, past)

	// 2. Corrupt PROCESSED file
	badFile := filepath.Join(autofixDir, "PROCESSED-bad.json")
	_ = os.WriteFile(badFile, []byte(`corrupt-json`), 0644)

	// 3. FIXED file missing batch_id
	noBatchFile := filepath.Join(autofixDir, "FIXED-nobatch.json")
	_ = os.WriteFile(noBatchFile, []byte(`{"progress": {}}`), 0644)

	// 4. Valid FIXED file
	validFile := filepath.Join(autofixDir, "FIXED-valid.json")
	validJSON := `{"batch_id": "b1", "progress": {"processed": 10, "fixed": 8, "failed": 1, "skipped": 1}}`
	_ = os.WriteFile(validFile, []byte(validJSON), 0644)

	sp := storage.NewNoopObjectStorage()
	h := NewAutofixBatchCleanupHandler(sp, tmpDir)
	ah, ok := h.(*AutofixBatchCleanupHandler)
	if !ok {
		t.Fatal("expected *AutofixBatchCleanupHandler")
	}

	job := &ScheduledJob{
		ID: "autofix-job",
		EnvironmentVariables: map[string]string{
			EnvKeyAutofixBatchMaxAgeHours: "1",
		},
	}

	if err := ah.executeAutofixBatchCleanupCore(context.Background(), job); err != nil {
		t.Fatalf("unexpected error in executeAutofixBatchCleanupCore: %v", err)
	}

	rec := buildBatchMetricErrorRecord("m1", "b1", time.Now().UTC().Format(time.RFC3339), errors.New("sample error"))
	if rec == nil {
		t.Fatal("expected non-nil error record")
	}

	h2 := NewAutofixBatchCleanupHandler(sp, filepath.Join(tmpDir, "nonexistent"))
	_ = h2.(*AutofixBatchCleanupHandler).executeAutofixBatchCleanupCore(context.Background(), job)
}

func TestExtended_MetricsCleanupHandlers(t *testing.T) {
	sp := storage.NewNoopObjectStorage()

	h1 := NewAggregationMetricsCleanupHandler(sp)
	amh, ok := h1.(*AggregationMetricsCleanupHandler)
	if !ok {
		t.Fatal("expected *AggregationMetricsCleanupHandler")
	}

	jobSkip := &ScheduledJob{
		ID: "agg-clean-skip",
		EnvironmentVariables: map[string]string{
			"RETENTION_DAYS": "0",
		},
	}
	if err := amh.executeAggregationMetricsCleanupCore(context.Background(), jobSkip); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jobClean := &ScheduledJob{
		ID: "agg-clean",
		EnvironmentVariables: map[string]string{
			"RETENTION_DAYS": "7",
		},
	}
	if err := amh.executeAggregationMetricsCleanupCore(context.Background(), jobClean); err != nil && !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("unexpected error: %v", err)
	}

	h2 := NewChangeJournalAggregationHandler(sp)
	cjh, ok := h2.(*ChangeJournalAggregationHandler)
	if !ok {
		t.Fatal("expected *ChangeJournalAggregationHandler")
	}
	_ = cjh.preExecutionHealthCheck(context.Background(), jobClean, objects.KindAuditAggregationMetric)
}

func TestExtended_ContextRefreshHandler(t *testing.T) {
	cadences := []struct {
		in      string
		wantErr bool
	}{
		{"P1D", false},
		{"P2W", false},
		{"PT1H", false},
		{"PT30M", false},
		{"PT45S", false},
		{"24h", false},
		{"1h30m", false},
		{"0 0 * * *", false},
		{"invalid", true},
		{"P", true},
	}

	for _, tc := range cadences {
		d, err := parseCadence(tc.in)
		if tc.wantErr && err == nil {
			t.Errorf("parseCadence(%q) expected error, got nil", tc.in)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("parseCadence(%q) unexpected error: %v", tc.in, err)
		}
		if !tc.wantErr && d <= 0 {
			t.Errorf("parseCadence(%q) expected positive duration, got %v", tc.in, d)
		}
	}

	if _, err := parseISODuration("NOT_P"); err == nil {
		t.Fatal("expected error without P prefix")
	}

	tmpDir := t.TempDir()
	sp := storage.NewNoopObjectStorage()
	h := NewContextRefreshHandler(sp, tmpDir)
	crh, ok := h.(*ContextRefreshHandler)
	if !ok {
		t.Fatal("expected *ContextRefreshHandler")
	}

	job := &ScheduledJob{ID: "ctx-refresh"}
	_ = crh.executeContextRefreshCore(context.Background(), job)
}

func TestExtended_CVSPipelineTickSync(t *testing.T) {
	if err := SyncCVSPipelineTickJobOnLifecycle(context.Background(), nil, "draft", "active", nil); err != nil {
		t.Fatal(err)
	}

	sp := storage.NewNoopObjectStorage()
	sessionObj := map[string]any{
		objects.FieldKeyID:    "cs-123",
		objects.FieldKeyTitle: "Convergence Session Alpha",
	}

	if err := SyncCVSPipelineTickJobOnLifecycle(context.Background(), sp, "active", "completed", sessionObj); err != nil {
		t.Fatal(err)
	}

	if !pipelineTickAutoRepointEnabled(map[string]any{EnvKeyPipelineTickAutoRepoint: "true"}) {
		t.Fatal("expected true")
	}
	if pipelineTickAutoRepointEnabled(map[string]any{EnvKeyPipelineTickAutoRepoint: "false"}) {
		t.Fatal("expected false")
	}
	if !pipelineTickAutoRepointEnabled(map[string]any{}) {
		t.Fatal("expected default true")
	}

	if !shouldRepointOnActivateTransition("draft", "active") {
		t.Fatal("expected repoint on active")
	}
	if shouldRepointOnActivateTransition("active", "active") {
		t.Fatal("expected false on same state")
	}

	_ = filterConvergenceSessionsByTitle([]map[string]any{sessionObj}, "Alpha")
	_ = convergenceSessionUpdatedAt(sessionObj)
}

func TestExtended_RetentionMaxCount(t *testing.T) {
	_ = isHighVolumeKind(objects.KindAuditEvent)
	_ = isHighVolumeKind("unknown_kind")

	_ = skipArchiveForOldestIDsPath(objects.KindAuditEvent, nil)
	if skipArchiveForOldestIDsPath("unknown_kind", []string{"active"}) {
		t.Fatal("expected false")
	}

	sp := storage.NewNoopObjectStorage()
	tmpDir := t.TempDir()
	h := NewRetentionToleranceHandler(sp, tmpDir)
	rth, ok := h.(*RetentionToleranceHandler)
	if !ok {
		t.Fatal("expected *RetentionToleranceHandler")
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	del, handled := rth.enforceMaxCountViaHVNoProtect(ctx, secCtx, "job1", "audit_event", 10, 5, 2, 100)
	if handled && del < 0 {
		t.Fatal("unexpected negative deleted")
	}
}

func TestExtended_NotificationsDisplay(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	nd := NewNotificationDisplay(logger)

	notif1 := CreateJobNotification("j1", "test", "cat", "completed", PriorityHigh, 10*time.Second, nil, map[string]any{
		"is_test_failure": false,
	})
	if notif1 == nil || notif1.Title == "" {
		t.Fatal("expected valid notification")
	}

	notif2 := CreateJobNotification("j2", "test", "cat", "failed", PriorityHigh, 5*time.Second, errors.New("some error"), map[string]any{
		"is_test_failure": true,
		KeyTestFailures:   []string{"TestA", "TestB"},
		KeyTestSummary:    map[string]any{"total_tests": 10, "passed_tests": 8, "failed_tests": 2, "skipped_tests": 0},
	})
	if notif2 == nil {
		t.Fatal("expected valid notification")
	}

	notif3 := CreateJobNotification("j3", "test", "cat", "failed", PriorityCritical, 0, errors.New("timeout"), map[string]any{
		"job_description": "a very long job description that will be truncated in notification message display",
	})
	if notif3 == nil {
		t.Fatal("expected valid notification")
	}

	notif4 := CreateJobNotification("j4", "test", "cat", "status_update", PriorityLow, 0, nil, map[string]any{
		objects.FieldKeyStatus: "running",
	})
	if notif4 == nil {
		t.Fatal("expected valid notification")
	}

	nd.Display(notif1)
	nd.Display(notif2)
	nd.Display(notif3)
	nd.Display(notif4)

	if s := formatDuration(500 * time.Millisecond); s != "500ms" {
		t.Fatalf("expected 500ms, got %s", s)
	}
	if s := formatDuration(45 * time.Second); s != "45s" {
		t.Fatalf("expected 45s, got %s", s)
	}
	if s := formatDuration(125 * time.Second); !strings.Contains(s, "m") {
		t.Fatalf("expected minutes, got %s", s)
	}
	if s := formatDuration(3700 * time.Second); !strings.Contains(s, "h") {
		t.Fatalf("expected hours, got %s", s)
	}
}

func TestExtended_PIDFileAndKeepAlive(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")

	_, err := readFileWithTimeout(pidFile, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error reading nonexistent file")
	}

	_ = os.WriteFile(pidFile, []byte("12345\n"), 0644)
	data, err := readFileWithTimeout(pidFile, 1*time.Second)
	if err != nil || strings.TrimSpace(string(data)) != "12345" {
		t.Fatalf("unexpected data: %s, err: %v", string(data), err)
	}

	_ = ResolveProjectRootFromCWD()
}

func TestExtended_JobLogWriter(t *testing.T) {
	tmpDir := t.TempDir()
	jobID := "job-log-test"

	AppendTestBundleEvent(tmpDir, jobID, map[string]any{"action": "start"})
	AppendTestBundleHealthEvent(tmpDir, jobID, map[string]any{"health": "ok"})
	AppendTestBundleProgressEvent(tmpDir, jobID, map[string]any{"progress": 50})

	AppendTestBundleEvent("", "", nil)
	AppendTestBundleHealthEvent("", "", nil)
	AppendTestBundleProgressEvent("", "", nil)

	TrackAndAppendTestBundleProgress(tmpDir, "SCH-run-bundle-test", "started")
	TrackAndAppendTestBundleProgress(tmpDir, "SCH-run-bundle-test", "passed")

	w, err := GetOrCreateJobLogWriter(tmpDir, "regular-job")
	if err != nil {
		t.Fatalf("failed to get/create job log writer: %v", err)
	}
	if w != nil {
		_ = w.WriteLine([]byte("sample log output"))
		CloseJobLogWriter(tmpDir, "regular-job")
	}
}

func TestExtended_RunWrapperExecution(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sp := storage.NewNoopObjectStorage()
	tmpDir := t.TempDir()
	h := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir)
	defer h.StopNotificationContext()
	rwh, ok := h.(*RunWrapperHandler)
	if !ok {
		t.Fatal("expected *RunWrapperHandler")
	}

	jobScript := &ScheduledJob{
		ID:         "job-script",
		Command:    "echo 'hello'\nexit 0",
		LogLevel:   "debug",
		RetryCount: 1,
	}
	prep1 := rwh.prepareRunWrapperExecution(jobScript)
	if !prep1.IsShellScript {
		t.Fatal("expected shell script to be true")
	}
	rwh.logRunWrapperExecutionStart(jobScript, prep1)

	jobTest := &ScheduledJob{
		ID:          "SCH-test-cmd",
		Command:     "go test -v ./pkg/scheduler -run TestSomething",
		CommandArgs: []string{"test", "-v", "github.com/zqk-os/zqk/pkg/scheduler"},
		Metadata: map[string]any{
			KeyBundleCommandFingerprint: "expected-fp",
		},
	}
	prep2 := rwh.prepareRunWrapperExecution(jobTest)
	rwh.warnIfTestBundleMetadataFingerprintMismatch(jobTest, prep2.CmdStr)
}

func TestExtended_JobStateRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir)

	jobID := "SCH-run-bundle-test1"
	execID := "exec-001"

	if err := reg.RegisterExecution(jobID, execID, os.Getpid()); err != nil {
		t.Fatalf("failed to register execution: %v", err)
	}

	jsr, ok := reg.(*JobStateRegistry)
	if !ok {
		t.Fatal("expected *JobStateRegistry")
	}

	state, err := reg.GetExecutionState(jobID)
	if err != nil || state == nil {
		t.Fatalf("expected state, got %v, err: %v", state, err)
	}
	if state.State != jobExecutionStateInProgress {
		t.Fatalf("expected in_progress, got %s", state.State)
	}

	state.State = jobExecutionStateCompleted
	now := time.Now().UTC()
	state.CompletedAt = &now
	if err := reg.UpdateState(jobID, state); err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	summary, err := jsr.Summarize(1 * time.Hour)
	if err != nil || summary == nil {
		t.Fatalf("expected summary, got %v, err: %v", summary, err)
	}
	if summary.ByState[jobExecutionStateCompleted] != 1 {
		t.Fatalf("expected 1 completed, got %d", summary.ByState[jobExecutionStateCompleted])
	}

	states, err := reg.ListInProgress()
	if err != nil {
		t.Fatalf("failed to list states: %v", err)
	}
	_ = states

	if err := reg.RegisterExecution("", "e1", 1); err == nil {
		t.Fatal("expected error on empty jobID")
	}
	if err := reg.RegisterExecution("j1", "", 1); err == nil {
		t.Fatal("expected error on empty execID")
	}

	dirs := jobStateDirsForLookup(tmpDir, jobID)
	if len(dirs) == 0 {
		t.Fatal("expected non-empty dirs")
	}
}
