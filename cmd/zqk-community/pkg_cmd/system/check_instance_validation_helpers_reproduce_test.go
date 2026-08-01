package system

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

// TestReproduceAutoFixableIssue reproduces the actual production scenario
// where auto_fixable is false even though lifecycle violations should generate fix commands
func TestReproduceAutoFixableIssue(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		objID           string
		kind            string
		field           string
		message         string
		rule            string
		objMap          map[string]any
		expectCmd       bool   // Whether we expect a fix command to be generated
		expectAutoFix   bool   // Whether we expect auto_fixable to be true
		reason          string // Why we expect this outcome
		nonDeterminable bool   // Whether this is a known non-determinable scenario
	}{
		{
			name:    "lifecycle priority_plan_ref violation - PRODUCTION SCENARIO",
			objID:   "ITEM-916",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': priority_plan_ref is set (required per DEC-priority-plan-ref-requirement)",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyCategory: "Developer Experience",
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
				objects.FieldKeyTitle:    "Check Violation Resolver",
			},
			expectCmd:       true,
			expectAutoFix:   true,
			reason:          "Lifecycle violation with clear precondition - should generate fix command",
			nonDeterminable: false,
		},
		{
			name:    "lifecycle milestone_refs violation - PRODUCTION SCENARIO",
			objID:   "ITEM-916",
			kind:    "backlog_item",
			field:   "status",
			message: "Precondition not met for status 'planned': At least one milestone_ref linked",
			rule:    "lifecycle",
			objMap: map[string]any{
				objects.FieldKeyCategory: "Developer Experience",
				objects.FieldKeyStatus:   objects.ObjectStatusPlanned,
				objects.FieldKeyTitle:    "Check Violation Resolver",
			},
			expectCmd:       true,
			expectAutoFix:   true,
			reason:          "Lifecycle violation with milestone_refs precondition - should generate fix command",
			nonDeterminable: false,
		},
		// Non-determinable scenarios (early exit conditions)
		{
			name:            "empty objID - non-determinable",
			objID:           "",
			kind:            "backlog_item",
			field:           "status",
			message:         "Precondition not met for status 'planned': priority_plan_ref is set",
			rule:            "lifecycle",
			objMap:          map[string]any{},
			expectCmd:       false,
			expectAutoFix:   false,
			reason:          "Empty objID makes it impossible to generate fix command",
			nonDeterminable: true,
		},
		{
			name:            "wrong rule type - generates generic required field command",
			objID:           "ITEM-916",
			kind:            "backlog_item",
			field:           "status",
			message:         "Precondition not met for status 'planned': priority_plan_ref is set",
			rule:            "required", // Wrong rule type - but generateFixCommand handles this
			objMap:          map[string]any{},
			expectCmd:       true, // Actually generates a generic command for required fields
			expectAutoFix:   true,
			reason:          "Rule type 'required' generates generic fix command (not lifecycle-specific)",
			nonDeterminable: false, // Actually determinable, just generic
		},
		{
			name:            "missing precondition marker - non-determinable",
			objID:           "ITEM-916",
			kind:            "backlog_item",
			field:           "status",
			message:         "Some other error message",
			rule:            "lifecycle",
			objMap:          map[string]any{},
			expectCmd:       false,
			expectAutoFix:   false,
			reason:          "Message must contain 'Precondition not met' marker",
			nonDeterminable: true,
		},
		{
			name:            "unknown precondition pattern - non-determinable",
			objID:           "ITEM-916",
			kind:            "backlog_item",
			field:           "status",
			message:         "Precondition not met for status 'planned': some_unknown_field is set",
			rule:            "lifecycle",
			objMap:          map[string]any{},
			expectCmd:       false,
			expectAutoFix:   false,
			reason:          "Unknown precondition patterns cannot be auto-fixed",
			nonDeterminable: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test generateFixCommand directly
			fixCmd := generateFixCommand(tc.objID, tc.kind, tc.field, tc.message, tc.rule, tc.objMap)

			gotCmd := fixCmd != emptyValue
			if gotCmd != tc.expectCmd {
				t.Errorf("generateFixCommand: expected cmd=%v, got cmd=%v (command: %q)\nReason: %s",
					tc.expectCmd, gotCmd, fixCmd, tc.reason)
			}

			// Test convertValidationResultsToIssues to verify auto_fixable is set correctly
			result := &validation.ValidationResult{
				Errors: []validation.ValidationError{
					{
						Field:   tc.field,
						Message: tc.message,
						Rule:    tc.rule,
					},
				},
			}

			issues := convertValidationResultsToIssues(result, nil, tc.objID, tc.kind, tc.objMap, "", nil)

			if len(issues) != 1 {
				t.Fatalf("Expected 1 issue, got %d", len(issues))
			}

			issue := issues[0]
			gotAutoFix := issue.AutoFixable
			if gotAutoFix != tc.expectAutoFix {
				t.Errorf("auto_fixable: expected %v, got %v\nReason: %s\nNon-determinable: %v",
					tc.expectAutoFix, gotAutoFix, tc.reason, tc.nonDeterminable)
			}

			// If we expect a command, verify it was set
			if tc.expectCmd && issue.FixCommand == emptyValue {
				t.Errorf("Expected fix command to be set, but it was empty")
			}

			// If non-determinable, verify we get empty command and auto_fixable=false
			if tc.nonDeterminable {
				if issue.FixCommand != emptyValue {
					t.Errorf("Non-determinable scenario should not generate fix command, got: %q", issue.FixCommand)
				}
				if issue.AutoFixable {
					t.Errorf("Non-determinable scenario should have auto_fixable=false, got true")
				}
			}
		})
	}
}

// TestIdentifyNonDeterminableScenarios documents known scenarios where
// fix command generation is non-determinable and should exit early
func TestIdentifyNonDeterminableScenarios(t *testing.T) {
	t.Parallel()
	scenarios := []struct {
		name        string
		condition   string
		earlyExit   bool
		description string
	}{
		{
			name:        "empty_objID",
			condition:   "objID == \"\"",
			earlyExit:   true,
			description: "Cannot generate command without object ID - fast path return",
		},
		{
			name:        "wrong_rule_type",
			condition:   "rule != \"lifecycle\"",
			earlyExit:   false, // Rule check happens later, but we don't enter lifecycle logic
			description: "Rule must be 'lifecycle' for lifecycle violations - skipped in lifecycle block",
		},
		{
			name:        "missing_precondition_marker",
			condition:   "!strings.Contains(message, \"Precondition not met\")",
			earlyExit:   false, // Check happens in lifecycle block
			description: "Message must contain 'Precondition not met' marker - skipped in lifecycle block",
		},
		{
			name:        "unknown_precondition_pattern",
			condition:   "Pattern not matched by any known precondition",
			earlyExit:   false, // Falls through all pattern checks
			description: "Unknown precondition patterns cannot be auto-fixed - returns empty string",
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			t.Logf("Scenario: %s", s.name)
			t.Logf("Condition: %s", s.condition)
			t.Logf("Early Exit: %v", s.earlyExit)
			t.Logf("Description: %s", s.description)

			// Document that these scenarios should result in auto_fixable=false
			// and should be recognized early to avoid unnecessary processing
		})
	}
}
