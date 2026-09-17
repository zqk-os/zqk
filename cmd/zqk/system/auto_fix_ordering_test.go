package system

import (
	"fmt"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestSortIssuesByDependency(t *testing.T) {
	t.Parallel()
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	tests := []struct {
		name     string
		issues   []Issue
		expected []Issue // Expected order (by message field name)
	}{
		{
			name: "goal_refs before milestone_refs before backlog_item_refs",
			issues: []Issue{
				{
					Tier:       1,
					Category:   "instance_validation",
					Message:    "milestone_refs: Precondition not met",
					FixCommand: fmt.Sprintf("%s object update BLI-001 --field milestone_refs+=<MILESTONE_ID:...>", cliCmd),
				},
				{
					Tier:       1,
					Category:   "instance_validation",
					Message:    "goal_refs: Precondition not met",
					FixCommand: fmt.Sprintf("%s object update BLI-001 --field goal_refs+=<GOAL_ID:...>", cliCmd),
				},
				{
					Tier:       1,
					Category:   "instance_validation",
					Message:    "backlog_item_refs: Precondition not met",
					FixCommand: fmt.Sprintf("%s object update MIL-001 --field backlog_item_refs+=<BACKLOG_ITEM_ID:...>", cliCmd),
				},
			},
			expected: []Issue{
				{Message: "goal_refs: Precondition not met"},         // Highest priority (4)
				{Message: "milestone_refs: Precondition not met"},    // Medium priority (2)
				{Message: "backlog_item_refs: Precondition not met"}, // Lowest priority (1)
			},
		},
		{
			name: "priority_plan_ref before milestone_refs",
			issues: []Issue{
				{
					Tier:       1,
					Category:   "instance_validation",
					Message:    "milestone_refs: Precondition not met",
					FixCommand: fmt.Sprintf("%s object update BLI-001 --field milestone_refs+=<MILESTONE_ID:...>", cliCmd),
				},
				{
					Tier:       1,
					Category:   "instance_validation",
					Message:    "priority_plan_ref: Precondition not met",
					FixCommand: fmt.Sprintf("%s object update BLI-001 --field priority_plan_ref=<PRIORITY_PLAN_ID:...>", cliCmd),
				},
			},
			expected: []Issue{
				{Message: "priority_plan_ref: Precondition not met"}, // Higher priority (3)
				{Message: "milestone_refs: Precondition not met"},    // Lower priority (2)
			},
		},
		{
			name: "same dependency level: sort by Tier",
			issues: []Issue{
				{
					Tier:       2,
					Category:   "instance_validation",
					Message:    "milestone_refs: Warning",
					FixCommand: fmt.Sprintf("%s object update BLI-001 --field milestone_refs+=<MILESTONE_ID:...>", cliCmd),
				},
				{
					Tier:       1,
					Category:   "instance_validation",
					Message:    "milestone_refs: Error",
					FixCommand: fmt.Sprintf("%s object update BLI-001 --field milestone_refs+=<MILESTONE_ID:...>", cliCmd),
				},
			},
			expected: []Issue{
				{Tier: 1, Message: "milestone_refs: Error"},   // Tier 1 first
				{Tier: 2, Message: "milestone_refs: Warning"}, // Tier 2 second
			},
		},
		{
			name: "unknown fields maintain relative order",
			issues: []Issue{
				{
					Tier:     1,
					Category: "instance_validation",
					Message:  "unknown_field: Error",
				},
				{
					Tier:       1,
					Category:   "instance_validation",
					Message:    "goal_refs: Precondition not met",
					FixCommand: fmt.Sprintf("%s object update BLI-001 --field goal_refs+=<GOAL_ID:...>", cliCmd),
				},
			},
			expected: []Issue{
				{Message: "goal_refs: Precondition not met"}, // Known field with priority
				{Message: "unknown_field: Error"},            // Unknown field (priority 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sorted := sortIssuesByDependency(tt.issues)

			// Verify order by comparing messages
			if len(sorted) != len(tt.expected) {
				t.Fatalf("Expected %d issues, got %d", len(tt.expected), len(sorted))
			}

			for i, expected := range tt.expected {
				if sorted[i].Message != expected.Message {
					t.Errorf("Issue %d: expected message %q, got %q", i, expected.Message, sorted[i].Message)
				}
				if expected.Tier != 0 && sorted[i].Tier != expected.Tier {
					t.Errorf("Issue %d: expected Tier %d, got %d", i, expected.Tier, sorted[i].Tier)
				}
			}
		})
	}
}

func TestGetFixDependencyPriority(t *testing.T) {
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	tests := []struct {
		name     string
		issue    Issue
		expected int
	}{
		{
			name: "goal_refs has highest priority",
			issue: Issue{
				Message: "goal_refs: Precondition not met",
			},
			expected: 4,
		},
		{
			name: "priority_plan_ref has high priority",
			issue: Issue{
				Message: "priority_plan_ref: Precondition not met",
			},
			expected: 3,
		},
		{
			name: "milestone_refs has medium priority",
			issue: Issue{
				Message: "milestone_refs: Precondition not met",
			},
			expected: 2,
		},
		{
			name: "backlog_item_refs has low priority",
			issue: Issue{
				Message: "backlog_item_refs: Precondition not met",
			},
			expected: 1,
		},
		{
			name: "extract from fix command",
			issue: Issue{
				Message:    "status: Precondition not met",
				FixCommand: fmt.Sprintf("%s object update BLI-001 --field goal_refs+=<GOAL_ID:...>", cliCmd),
			},
			expected: 4,
		},
		{
			name: "unknown field returns 0",
			issue: Issue{
				Message: "unknown_field: Error",
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priority := getFixDependencyPriority(tt.issue)
			if priority != tt.expected {
				t.Errorf("Expected priority %d, got %d", tt.expected, priority)
			}
		})
	}
}

func TestSortIssuesByDependency_StableSort(t *testing.T) {
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	// Test that sorting is stable (same priority issues maintain relative order)
	issues := []Issue{
		{
			Tier:       1,
			Category:   "instance_validation",
			Message:    "goal_refs: Error 1",
			FixCommand: fmt.Sprintf("%s object update BLI-001 --field goal_refs+=<GOAL_ID:...>", cliCmd),
		},
		{
			Tier:       1,
			Category:   "instance_validation",
			Message:    "goal_refs: Error 2",
			FixCommand: fmt.Sprintf("%s object update BLI-002 --field goal_refs+=<GOAL_ID:...>", cliCmd),
		},
	}

	sorted := sortIssuesByDependency(issues)

	// Both have same priority and tier, should maintain order
	if len(sorted) != 2 {
		t.Fatalf("Expected 2 issues, got %d", len(sorted))
	}

	// Verify they're in the same order (stable sort)
	if sorted[0].Message != issues[0].Message {
		t.Errorf("Expected first issue to maintain order, got %q", sorted[0].Message)
	}
	if sorted[1].Message != issues[1].Message {
		t.Errorf("Expected second issue to maintain order, got %q", sorted[1].Message)
	}
}

func TestSortIssuesByDependency_Integration(t *testing.T) {
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}
	// Integration test: complex scenario with multiple dependency levels and tiers
	issues := []Issue{
		{
			Tier:       2,
			Category:   "instance_validation",
			Message:    "milestone_refs: Warning",
			FixCommand: fmt.Sprintf("%s object update BLI-001 --field milestone_refs+=<MILESTONE_ID:...>", cliCmd),
		},
		{
			Tier:       1,
			Category:   "instance_validation",
			Message:    "goal_refs: Error",
			FixCommand: fmt.Sprintf("%s object update BLI-001 --field goal_refs+=<GOAL_ID:...>", cliCmd),
		},
		{
			Tier:       1,
			Category:   "instance_validation",
			Message:    "priority_plan_ref: Error",
			FixCommand: fmt.Sprintf("%s object update BLI-001 --field priority_plan_ref=<PRIORITY_PLAN_ID:...>", cliCmd),
		},
		{
			Tier:       1,
			Category:   "instance_validation",
			Message:    "backlog_item_refs: Error",
			FixCommand: fmt.Sprintf("%s object update MIL-001 --field backlog_item_refs+=<BACKLOG_ITEM_ID:...>", cliCmd),
		},
	}

	sorted := sortIssuesByDependency(issues)

	// Expected order (dependency priority first, then Tier):
	// 1. goal_refs (priority 4, Tier 1) - highest dependency priority
	// 2. priority_plan_ref (priority 3, Tier 1) - second highest dependency priority
	// 3. milestone_refs (priority 2, Tier 2) - medium dependency priority (priority 2 > priority 1)
	// 4. backlog_item_refs (priority 1, Tier 1) - lowest dependency priority

	expectedOrder := []string{
		"goal_refs: Error",         // Priority 4, Tier 1 (highest dependency priority)
		"priority_plan_ref: Error", // Priority 3, Tier 1 (second highest dependency priority)
		"milestone_refs: Warning",  // Priority 2, Tier 2 (medium dependency priority)
		"backlog_item_refs: Error", // Priority 1, Tier 1 (lowest dependency priority)
	}

	if len(sorted) != len(expectedOrder) {
		t.Fatalf("Expected %d issues, got %d", len(expectedOrder), len(sorted))
	}

	for i, expectedMsg := range expectedOrder {
		if sorted[i].Message != expectedMsg {
			t.Errorf("Issue %d: expected %q, got %q", i, expectedMsg, sorted[i].Message)
		}
	}
}
