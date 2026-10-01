package autofix

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zqk-os/zqk/pkg/systemcheck"
)

func TestGetFixDependencyPriority(t *testing.T) {
	tests := []struct {
		name     string
		issue    systemcheck.Issue
		expected int
	}{
		{
			name: "goal_refs in message",
			issue: systemcheck.Issue{
				Message: "goal_refs: missing reference",
			},
			expected: 4,
		},
		{
			name: "priority_plan_ref in message",
			issue: systemcheck.Issue{
				Message: "priority_plan_ref: missing reference",
			},
			expected: 3,
		},
		{
			name: "milestone_refs in message",
			issue: systemcheck.Issue{
				Message: "milestone_refs: missing reference",
			},
			expected: 2,
		},
		{
			name: "backlog_item_refs in message",
			issue: systemcheck.Issue{
				Message: "backlog_item_refs: missing reference",
			},
			expected: 1,
		},
		{
			name: "field in fix command with colon message",
			issue: systemcheck.Issue{
				Message:    "instance_validation: precondition failed",
				FixCommand: "zqk object update BLI-1 --field goal_refs+=GOAL-1",
			},
			expected: 4,
		},
		{
			name: "unknown field",
			issue: systemcheck.Issue{
				Message: "description: must be non-empty",
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priority := GetFixDependencyPriority(tt.issue)
			assert.Equal(t, tt.expected, priority)
		})
	}
}

func TestSortIssuesByDependency(t *testing.T) {
	issues := []systemcheck.Issue{
		{
			Tier:    2,
			Message: "backlog_item_refs: issue A", // Priority 1, Tier 2
		},
		{
			Tier:    1,
			Message: "backlog_item_refs: issue B", // Priority 1, Tier 1
		},
		{
			Tier:    2,
			Message: "goal_refs: issue C", // Priority 4, Tier 2
		},
		{
			Tier:    1,
			Message: "milestone_refs: issue D", // Priority 2, Tier 1
		},
	}

	sorted := SortIssuesByDependency(issues)
	assert.Equal(t, "goal_refs: issue C", sorted[0].Message)
	assert.Equal(t, "milestone_refs: issue D", sorted[1].Message)
	assert.Equal(t, "backlog_item_refs: issue B", sorted[2].Message)
	assert.Equal(t, "backlog_item_refs: issue A", sorted[3].Message)
}
