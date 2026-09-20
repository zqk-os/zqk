package rollup

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

// Calculate aggregates status counts, effort totals, and distributions from a list of child objects.
// It is completely kind-agnostic and processes any child maps (e.g. backlog_item, agent_task, etc.).
func Calculate(children []map[string]any) RollupSummary {
	var summary RollupSummary
	summary.TotalCount = len(children)
	if summary.TotalCount == 0 {
		summary.EstimatedEffort.Formatted = "0h"
		summary.ActualEffort.Formatted = "0h"
		return summary
	}

	checker := objects.GetGlobalStatusChecker()

	var estValues []float64
	var actValues []float64

	for _, child := range children {
		if child == nil {
			continue
		}

		kind, _ := child[objects.FieldKeyKind].(string)
		status, _ := child[objects.FieldKeyStatus].(string)
		status = strings.TrimSpace(status)

		// Check completion / work_done
		if isStatusComplete(checker, kind, status) {
			summary.CompletedCount++
		} else if isStatusInProgress(checker, kind, status) {
			summary.InProgressCount++
		} else if isStatusHalted(checker, kind, status) {
			summary.HaltedCount++
		}

		// Effort metrics
		if estRaw, ok := child[objects.FieldKeyEstimatedEffort].(string); ok {
			if hours, parsed := validation.ParseEffortHours(estRaw); parsed {
				estValues = append(estValues, hours)
			}
		}
		if actRaw, ok := child[objects.FieldKeyActualEffort].(string); ok {
			if hours, parsed := validation.ParseEffortHours(actRaw); parsed {
				actValues = append(actValues, hours)
			}
		}
	}

	summary.OpenCount = summary.TotalCount - summary.CompletedCount
	if summary.TotalCount > 0 {
		summary.PercentComplete = (float64(summary.CompletedCount) / float64(summary.TotalCount)) * 100.0
	}

	summary.EstimatedEffort = computeEffortStats(estValues)
	summary.ActualEffort = computeEffortStats(actValues)

	return summary
}

func computeEffortStats(values []float64) EffortStats {
	if len(values) == 0 {
		return EffortStats{
			TotalHours: 0,
			AvgHours:   0,
			MinHours:   0,
			MaxHours:   0,
			Formatted:  "0h",
		}
	}

	var sum float64
	min := values[0]
	max := values[0]

	for _, v := range values {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	avg := sum / float64(len(values))
	formatted := validation.FormatEffortHours(sum)

	return EffortStats{
		TotalHours: sum,
		AvgHours:   avg,
		MinHours:   min,
		MaxHours:   max,
		Formatted:  formatted,
	}
}

func isStatusComplete(checker objects.IStatusChecker, kind, status string) bool {
	if status == "" {
		return false
	}
	if checker != nil && checker.IsWorkDone(kind, status) {
		return true
	}
	switch status {
	case objects.ObjectStatusComplete, objects.ObjectStatusCompleted, objects.ObjectStatusValidated:
		return true
	default:
		return false
	}
}

func isStatusInProgress(checker objects.IStatusChecker, kind, status string) bool {
	if status == "" {
		return false
	}
	if checker != nil && checker.Role(kind, status) == objects.LifecycleRoleExecutionLocked {
		return true
	}
	return status == objects.ObjectStatusInProgress
}

func isStatusHalted(checker objects.IStatusChecker, kind, status string) bool {
	if status == "" {
		return false
	}
	if checker != nil && checker.Role(kind, status) == objects.LifecycleRoleHalted {
		return true
	}
	switch status {
	case objects.ObjectStatusBlocked, objects.ObjectStatusPaused:
		return true
	default:
		return false
	}
}
