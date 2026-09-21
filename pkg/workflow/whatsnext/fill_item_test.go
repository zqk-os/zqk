package whatsnext

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
)

func TestCompileFillItem_rank(t *testing.T) {
	t.Parallel()
	if got := CompileFillItem(nil); got != nil {
		t.Fatalf("nil ambience: %+v", got)
	}
	ghost := CompileFillItem(&KernelAmbience{GhostRefCount: 3, Available: false, DraftPlaneTotal: 9})
	if ghost == nil || ghost.Kind != FillKindGhostRef || ghost.CommandHint != FillCmdAutofixDangling() || !ghost.AutoSubmit {
		t.Fatalf("ghost must win: %+v", ghost)
	}
	if ghost.SubmitArgs != fillSubmitAutofixDangling {
		t.Fatalf("ghost submit args: %q", ghost.SubmitArgs)
	}
	cache := CompileFillItem(&KernelAmbience{Available: false, DraftPlaneTotal: 2})
	if cache == nil || cache.Kind != FillKindCheckCache || !cache.AutoSubmit {
		t.Fatalf("missing cache: %+v", cache)
	}
	if !strings.Contains(cache.SubmitArgs, paths.ProjectDataDir) {
		t.Fatalf("check-cache submit must use ProjectDataDir, got %q", cache.SubmitArgs)
	}
	metrics := CompileFillItem(&KernelAmbience{
		Available: true,
		MetricsRollup: &MetricsRollupSnapshot{
			NextAdminAction: "zqk scheduler test-failures",
		},
		DraftPlaneTotal: 1,
	})
	if metrics == nil || metrics.Kind != FillKindMetrics || metrics.CommandHint != "zqk scheduler test-failures" {
		t.Fatalf("metrics zqk command: %+v", metrics)
	}
	prose := CompileFillItem(&KernelAmbience{
		Available: true,
		MetricsRollup: &MetricsRollupSnapshot{
			NextAdminAction: "triage blocking issues",
		},
	})
	if prose != nil {
		t.Fatalf("prose next_admin_action must not be a fill command: %+v", prose)
	}
	draftTPM := CompileFillItem(&KernelAmbience{Available: true, DraftPlaneTotal: 4})
	if draftTPM == nil || draftTPM.CommandHint != FillCmdDraftSweepDryRun() || draftTPM.AutoSubmit {
		t.Fatalf("TPM draft: %+v", draftTPM)
	}
	draftPeer := CompileFillItem(&KernelAmbience{
		Available:       true,
		DraftPlaneTotal: 4,
		SeatMode:        SeatModePeerExecution,
	})
	if draftPeer == nil || draftPeer.CommandHint != FillCmdDraftPromoteDryRun() || draftPeer.AutoSubmit {
		t.Fatalf("peer draft: %+v", draftPeer)
	}
}

func TestApplyFillToInstruction(t *testing.T) {
	t.Parallel()
	fill := &FillItem{Kind: FillKindGhostRef, CommandHint: FillCmdAutofixDangling()}
	if got := ApplyFillToInstruction("shutdown", fill); got != "continue" {
		t.Fatalf("got %q", got)
	}
	if got := ApplyFillToInstruction("continue", fill); got != "continue" {
		t.Fatalf("got %q", got)
	}
	if got := ApplyFillToInstruction("shutdown", nil); got != "shutdown" {
		t.Fatalf("got %q", got)
	}
}
