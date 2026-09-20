package whatsnext

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// PriorityPlanPackagingCue returns a PRI≈PR wrap hint when an execution-facing
// priority_plan has finished children and no open work. Empty when the plan is
// mid-execution (open children) or not wrap-eligible.
func PriorityPlanPackagingCue(planID, title, planStatus string, counts map[string]int) string {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return ""
	}
	if !planStatusEligibleForPackagingCue(planStatus) {
		return ""
	}
	open, finished, total := summarizePlanChildWork(counts)
	if total == 0 || open > 0 || finished == 0 {
		return ""
	}
	label := strings.TrimSpace(title)
	if label == "" {
		label = planID
	} else {
		label = fmt.Sprintf("%s (%s)", label, planID)
	}
	return fmt.Sprintf(
		"PRI≈PR wrap: %s has no open children — package on a feature branch and open one PR for this priority_plan (POL-WORKFLOW-002).",
		label,
	)
}

func planStatusEligibleForPackagingCue(status string) bool {
	st := strings.ToLower(strings.TrimSpace(status))
	switch st {
	case objects.ObjectStatusActive, objects.ObjectStatusInProgress, objects.ObjectStatusComplete:
		return true
	default:
		return false
	}
}

func summarizePlanChildWork(counts map[string]int) (open, finished, total int) {
	for st, n := range counts {
		if n <= 0 {
			continue
		}
		total += n
		if objects.BacklogCountsAsOpenWork(st) {
			open += n
		} else {
			finished += n
		}
	}
	return open, finished, total
}
