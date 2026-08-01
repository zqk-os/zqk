package scheduler

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

func TestParseTriggerType(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	tests := []struct {
		name     string
		input    string
		expected string
		jobID    string
	}{
		{
			name:     "Valid timer trigger",
			input:    "timer",
			expected: "timer",
			jobID:    "JOB-001",
		},
		{
			name:     "Valid manual trigger",
			input:    "manual",
			expected: "manual",
			jobID:    "JOB-002",
		},
		{
			name:     "Empty string defaults to timer",
			input:    "",
			expected: "timer",
			jobID:    "JOB-003",
		},
		{
			name:     "Unknown trigger defaults to timer",
			input:    "unknown",
			expected: "timer",
			jobID:    "JOB-004",
		},
		{
			name:     "Valid lifecycle trigger",
			input:    "lifecycle",
			expected: "lifecycle",
			jobID:    "JOB-005",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseTriggerType(tt.input, logger, tt.jobID)
			if result != tt.expected {
				t.Errorf("ParseTriggerType(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateTriggerRequirements(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	tests := []struct {
		name            string
		triggerType     string
		scheduleExpr    string
		workflowRef     string
		eventFilter     string
		lifecycleFilter string
		expectedValid   bool
		jobID           string
	}{
		{
			name:          "Timer with schedule expression",
			triggerType:   "timer",
			scheduleExpr:  "0 */6 * * *",
			expectedValid: true,
			jobID:         "JOB-001",
		},
		{
			name:          "Timer without schedule expression",
			triggerType:   "timer",
			scheduleExpr:  "",
			expectedValid: false,
			jobID:         "JOB-002",
		},
		{
			name:          "Workflow with workflow_ref",
			triggerType:   "workflow",
			workflowRef:   "WF-001",
			expectedValid: true,
			jobID:         "JOB-003",
		},
		{
			name:          "Workflow without workflow_ref",
			triggerType:   "workflow",
			workflowRef:   "",
			expectedValid: false,
			jobID:         "JOB-004",
		},
		{
			name:          "Event with event_filter",
			triggerType:   "event",
			eventFilter:   "object_created:backlog_item",
			expectedValid: true,
			jobID:         "JOB-005",
		},
		{
			name:          "Event without event_filter",
			triggerType:   "event",
			eventFilter:   "",
			expectedValid: false,
			jobID:         "JOB-006",
		},
		{
			name:            "Lifecycle with lifecycle_filter",
			triggerType:     "lifecycle",
			lifecycleFilter: "backlog_item:*->in_progress",
			expectedValid:   true,
			jobID:           "JOB-007",
		},
		{
			name:            "Lifecycle without lifecycle_filter",
			triggerType:     "lifecycle",
			lifecycleFilter: "",
			expectedValid:   false,
			jobID:           "JOB-008",
		},
		{
			name:          "Manual trigger (no requirements)",
			triggerType:   "manual",
			expectedValid: true,
			jobID:         "JOB-009",
		},
		{
			name:          "Immediate trigger (no requirements)",
			triggerType:   "immediate",
			expectedValid: true,
			jobID:         "JOB-010",
		},
		{
			name:          "Unknown trigger type",
			triggerType:   "unknown",
			expectedValid: false,
			jobID:         "JOB-011",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateTriggerRequirements(
				tt.triggerType,
				tt.scheduleExpr,
				tt.workflowRef,
				tt.eventFilter,
				tt.lifecycleFilter,
				logger,
				tt.jobID,
				nil, // No metrics for unit test
			)
			if result.Valid != tt.expectedValid {
				t.Errorf("ValidateTriggerRequirements() = %v, expected %v (reason: %s)",
					result.Valid, tt.expectedValid, result.Reason)
			}
		})
	}
}
