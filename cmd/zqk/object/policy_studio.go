package object

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// DSLTokenType classifies autocomplete tokens.
type DSLTokenType string

const (
	TokenTypeField    DSLTokenType = "field"
	TokenTypeOperator DSLTokenType = "operator"
	TokenTypeValue    DSLTokenType = "value"
	TokenTypeLogical  DSLTokenType = "logical"
)

// DSLTokenSuggestion represents an autocompletion token recommendation.
type DSLTokenSuggestion struct {
	Token       string       `json:"token" yaml:"token"`
	Type        DSLTokenType `json:"type" yaml:"type"`
	Description string       `json:"description" yaml:"description"`
}

// PolicyRule defines a validation policy rule with an evaluatable DSL expression.
type PolicyRule struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	TargetKind  string `json:"target_kind" yaml:"target_kind"`
	Expression  string `json:"expression" yaml:"expression"`
	Severity    string `json:"severity" yaml:"severity"` // "blocker", "warning", "info"
}

// RuleEvaluationResult captures the outcome of dry-running a policy rule across objects.
type RuleEvaluationResult struct {
	RuleID          string   `json:"rule_id" yaml:"rule_id"`
	Name            string   `json:"name" yaml:"name"`
	Expression      string   `json:"expression" yaml:"expression"`
	Passed          bool     `json:"passed" yaml:"passed"`
	TotalEvaluated  int      `json:"total_evaluated" yaml:"total_evaluated"`
	ViolationsCount int      `json:"violations_count" yaml:"violations_count"`
	OffendingIDs    []string `json:"offending_ids,omitempty" yaml:"offending_ids,omitempty"`
	Summary         string   `json:"summary" yaml:"summary"`
}

// SuggestDSLTokens returns autocomplete suggestions for a given kind and input buffer.
func SuggestDSLTokens(kind string, input string) []DSLTokenSuggestion {
	input = strings.TrimSpace(input)
	var suggestions []DSLTokenSuggestion

	// Get registered fields for kind
	reg := objects.GetGlobalFieldRegistry()
	var registeredFieldNames []string
	fieldMap := make(map[string]objects.FieldInfo)
	if reg != nil {
		if kf, err := reg.GetFieldsForKind(kind); err == nil && kf != nil {
			for _, f := range kf.AllFields {
				registeredFieldNames = append(registeredFieldNames, f.Name)
				fieldMap[f.Name] = f
			}
		}
	}
	sort.Strings(registeredFieldNames)

	tokens := strings.Fields(input)
	if len(tokens) == 0 {
		// Empty input: suggest fields
		for _, name := range registeredFieldNames {
			desc := "Schema field"
			if meta, ok := fieldMap[name]; ok && meta.Description != "" {
				desc = meta.Description
			}
			suggestions = append(suggestions, DSLTokenSuggestion{
				Token:       name,
				Type:        TokenTypeField,
				Description: desc,
			})
		}
		return suggestions
	}

	lastToken := tokens[len(tokens)-1]

	// Check if last token is a logical operator (&&, AND, ||, OR)
	if lastToken == "&&" || strings.EqualFold(lastToken, "AND") || lastToken == "||" || strings.EqualFold(lastToken, "OR") {
		for _, name := range registeredFieldNames {
			suggestions = append(suggestions, DSLTokenSuggestion{
				Token:       name,
				Type:        TokenTypeField,
				Description: "Schema field",
			})
		}
		return suggestions
	}

	// Check if last token is an operator
	isOp := false
	for _, op := range []string{"==", "!=", "contains", "not_contains", ">", "<", ">=", "<=", "in"} {
		if strings.EqualFold(lastToken, op) {
			isOp = true
			break
		}
	}

	if isOp && len(tokens) >= 2 {
		targetField := tokens[len(tokens)-2]
		// Value suggestions based on field
		if targetField == objects.FieldKeyStatus {
			loader := objects.GetGlobalLifecycleLoader()
			if loader != nil {
				if lc, err := loader.LoadLifecycle(kind); err == nil && lc != nil {
					for _, st := range lc.Statuses {
						suggestions = append(suggestions, DSLTokenSuggestion{
							Token:       fmt.Sprintf("%q", st.Value),
							Type:        TokenTypeValue,
							Description: fmt.Sprintf("Lifecycle status: %s", st.Display),
						})
					}
				}
			}
			if len(suggestions) == 0 {
				for _, st := range []string{"exploring", "planned", "testing", "in_progress", "complete"} {
					suggestions = append(suggestions, DSLTokenSuggestion{
						Token:       fmt.Sprintf("%q", st),
						Type:        TokenTypeValue,
						Description: "Status value",
					})
				}
			}
			return suggestions
		}

		if targetField == objects.FieldKeyPriority || targetField == "priority_tier" {
			for _, p := range []string{"P0", "P1", "P2", "P3", "P4", "critical", "high", "medium", "low"} {
				suggestions = append(suggestions, DSLTokenSuggestion{
					Token:       fmt.Sprintf("%q", p),
					Type:        TokenTypeValue,
					Description: "Priority value",
				})
			}
			return suggestions
		}

		// Generic values
		suggestions = append(suggestions,
			DSLTokenSuggestion{Token: "\"\"", Type: TokenTypeValue, Description: "Empty string"},
			DSLTokenSuggestion{Token: "true", Type: TokenTypeValue, Description: "Boolean true"},
			DSLTokenSuggestion{Token: "false", Type: TokenTypeValue, Description: "Boolean false"},
		)
		return suggestions
	}

	// Check if last token matches a registered field exactly
	if _, ok := fieldMap[lastToken]; ok {
		// Suggest operators
		suggestions = append(suggestions,
			DSLTokenSuggestion{Token: "==", Type: TokenTypeOperator, Description: "Equal to value"},
			DSLTokenSuggestion{Token: "!=", Type: TokenTypeOperator, Description: "Not equal to value"},
			DSLTokenSuggestion{Token: "is_populated", Type: TokenTypeOperator, Description: "Field is set and non-empty"},
			DSLTokenSuggestion{Token: "is_empty", Type: TokenTypeOperator, Description: "Field is missing or empty"},
			DSLTokenSuggestion{Token: "is_not_empty", Type: TokenTypeOperator, Description: "Field contains elements/text"},
			DSLTokenSuggestion{Token: "contains", Type: TokenTypeOperator, Description: "Collection or string contains token"},
			DSLTokenSuggestion{Token: ">", Type: TokenTypeOperator, Description: "Greater than numeric/effort"},
			DSLTokenSuggestion{Token: "<", Type: TokenTypeOperator, Description: "Less than numeric/effort"},
		)
		return suggestions
	}

	// Partial match on fields
	lowerLast := strings.ToLower(lastToken)
	for _, name := range registeredFieldNames {
		if strings.HasPrefix(strings.ToLower(name), lowerLast) {
			suggestions = append(suggestions, DSLTokenSuggestion{
				Token:       name,
				Type:        TokenTypeField,
				Description: "Schema field",
			})
		}
	}

	// Partial match on operators
	for _, op := range []string{"==", "!=", "is_populated", "is_empty", "is_not_empty", "contains", "&&"} {
		if strings.HasPrefix(strings.ToLower(op), lowerLast) {
			suggestions = append(suggestions, DSLTokenSuggestion{
				Token:       op,
				Type:        TokenTypeOperator,
				Description: "DSL Operator",
			})
		}
	}

	return suggestions
}

// EvaluateDSLExpression evaluates a DSL expression against an object map.
// Supports compound expressions with '&&' or 'AND'.
func EvaluateDSLExpression(expr string, obj map[string]any) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}

	// Split by logical AND (&& or AND)
	var clauses []string
	parts := strings.Split(expr, "&&")
	for _, p := range parts {
		andSubparts := strings.Split(p, " AND ")
		for _, sp := range andSubparts {
			trimmed := strings.TrimSpace(sp)
			if trimmed != "" {
				clauses = append(clauses, trimmed)
			}
		}
	}

	for _, clause := range clauses {
		passed, err := evaluateSingleClause(clause, obj)
		if err != nil {
			return false, err
		}
		if !passed {
			return false, nil
		}
	}

	return true, nil
}

func evaluateSingleClause(clause string, obj map[string]any) (bool, error) {
	tokens := strings.Fields(clause)
	if len(tokens) == 0 {
		return true, nil
	}

	// 1. Unary predicates: <field> is_populated | <field> is_empty | <field> is_not_empty
	if len(tokens) == 2 {
		fieldName := tokens[0]
		predicate := strings.ToLower(tokens[1])
		val, exists := obj[fieldName]

		switch predicate {
		case "is_populated":
			if !exists || val == nil {
				return false, nil
			}
			if s, ok := val.(string); ok {
				return strings.TrimSpace(s) != "", nil
			}
			v := reflect.ValueOf(val)
			if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
				return v.Len() > 0, nil
			}
			return true, nil

		case "is_empty":
			if !exists || val == nil {
				return true, nil
			}
			if s, ok := val.(string); ok {
				return strings.TrimSpace(s) == "", nil
			}
			v := reflect.ValueOf(val)
			if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
				return v.Len() == 0, nil
			}
			return false, nil

		case "is_not_empty":
			if !exists || val == nil {
				return false, nil
			}
			if s, ok := val.(string); ok {
				return strings.TrimSpace(s) != "", nil
			}
			v := reflect.ValueOf(val)
			if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
				return v.Len() > 0, nil
			}
			return true, nil
		}
	}

	// 2. Binary operators: <field> <op> <value>
	if len(tokens) >= 3 {
		fieldName := tokens[0]
		op := tokens[1]
		targetVal := strings.Join(tokens[2:], " ")
		targetVal = strings.Trim(targetVal, "\"'")

		val, exists := obj[fieldName]
		if !exists || val == nil {
			if op == "!=" {
				return targetVal != "", nil
			}
			return false, nil
		}

		actualStr := fmt.Sprintf("%v", val)

		switch op {
		case "==":
			return actualStr == targetVal, nil
		case "!=":
			return actualStr != targetVal, nil
		case "contains":
			if slice, ok := val.([]any); ok {
				for _, item := range slice {
					if fmt.Sprintf("%v", item) == targetVal {
						return true, nil
					}
				}
				return false, nil
			}
			if slice, ok := val.([]string); ok {
				for _, item := range slice {
					if item == targetVal {
						return true, nil
					}
				}
				return false, nil
			}
			return strings.Contains(actualStr, targetVal), nil

		case "not_contains":
			if slice, ok := val.([]any); ok {
				for _, item := range slice {
					if fmt.Sprintf("%v", item) == targetVal {
						return false, nil
					}
				}
				return true, nil
			}
			if slice, ok := val.([]string); ok {
				for _, item := range slice {
					if item == targetVal {
						return false, nil
					}
				}
				return true, nil
			}
			return !strings.Contains(actualStr, targetVal), nil

		case ">", "<", ">=", "<=":
			actualNum, err1 := strconv.ParseFloat(actualStr, 64)
			targetNum, err2 := strconv.ParseFloat(targetVal, 64)
			if err1 == nil && err2 == nil {
				switch op {
				case ">":
					return actualNum > targetNum, nil
				case "<":
					return actualNum < targetNum, nil
				case ">=":
					return actualNum >= targetNum, nil
				case "<=":
					return actualNum <= targetNum, nil
				}
			}
			return false, nil
		}
	}

	return false, fmt.Errorf("unsupported DSL clause: %q", clause)
}

// DefaultPolicyRulesForKind provides canonical evaluation rules for a kind.
func DefaultPolicyRulesForKind(kind string) []PolicyRule {
	switch kind {
	case objects.KindBacklogItem:
		return []PolicyRule{
			{
				ID:          "POL-INTEGRITY-LINEAGE-001",
				Name:        "Unbroken Strategic Lineage",
				Description: "Work items must link upward to milestone and requirement",
				TargetKind:  kind,
				Expression:  "requirement_refs is_not_empty && milestone_refs is_not_empty",
				Severity:    "blocker",
			},
			{
				ID:          "POL-ESTIMATED-EFFORT-001",
				Name:        "Shovel-Ready Effort Bound",
				Description: "Work units must declare estimated effort for capacity scheduling",
				TargetKind:  kind,
				Expression:  "estimated_effort is_populated",
				Severity:    "warning",
			},
			{
				ID:          "POL-CRITERIA-COMPLETION-001",
				Name:        "Acceptance Criteria Bound",
				Description: "Items must have criteria references linked for TDD verification",
				TargetKind:  kind,
				Expression:  "criteria_refs is_not_empty",
				Severity:    "blocker",
			},
		}

	case objects.KindCriteria:
		return []PolicyRule{
			{
				ID:          "POL-CRITERIA-CATEGORY-001",
				Name:        "Mandatory Criteria Category",
				Description: "Criteria must have an explicit category defined (test, architectural, etc.)",
				TargetKind:  kind,
				Expression:  "category is_populated",
				Severity:    "blocker",
			},
			{
				ID:          "POL-CRITERIA-DESCRIPTION-001",
				Name:        "Substantive Description",
				Description: "Criteria must declare clear verification instructions",
				TargetKind:  kind,
				Expression:  "description is_populated",
				Severity:    "warning",
			},
		}

	default:
		return []PolicyRule{
			{
				ID:          "POL-DEFAULT-IDENTITY-001",
				Name:        "Valid Identity and Title",
				Description: "All objects must possess non-empty identifier and title",
				TargetKind:  kind,
				Expression:  "id is_populated && title is_populated",
				Severity:    "blocker",
			},
			{
				ID:          "POL-DEFAULT-STATUS-001",
				Name:        "Lifecycle Status Assigned",
				Description: "Objects must have valid lifecycle status",
				TargetKind:  kind,
				Expression:  "status is_populated",
				Severity:    "blocker",
			},
		}
	}
}

// RunPolicyStudioDryRun evaluates a set of policy rules across all objects of kind in storage.
func RunPolicyStudioDryRun(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, storageCtx *storage.StorageContext, kind string, rules []PolicyRule) ([]RuleEvaluationResult, error) {
	if sp == nil {
		return nil, fmt.Errorf("storage provider is nil")
	}
	if storageCtx == nil {
		storageCtx = &storage.StorageContext{}
	}

	// Fetch objects of kind
	filter := storage.ListFilter{Kind: kind}
	queryResult, err := sp.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects of kind %s: %w", kind, err)
	}

	records := queryResult.Objects
	var results []RuleEvaluationResult

	for _, rule := range rules {
		res := RuleEvaluationResult{
			RuleID:         rule.ID,
			Name:           rule.Name,
			Expression:     rule.Expression,
			TotalEvaluated: len(records),
			Passed:         true,
		}

		for _, obj := range records {
			id, _ := obj[objects.FieldKeyID].(string)
			matches, evalErr := EvaluateDSLExpression(rule.Expression, obj)
			if evalErr != nil || !matches {
				res.ViolationsCount++
				res.Passed = false
				if len(res.OffendingIDs) < 5 {
					res.OffendingIDs = append(res.OffendingIDs, id)
				}
			}
		}

		if res.Passed {
			res.Summary = fmt.Sprintf("All %d %s objects satisfy rule", len(records), kind)
		} else {
			res.Summary = fmt.Sprintf("%d of %d objects violate rule (offenders: %s)", res.ViolationsCount, len(records), strings.Join(res.OffendingIDs, ", "))
		}

		results = append(results, res)
	}

	return results, nil
}
