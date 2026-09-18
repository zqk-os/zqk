package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func TestMaybeWakeOnStageHold_WakesAfterHoldWindow(t *testing.T) {
	root := t.TempDir()
	whatsnext.EnsureCAPStage(root, "cap_stage_orchestrating")
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindAgentTask:   {},
		objects.KindBacklogItem: {},
		objects.KindConvergenceSession: {
			{objects.FieldKeyID: "CVS-1", objects.FieldKeyStatus: objects.ObjectStatusActive},
		},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_orchestrating",
		EnteredAt: time.Now().UTC().Add(-45 * time.Minute).Format(time.RFC3339),
		PlanID:    "PRI-TEST",
		CvsID:     "CVS-1",
	})

	err := h.maybeAdvanceCAPStage(context.Background(), "cap_stage_orchestrating")
	if err == nil {
		t.Fatal("expected hold without delivery")
	}
	raw, err := h.readStateFile(capHoldWakeFile)
	if err != nil {
		t.Fatalf("expected hold wake state: %v", err)
	}
	m, ok := raw.(map[string]any)
	if !ok || m["stage"] != "cap_stage_orchestrating" {
		t.Fatalf("hold wake state: %#v", raw)
	}
	// Rate-limit: second call within period should not rewrite last_wake_at to a newer value beyond period noise.
	first := m["last_wake_at"]
	_ = h.maybeAdvanceCAPStage(context.Background(), "cap_stage_orchestrating")
	raw2, _ := h.readStateFile(capHoldWakeFile)
	m2 := raw2.(map[string]any)
	if m2["last_wake_at"] != first {
		t.Fatalf("expected rate-limited hold wake, got %v then %v", first, m2["last_wake_at"])
	}
}

func TestMaybeWakeOnStageHold_SkipsFreshHold(t *testing.T) {
	root := t.TempDir()
	whatsnext.EnsureCAPStage(root, "cap_stage_orchestrating")
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindAgentTask:          {},
		objects.KindBacklogItem:        {},
		objects.KindConvergenceSession: {{objects.FieldKeyID: "CVS-1", objects.FieldKeyStatus: objects.ObjectStatusActive}},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_orchestrating",
		EnteredAt: time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
		PlanID:    "PRI-TEST",
		CvsID:     "CVS-1",
	})
	_ = h.maybeAdvanceCAPStage(context.Background(), "cap_stage_orchestrating")
	if _, err := h.readStateFile(capHoldWakeFile); err == nil {
		t.Fatal("fresh hold should not wake")
	}
}

func TestMaybeAdvanceCAPStage_GroomingHoldsWithoutArtifacts(t *testing.T) {
	root := t.TempDir()
	whatsnext.EnsureCAPStage(root, "cap_stage_grooming")

	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindPriorityPlan:  {},
		objects.KindBacklogItem:   {},
		objects.KindStrategicPlan: {},
		objects.KindConvergenceSession: {
			{objects.FieldKeyID: "CVS-1", objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeyRelatedObjectRefs: []string{"PLN-1"}},
		},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_grooming",
		EnteredAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		PlanID:    "PLN-1",
		CvsID:     "CVS-1",
	})

	err := h.maybeAdvanceCAPStage(context.Background(), "cap_stage_grooming")
	if err == nil {
		t.Fatal("expected hold without grooming artifacts")
	}
	if got := whatsnext.PeekCAPStage(root); got != "cap_stage_grooming" {
		t.Fatalf("cycle advanced to %q", got)
	}
}

func TestMaybeAdvanceCAPStage_GroomingAdvancesOnGraphMutation(t *testing.T) {
	root := t.TempDir()
	whatsnext.EnsureCAPStage(root, "cap_stage_grooming")

	entered := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindPriorityPlan: {
			{
				objects.FieldKeyID:        "PRI-NEW",
				objects.FieldKeyUpdatedAt: now,
			},
		},
		objects.KindBacklogItem:   {},
		objects.KindStrategicPlan: {},
		objects.KindConvergenceSession: {
			{objects.FieldKeyID: "CVS-1", objects.FieldKeyStatus: objects.ObjectStatusActive},
		},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_grooming",
		EnteredAt: entered,
		PlanID:    "PLN-1",
		CvsID:     "CVS-1",
	})

	if err := h.maybeAdvanceCAPStage(context.Background(), "cap_stage_grooming"); err != nil {
		t.Fatalf("expected advance, got %v", err)
	}
	if got := whatsnext.PeekCAPStage(root); got != "cap_stage_orchestrating" {
		t.Fatalf("expected orchestrating, got %q", got)
	}
	if _, ok := whatsnext.LastCapAdvanceJournalEntry(root); !ok {
		t.Fatal("expected journal entry")
	}
}

func TestMaybeAdvanceCAPStage_UnboundCVSHolds(t *testing.T) {
	root := t.TempDir()
	whatsnext.EnsureCAPStage(root, "cap_stage_grooming")
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindPriorityPlan:       {},
		objects.KindBacklogItem:        {},
		objects.KindStrategicPlan:      {},
		objects.KindConvergenceSession: {},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_grooming",
		EnteredAt: time.Now().UTC().Format(time.RFC3339),
		PlanID:    "PLN-1",
	})
	err := h.maybeAdvanceCAPStage(context.Background(), "cap_stage_grooming")
	if err == nil || !strings.Contains(err.Error(), "unbound") {
		t.Fatalf("expected unbound hold, got %v", err)
	}
}

func TestMaybeAdvanceCAPStage_ForgeReceiptRejected(t *testing.T) {
	root := t.TempDir()
	whatsnext.EnsureCAPStage(root, "cap_stage_orchestrating")
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindAgentTask:          {},
		objects.KindBacklogItem:        {},
		objects.KindConvergenceSession: {{objects.FieldKeyID: "CVS-1", objects.FieldKeyStatus: objects.ObjectStatusActive}},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_orchestrating",
		EnteredAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		CvsID:     "CVS-1",
	})
	h.writeStateFile(capStageReceiptFile, map[string]any{
		"stage":        "cap_stage_orchestrating",
		"artifact_ids": []any{},
	})
	err := h.maybeAdvanceCAPStage(context.Background(), "cap_stage_orchestrating")
	if err == nil {
		t.Fatal("expected hold on empty receipt")
	}
}

func TestDispatchDeliveryComplete_OpenATKInsufficient(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339)
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindAgentTask: {
			{objects.FieldKeyID: "ATK-1", objects.FieldKeyStatus: objects.ObjectStatusProposed, objects.FieldKeyUpdatedAt: now},
		},
		objects.KindBacklogItem: {},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_orchestrating",
		EnteredAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		CvsID:     "CVS-1",
	})
	ok, detail := h.dispatchDeliveryComplete(context.Background(), "cap_stage_orchestrating")
	if ok {
		t.Fatalf("open ATK should be insufficient: %s", detail)
	}
}

func TestDispatchDeliveryComplete_InProgressBacklogInsufficient(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339)
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindAgentTask: {},
		objects.KindBacklogItem: {
			{
				objects.FieldKeyID:        "BLI-1",
				objects.FieldKeyStatus:    objects.ObjectStatusInProgress,
				objects.FieldKeyUpdatedAt: now,
			},
		},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_orchestrating",
		EnteredAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		CvsID:     "CVS-1",
	})

	ok, detail := h.dispatchDeliveryComplete(context.Background(), "cap_stage_orchestrating")
	if ok {
		t.Fatalf("in-progress backlog should be insufficient: %s", detail)
	}
}

func TestDispatchDeliveryComplete_ImplementedATKCounts(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339)
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindAgentTask: {
			{objects.FieldKeyID: "ATK-1", objects.FieldKeyStatus: objects.ObjectStatusImplemented, objects.FieldKeyUpdatedAt: now},
		},
		objects.KindBacklogItem: {},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_design",
		EnteredAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		CvsID:     "CVS-1",
	})
	ok, detail := h.dispatchDeliveryComplete(context.Background(), "cap_stage_design")
	if !ok {
		t.Fatalf("implemented ATK should count as delivery: %s", detail)
	}
}

func TestCAPStageAttemptCeiling(t *testing.T) {
	t.Parallel()

	if capStageAttemptExhausted(capStagePending{FailureAttempts: capStageMaxAttempts}) {
		t.Fatal("attempt ceiling should allow the final bounded attempt")
	}
	if !capStageAttemptExhausted(capStagePending{FailureAttempts: capStageMaxAttempts + 1}) {
		t.Fatal("attempt ceiling should quarantine after the bounded attempts are exhausted")
	}
}

func TestMarkStageEnteredResetsQuarantineAfterRetryWindow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindConvergenceSession: {
			{objects.FieldKeyID: "CVS-1", objects.FieldKeyStatus: objects.ObjectStatusActive},
		},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_review",
		EnteredAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		PlanID:    "PRI-CAP",
		CvsID:     "CVS-1",
		Attempts:  capStageMaxAttempts + 4,
	})
	h.writeStateFile(capQuarantineFile, map[string]any{
		"stage":                   "cap_stage_review",
		capFieldPlanID:            "PRI-CAP",
		"cvs_id":                  "CVS-1",
		objects.FieldKeyCreatedAt: time.Now().UTC().Add(-capStageQuarantineRetryAfter).Format(time.RFC3339),
	})

	h.markStageEntered("cap_stage_review", "PRI-CAP")

	pending, err := h.readPendingStage()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Attempts != 1 {
		t.Fatalf("attempts = %d, want fresh window at 1", pending.Attempts)
	}
	if _, err := fileutil.Stat(h.capStatePath(capQuarantineFile)); !fileutil.IsNotExist(err) {
		t.Fatalf("quarantine should be cleared, got err=%v", err)
	}
}

func TestMarkStageEnteredResetsQuarantineWhenCVSScopeChanges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindConvergenceSession: {
			{objects.FieldKeyID: "CVS-NEW", objects.FieldKeyStatus: objects.ObjectStatusActive},
		},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_review",
		EnteredAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		PlanID:    "PRI-CAP",
		CvsID:     "CVS-OLD",
		Attempts:  capStageMaxAttempts + 4,
	})
	h.writeStateFile(capQuarantineFile, map[string]any{
		"stage":                   "cap_stage_review",
		capFieldPlanID:            "PRI-CAP",
		"cvs_id":                  "CVS-OLD",
		objects.FieldKeyCreatedAt: time.Now().UTC().Format(time.RFC3339),
	})

	h.markStageEntered("cap_stage_review", "PRI-CAP")

	pending, err := h.readPendingStage()
	if err != nil {
		t.Fatal(err)
	}
	if pending.CvsID != "CVS-NEW" || pending.Attempts != 1 {
		t.Fatalf("pending = %#v, want CVS-NEW with fresh attempt window", pending)
	}
}

func TestMarkStageEnteredPersistsAttemptWatermark(t *testing.T) {
	root := t.TempDir()
	h := NewCapOrchestratorHandler(&mockCapStorage{}, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)

	h.markStageEntered("cap_stage_orchestrating", "PRI-CAP")
	h.markStageEntered("cap_stage_orchestrating", "PRI-CAP")
	pending, err := h.readPendingStage()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Attempts != 2 || pending.LastAttemptAt == "" {
		t.Fatalf("attempt watermark = %#v, want attempts=2 with last_attempt_at", pending)
	}
}

func TestClearCAPStageFailureAttemptsPreservesEntryWatermark(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	h := NewCapOrchestratorHandler(&mockCapStorage{}, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	enteredAt := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:           "cap_stage_review",
		EnteredAt:       enteredAt,
		Attempts:        12,
		FailureAttempts: capStageMaxAttempts,
	})

	h.clearCAPStageFailureAttempts("cap_stage_review")

	pending, err := h.readPendingStage()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Attempts != 12 || pending.FailureAttempts != 0 || pending.EnteredAt != enteredAt {
		t.Fatalf("pending = %#v, want failure streak reset with counters and watermark preserved", pending)
	}
}

func TestCAPStageQuarantineActiveRequiresMatchingScope(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	h := NewCapOrchestratorHandler(&mockCapStorage{}, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	pending := capStagePending{Stage: "cap_stage_review", PlanID: "PRI-CAP", CvsID: "CVS-1"}
	h.writeStateFile(capQuarantineFile, map[string]any{
		"stage":        pending.Stage,
		capFieldPlanID: pending.PlanID,
		"cvs_id":       pending.CvsID,
	})
	if !h.capStageQuarantineActive(pending) {
		t.Fatal("matching quarantine scope should be active")
	}
	pending.CvsID = "CVS-2"
	if h.capStageQuarantineActive(pending) {
		t.Fatal("quarantine from another CVS must not hold the new scope")
	}
}

func TestCapCycleTamperDetected(t *testing.T) {
	root := t.TempDir()
	whatsnext.EnsureCAPStage(root, "cap_stage_grooming")
	_ = whatsnext.AppendCapAdvanceJournal(root, whatsnext.CapAdvanceJournalEntry{
		CompletedStage: "cap_stage_planning",
		NextStage:      "cap_stage_design",
		CvsID:          "CVS-1",
		Source:         "cap_orchestrator",
	})
	whatsnext.EnsureCAPStage(root, "cap_stage_metrics") // tamper
	if !whatsnext.CapCycleTamperDetected(root) {
		t.Fatal("expected tamper")
	}
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindConvergenceSession: {{objects.FieldKeyID: "CVS-1", objects.FieldKeyStatus: objects.ObjectStatusActive}},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{Stage: "cap_stage_metrics", EnteredAt: time.Now().UTC().Format(time.RFC3339), CvsID: "CVS-1"})
	err := h.maybeAdvanceCAPStage(context.Background(), "cap_stage_metrics")
	if err == nil || !strings.Contains(err.Error(), "tamper") {
		t.Fatalf("expected tamper hold, got %v", err)
	}
}

func TestWriteStateFile_UsesCanonicalStateDir(t *testing.T) {
	root := t.TempDir()
	h := NewCapOrchestratorHandler(&mockCapStorage{}, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile("cap_metrics_latest.json", map[string]any{"ok": true})
	want := filepath.Join(root, paths.ProjectDataDir, paths.StateDir, "cap_metrics_latest.json")
	if _, err := fileutil.Stat(want); err != nil {
		t.Fatalf("expected file at %s: %v", want, err)
	}
}

func TestReviewDeliveryComplete_RequiresFreshTimestamp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	h := NewCapOrchestratorHandler(&mockCapStorage{}, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)

	h.writeStateFile(capReviewResultFile, map[string]any{
		"timestamp":        time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339),
		"system_check":     true,
		"scheduler_health": true,
		"tests_passed":     true,
	})
	ok, detail := h.reviewDeliveryComplete()
	if ok {
		t.Fatal("expected stale review to block")
	}
	if detail == "" || !strings.Contains(strings.ToLower(detail), "stale") {
		t.Fatalf("detail=%q", detail)
	}

	h.writeStateFile(capReviewResultFile, map[string]any{
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
		"system_check":     true,
		"scheduler_health": true,
		"tests_passed":     true,
	})
	ok, detail = h.reviewDeliveryComplete()
	if !ok {
		t.Fatalf("expected clear, got %q", detail)
	}
}

func TestReviewResultStaleReason(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	if got := reviewResultStaleReason(map[string]any{}, now, time.Hour); got == "" {
		t.Fatal("expected missing timestamp")
	}
	fresh := map[string]any{"timestamp": now.Add(-30 * time.Minute).Format(time.RFC3339)}
	if got := reviewResultStaleReason(fresh, now, time.Hour); got != "" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestVerifyCriticalPackagesHealth_ZeroJobsSoftCoverageGap(t *testing.T) {
	root := t.TempDir()
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindSchedulerJob: {},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	ok, hard, soft := h.verifyCriticalPackagesHealth(context.Background(), pkgctx.NewSystemSecurityContext())
	if ok {
		t.Fatal("expected readiness fail-closed with zero jobs")
	}
	if len(hard) != 0 {
		t.Fatalf("coverage gaps must be soft (no CAP job hard fail); hard=%v", hard)
	}
	if len(soft) == 0 {
		t.Fatal("expected soft coverage gap detail")
	}
}

func TestVerifyCriticalPackagesHealth_UsesRecentRunsNotNewestCreatedJob(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	lastRun := time.Now().UTC().Add(-time.Minute)
	jobs := []map[string]any{
		{
			objects.FieldKeyID:        "SCH-SCHEDULER-CURRENT",
			objects.FieldKeyJobType:   JobTypeRunWrapper,
			objects.FieldKeyLastRunAt: lastRun.Format(time.RFC3339),
			objects.FieldKeyMetadata: map[string]any{
				KeyTestBundleMetaPackagePath: "pkg/scheduler",
			},
		},
		{
			objects.FieldKeyID:        "SCH-STORAGE-CURRENT",
			objects.FieldKeyJobType:   JobTypeRunWrapper,
			objects.FieldKeyLastRunAt: lastRun.Format(time.RFC3339),
			objects.FieldKeyMetadata: map[string]any{
				KeyTestBundleMetaPackagePath: "pkg/storage",
			},
		},
		{
			objects.FieldKeyID:        "SCH-ARCHIVED-NEVER-RAN",
			objects.FieldKeyJobType:   JobTypeRunWrapper,
			objects.FieldKeyCreatedAt: time.Now().UTC().Format(time.RFC3339),
			objects.FieldKeyMetadata: map[string]any{
				KeyTestBundleMetaPackagePath: "pkg/scheduler/transceiver/adapters",
			},
		},
	}
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindSchedulerJob: jobs,
	}}
	healthPath := TestBundlesHealthFilePath(root)
	if err := fileutil.MkdirAll(filepath.Dir(healthPath), 0o755); err != nil {
		t.Fatal(err)
	}
	var health bytes.Buffer
	encoder := json.NewEncoder(&health)
	for _, jobID := range []string{"SCH-SCHEDULER-CURRENT", "SCH-STORAGE-CURRENT"} {
		if err := encoder.Encode(map[string]any{
			KeyJobID:       jobID,
			KeyTestOutcome: "pass",
			KeyTimestamp:   lastRun.Add(time.Second).Format(time.RFC3339),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := fileutil.WriteFile(healthPath, health.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	ok, hard, soft := h.verifyCriticalPackagesHealth(context.Background(), pkgctx.NewSystemSecurityContext())
	if !ok || len(hard) != 0 || len(soft) != 0 {
		t.Fatalf("ok=%v hard=%v soft=%v", ok, hard, soft)
	}
}

func TestCriticalRootForPackagePath(t *testing.T) {
	t.Parallel()
	if got := criticalRootForPackagePath("pkg/storage"); got != "pkg/storage" {
		t.Fatalf("got %q", got)
	}
	if got := criticalRootForPackagePath("./pkg/scheduler/foo"); got != "pkg/scheduler" {
		t.Fatalf("got %q", got)
	}
	if got := criticalRootForPackagePath("pkg/mcp"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestReviewSystemCheck_PublicBlockersFail(t *testing.T) {
	t.Parallel()
	n, err := parseSystemCheckPublicBlockers([]byte(`{"summary":{"public_blockers":12}}`))
	if err != nil || n != 12 {
		t.Fatalf("got %d %v", n, err)
	}
	n, err = parseSystemCheckPublicBlockers([]byte(`{"system_check":true}`))
	if err != nil || n != 0 {
		t.Fatalf("got %d %v", n, err)
	}
}

func TestResolveBoundCVS_DoesNotCacheFromDisk(t *testing.T) {
	root := t.TempDir()

	h := NewCapOrchestratorHandler(nil, root, logging.GetLoggerFromProfile("system")).(*CapOrchestratorHandler)
	h.writeStateFile("cap_stage_pending.json", map[string]any{
		"stage":  "cap_stage_review",
		"cvs_id": "CVS-OLD",
	})

	mStorage := &mockCapStorage{
		listed: map[string][]map[string]any{
			objects.KindConvergenceSession: {
				{
					objects.FieldKeyID:                "CVS-NEW",
					objects.FieldKeyStatus:            objects.ObjectStatusActive,
					objects.FieldKeyRelatedObjectRefs: []any{"PRI-123"},
				},
			},
		},
	}
	h.storage = mStorage

	cvsID, _ := h.resolveBoundCVS(context.Background(), "PRI-123")
	if cvsID != "CVS-NEW" {
		t.Fatalf("expected CVS-NEW, got %q (cached from disk?)", cvsID)
	}
}

func TestRefreshPendingBoundCVS_RebindsTerminal(t *testing.T) {
	root := t.TempDir()
	h := NewCapOrchestratorHandler(nil, root, logging.GetLoggerFromProfile("system")).(*CapOrchestratorHandler)
	h.storage = &mockCapStorage{
		byID: map[string]map[string]any{
			"CVS-OLD": {
				objects.FieldKeyID:     "CVS-OLD",
				objects.FieldKeyStatus: objects.ObjectStatusCompleted,
			},
			"CVS-NEW": {
				objects.FieldKeyID:     "CVS-NEW",
				objects.FieldKeyStatus: objects.ObjectStatusActive,
			},
			"PRI-123": {
				objects.FieldKeyID:                "PRI-123",
				objects.FieldKeyRelatedObjectRefs: []any{"CVS-NEW"},
			},
		},
		listed: map[string][]map[string]any{
			objects.KindConvergenceSession: {
				{
					objects.FieldKeyID:                "CVS-NEW",
					objects.FieldKeyStatus:            objects.ObjectStatusActive,
					objects.FieldKeyRelatedObjectRefs: []any{"PRI-123"},
				},
			},
		},
	}
	pending := capStagePending{
		Stage:  "cap_stage_grooming",
		PlanID: "PRI-123",
		CvsID:  "CVS-OLD",
	}
	if !h.refreshPendingBoundCVS(context.Background(), &pending) {
		t.Fatal("expected rewrite")
	}
	if pending.CvsID != "CVS-NEW" {
		t.Fatalf("got %q", pending.CvsID)
	}
}

func TestResolveBoundCVS_SkipsTerminalPlanRef(t *testing.T) {
	root := t.TempDir()
	h := NewCapOrchestratorHandler(nil, root, logging.GetLoggerFromProfile("system")).(*CapOrchestratorHandler)
	h.storage = &mockCapStorage{
		byID: map[string]map[string]any{
			"CVS-DEAD": {
				objects.FieldKeyID:     "CVS-DEAD",
				objects.FieldKeyStatus: objects.ObjectStatusCompleted,
			},
			"CVS-LIVE": {
				objects.FieldKeyID:     "CVS-LIVE",
				objects.FieldKeyStatus: objects.ObjectStatusActive,
			},
			"PRI-123": {
				objects.FieldKeyID:                "PRI-123",
				objects.FieldKeyRelatedObjectRefs: []any{"CVS-DEAD", "CVS-LIVE"},
			},
		},
	}
	cvsID, _ := h.resolveBoundCVS(context.Background(), "PRI-123")
	if cvsID != "CVS-LIVE" {
		t.Fatalf("expected CVS-LIVE, got %q", cvsID)
	}
}
// tdd refresh
