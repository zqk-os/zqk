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

// IsRefactorOrExtractionCriterion reports whether a criterion is targeted at refactoring or code extraction
// based on title, excluding milestone-scoped child criteria and common noun extraction phrases.
func IsRefactorOrExtractionCriterion(title string) bool {
	titleLower := strings.ToLower(strings.TrimSpace(title))
	if titleLower == "" {
		return false
	}
	if strings.Contains(titleLower, "documentation") ||
		strings.Contains(titleLower, "knowledge base") ||
		strings.Contains(titleLower, "boundary & error") ||
		strings.Contains(titleLower, "functional acceptance") {
		return false
	}

	// Common nouns like "extraction" in functional criteria (e.g. "AST symbol extraction", "feature extraction",
	// "entity extraction", "token extraction", "data extraction", "archive extraction", "metadata extraction")
	// must not falsely trigger refactoring delta rules.
	if strings.Contains(titleLower, "symbol extraction") ||
		strings.Contains(titleLower, "feature extraction") ||
		strings.Contains(titleLower, "entity extraction") ||
		strings.Contains(titleLower, "token extraction") ||
		strings.Contains(titleLower, "data extraction") ||
		strings.Contains(titleLower, "archive extraction") ||
		strings.Contains(titleLower, "metadata extraction") ||
		strings.Contains(titleLower, "context extraction") ||
		strings.Contains(titleLower, "path extraction") ||
		strings.Contains(titleLower, "key extraction") ||
		strings.Contains(titleLower, "extract symbol") ||
		strings.Contains(titleLower, "extract token") ||
		strings.Contains(titleLower, "extract feature") {
		return false
	}

	if strings.HasPrefix(titleLower, "refactor") ||
		strings.Contains(titleLower, " refactor ") ||
		strings.Contains(titleLower, "refactoring") ||
		strings.HasPrefix(titleLower, "extract ") ||
		strings.Contains(titleLower, "code extraction") {
		return true
	}
	return false
}

// IsRefactorCriterion reports whether a criterion object designates a refactoring task
// requiring measured deltas. When an explicit category is designated, only category "refactor"
// triggers. When category is omitted, title heuristics are evaluated while ignoring common noun extractions.
func IsRefactorCriterion(obj map[string]any) bool {
	if obj == nil {
		return false
	}
	cat := strings.ToLower(strings.TrimSpace(objects.GetString(obj, objects.FieldKeyCategory)))
	if cat != "" {
		return cat == "refactor"
	}
	title := strings.TrimSpace(objects.GetString(obj, objects.FieldKeyTitle))
	return IsRefactorOrExtractionCriterion(title)
}

// ValidateCriteriaRefactorMeasuredDeltas verifies that refactor or extraction criteria assert measured deltas.
func ValidateCriteriaRefactorMeasuredDeltas(obj map[string]any) *ValidationError {
	if !IsRefactorCriterion(obj) {
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
