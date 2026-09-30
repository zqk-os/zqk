package validation

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	// RuleCriteriaRefactorMeasuredDeltas identifies the validation rule requiring refactor criteria to specify measured deltas.
	RuleCriteriaRefactorMeasuredDeltas = "criteria_refactor_measured_deltas"

	// MsgCriteriaRefactorMeasuredDeltas is the canonical error message for refactor criteria lacking measured delta assertions.
	MsgCriteriaRefactorMeasuredDeltas = "Refactor or extraction criteria must assert measured deltas (e.g. file count down by N, symbol absent), not mere existence prose."
)

// HasMeasuredDeltaAssertion reports whether a completeness_validation predicate string contains a measured delta assertion.
func HasMeasuredDeltaAssertion(predicateExpr string) bool {
	vTrim := strings.TrimSpace(predicateExpr)
	if vTrim == "" {
		return false
	}
	vLower := strings.ToLower(vTrim)
	return strings.HasPrefix(vLower, "ast_semantic_match:") ||
		strings.HasPrefix(vLower, "command_exit_code:") ||
		strings.HasPrefix(vLower, "query_metric:") ||
		strings.HasPrefix(vLower, "content_size_positive:") ||
		strings.Contains(vLower, "measure") ||
		strings.Contains(vLower, "count") ||
		strings.Contains(vLower, "assert")
}

// ValidateCriteriaRefactorMeasuredDeltas verifies that refactor or extraction criteria assert measured deltas.
func ValidateCriteriaRefactorMeasuredDeltas(obj map[string]any) *ValidationError {
	title, _ := obj[objects.FieldKeyTitle].(string)
	titleLower := strings.ToLower(title)
	if !strings.Contains(titleLower, "refactor") && !strings.Contains(titleLower, "extraction") {
		return nil
	}
	vals := stringSliceField(obj[objects.FieldKeyCompletenessValidation])
	for _, v := range vals {
		if HasMeasuredDeltaAssertion(v) {
			return nil
		}
	}
	return &ValidationError{
		Field:   objects.FieldKeyCompletenessValidation,
		Message: MsgCriteriaRefactorMeasuredDeltas,
		Rule:    RuleCriteriaRefactorMeasuredDeltas,
	}
}
