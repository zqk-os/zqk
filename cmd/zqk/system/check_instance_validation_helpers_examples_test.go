package system

import (
	"fmt"
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestFixCommandExamples demonstrates what the generated fix commands look like
// This test is for documentation/examples - it shows the actual output format
func TestFixCommandExamples(t *testing.T) {
	tests := []struct {
		name    string
		objID   string
		kind    string
		field   string
		message string
		rule    string
		objMap  map[string]any
	}{
		{
			name:    "Backlog item needing milestone - full context",
			objID:   "BLI-917",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': milestone_refs is not empty",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyCategory: "feature",
				objects.FieldKeyTags:     []string{"branding", "white-label"},
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
				objects.FieldKeyTitle:    "White-Label Branding System",
			},
		},
		{
			name:    "Backlog item needing priority plan - minimal context",
			objID:   "BLI-123",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': priority_plan_ref is set",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyStatus: objects.ObjectStatusPlanned,
			},
		},
		{
			name:    "Backlog item needing workstream - with category and tags",
			objID:   "BLI-456",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': workstream_refs is not empty",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyCategory: "infrastructure",
				objects.FieldKeyTags:     []string{"devops", "ci-cd", "deployment"},
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
			},
		},
		{
			name:    "Backlog item needing goal - full context with title",
			objID:   "BLI-789",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': goal_refs is not empty",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyCategory: "security",
				objects.FieldKeyTags:     []string{"encryption", "compliance"},
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
				objects.FieldKeyTitle:    "Implement end-to-end encryption",
			},
		},
		{
			name:    "Required field missing",
			objID:   "BLI-999",
			kind:    "backlog_item",
			field:   "title",
			message: "title: field is required",
			rule:    "required",
			objMap:  map[string]any{},
		},
	}

	fmt.Fprintln(os.Stdout, "\n=== Generated Fix Command Examples ===")
	fmt.Fprintln(os.Stdout)

	for i, tt := range tests {
		cmd := generateFixCommand(tt.objID, tt.kind, tt.field, tt.message, tt.rule, tt.objMap)

		fmt.Fprintf(os.Stdout, "%d. %s\n", i+1, tt.name)
		fmt.Fprintf(os.Stdout, "   Input:  %s (rule: %s)\n", tt.message, tt.rule)
		if cmd != emptyValue {
			fmt.Fprintf(os.Stdout, "   Output: %s\n", cmd)

			// Show the query hint if present
			if containsPlaceholder(cmd) {
				hint := extractQueryHint(cmd)
				if hint != emptyValue {
					fmt.Fprintf(os.Stdout, "   Query Hint: %s\n", hint)
					fmt.Fprintf(os.Stdout, "   → Will query objects matching: %s\n", formatQueryHint(hint))
				}
			}
		} else {
			fmt.Fprintf(os.Stdout, "   Output: <no command generated>\n")
		}
		fmt.Fprintln(os.Stdout)
	}
}

// Helper functions for demonstration
func containsPlaceholder(cmd string) bool {
	return len(cmd) > 0 && (cmd[len(cmd)-1] == '>' ||
		containsSubstringInCommand(cmd, "<MILESTONE_ID:") ||
		containsSubstringInCommand(cmd, "<PRIORITY_PLAN_ID:") ||
		containsSubstringInCommand(cmd, "<WORKSTREAM_ID:") ||
		containsSubstringInCommand(cmd, "<GOAL_ID:"))
}

func extractQueryHint(cmd string) string {
	// Extract query hint from placeholder like <MILESTONE_ID:category=X&tags=Y>
	start := -1
	for i := 0; i < len(cmd)-1; i++ {
		if cmd[i] == '<' {
			// Find the colon after the ID
			for j := i + 1; j < len(cmd); j++ {
				if cmd[j] == ':' {
					start = j + 1
					break
				}
				if cmd[j] == '>' {
					break
				}
			}
			break
		}
	}
	if start == -1 {
		return ""
	}

	end := start
	for end < len(cmd) && cmd[end] != '>' {
		end++
	}

	if end > start {
		return cmd[start:end]
	}
	return ""
}

func formatQueryHint(hint string) string {
	// Format query hint for readability
	if hint == emptyValue {
		return "none"
	}
	// Replace & with " AND " for readability
	formatted := hint
	for i := 0; i < len(formatted)-1; i++ {
		if formatted[i] == '&' {
			formatted = formatted[:i] + " AND " + formatted[i+1:]
			i += 4 // Skip " AND "
		}
	}
	return formatted
}

func containsSubstringInCommand(cmd, substr string) bool {
	for i := 0; i <= len(cmd)-len(substr); i++ {
		if cmd[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
