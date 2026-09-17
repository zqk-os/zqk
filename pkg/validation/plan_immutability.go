package validation

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// TRACK: BLI-REDACTED — plan immutability / execution-facing gate for BLI in_progress.

// PlanStatusAllowsBacklogInProgress reports whether a priority_plan status may host
// backlog_item work (shovel-ready active or execution-locked in_progress).
func PlanStatusAllowsBacklogInProgress(planStatus string) bool {
	switch strings.ToLower(strings.TrimSpace(planStatus)) {
	case objects.ObjectStatusActive, objects.ObjectStatusInProgress:
		return true
	default:
		return false
	}
}

// ErrBacklogInProgressRequiresExecutionFacingPlan formats the membership refusal.
func ErrBacklogInProgressRequiresExecutionFacingPlan(bliID, planID, planStatus string) error {
	return fmt.Errorf(
		"item '%s' links to priority plan %s in status %q; in_progress requires plan status active or in_progress",
		bliID, planID, planStatus,
	)
}
