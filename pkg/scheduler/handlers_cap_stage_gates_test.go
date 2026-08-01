package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/workflow/whatsnext"
)

func TestMaybeAdvanceCAPStage_GroomingHoldsWithoutArtifacts(t *testing.T) {
	root := t.TempDir()
	whatsnext.EnsureCAPStage(root, "cap_stage_grooming")

	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindPriorityPlan:  {},
		objects.KindBacklogItem:   {},
		objects.KindStrategicPlan: {},
		objects.KindConvergenceSession: {
			{objects.FieldKeyID: "CONV-1", objects.FieldKeyStatus: "active", objects.FieldKeyRelatedObjectRefs: []string{"PLN-1"}},
		},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_grooming",
		EnteredAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		PlanID:    "PLN-1",
		CvsID:     "CONV-1",
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
				objects.FieldKeyID:        "PLAN-NEW",
				objects.FieldKeyUpdatedAt: now,
			},
		},
		objects.KindBacklogItem:   {},
		objects.KindStrategicPlan: {},
		objects.KindConvergenceSession: {
			{objects.FieldKeyID: "CONV-1", objects.FieldKeyStatus: "active"},
		},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_grooming",
		EnteredAt: entered,
		PlanID:    "PLN-1",
		CvsID:     "CONV-1",
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
		objects.KindConvergenceSession: {{objects.FieldKeyID: "CONV-1", objects.FieldKeyStatus: "active"}},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{
		Stage:     "cap_stage_orchestrating",
		EnteredAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		CvsID:     "CONV-1",
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
		CvsID:     "CONV-1",
	})
	ok, detail := h.dispatchDeliveryComplete(context.Background(), "cap_stage_orchestrating")
	if ok {
		t.Fatalf("open ATK should be insufficient: %s", detail)
	}
}

func TestCapCycleTamperDetected(t *testing.T) {
	root := t.TempDir()
	whatsnext.EnsureCAPStage(root, "cap_stage_grooming")
	_ = whatsnext.AppendCapAdvanceJournal(root, whatsnext.CapAdvanceJournalEntry{
		CompletedStage: "cap_stage_planning",
		NextStage:      "cap_stage_design",
		CvsID:          "CONV-1",
		Source:         "cap_orchestrator",
	})
	whatsnext.EnsureCAPStage(root, "cap_stage_metrics") // tamper
	if !whatsnext.CapCycleTamperDetected(root) {
		t.Fatal("expected tamper")
	}
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindConvergenceSession: {{objects.FieldKeyID: "CONV-1", objects.FieldKeyStatus: "active"}},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capStagePendingFile, capStagePending{Stage: "cap_stage_metrics", EnteredAt: time.Now().UTC().Format(time.RFC3339), CvsID: "CONV-1"})
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
	if _, err := os.Stat(want); err != nil {
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

func TestVerifyCriticalPackagesHealth_ZeroJobsFails(t *testing.T) {
	root := t.TempDir()
	mStorage := &mockCapStorage{listed: map[string][]map[string]any{
		objects.KindSchedulerJob: {},
	}}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	ok, errs := h.verifyCriticalPackagesHealth(context.Background(), pkgctx.NewSystemSecurityContext())
	if ok {
		t.Fatal("expected fail closed with zero jobs")
	}
	if len(errs) == 0 {
		t.Fatal("expected error detail")
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
