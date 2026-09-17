package system

import (
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// fixDependencyOrder defines the dependency hierarchy for reference fields
// Higher values = fewer dependencies (fix first)
// Lower values = more dependencies (fix later)
var fixDependencyOrder = map[string]int{
	objects.FieldKeyGoalRefs:        4, // Goals are top-level (fewest dependencies)
	objects.FieldKeyPriorityPlanRef: 3, // Priority plans are high-level
	objects.FieldKeyMilestoneRefs:   2, // Milestones depend on goals/priority plans
	objects.FieldKeyWorkstreamRefs:  2, // Workstreams are similar to milestones
	objects.FieldKeyBacklogItemRefs: 1, // Backlog items depend on milestones/goals
}

// getFixDependencyPriority returns the dependency priority for an issue
// Higher values = fewer dependencies (fix first)
// Returns 0 if the issue doesn't have a known dependency priority
func getFixDependencyPriority(issue Issue) int {
	// Extract field name from message (format: "field_name: error message")
	parts := strings.SplitN(issue.Message, ":", 2)
	if len(parts) < 2 {
		return 0
	}

	fieldName := strings.TrimSpace(parts[0])

	// Check if this field has a dependency order
	if priority, ok := fixDependencyOrder[fieldName]; ok {
		return priority
	}

	// For lifecycle precondition violations, extract field from fix command
	if issue.FixCommand != emptyValue {
		// Extract field name from fix command
		// Format: "<CLI_COMMAND> object update ID --field field_name+=<PLACEHOLDER>"
		// Format: "<CLI_COMMAND> object update ID --field field_name=<PLACEHOLDER>"
		if strings.Contains(issue.FixCommand, "--field") {
			fieldPart := issue.FixCommand
			fieldStart := strings.Index(fieldPart, "--field")
			if fieldStart != -1 {
				fieldPart = fieldPart[fieldStart+len("--field"):]
				// Extract field name (up to += or =)
				fieldNameEnd := strings.Index(fieldPart, "+=")
				if fieldNameEnd == -1 {
					fieldNameEnd = strings.Index(fieldPart, "=")
				}
				if fieldNameEnd != -1 {
					fieldNameFromCmd := strings.TrimSpace(fieldPart[:fieldNameEnd])
					if priority, ok := fixDependencyOrder[fieldNameFromCmd]; ok {
						return priority
					}
				}
			}
		}
	}

	return 0
}

// sortIssuesByDependency sorts issues by dependency hierarchy
// Issues with fewer dependencies (higher-level) are fixed first
// Within the same dependency level, sort by Tier (Tier 1 = blocking issues first)
func sortIssuesByDependency(issues []Issue) []Issue {
	sorted := make([]Issue, len(issues))
	copy(sorted, issues)

	sort.Slice(sorted, func(i, j int) bool {
		priorityI := getFixDependencyPriority(sorted[i])
		priorityJ := getFixDependencyPriority(sorted[j])

		// First, sort by dependency priority (higher = fix first)
		if priorityI != priorityJ {
			return priorityI > priorityJ
		}

		// Same dependency level: sort by Tier (lower tier = higher priority)
		// Tier 1 (blocking) before Tier 2 (warning) before Tier 3 (informational)
		if sorted[i].Tier != sorted[j].Tier {
			return sorted[i].Tier < sorted[j].Tier
		}

		// Same tier: maintain original order (stable sort)
		return false
	})

	return sorted
}
