package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_CapStageGates_Transitions(t *testing.T) {
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

	h := &CapOrchestratorHandler{
		storage:     sp,
		projectRoot: tmpDir,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	// 1. Path helpers
	dir := h.capStateDir()
	if dir == "" {
		t.Errorf("expected non-empty capStateDir")
	}
	p1 := h.capStatePath("test.json")
	p2 := h.legacyCapStatePath("test.json")
	if p1 == "" || p2 == "" {
		t.Errorf("expected paths")
	}

	// 2. Stage entry and failure attempts
	h.markStageEntered("plan", "PRI-1")
	h.recordCAPStageFailureAttempt("plan")
	h.clearCAPStageFailureAttempts("plan")
	h.clearGroomingPlannedZeroLatch()

	// 3. Quarantine helpers
	pending := capStagePending{
		Stage:           "plan",
		PlanID:          "PRI-1",
		FailureAttempts: capStageMaxAttempts + 1,
	}
	if !capStageAttemptExhausted(pending) {
		t.Errorf("expected attempt exhausted")
	}
	h.quarantineCAPStage(pending)
	if !h.capStageQuarantineActive(pending) {
		t.Errorf("expected quarantine active")
	}
	_ = h.capStageQuarantineRetryReady(pending, time.Now().Add(2*time.Hour))
	h.clearCAPStageQuarantine()

	// 4. Status eligibility and terminal checks
	if !cvsStatusEligibleForCAP("active") || !cvsStatusEligibleForCAP("paused") {
		t.Errorf("expected active/paused eligible")
	}
	if cvsStatusEligibleForCAP("archived") {
		t.Errorf("expected archived not eligible")
	}

	if !agentTaskDeliveryTerminal(objects.ObjectStatusComplete) ||
		!agentTaskDeliveryTerminal(objects.ObjectStatusCompleted) ||
		!agentTaskDeliveryTerminal("done") {
		t.Errorf("expected terminal states true")
	}
	if agentTaskDeliveryTerminal(objects.ObjectStatusInProgress) {
		t.Errorf("expected in_progress not terminal")
	}

	// 5. relatedStringRefs
	refs := relatedStringRefs([]string{"r1", "r2"})
	if len(refs) != 2 {
		t.Errorf("unexpected refs: %v", refs)
	}
	refsAny := relatedStringRefs([]any{"r3", 123})
	if len(refsAny) != 1 || refsAny[0] != "r3" {
		t.Errorf("unexpected refsAny: %v", refsAny)
	}
	if relatedStringRefs(nil) != nil {
		t.Errorf("expected nil for nil input")
	}

	// 6. reviewResultStaleReason & parseFlexibleTime & objectTouchedSince
	now := time.Now().UTC()
	mFresh := map[string]any{"timestamp": now.Format(time.RFC3339)}
	if reviewResultStaleReason(mFresh, now, 1*time.Hour) != "" {
		t.Errorf("expected empty stale reason for fresh review")
	}
	mStale := map[string]any{"timestamp": now.Add(-2 * time.Hour).Format(time.RFC3339)}
	if reviewResultStaleReason(mStale, now, 1*time.Hour) == "" {
		t.Errorf("expected stale reason for older review")
	}

	tFlex, errFlex := parseFlexibleTime(now.Format(time.RFC3339))
	if errFlex != nil || tFlex.IsZero() {
		t.Errorf("parseFlexibleTime failed: %v", errFlex)
	}

	objTouched := map[string]any{objects.FieldKeyUpdatedAt: now.Format(time.RFC3339)}
	if !objectTouchedSince(objTouched, now.Add(-1*time.Hour)) {
		t.Errorf("expected objectTouchedSince true")
	}

	// 7. stateFile utilities
	h.writeStateFile("custom_state.json", map[string]any{"key": "val"})
	if !h.stateFileExists("custom_state.json") {
		t.Errorf("expected stateFileExists true")
	}
	if !h.stateFileNonEmpty("custom_state.json") {
		t.Errorf("expected stateFileNonEmpty true")
	}
	_ = h.stateFileFresh("custom_state.json", "1h")
	readData, errRead := h.readStateFile("custom_state.json")
	if errRead != nil || readData == nil {
		t.Errorf("readStateFile failed: %v", errRead)
	}
}

func TestExtended_MetricsCleanupHandlers_Registration(t *testing.T) {
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

	job := &ScheduledJob{
		ID:          "SCH-test-cleanup",
		JobType:     JobTypeAggregationMetricsCleanup,
		Category:    CategoryMaintenance,
		TriggerType: "timer",
	}

	// 1. ChangeJournalAggregationHandler
	cjHandler := NewChangeJournalAggregationHandler(sp)
	_ = cjHandler.Execute(ctx, job)

	// 2. AggregationMetricsCleanupHandler
	amHandler := NewAggregationMetricsCleanupHandler(sp)
	_ = amHandler.Execute(ctx, job)

	// 3. GenericMetricsCleanupHandler
	gmHandler := NewGenericMetricsCleanupHandler(sp)
	_ = gmHandler.Execute(ctx, job)
}

func TestExtended_CallbackListener_RoutesAndActivity(t *testing.T) {
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

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sched := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)
	sched.projectRoot = tmpDir

	h := &CallbackListenerHandler{
		storage:    sp,
		logger:     logger,
		scheduler:  sched,
		activeJobs: make(map[string]time.Time),
	}

	job := &ScheduledJob{
		ID:          "SCH-listener-test",
		JobType:     JobTypeCallbackListener,
		Category:    CategorySystem,
		TriggerType: "immediate",
	}

	// 1. handleHealthCheck
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)
	h.handleHealthCheck(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for health check, got %d", w.Code)
	}

	// 2. handleJobStatus
	wStatus := httptest.NewRecorder()
	h.handleJobStatus(wStatus, r, map[string]any{"job_id": "SCH-1", "status": "running"}, job)
	if wStatus.Code != http.StatusOK {
		t.Errorf("expected 200 for handleJobStatus, got %d", wStatus.Code)
	}

	// 3. handleEmitEvent
	wEmit := httptest.NewRecorder()
	h.handleEmitEvent(wEmit, r, map[string]any{"event_type": "custom", "payload": map[string]any{"x": 1}}, job)

	// 4. updateActivity
	h.updateActivity(map[string]any{"job_id": "SCH-1"})
}
