package system

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/validation"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestGenerateFixCommand tests fix command generation with various validation errors
func TestGenerateFixCommand(t *testing.T) {
	t.Parallel()
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}

	tests := []struct {
		name     string
		objID    string
		kind     string
		field    string
		message  string
		rule     string
		objMap   map[string]any
		expected string
	}{
		{
			name:    "milestone_refs precondition with context",
			objID:   "BLI-123",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': milestone_refs is not empty",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyCategory: "feature",
				objects.FieldKeyTags:     []string{"backend", "api"},
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
				objects.FieldKeyTitle:    "Implement user authentication",
			},
			expected: fmt.Sprintf("%s object update BLI-123 --field milestone_ref=<MILESTONE_ID:category=feature&tags=backend,api&status=planned&title_pattern=Implement user authentication>", cliCmd),
		},
		{
			name:    "milestone_refs precondition minimal context",
			objID:   "BLI-456",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': at least one milestone_ref linked",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyStatus: objects.ObjectStatusPlanned,
			},
			expected: fmt.Sprintf("%s object update BLI-456 --field milestone_ref=<MILESTONE_ID:status=planned>", cliCmd),
		},
		{
			name:    "priority_plan_ref precondition",
			objID:   "BLI-789",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': priority_plan_ref is set",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyCategory: "bug",
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
			},
			expected: fmt.Sprintf("%s object update BLI-789 --field priority_plan_ref=<PRIORITY_PLAN_ID:category=bug&status=active>", cliCmd),
		},
		{
			name:    "workstream_refs precondition",
			objID:   "BLI-101",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': workstream_refs is not empty",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyCategory: "infrastructure",
				objects.FieldKeyTags:     []any{"devops", "ci-cd"},
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
				objects.FieldKeyTitle:    "Setup deployment pipeline",
			},
			expected: fmt.Sprintf("%s object update BLI-101 --field workstream_refs+=<WORKSTREAM_ID:category=infrastructure&tags=devops,ci-cd&status=planned&title_pattern=Setup deployment pipeline>", cliCmd),
		},
		{
			name:    "goal_refs precondition",
			objID:   "BLI-202",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': goal_refs is not empty",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyCategory: "security",
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
				objects.FieldKeyTitle:    "Implement encryption",
			},
			expected: fmt.Sprintf("%s object update BLI-202 --field goal_refs+=<GOAL_ID:category=security&status=planned&title_pattern=Implement encryption>", cliCmd),
		},
		{
			name:     "required field violation",
			objID:    "BLI-303",
			kind:     "backlog_item",
			field:    "title",
			message:  "title: field is required",
			rule:     "required",
			objMap:   map[string]any{},
			expected: fmt.Sprintf("%s object update BLI-303 --field title=<VALUE>", cliCmd),
		},
		{
			name:     "non-lifecycle error returns empty",
			objID:    "BLI-404",
			kind:     "backlog_item",
			field:    "title",
			message:  "title: invalid format",
			rule:     "format",
			objMap:   map[string]any{},
			expected: "",
		},
		{
			name:     "empty objID returns empty",
			objID:    "",
			kind:     "backlog_item",
			field:    "status",
			message:  "Precondition not met for status 'planned': milestone_refs is not empty",
			rule:     "lifecycle",
			objMap:   map[string]any{},
			expected: "",
		},
		{
			name:    "tags as string array",
			objID:   "BLI-505",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': milestone_refs is not empty",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyCategory: "feature",
				objects.FieldKeyTags:     []string{"frontend", "ui"},
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
			},
			expected: fmt.Sprintf("%s object update BLI-505 --field milestone_ref=<MILESTONE_ID:category=feature&tags=frontend,ui&status=planned>", cliCmd),
		},
		{
			name:    "tags as interface array",
			objID:   "BLI-606",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': milestone_refs is not empty",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyCategory: "feature",
				objects.FieldKeyTags:     []any{"backend", "database"},
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
			},
			expected: fmt.Sprintf("%s object update BLI-606 --field milestone_ref=<MILESTONE_ID:category=feature&tags=backend,database&status=planned>", cliCmd),
		},
		{
			name:    "full context all fields",
			objID:   "BLI-707",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': milestone_refs is not empty",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyCategory: "enhancement",
				objects.FieldKeyTags:     []string{"performance", "optimization", "caching"},
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
				objects.FieldKeyTitle:    "Optimize database queries",
			},
			expected: fmt.Sprintf("%s object update BLI-707 --field milestone_ref=<MILESTONE_ID:category=enhancement&tags=performance,optimization,caching&status=planned&title_pattern=Optimize database queries>", cliCmd),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateFixCommand(tt.objID, tt.kind, tt.field, tt.message, tt.rule, tt.objMap)
			if result != tt.expected {
				t.Errorf("generateFixCommand() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestBuildQueryHint tests query hint building with various contextual information
func TestBuildQueryHint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		targetKind    string
		category      string
		tags          []string
		status        string
		title         string
		expected      string
		shouldContain []string
	}{
		{
			name:          "all fields present",
			targetKind:    "milestone",
			category:      "feature",
			tags:          []string{"backend", "api"},
			status:        objects.ObjectStatusPlanned,
			title:         "Implement authentication",
			shouldContain: []string{"category=feature", "tags=backend,api", "title_pattern=Implement authentication"},
			// Note: status=planned is skipped for milestones (milestones don't have "planned" status)
		},
		{
			name:          "priority_plan uses active status",
			targetKind:    "priority_plan",
			category:      "feature",
			status:        objects.ObjectStatusPlanned,
			expected:      "category=feature&status=active",
			shouldContain: []string{"status=active"},
		},
		{
			name:          "only category",
			targetKind:    "milestone",
			category:      "bug",
			expected:      "category=bug",
			shouldContain: []string{"category=bug"},
		},
		{
			name:          "only tags",
			targetKind:    "milestone",
			tags:          []string{"frontend"},
			expected:      "tags=frontend",
			shouldContain: []string{"tags=frontend"},
		},
		{
			name:          "only status",
			targetKind:    "milestone",
			status:        objects.ObjectStatusInProgress,
			expected:      "status=in_progress",
			shouldContain: []string{"status=in_progress"},
		},
		{
			name:          "only title",
			targetKind:    "milestone",
			title:         "User authentication",
			expected:      "title_pattern=User authentication",
			shouldContain: []string{"title_pattern=User authentication"},
		},
		{
			name:          "empty context",
			targetKind:    "milestone",
			expected:      "",
			shouldContain: []string{},
		},
		{
			name:          "multiple tags",
			targetKind:    "milestone",
			tags:          []string{"backend", "api", "security"},
			expected:      "tags=backend,api,security",
			shouldContain: []string{"tags=backend,api,security"},
		},
		{
			name:          "category and status",
			targetKind:    "milestone",
			category:      "enhancement",
			status:        objects.ObjectStatusComplete,
			expected:      "category=enhancement&status=complete",
			shouldContain: []string{"category=enhancement", "status=complete"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildQueryHint(tt.targetKind, tt.category, tt.tags, tt.status, tt.title)

			// If expected is specified, check exact match
			if tt.expected != emptyValue {
				if result != tt.expected {
					t.Errorf("buildQueryHint() = %q, want %q", result, tt.expected)
				}
			}

			// Check that all required parts are contained
			for _, part := range tt.shouldContain {
				if !testContainsSubstring(result, part) {
					t.Errorf("buildQueryHint() = %q, should contain %q", result, part)
				}
			}

			// If expected empty, verify it's empty
			if tt.expected == emptyValue && len(tt.shouldContain) == 0 {
				if result != emptyValue {
					t.Errorf("buildQueryHint() = %q, want empty string", result)
				}
			}
		})
	}
}

// TestConvertValidationResultsToIssues tests issue conversion with fix command generation
func TestConvertValidationResultsToIssues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		result         *validation.ValidationResult
		err            error
		objID          string
		kind           string
		objMap         map[string]any
		expectedIssues int
		checkFixCmd    func(t *testing.T, issues []Issue)
	}{
		{
			name: "lifecycle error generates fix command",
			result: &validation.ValidationResult{
				Errors: []validation.ValidationError{
					{
						Field:   "status",
						Message: "Precondition not met for status 'planned': milestone_refs is not empty",
						Rule:    "lifecycle",
					},
				},
			},
			objID: "BLI-123",
			kind:  "backlog_item",
			objMap: map[string]any{
				objects.FieldKeyCategory: "feature",
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
			},
			expectedIssues: 1,
			checkFixCmd: func(t *testing.T, issues []Issue) {
				if len(issues) == 0 {
					t.Fatal("Expected at least one issue")
				}
				issue := issues[0]
				if !issue.AutoFixable {
					t.Error("Expected issue to be auto-fixable")
				}
				if issue.FixCommand == emptyValue {
					t.Error("Expected fix command to be generated")
				}
				if !testContainsSubstring(issue.FixCommand, "milestone_ref=") {
					t.Errorf("Expected fix command to contain milestone_ref=, got %q", issue.FixCommand)
				}
				if !testContainsSubstring(issue.FixCommand, "BLI-123") {
					t.Errorf("Expected fix command to contain object ID, got %q", issue.FixCommand)
				}
				if issue.Tier != 1 {
					t.Errorf("Expected Tier 1 (blocking) for lifecycle error, got %d", issue.Tier)
				}
			},
		},
		{
			name: "required field error generates fix command",
			result: &validation.ValidationResult{
				Errors: []validation.ValidationError{
					{
						Field:   "title",
						Message: "field is required",
						Rule:    "required",
					},
				},
			},
			objID:          "BLI-456",
			kind:           "backlog_item",
			objMap:         map[string]any{},
			expectedIssues: 1,
			checkFixCmd: func(t *testing.T, issues []Issue) {
				if len(issues) == 0 {
					t.Fatal("Expected at least one issue")
				}
				issue := issues[0]
				if !issue.AutoFixable {
					t.Error("Expected issue to be auto-fixable")
				}
				if issue.FixCommand == emptyValue {
					t.Error("Expected fix command to be generated")
				}
				if !testContainsSubstring(issue.FixCommand, "title=") {
					t.Errorf("Expected fix command to contain title=, got %q", issue.FixCommand)
				}
				if issue.Tier != 1 {
					t.Errorf("Expected Tier 1 (blocking) for required field error, got %d", issue.Tier)
				}
			},
		},
		{
			name: "warning generates fix command but not auto-fixable",
			result: &validation.ValidationResult{
				Warnings: []validation.ValidationWarning{
					{
						Field:   "description",
						Message: "field is recommended",
						Rule:    "recommended",
					},
				},
			},
			objID:          "BLI-789",
			kind:           "backlog_item",
			objMap:         map[string]any{},
			expectedIssues: 1,
			checkFixCmd: func(t *testing.T, issues []Issue) {
				if len(issues) == 0 {
					t.Fatal("Expected at least one issue")
				}
				issue := issues[0]
				if issue.Tier != 3 {
					t.Errorf("Expected Tier 3 (informational) for warning, got %d", issue.Tier)
				}
				// Warnings may have fix commands but aren't auto-fixable by default
			},
		},
		{
			name: "multiple errors generate multiple fix commands",
			result: &validation.ValidationResult{
				Errors: []validation.ValidationError{
					{
						Field:   "status",
						Message: "Precondition not met for status 'planned': milestone_refs is not empty",
						Rule:    "lifecycle",
					},
					{
						Field:   objects.FieldKeyPriorityPlanRef,
						Message: "Precondition not met for status 'planned': priority_plan_ref is set",
						Rule:    "lifecycle",
					},
				},
			},
			objID: "BLI-101",
			kind:  "backlog_item",
			objMap: map[string]any{
				objects.FieldKeyCategory: "feature",
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
			},
			expectedIssues: 2,
			checkFixCmd: func(t *testing.T, issues []Issue) {
				if len(issues) != 2 {
					t.Fatalf("Expected 2 issues, got %d", len(issues))
				}

				// Check first issue (milestone_refs)
				if !testContainsSubstring(issues[0].FixCommand, "milestone_ref=") && !testContainsSubstring(issues[0].FixCommand, "priority_plan_ref=") {
					t.Errorf("Expected first issue to have milestone_ref or priority_plan_ref fix command, got %q", issues[0].FixCommand)
				}

				// Check second issue
				if !testContainsSubstring(issues[1].FixCommand, "milestone_ref=") && !testContainsSubstring(issues[1].FixCommand, "priority_plan_ref=") {
					t.Errorf("Expected second issue to have milestone_ref or priority_plan_ref fix command, got %q", issues[1].FixCommand)
				}

				// Ensure they're different
				if issues[0].FixCommand == issues[1].FixCommand {
					t.Error("Expected different fix commands for different errors")
				}
			},
		},
		{
			name:           "validation error returns single issue",
			result:         nil,
			err:            errors.New("spec not found"),
			objID:          "BLI-202",
			kind:           "backlog_item",
			objMap:         map[string]any{},
			expectedIssues: 1,
			checkFixCmd: func(t *testing.T, issues []Issue) {
				if len(issues) == 0 {
					t.Fatal("Expected at least one issue")
				}
				issue := issues[0]
				if issue.FixCommand != emptyValue {
					t.Error("Expected no fix command for validation error")
				}
				if issue.Tier != 2 {
					t.Errorf("Expected Tier 2 (warning) for validation error, got %d", issue.Tier)
				}
			},
		},
		{
			name: "pattern error is marked auto-fixable",
			result: &validation.ValidationResult{
				Errors: []validation.ValidationError{
					{
						Field:   "namespace_id",
						Message: "Field namespace_id does not match pattern ^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$",
						Rule:    "pattern",
					},
				},
			},
			objID:          "AUD-31",
			kind:           "audit_event",
			objMap:         map[string]any{},
			expectedIssues: 1,
			checkFixCmd: func(t *testing.T, issues []Issue) {
				if len(issues) == 0 {
					t.Fatal("Expected at least one issue")
				}
				issue := issues[0]
				if !issue.AutoFixable {
					t.Error("Expected pattern error to be auto-fixable")
				}
				if issue.Tier != 2 {
					t.Errorf("Expected Tier 2 (warning) for pattern error, got %d", issue.Tier)
				}
				if issue.FixCommand != emptyValue {
					t.Error("Pattern errors should not have fix commands (handled by spec-based fixes)")
				}
			},
		},
		{
			name: "datatype error from production - health_check_duration_ms",
			result: &validation.ValidationResult{
				Errors: []validation.ValidationError{
					{
						Field:   "health_check_duration_ms",
						Message: "Field health_check_duration_ms has invalid datatype: expected number, got",
						Rule:    "datatype",
					},
				},
			},
			objID:          "SCH-123",
			kind:           "scheduler_job",
			objMap:         map[string]any{},
			expectedIssues: 1,
			checkFixCmd: func(t *testing.T, issues []Issue) {
				if len(issues) == 0 {
					t.Fatal("Expected at least one issue")
				}
				issue := issues[0]
				if !issue.AutoFixable {
					t.Errorf("Expected datatype error to be auto-fixable. Message: %q, Rule: %q", issue.Message, "datatype")
				}
				if issue.Tier != 2 {
					t.Errorf("Expected Tier 2 (warning) for datatype error, got %d", issue.Tier)
				}
			},
		},
		{
			name: "required field error from production - minCount",
			result: &validation.ValidationResult{
				Errors: []validation.ValidationError{
					{
						Field:   "title",
						Message: "Field title is required (minCount: 1)",
						Rule:    "minCount",
					},
				},
			},
			objID:          "BLI-456",
			kind:           "backlog_item",
			objMap:         map[string]any{},
			expectedIssues: 1,
			checkFixCmd: func(t *testing.T, issues []Issue) {
				if len(issues) == 0 {
					t.Fatal("Expected at least one issue")
				}
				issue := issues[0]
				if !issue.AutoFixable {
					t.Errorf("Expected required/minCount error to be auto-fixable. Message: %q, Rule: %q", issue.Message, "minCount")
				}
				// Note: minCount stays as Tier 2 (not Tier 1) because Rule is "minCount", not "required"
				// Only Rule == "required" or Rule == "lifecycle" become Tier 1
				if issue.Tier != 2 {
					t.Errorf("Expected Tier 2 (warning) for minCount error (Rule is 'minCount', not 'required'), got %d", issue.Tier)
				}
			},
		},
		{
			name: "pattern error from production - id pattern",
			result: &validation.ValidationResult{
				Errors: []validation.ValidationError{
					{
						Field:   "id",
						Message: "Field id does not match pattern ^[A-Z]+-\\d{3,}$",
						Rule:    "pattern",
					},
				},
			},
			objID:          "TEST-1",
			kind:           "test_case",
			objMap:         map[string]any{},
			expectedIssues: 1,
			checkFixCmd: func(t *testing.T, issues []Issue) {
				if len(issues) == 0 {
					t.Fatal("Expected at least one issue")
				}
				issue := issues[0]
				if !issue.AutoFixable {
					t.Errorf("Expected pattern error to be auto-fixable. Message: %q, Rule: %q", issue.Message, "pattern")
				}
				if issue.Tier != 2 {
					t.Errorf("Expected Tier 2 (warning) for pattern error, got %d", issue.Tier)
				}
			},
		},
		{
			name: "enum error from production - job_type",
			result: &validation.ValidationResult{
				Errors: []validation.ValidationError{
					{
						Field:   "job_type",
						Message: "Field job_type has invalid value: must be one of [context_refresh manifest_snapshot test_run]",
						Rule:    "in",
					},
				},
			},
			objID:          "SCH-789",
			kind:           "scheduler_job",
			objMap:         map[string]any{},
			expectedIssues: 1,
			checkFixCmd: func(t *testing.T, issues []Issue) {
				if len(issues) == 0 {
					t.Fatal("Expected at least one issue")
				}
				issue := issues[0]
				if !issue.AutoFixable {
					t.Errorf("Expected enum/in error to be auto-fixable. Message: %q, Rule: %q", issue.Message, "in")
				}
				if issue.Tier != 2 {
					t.Errorf("Expected Tier 2 (warning) for enum error, got %d", issue.Tier)
				}
			},
		},
		{
			name: "datatype error from production - last_activity time.Time",
			result: &validation.ValidationResult{
				Errors: []validation.ValidationError{
					{
						Field:   "last_activity",
						Message: "Field last_activity has invalid datatype: expected string, got time.Time",
						Rule:    "datatype",
					},
				},
			},
			objID:          "SCH-999",
			kind:           "scheduler_job",
			objMap:         map[string]any{},
			expectedIssues: 1,
			checkFixCmd: func(t *testing.T, issues []Issue) {
				if len(issues) == 0 {
					t.Fatal("Expected at least one issue")
				}
				issue := issues[0]
				if !issue.AutoFixable {
					t.Errorf("Expected datatype error to be auto-fixable. Message: %q, Rule: %q", issue.Message, "datatype")
				}
				if issue.Tier != 2 {
					t.Errorf("Expected Tier 2 (warning) for datatype error, got %d", issue.Tier)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := convertValidationResultsToIssues(tt.result, tt.err, tt.objID, tt.kind, tt.objMap, "", nil)

			if len(issues) != tt.expectedIssues {
				t.Errorf("Expected %d issues, got %d", tt.expectedIssues, len(issues))
			}

			if tt.checkFixCmd != nil {
				tt.checkFixCmd(t, issues)
			}
		})
	}
}

// testContainsSubstring checks if a string contains a substring (case-sensitive)
func testContainsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}
