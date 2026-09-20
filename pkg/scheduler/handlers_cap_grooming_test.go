package scheduler

import (
	"context"
	"os"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestExecuteGroomingStage_PlannedZeroStopsAfterMaxTicks(t *testing.T) {
	root := t.TempDir()
	mStorage := &mockCapStorage{
		created: make([]map[string]any, 0),
		listed: map[string][]map[string]any{
			objects.KindAgentInstruction: {},
		},
	}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	ctx := context.Background()
	planID := "PRI-TEST-GROOM"
	counts := map[string]int{objects.ObjectStatusPlanned: 0, objects.ObjectStatusComplete: 2}

	for i := 1; i <= capGroomingPlannedZeroMaxTicks; i++ {
		h.markStageEntered("cap_stage_grooming", planID)
		if err := h.executeGroomingStage(ctx, "zqk", planID, 0, "Test plan", objects.ObjectStatusActive, counts); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
		if n := len(mStorage.getCreated()); n != i {
			t.Fatalf("tick %d: created=%d want %d", i, n, i)
		}
	}

	h.markStageEntered("cap_stage_grooming", planID)
	err := h.executeGroomingStage(ctx, "zqk", "PRI-TEST-GROOM", 0, "Test plan", objects.ObjectStatusActive, counts)
	if err == nil || !strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("exhaust tick expected exhausted error, got: %v", err)
	}
	if n := len(mStorage.getCreated()); n != capGroomingPlannedZeroMaxTicks {
		t.Fatalf("after exhaust created=%d want %d (must not dispatch another no-op)", n, capGroomingPlannedZeroMaxTicks)
	}
	if _, err := os.Stat(h.capStatePath(capGroomingPlannedZeroFile)); err != nil {
		t.Fatalf("expected packaging_cue latch file: %v", err)
	}
}

func TestExecuteGroomingStage_PlannedFuelClearsLatchAndDispatches(t *testing.T) {
	root := t.TempDir()
	mStorage := &mockCapStorage{
		created: make([]map[string]any, 0),
		listed: map[string][]map[string]any{
			objects.KindAgentInstruction: {},
		},
	}
	h := NewCapOrchestratorHandler(mStorage, root, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).(*CapOrchestratorHandler)
	h.writeStateFile(capGroomingPlannedZeroFile, map[string]any{objects.FieldKeyStatus: "exhausted"})
	h.markStageEntered("cap_stage_grooming", "PRI-TEST-GROOM")
	counts := map[string]int{objects.ObjectStatusPlanned: 2}
	if err := h.executeGroomingStage(context.Background(), "zqk", "PRI-TEST-GROOM", 2, "Test plan", objects.ObjectStatusActive, counts); err != nil {
		t.Fatalf("dispatch with planned fuel: %v", err)
	}
	if n := len(mStorage.getCreated()); n != 1 {
		t.Fatalf("created=%d want 1", n)
	}
	if _, err := os.Stat(h.capStatePath(capGroomingPlannedZeroFile)); !os.IsNotExist(err) {
		t.Fatalf("latch should be cleared when planned>0, stat err=%v", err)
	}
}
