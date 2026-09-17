package scheduler

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/logging"
)

// TriggerValidationResult represents the result of trigger validation
type TriggerValidationResult struct {
	Valid   bool
	Reason  string
	Trigger string // Normalized trigger type
}

// ParseTriggerType parses and normalizes a trigger type
func ParseTriggerType(triggerType string, logger logging.Logger, jobID string) string {
	switch triggerType {
	case TriggerTypeTimer, TriggerTypeManual, TriggerTypeImmediate, TriggerTypeWorkflow, TriggerTypeEvent, TriggerTypeLifecycle:
		return triggerType
	case "":
		return "timer" // Default trigger type
	default:
		if logger != nil {
			SchedulerJobManagementLog(logger).Warn(LogEventSchedulerTriggerParseUnknownType).
				JobID(jobID).
				String("trigger_type", triggerType).
				Log()
		}
		return "timer"
	}
}

// ValidateTriggerRequirements validates that a trigger has all required fields
func ValidateTriggerRequirements(
	triggerType string,
	scheduleExpr string,
	workflowRef string,
	eventFilter string,
	lifecycleFilter string,
	logger logging.Logger,
	jobID string,
	metrics SchedulerMetricsCollector,
) TriggerValidationResult {
	valid := true
	reason := ""

	switch triggerType {
	case "timer":
		if scheduleExpr == emptyValue {
			valid = false
			reason = "timer trigger requires schedule_expression"
			if logger != nil {
				logging.Fluent(logger).Warn(reason).WithFields(jobLogFieldsByID(jobID)...).Log()
			}
		}
	case TriggerTypeWorkflow:
		if workflowRef == emptyValue {
			valid = false
			reason = "workflow trigger requires workflow_ref"
			if logger != nil {
				logging.Fluent(logger).Warn(reason).WithFields(jobLogFieldsByID(jobID)...).Log()
			}
		}
	case "event":
		if eventFilter == emptyValue {
			valid = false
			reason = "event trigger requires event_filter"
			if logger != nil {
				logging.Fluent(logger).Warn(reason).WithFields(jobLogFieldsByID(jobID)...).Log()
			}
		}
	case TriggerTypeLifecycle:
		if lifecycleFilter == emptyValue {
			valid = false
			reason = "lifecycle trigger requires lifecycle_filter"
			if logger != nil {
				logging.Fluent(logger).Warn(reason).WithFields(jobLogFieldsByID(jobID)...).Log()
			}
		}
	case "manual", "immediate":
		// No additional requirements
	default:
		valid = false
		reason = fmt.Sprintf("unknown trigger type: %s", triggerType)
		if logger != nil {
			logging.Fluent(logger).Warn(reason).WithFields(jobLogFieldsByID(jobID)...).Log()
		}
	}

	// Record metrics
	if metrics != nil {
		metrics.RecordTriggerValidation(jobID, triggerType, valid, reason)
	}

	return TriggerValidationResult{
		Valid:   valid,
		Reason:  reason,
		Trigger: triggerType,
	}
}

// containsIgnoreCase checks if a string contains a substring (case-insensitive)
//
//nolint:unused // Helper function - reserved for future use
func containsIgnoreCase(s, substr string) bool {
	sLower := strings.ToLower(s)
	substrLower := strings.ToLower(substr)
	return strings.Contains(sLower, substrLower)
}
