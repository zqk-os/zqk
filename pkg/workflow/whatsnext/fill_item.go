package whatsnext

import (
	"fmt"
	"strings"
)

// Fill kinds compiled from cached kernel ambience. Idle seats do this
// instead of reminting ORCHESTRATE_PLAN or emitting shutdown.
const (
	FillKindGhostRef   = "ghost_ref"
	FillKindCheckCache = "check_cache_missing"
	FillKindMetrics    = "metrics"
	FillKindDraftPlane = "draft_plane"
)

// Fill commands are cheap or scheduler-shaped. Do not put a foreground
// `zqk system check --details` here — that is the idle-tick thread bomb.
const (
	FillCmdAutofixDangling   = "zqk system check autofix dangling"
	FillCmdRefreshCheckCache = "zqk scheduler submit \"zqk system check --format json -o .zqk/logs/system-check.json\" --title \"refresh system-check cache\""
	FillCmdDraftSweepDryRun  = "zqk object draft sweep --dry-run"
	FillCmdDraftList         = "zqk object list --filter status=draft"
)

// FillItem is one claimable kernel job when the lead has no live ATK.
// TRACK: BLI-COMMS-TPM-DUTY-ORCHESTRATE-001
type FillItem struct {
	Kind         string `json:"kind"`
	Reason       string `json:"reason"`
	CommandHint  string `json:"command_hint"`
	LeaseSeconds int    `json:"lease_seconds,omitempty"`
}

const fillLeaseSeconds = 2700 // 45m, matches seat-worker orch/fill cooldown

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
			CommandHint:  FillCmdAutofixDangling,
			LeaseSeconds: fillLeaseSeconds,
		}
	}
	if !amb.Available {
		return &FillItem{
			Kind:         FillKindCheckCache,
			Reason:       "no system-check cache",
			CommandHint:  FillCmdRefreshCheckCache,
			LeaseSeconds: fillLeaseSeconds,
		}
	}
	if amb.MetricsRollup != nil {
		cmd := strings.TrimSpace(amb.MetricsRollup.NextAdminAction)
		if strings.HasPrefix(cmd, "zqk ") {
			return &FillItem{
				Kind:         FillKindMetrics,
				Reason:       "metrics rollup next_admin_action",
				CommandHint:  cmd,
				LeaseSeconds: fillLeaseSeconds,
			}
		}
	}
	if amb.DraftPlaneTotal > 0 {
		cmd := FillCmdDraftList
		if amb.SeatMode != SeatModePeerExecution {
			cmd = FillCmdDraftSweepDryRun
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
