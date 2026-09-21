package whatsnext

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
)

// Fill kinds compiled from cached kernel ambience. Idle seats do this
// instead of reminting ORCHESTRATE_PLAN or emitting shutdown.
const (
	FillKindGhostRef          = "ghost_ref"
	FillKindCheckCache        = "check_cache_missing"
	FillKindMetrics           = "metrics"
	FillKindDraftPlane        = "draft_plane"
	FillKindStaleTestCatalyst = "stale_test_catalyst"
	FillKindStaleAgentTask    = "stale_agent_task"
)

const (
	fillSubmitAutofixDangling = "system check autofix dangling"
	fillSubmitCheckCacheFmt   = "system check --format json -o %s"
	fillHintDraftSweep        = "object draft sweep --dry-run"
	fillHintDraftPromote      = "object draft promote --dry-run --all"
	fillLeaseSeconds          = 2700 // 45m, matches seat-worker orch/fill cooldown
)

// Fill commands are cheap or scheduler-shaped. Do not put a foreground
// `system check --details` here — that is the idle-tick thread bomb.
// Draft plane is a location (object_drafts under the project data dir), not lifecycle status=draft.
// TRACK: follow-up in kernel backlog

func productCLI() string {
	return paths.CLIName()
}

func systemCheckCacheRelPath() string {
	return filepath.ToSlash(filepath.Join(paths.ProjectDataDir, paths.LogsDir, "system-check.json"))
}

// FillCmdAutofixDangling is the operator-visible hint for GhostRef repair.
func FillCmdAutofixDangling() string {
	return productCLI() + " " + fillSubmitAutofixDangling
}

// FillCmdRefreshCheckCache is the operator-visible hint for a missing system-check cache.
func FillCmdRefreshCheckCache() string {
	return productCLI() + " " + fmt.Sprintf(fillSubmitCheckCacheFmt, systemCheckCacheRelPath())
}

// FillCmdDraftSweepDryRun is the operator-visible TPM draft-plane hint.
func FillCmdDraftSweepDryRun() string {
	return productCLI() + " " + fillHintDraftSweep
}

// FillCmdDraftPromoteDryRun is the operator-visible peer draft-plane hint.
func FillCmdDraftPromoteDryRun() string {
	return productCLI() + " " + fillHintDraftPromote
}

// FillItem is one claimable kernel job when the lead has no live ATK.
// TRACK: BLI-COMMS-TPM-DUTY-ORCHESTRATE-001
type FillItem struct {
	Kind         string `json:"kind"`
	Reason       string `json:"reason"`
	CommandHint  string `json:"command_hint"`
	LeaseSeconds int    `json:"lease_seconds,omitempty"`
	// AutoSubmit means a seat-worker may scheduler-submit SubmitArgs (no kind switch).
	AutoSubmit bool `json:"auto_submit,omitempty"`
	// SubmitArgs is the product-CLI subcommand line (no executable prefix).
	SubmitArgs string `json:"submit_args,omitempty"`
}

// CompileFillItem picks one ranked fill from cached ambience.
// Does not run system check. Nil when there is nothing to fish.
func CompileFillItem(amb *KernelAmbience) *FillItem {
	if amb == nil {
		return nil
	}
	if amb.GhostRefCount > 0 {
		return &FillItem{
			Kind:         FillKindGhostRef,
			Reason:       fmt.Sprintf("%d GhostRefs", amb.GhostRefCount),
			CommandHint:  FillCmdAutofixDangling(),
			LeaseSeconds: fillLeaseSeconds,
			AutoSubmit:   true,
			SubmitArgs:   fillSubmitAutofixDangling,
		}
	}
	if amb.StaleAgentTask != nil && amb.StaleAgentTask.TaskID != "" {
		return &FillItem{
			Kind:         FillKindStaleAgentTask,
			Reason:       fmt.Sprintf("stale in_progress agent_task %s: %s", amb.StaleAgentTask.TaskID, amb.StaleAgentTask.CommandHint),
			CommandHint:  amb.StaleAgentTask.CommandHint,
			LeaseSeconds: fillLeaseSeconds,
		}
	}
	if amb.StaleTestCatalyst != nil && amb.StaleTestCatalyst.TestCaseID != "" {
		return &FillItem{
			Kind:         FillKindStaleTestCatalyst,
			Reason:       fmt.Sprintf("stale in_progress BLI %s: unblock via test catalyst", amb.StaleTestCatalyst.BacklogItemID),
			CommandHint:  amb.StaleTestCatalyst.CommandHint,
			LeaseSeconds: fillLeaseSeconds,
		}
	}
	if !amb.Available {
		args := fmt.Sprintf(fillSubmitCheckCacheFmt, systemCheckCacheRelPath())
		return &FillItem{
			Kind:         FillKindCheckCache,
			Reason:       "no system-check cache",
			CommandHint:  FillCmdRefreshCheckCache(),
			LeaseSeconds: fillLeaseSeconds,
			AutoSubmit:   true,
			SubmitArgs:   args,
		}
	}
	if amb.MetricsRollup != nil {
		cmd := strings.TrimSpace(amb.MetricsRollup.NextAdminAction)
		if looksLikeProductCLI(cmd) {
			return &FillItem{
				Kind:         FillKindMetrics,
				Reason:       "metrics rollup next_admin_action",
				CommandHint:  cmd,
				LeaseSeconds: fillLeaseSeconds,
			}
		}
	}
	if amb.DraftPlaneTotal > 0 {
		cmd := FillCmdDraftPromoteDryRun()
		if amb.SeatMode != SeatModePeerExecution {
			cmd = FillCmdDraftSweepDryRun()
		}
		return &FillItem{
			Kind:         FillKindDraftPlane,
			Reason:       fmt.Sprintf("draft plane %d", amb.DraftPlaneTotal),
			CommandHint:  cmd,
			LeaseSeconds: fillLeaseSeconds,
		}
	}
	return nil
}

func looksLikeProductCLI(cmd string) bool {
	for _, name := range []string{brand.ExecutableName(), brand.CanonicalExecutableToken} {
		if name != "" && strings.HasPrefix(cmd, name+" ") {
			return true
		}
	}
	return false
}

// ApplyFillToInstruction keeps seats working when kernel fill exists.
func ApplyFillToInstruction(instruction string, fill *FillItem) string {
	if fill == nil || fill.CommandHint == "" {
		return instruction
	}
	if instruction == "shutdown" {
		return "continue"
	}
	return instruction
}
