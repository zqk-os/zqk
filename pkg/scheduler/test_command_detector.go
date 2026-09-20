package scheduler

import (
	"sort"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// TestCommandDetector determines whether a command line (command + args) is a "test command"
// (e.g. go test, or shell -c "go test ..."). Used for notification behavior and output sanitization.
type TestCommandDetector interface {
	IsTestCommand(command string, args []string) bool
}

// TestCommandRule represents a single rule for matching a test command.
// Rules are evaluated in order; first match wins.
// Uses a condition-based pattern similar to transceiver routing rules for consistency.
type TestCommandRule struct {
	// Conditions: all conditions must match for the rule to match
	// Field can be: "command" (command string), "args" (any arg), "shell_script" (args[1] when args[0]=="-c")
	// Operator can be: "eq" (equals), "contains" (substring), "in" (value in args array)
	Conditions []TestCommandCondition
}

// TestCommandCondition defines a condition for matching command/args.
// Aligns with transceiver.Condition pattern for consistency.
type TestCommandCondition struct {
	Field    string // "command", "args", "shell_script"
	Operator string // "eq", "contains", "in"
	Value    any    // Comparison value (string for eq/contains, string for in)
}

// RulesTestCommandDetector implements TestCommandDetector from a list of rules.
type RulesTestCommandDetector struct {
	Rules []TestCommandRule
}

// IsTestCommand returns true if (command, args) matches any rule.
func (d *RulesTestCommandDetector) IsTestCommand(command string, args []string) bool {
	for _, r := range d.Rules {
		if d.ruleMatches(r, command, args) {
			return true
		}
	}
	return false
}

func (d *RulesTestCommandDetector) ruleMatches(r TestCommandRule, command string, args []string) bool {
	// All conditions must match
	for _, cond := range r.Conditions {
		if !d.evaluateCondition(cond, command, args) {
			return false
		}
	}
	return true
}

// evaluateCondition evaluates a single condition against command/args.
func (d *RulesTestCommandDetector) evaluateCondition(cond TestCommandCondition, command string, args []string) bool {
	var fieldValue string
	var exists bool

	// Get field value based on field name
	switch cond.Field {
	case "command":
		fieldValue = command
		exists = true
	case "args":
		// For "args" field, we check if any arg matches (for "eq" or "in") or contains (for "contains")
		// This is handled specially in the operator evaluation
		exists = len(args) > 0
	case "shell_script":
		// shell_script is args[1] when args[0] == "-c"
		if len(args) >= 2 && args[0] == "-c" {
			fieldValue = args[1]
			exists = true
		}
	default:
		return false // Unknown field
	}

	// Evaluate operator
	switch cond.Operator {
	case "eq", "==":
		if cond.Field == "args" {
			// For args field with eq, check if any arg equals the value
			valStr, ok := cond.Value.(string)
			if !ok {
				return false
			}
			for _, arg := range args {
				if arg == valStr {
					return true
				}
			}
			return false
		}
		valStr, ok := cond.Value.(string)
		if !ok || !exists {
			return false
		}
		return fieldValue == valStr
	case "contains":
		if cond.Field == "args" {
			// For args field with contains, check if any arg contains the value
			valStr, ok := cond.Value.(string)
			if !ok {
				return false
			}
			for _, arg := range args {
				if strings.Contains(arg, valStr) {
					return true
				}
			}
			return false
		}
		valStr, ok := cond.Value.(string)
		if !ok || !exists {
			return false
		}
		return strings.Contains(fieldValue, valStr)
	case "in":
		// Value should be in the args array (for "args" field) or check if fieldValue is in value array
		if cond.Field == "args" {
			valStr, ok := cond.Value.(string)
			if !ok {
				return false
			}
			for _, arg := range args {
				if arg == valStr {
					return true
				}
			}
			return false
		}
		// For other fields, check if fieldValue is in the value array
		if arr, ok := cond.Value.([]any); ok {
			for _, v := range arr {
				if vStr, ok := v.(string); ok && fieldValue == vStr {
					return true
				}
			}
		}
		return false
	default:
		return false // Unknown operator
	}
}

// DefaultTestCommandRules returns the built-in rules used when no config file is present.
// These match the original behavior: go test, command contains "test", shell -c "go test"
func DefaultTestCommandRules() []TestCommandRule {
	return []TestCommandRule{
		{
			Conditions: []TestCommandCondition{
				{Field: "command", Operator: "eq", Value: "go"},
				{Field: "args", Operator: "in", Value: "test"},
			},
		},
		{
			Conditions: []TestCommandCondition{
				{Field: "command", Operator: "contains", Value: "test"},
			},
		},
		{
			Conditions: []TestCommandCondition{
				{Field: "shell_script", Operator: "contains", Value: "go test"},
			},
		},
	}
}

// DefaultTestCommandDetector returns a detector with default rules only.
func DefaultTestCommandDetector() TestCommandDetector {
	return &RulesTestCommandDetector{Rules: DefaultTestCommandRules()}
}

// LoadTestCommandDetectorFromStorage loads test_command_rule objects from storage and builds a detector.
// Rules are sorted by priority (lower number = evaluated first). If no rules exist, returns DefaultTestCommandDetector().
func LoadTestCommandDetectorFromStorage(storage storagepkg.ObjectStorageProvider) TestCommandDetector {
	if storage == nil {
		return DefaultTestCommandDetector()
	}
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	result, err := storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind:    objects.KindTestCommandRule,
		Limit:   500,
		SortBy:  "priority",
		SortAsc: true,
	})
	if err != nil || len(result.Objects) == 0 {
		return DefaultTestCommandDetector()
	}

	// Sort by priority (lower = higher priority, first)
	sort.Slice(result.Objects, func(i, j int) bool {
		pi, _ := toInt(result.Objects[i][objects.FieldKeyPriority])
		pj, _ := toInt(result.Objects[j][objects.FieldKeyPriority])
		return pi < pj
	})

	rules := make([]TestCommandRule, 0, len(result.Objects))
	for _, obj := range result.Objects {
		rule, ok := objectToTestCommandRule(obj)
		if !ok {
			continue
		}
		rules = append(rules, rule)
	}
	if len(rules) == 0 {
		return DefaultTestCommandDetector()
	}

	return &RulesTestCommandDetector{Rules: rules}
}

// objectToTestCommandRule converts a stored object (map[string]any) to TestCommandRule.
// Object must have "conditions" as a list of maps with field, operator, value.
func objectToTestCommandRule(obj map[string]any) (TestCommandRule, bool) {
	raw, ok := obj[objects.FieldKeyConditions].([]any)
	if !ok || len(raw) == 0 {
		return TestCommandRule{}, false
	}
	conditions := make([]TestCommandCondition, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		field := getString(m, "field")
		operator := getString(m, "operator")
		if field == emptyValue || operator == emptyValue {
			continue
		}
		conditions = append(conditions, TestCommandCondition{
			Field:    field,
			Operator: operator,
			Value:    m["value"],
		})
	}
	if len(conditions) == 0 {
		return TestCommandRule{}, false
	}
	return TestCommandRule{Conditions: conditions}, true
}

func getString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	default:
		return 0, false
	}
}
