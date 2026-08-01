package system

import (
	"fmt"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestResolveFixCommandPlaceholderExamples demonstrates the resolution process
// This shows what the resolution logic looks like step-by-step
func TestResolveFixCommandPlaceholderExamples(t *testing.T) {
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}

	examples := []struct {
		name        string
		command     string
		description string
	}{
		{
			name:        "Milestone resolution with full context",
			command:     fmt.Sprintf(`%s object update ITEM-917 --field milestone_refs+=<MILESTONE_ID:category=feature&tags=branding,white-label&status=planned&title_pattern=White-Label Branding System>`, cliCmd),
			description: "Command with milestone placeholder that needs resolution",
		},
		{
			name:        "Priority plan resolution",
			command:     fmt.Sprintf(`%s object update ITEM-123 --field priority_plan_ref=<PRIORITY_PLAN_ID:status=active>`, cliCmd),
			description: "Command with priority plan placeholder",
		},
		{
			name:        "Workstream resolution",
			command:     fmt.Sprintf(`%s object update ITEM-456 --field workstream_refs+=<WORKSTREAM_ID:category=infrastructure&tags=devops,ci-cd&status=planned>`, cliCmd),
			description: "Command with workstream placeholder",
		},
		{
			name:        "Goal resolution",
			command:     fmt.Sprintf(`%s object update ITEM-789 --field goal_refs+=<GOAL_ID:category=security&tags=encryption,compliance&status=planned&title_pattern=Implement end-to-end encryption>`, cliCmd),
			description: "Command with goal placeholder",
		},
	}

	fmt.Println("\n=== Fix Command Resolution Process ===")
	fmt.Println()

	for i, ex := range examples {
		fmt.Printf("%d. %s\n", i+1, ex.name)
		fmt.Printf("   Description: %s\n", ex.description)
		fmt.Printf("   Input Command:  %s\n", ex.command)

		// Step 1: Parse placeholder
		placeholder, targetKind := parsePlaceholderFromCommand(ex.command)
		if placeholder != emptyValue {
			fmt.Printf("   Step 1 - Placeholder: %s\n", placeholder)
			fmt.Printf("   Step 1 - Target Kind: %s\n", targetKind)

			// Step 2: Extract query hint
			queryHint := extractQueryHintFromPlaceholder(placeholder)
			if queryHint != emptyValue {
				fmt.Printf("   Step 2 - Query Hint: %s\n", queryHint)

				// Step 3: Parse to ListFilter
				filter := parseQueryHintToFilter(targetKind, queryHint)
				fmt.Printf("   Step 3 - ListFilter:\n")
				fmt.Printf("      Kind: %s\n", filter.Kind)
				fmt.Printf("      Filters: %v\n", filter.Filters)

				// Step 4: Show what the query would look like
				fmt.Printf("   Step 4 - Query Logic:\n")
				fmt.Printf("      SELECT %s WHERE\n", targetKind)
				for key, value := range filter.Filters {
					if valueMap, ok := value.(map[string]any); ok {
						// Operator-based filter
						for op, opValue := range valueMap {
							fmt.Printf("        AND %s %s %v\n", key, op, opValue)
						}
					} else {
						// Simple equality
						fmt.Printf("        AND %s = %v\n", key, value)
					}
				}

				// Step 5: Show resolution scenarios
				fmt.Printf("   Step 5 - Resolution Scenarios:\n")
				fmt.Printf("      - If 1 match found: Replace placeholder with object ID\n")
				fmt.Printf("      - If multiple matches: Leave as placeholder (or use best match)\n")
				fmt.Printf("      - If no matches: Leave as placeholder (or generate create command)\n")

				// Show example resolved command
				fmt.Printf("   Example Resolved Command (if MIL-045 matched):\n")
				resolved := replacePlaceholderInCommand(ex.command, placeholder, "MIL-045")
				fmt.Printf("      %s\n", resolved)
			}
		} else {
			fmt.Printf("   No placeholder found in command\n")
		}
		fmt.Println()
	}
}

// TestParsePlaceholderFromCommand tests placeholder parsing
func TestParsePlaceholderFromCommand(t *testing.T) {
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}

	tests := []struct {
		command             string
		expectedPlaceholder string
		expectedKind        string
	}{
		{
			command:             fmt.Sprintf(`%s object update ITEM-917 --field milestone_refs+=<MILESTONE_ID:category=feature>`, cliCmd),
			expectedPlaceholder: `<MILESTONE_ID:category=feature>`,
			expectedKind:        "milestone",
		},
		{
			command:             fmt.Sprintf(`%s object update ITEM-123 --field priority_plan_ref=<PRIORITY_PLAN_ID:status=active>`, cliCmd),
			expectedPlaceholder: `<PRIORITY_PLAN_ID:status=active>`,
			expectedKind:        "priority_plan",
		},
		{
			command:             fmt.Sprintf(`%s object update ITEM-999 --field title=<VALUE>`, cliCmd),
			expectedPlaceholder: `<VALUE>`,
			expectedKind:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expectedKind, func(t *testing.T) {
			placeholder, kind := parsePlaceholderFromCommand(tt.command)
			if placeholder != tt.expectedPlaceholder {
				t.Errorf("parsePlaceholderFromCommand() placeholder = %q, want %q", placeholder, tt.expectedPlaceholder)
			}
			if kind != tt.expectedKind {
				t.Errorf("parsePlaceholderFromCommand() kind = %q, want %q", kind, tt.expectedKind)
			}
		})
	}
}

// TestParseQueryHintToFilter tests query hint to filter conversion
func TestParseQueryHintToFilter(t *testing.T) {
	tests := []struct {
		name        string
		targetKind  string
		queryHint   string
		checkFilter func(t *testing.T, filter storage.ListFilter)
	}{
		{
			name:       "category and status",
			targetKind: "milestone",
			queryHint:  "category=feature&status=planned",
			checkFilter: func(t *testing.T, filter storage.ListFilter) {
				if filter.Kind != "milestone" {
					t.Errorf("Expected kind milestone, got %s", filter.Kind)
				}
				if filter.Filters[objects.FieldKeyCategory] != "feature" {
					t.Errorf("Expected category=feature, got %v", filter.Filters[objects.FieldKeyCategory])
				}
				if filter.Filters[objects.FieldKeyStatus] != "planned" {
					t.Errorf("Expected status=planned, got %v", filter.Filters[objects.FieldKeyStatus])
				}
			},
		},
		{
			name:       "tags with $hasAny operator",
			targetKind: "milestone",
			queryHint:  "tags=backend,api",
			checkFilter: func(t *testing.T, filter storage.ListFilter) {
				tagsFilter, ok := filter.Filters[objects.FieldKeyTags].(map[string]any)
				if !ok {
					t.Fatalf("Expected tags filter to be map[string]any, got %T", filter.Filters[objects.FieldKeyTags])
				}
				// Implementation uses $hasAny for comma-separated tags
				tags, ok := tagsFilter["$hasAny"].([]string)
				if !ok {
					tagsInterface, ok := tagsFilter["$hasAny"].([]any)
					if !ok {
						t.Fatalf("Expected $hasAny to contain slice, got %T", tagsFilter["$hasAny"])
					}
					if len(tagsInterface) != 2 {
						t.Errorf("Expected 2 tags, got %d", len(tagsInterface))
					}
				} else {
					if len(tags) != 2 || tags[0] != "backend" || tags[1] != "api" {
						t.Errorf("Expected tags [backend, api], got %v", tags)
					}
				}
			},
		},
		{
			name:       "title pattern with $contains",
			targetKind: "milestone",
			queryHint:  "title_pattern=White-Label",
			checkFilter: func(t *testing.T, filter storage.ListFilter) {
				titleFilter, ok := filter.Filters[objects.FieldKeyTitle].(map[string]any)
				if !ok {
					t.Fatalf("Expected title filter to be map[string]any, got %T", filter.Filters[objects.FieldKeyTitle])
				}
				if titleFilter["$contains"] != "White-Label" {
					t.Errorf("Expected $contains=White-Label, got %v", titleFilter["$contains"])
				}
			},
		},
		{
			name:       "full query hint",
			targetKind: "milestone",
			queryHint:  "category=feature&tags=branding,white-label&status=planned&title_pattern=White-Label Branding System",
			checkFilter: func(t *testing.T, filter storage.ListFilter) {
				if filter.Kind != "milestone" {
					t.Errorf("Expected kind milestone, got %s", filter.Kind)
				}
				if filter.Filters[objects.FieldKeyCategory] != "feature" {
					t.Errorf("Expected category=feature")
				}
				if filter.Filters[objects.FieldKeyStatus] != "planned" {
					t.Errorf("Expected status=planned")
				}
				// Check tags and title_pattern are present (detailed checks above)
				if filter.Filters[objects.FieldKeyTags] == nil {
					t.Error("Expected tags filter")
				}
				if filter.Filters[objects.FieldKeyTitle] == nil {
					t.Error("Expected title filter")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := parseQueryHintToFilter(tt.targetKind, tt.queryHint)
			tt.checkFilter(t, filter)
		})
	}
}

// TestReplacePlaceholderInCommand tests placeholder replacement
func TestReplacePlaceholderInCommand(t *testing.T) {
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}

	tests := []struct {
		command     string
		placeholder string
		resolvedID  string
		expected    string
	}{
		{
			command:     fmt.Sprintf(`%s object update ITEM-917 --field milestone_refs+=<MILESTONE_ID:category=feature>`, cliCmd),
			placeholder: `<MILESTONE_ID:category=feature>`,
			resolvedID:  "MIL-045",
			expected:    fmt.Sprintf(`%s object update ITEM-917 --field milestone_refs+=MIL-045`, cliCmd),
		},
		{
			command:     fmt.Sprintf(`%s object update ITEM-123 --field priority_plan_ref=<PRIORITY_PLAN_ID:status=active>`, cliCmd),
			placeholder: `<PRIORITY_PLAN_ID:status=active>`,
			resolvedID:  "PLAN-209",
			expected:    fmt.Sprintf(`%s object update ITEM-123 --field priority_plan_ref=PLAN-209`, cliCmd),
		},
	}

	for _, tt := range tests {
		t.Run(tt.resolvedID, func(t *testing.T) {
			result := replacePlaceholderInCommand(tt.command, tt.placeholder, tt.resolvedID)
			if result != tt.expected {
				t.Errorf("replacePlaceholderInCommand() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Mock test showing resolution flow (without actual storage)
func TestResolveFixCommandPlaceholderFlow(t *testing.T) {
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}

	// This is a demonstration of the flow - actual implementation would need storage provider
	command := fmt.Sprintf(`%s object update ITEM-917 --field milestone_refs+=<MILESTONE_ID:category=feature&tags=branding,white-label&status=planned>`, cliCmd)

	// Step 1: Parse placeholder
	placeholder, targetKind := parsePlaceholderFromCommand(command)
	if placeholder == emptyValue {
		t.Fatal("Expected placeholder to be found")
	}
	if targetKind != "milestone" {
		t.Errorf("Expected targetKind=milestone, got %s", targetKind)
	}

	// Step 2: Extract query hint
	queryHint := extractQueryHintFromPlaceholder(placeholder)
	if queryHint == emptyValue {
		t.Fatal("Expected query hint to be extracted")
	}

	// Step 3: Parse to filter
	filter := parseQueryHintToFilter(targetKind, queryHint)
	if filter.Kind != "milestone" {
		t.Errorf("Expected filter.Kind=milestone, got %s", filter.Kind)
	}
	if filter.Filters[objects.FieldKeyCategory] != "feature" {
		t.Errorf("Expected category=feature in filters")
	}

	// Step 4: Resolution (would query storage here)
	// For this test, we just verify the placeholder replacement works
	resolvedCommand := replacePlaceholderInCommand(command, placeholder, "MIL-045")
	expected := fmt.Sprintf(`%s object update ITEM-917 --field milestone_refs+=MIL-045`, cliCmd)
	if resolvedCommand != expected {
		t.Errorf("Expected resolved command %q, got %q", expected, resolvedCommand)
	}
}
